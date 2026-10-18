package domain

// Event publishers do not know subscribers, which are wired in cmd/mdm; storage publishes only after commit.
type Event interface {
	event()
}

type VideoIngestChanged struct {
	VideoID int64
	// Stage is empty when the video row was deleted.
	Stage JobKind
}

// JobsQueued contains distinct stages with newly available work.
type JobsQueued struct {
	Kinds []JobKind
}

type ProcessingChanged struct{}

type ScanChanged struct{}

// ScanActivityChanged carries no snapshot; subscribers read the current in-memory value.
type ScanActivityChanged struct{}

// ContentUnreferenced reports no references at commit; deletion consumers must recheck because content may be reimported.
type ContentUnreferenced struct {
	ContentKeys []string
}

func (VideoIngestChanged) event()  {}
func (JobsQueued) event()          {}
func (ProcessingChanged) event()   {}
func (ScanChanged) event()         {}
func (ScanActivityChanged) event() {}
func (ContentUnreferenced) event() {}
