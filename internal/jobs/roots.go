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

// DiskResolver returns the disk of a media folder, or an error when the folder cannot be read.
type DiskResolver func(path string) (domain.Disk, error)

type rootState struct {
	path        string
	disk        domain.Disk
	unavailable bool
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
	for _, kind := range domain.JobKinds {
		if domain.JobDemand(kind).Read > 0 {
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

	mu        sync.Mutex
	known     map[int64]rootState
	recheck   bool
	checkedAt time.Time
}

func newRoots(resolve DiskResolver, logger *slog.Logger) *roots {
	return &roots{resolve: resolve, logger: logger, known: map[int64]rootState{}}
}

func (r *roots) requestRecheck() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recheck = true
}

// refresh resolves the folders that need it and forgets removed ones; it runs on the scheduler's loop only.
func (r *roots) refresh(folders []domain.MediaFolder, now time.Time) {
	for _, folder := range r.pending(folders, now) {
		disk, err := r.resolve(folder.Path)
		r.set(folder.ID, rootState{path: folder.Path, disk: disk, unavailable: err != nil}, now)
		if err != nil {
			r.logger.Warn("parked the jobs of a media folder that cannot be read",
				slog.String("path", folder.Path), slog.Any("error", err))
			continue
		}
		r.logger.Debug("resolved the disk of a media folder", slog.String("path", folder.Path),
			slog.String("disk", disk.Key), slog.String("type", disk.Type.String()), slog.Int("readSlots", disk.ReadSlots()))
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
		if !ok || state.path != folder.Path || (recheck && state.unavailable) {
			pending = append(pending, folder)
		}
	}
	for id := range r.known {
		if !current[id] {
			delete(r.known, id)
		}
	}
	return pending
}

func (r *roots) set(id int64, state rootState, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if state.unavailable && !r.anyUnavailable() {
		// The minute until the recheck starts when the first folder becomes unavailable.
		r.checkedAt = now
	}
	r.known[id] = state
}

// markUnavailable parks the folder's jobs until a recheck finds it readable.
func (r *roots) markUnavailable(id int64, now time.Time) {
	r.mu.Lock()
	state, ok := r.known[id]
	r.mu.Unlock()
	if ok {
		state.unavailable = true
		r.set(id, state, now)
	}
}

func (r *roots) path(id int64) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.known[id]
	return state.path, ok
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
