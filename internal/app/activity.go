package app

import (
	"strconv"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// Scan registration key; jobs use "job:<id>".
const scanActivityKey = "scan"

// Keep only in-memory active work, showing the most recently started remaining activity and
// emitting ScanActivityChanged whenever the visible activity changes.
type activities struct {
	mu      sync.Mutex
	seq     uint64
	running map[string]runningActivity
	publish func()
}

type runningActivity struct {
	seq      uint64
	activity domain.ScanActivity
}

func newActivities(publish func()) *activities {
	return &activities{running: map[string]runningActivity{}, publish: publish}
}

// Restarting an existing key makes it the newest activity.
func (a *activities) begin(key string, activity domain.ScanActivity) {
	a.mu.Lock()
	a.seq++
	a.running[key] = runningActivity{seq: a.seq, activity: activity}
	a.mu.Unlock()
	a.changed()
}

// Emit only when the visible activity ends.
func (a *activities) end(key string) {
	a.mu.Lock()
	ended, ok := a.running[key]
	top := a.topLocked()
	delete(a.running, key)
	a.mu.Unlock()
	if ok && ended.seq == top.seq {
		a.changed()
	}
}

// An empty Kind means no active work.
func (a *activities) current() domain.ScanActivity {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.topLocked().activity
}

func (a *activities) topLocked() runningActivity {
	var top runningActivity
	for _, running := range a.running {
		if running.seq > top.seq {
			top = running
		}
	}
	return top
}

func (a *activities) changed() {
	if a.publish != nil {
		a.publish()
	}
}

func jobActivityKey(job domain.Job) string {
	return "job:" + strconv.FormatInt(job.ID, 10)
}
