package domain

import "time"

type MediaBroadcast struct {
	Station string `json:",omitempty"`
	Program string `json:",omitempty"`
}

// Zero Start and negative Duration represent undefined EIT values.
type BroadcastEvent struct {
	Name     string
	Start    time.Time
	Duration time.Duration
}

func NewMediaBroadcast(station, program string) *MediaBroadcast {
	broadcast := MediaBroadcast{Station: CleanMediaText(station), Program: CleanMediaText(program)}
	if broadcast == (MediaBroadcast{}) {
		return nil
	}
	return &broadcast
}

// SelectBroadcastProgram chooses maximum recording overlap, preferring following on ties; undefined durations end at the next event or recording end, and unknown/nonoverlapping times yield empty.
func SelectBroadcastProgram(recordedAt time.Time, durationMs int64, present, following *BroadcastEvent) string {
	if recordedAt.IsZero() || durationMs <= 0 {
		return ""
	}
	recordingEnd := recordedAt.Add(time.Duration(durationMs) * time.Millisecond)
	var presentEnd time.Time
	if following != nil {
		presentEnd = following.Start
	}
	presentOverlap := eventOverlap(present, presentEnd, recordedAt, recordingEnd)
	followingOverlap := eventOverlap(following, recordingEnd, recordedAt, recordingEnd)
	switch {
	case followingOverlap > 0 && followingOverlap >= presentOverlap:
		return CleanMediaText(following.Name)
	case presentOverlap > 0:
		return CleanMediaText(present.Name)
	default:
		return ""
	}
}

func eventOverlap(event *BroadcastEvent, undefinedEnd, from, to time.Time) time.Duration {
	if event == nil || event.Start.IsZero() {
		return 0
	}
	end := undefinedEnd
	if event.Duration >= 0 {
		end = event.Start.Add(event.Duration)
	}
	if end.IsZero() {
		return 0
	}
	start := event.Start
	if start.Before(from) {
		start = from
	}
	if end.After(to) {
		end = to
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start)
}
