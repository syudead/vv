package app

import (
	"cmp"
	"slices"
	"strconv"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// Scan registration key; jobs use "job:<id>".
const scanActivityKey = "scan"

// RunningWork is one running job, or the file a scan registers, as the background status lists it.
type RunningWork struct {
	// Job is empty for a scan registering a file.
	Job domain.JobKind
	// VideoID is zero when the work names its video by ContentKey or names none.
	VideoID    int64
	ContentKey string
	// Path is empty when the work has no location.
	Path string
}

// Keep only in-memory active work: every running job and scan registration for the background status, and among
// them the most recently started import activity for the scan status. visible is called whenever that import activity
// changes, and listed whenever the running work does.
type activities struct {
	mu      sync.Mutex
	seq     uint64
	running map[string]runningActivity
	visible func()
	listed  func()
}

type runningActivity struct {
	seq uint64
	// activity has an empty Kind for work the scan status does not show.
	activity domain.ScanActivity
	work     RunningWork
}

func newActivities(visible, listed func()) *activities {
	return &activities{running: map[string]runningActivity{}, visible: visible, listed: listed}
}

// Restarting an existing key makes it the newest activity.
func (a *activities) begin(key string, activity domain.ScanActivity, work RunningWork) {
	a.mu.Lock()
	a.seq++
	a.running[key] = runningActivity{seq: a.seq, activity: activity, work: work}
	a.mu.Unlock()
	if activity.Active() {
		notify(a.visible)
	}
	notify(a.listed)
}

// Emit the visible change only when the visible activity ends.
func (a *activities) end(key string) {
	a.mu.Lock()
	ended, ok := a.running[key]
	top := a.topLocked()
	delete(a.running, key)
	a.mu.Unlock()
	if !ok {
		return
	}
	if ended.activity.Active() && ended.seq == top.seq {
		notify(a.visible)
	}
	notify(a.listed)
}

// An empty Kind means no active work.
func (a *activities) current() domain.ScanActivity {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.topLocked().activity
}

// list returns the running work, oldest first.
func (a *activities) list() []RunningWork {
	a.mu.Lock()
	entries := make([]runningActivity, 0, len(a.running))
	for _, running := range a.running {
		entries = append(entries, running)
	}
	a.mu.Unlock()
	slices.SortFunc(entries, func(x, y runningActivity) int { return cmp.Compare(x.seq, y.seq) })
	work := make([]RunningWork, len(entries))
	for i, entry := range entries {
		work[i] = entry.work
	}
	return work
}

func (a *activities) topLocked() runningActivity {
	var top runningActivity
	for _, running := range a.running {
		if running.activity.Active() && running.seq > top.seq {
			top = running
		}
	}
	return top
}

func notify(f func()) {
	if f != nil {
		f()
	}
}

func jobActivityKey(job domain.Job) string {
	return "job:" + strconv.FormatInt(job.ID, 10)
}
