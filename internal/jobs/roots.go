package jobs

import (
	"cmp"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// RootRecheckInterval is how often the scheduler checks unavailable media folders again while any is unavailable.
const RootRecheckInterval = time.Minute

// DiskResolveTimeout bounds how long a turn waits for media folders' disks, so a share that stops answering parks its
// folder instead of stalling the grants on every disk.
const DiskResolveTimeout = 5 * time.Second

// DiskResolver returns the disk of a media folder, or an error when the folder cannot be read. populated reports
// that the library holds a location under the folder, so a folder that lists nothing is an unmounted mount point
// rather than a new, empty folder, and counts as unreadable.
type DiskResolver func(path string, populated bool) (domain.Disk, error)

type rootState struct {
	path        string
	disk        domain.Disk
	unavailable bool
	// resolving marks a resolution still running, which is not started again; late marks one the turn stopped
	// waiting for, whose answer wakes the scheduler.
	resolving bool
	late      bool
}

// diskGroup is one turn of the rotation over disks: the available roots of one disk and the kinds that read it, or,
// with no disk and nil roots, the kinds that read no source file.
type diskGroup struct {
	disk  string
	roots []int64
	kinds []domain.JobKind
}

var readKinds, noReadKinds = splitKindsByRead()

func splitKindsByRead() (read, noRead []domain.JobKind) {
	for _, kind := range domain.ScheduledJobKinds {
		if domain.JobReadsSource(kind) {
			read = append(read, kind)
		} else {
			noRead = append(noRead, kind)
		}
	}
	return read, noRead
}

// roots keeps each media folder's disk and availability in memory only, because a remount or a replaced disk would
// make a stored value wrong. A folder is resolved when first seen, and an unavailable one again on a recheck.
type roots struct {
	resolve DiskResolver
	logger  *slog.Logger
	wake    func()

	mu        sync.Mutex
	known     map[int64]rootState
	recheck   bool
	checkedAt time.Time
}

func newRoots(resolve DiskResolver, logger *slog.Logger, wake func()) *roots {
	return &roots{resolve: resolve, logger: logger, wake: wake, known: map[int64]rootState{}}
}

func (r *roots) requestRecheck() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recheck = true
}

// refresh resolves the folders that need it and forgets removed ones; it runs on the scheduler's loop only. populated
// reports whether the library holds a location under a folder, read only for the folders resolved now and inside
// their resolution. A resolution that outlasts DiskResolveTimeout parks its folder until it answers, because a read
// from an unresponsive share or a stalled library lookup can block without end.
func (r *roots) refresh(folders []domain.MediaFolder, now time.Time, populated func(domain.MediaFolder) bool) {
	pending := r.pending(folders, now)
	if len(pending) == 0 {
		return
	}
	done := make(chan struct{}, len(pending))
	for _, folder := range pending {
		go func() {
			r.resolveOne(folder, populated(folder))
			done <- struct{}{}
		}()
	}
	timeout := time.NewTimer(DiskResolveTimeout)
	defer timeout.Stop()
	for range pending {
		select {
		case <-done:
		case <-timeout.C:
			r.parkUnanswered(now)
			return
		}
	}
}

func (r *roots) resolveOne(folder domain.MediaFolder, populated bool) {
	disk, err := r.resolve(folder.Path, populated)
	r.mu.Lock()
	state, ok := r.known[folder.ID]
	if !ok || !state.resolving || state.path != folder.Path {
		// The folder was removed or moved while its disk was resolved.
		r.mu.Unlock()
		return
	}
	r.setLocked(folder.ID, rootState{path: folder.Path, disk: disk, unavailable: err != nil}, time.Now())
	r.mu.Unlock()
	if err != nil {
		r.logger.Warn("parked the jobs of a media folder that cannot be read",
			slog.String("path", folder.Path), slog.Any("error", err))
	} else {
		r.logger.Debug("resolved the disk of a media folder", slog.String("path", folder.Path),
			slog.String("disk", disk.Key), slog.String("type", disk.Type.String()), slog.Int("readSlots", disk.ReadSlots()))
	}
	if state.late {
		r.wake()
	}
}

// parkUnanswered marks the folders whose resolution is still running unavailable, and has their answer wake the
// scheduler.
func (r *roots) parkUnanswered(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, state := range r.known {
		if !state.resolving {
			continue
		}
		state.late = true
		if !state.unavailable {
			r.logger.Warn("parked the jobs of a media folder whose disk does not answer", slog.String("path", state.path))
			state.unavailable = true
		}
		r.setLocked(id, state, now)
	}
}

func (r *roots) pending(folders []domain.MediaFolder, now time.Time) []domain.MediaFolder {
	r.mu.Lock()
	defer r.mu.Unlock()
	recheck := r.recheck || (r.anyUnavailable() && now.Sub(r.checkedAt) >= RootRecheckInterval)
	if recheck {
		r.recheck = false
		r.checkedAt = now
	}
	current := make(map[int64]bool, len(folders))
	var pending []domain.MediaFolder
	for _, folder := range folders {
		current[folder.ID] = true
		state, ok := r.known[folder.ID]
		rechecked := recheck && state.unavailable && !state.resolving
		if ok && state.path == folder.Path && !rechecked {
			continue
		}
		if !ok || state.path != folder.Path {
			state = rootState{path: folder.Path}
		}
		state.resolving, state.late = true, false
		r.known[folder.ID] = state
		pending = append(pending, folder)
	}
	for id := range r.known {
		if !current[id] {
			delete(r.known, id)
		}
	}
	return pending
}

func (r *roots) setLocked(id int64, state rootState, now time.Time) {
	if state.unavailable && !r.anyUnavailable() {
		// The minute until the recheck starts when the first folder becomes unavailable.
		r.checkedAt = now
	}
	r.known[id] = state
}

// markUnavailable parks the folder's jobs until a recheck finds it readable.
func (r *roots) markUnavailable(id int64, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if state, ok := r.known[id]; ok {
		state.unavailable = true
		r.setLocked(id, state, now)
	}
}

func (r *roots) path(id int64) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.known[id]
	return state.path, ok
}

// diskOf returns the disk of the media folder that holds path, taking the deepest folder when folders nest; it is
// false when no resolved folder holds path.
func (r *roots) diskOf(path string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var disk, root string
	found := false
	for _, state := range r.known {
		if state.disk.Key == "" || !domain.PathWithinRoot(state.path, path) {
			continue
		}
		if !found || len(state.path) > len(root) {
			disk, root, found = state.disk.Key, state.path, true
		}
	}
	return disk, found
}

// disk returns the resolved disk of the available folder at path.
func (r *roots) disk(path string) (domain.Disk, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, state := range r.known {
		if state.path == path && !state.resolving && !state.unavailable {
			return state.disk, true
		}
	}
	return domain.Disk{}, false
}

func (r *roots) anyUnavailable() bool {
	for _, state := range r.known {
		if state.unavailable {
			return true
		}
	}
	return false
}

// untilRecheck is the wait until the next recheck, and false while every folder is available, so a healthy library
// is never polled.
func (r *roots) untilRecheck(now time.Time) (time.Duration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.anyUnavailable() {
		return 0, false
	}
	return max(r.checkedAt.Add(RootRecheckInterval).Sub(now), 0), true
}

// snapshot groups the available folders by disk in key order, then adds the group of kinds that read no source. It
// also returns each disk's read slots and the unavailable folders.
func (r *roots) snapshot() (groups []diskGroup, readSlots map[string]int, unavailable []int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	readSlots = map[string]int{}
	byDisk := map[string][]int64{}
	for id, state := range r.known {
		if state.unavailable {
			unavailable = append(unavailable, id)
			continue
		}
		byDisk[state.disk.Key] = append(byDisk[state.disk.Key], id)
		readSlots[state.disk.Key] = state.disk.ReadSlots()
	}
	for disk, ids := range byDisk {
		slices.Sort(ids)
		groups = append(groups, diskGroup{disk: disk, roots: ids, kinds: readKinds})
	}
	slices.SortFunc(groups, func(a, b diskGroup) int { return cmp.Compare(a.disk, b.disk) })
	slices.Sort(unavailable)
	return append(groups, diskGroup{kinds: noReadKinds}), readSlots, unavailable
}
