package domain

import "time"

type ImportStatus string

const (
	// ImportFinding suppresses counts until enumeration finishes.
	ImportFinding ImportStatus = "finding"
	ImportRunning ImportStatus = "running"
	ImportDone    ImportStatus = "done"
	ImportPartial ImportStatus = "partial"
	// ImportFailed takes precedence over other states.
	ImportFailed ImportStatus = "failed"
)

type ImportTally struct {
	State              ScanState
	ScanTotal          int
	ScanCompleted      int
	ScanFailed         int
	Videos             int
	SettledVideos      int
	UnregisteredIssues int
	FailedIssues       int
}

// Enumerated remains false while a running scan has zero targets because totals are recorded only after enumerating all roots.
func (t ImportTally) Enumerated() bool {
	return t.State != ScanRunning || t.ScanTotal > 0
}

// pendingFiles excludes unattempted files once the scan closes.
func (t ImportTally) pendingFiles() int {
	if t.State != ScanRunning {
		return 0
	}
	return max(0, t.ScanTotal-t.ScanCompleted-t.ScanFailed)
}

// VideosTotal includes videos, unregistered-file issues, and files awaiting registration.
func (t ImportTally) VideosTotal() int {
	return max(0, t.Videos) + max(0, t.UnregisteredIssues) + t.pendingFiles()
}

// VideosSettled includes settled videos and unregistered-file issues, capped at the total.
func (t ImportTally) VideosSettled() int {
	settled := min(max(0, t.SettledVideos), max(0, t.Videos)) + max(0, t.UnregisteredIssues)
	return min(settled, t.VideosTotal())
}

// Status precedence is failed, finding, running, partial, then done.
func (t ImportTally) Status() ImportStatus {
	switch {
	case t.State == ScanFailed:
		return ImportFailed
	case !t.Enumerated():
		return ImportFinding
	case t.State == ScanRunning || t.VideosSettled() < t.VideosTotal():
		return ImportRunning
	case t.FailedIssues > 0:
		return ImportPartial
	default:
		return ImportDone
	}
}

type ImportProgress struct {
	Status  ImportStatus
	Counted bool
	Total   int
	Settled int
	// SettledAt is set only for done or partial imports.
	SettledAt time.Time
}

func (t ImportTally) Progress(settledAt time.Time) ImportProgress {
	status := t.Status()
	out := ImportProgress{Status: status}
	if status != ImportFinding {
		out.Counted = true
		out.Total = t.VideosTotal()
		out.Settled = t.VideosSettled()
	}
	if status == ImportDone || status == ImportPartial {
		out.SettledAt = settledAt
	}
	return out
}
