package domain

// WatchState is derived from playback_progress and matches the UI definition in web/src/lib/format.ts.
type WatchState string

const (
	WatchStateUnwatched  WatchState = "unwatched"
	WatchStateInProgress WatchState = "inProgress"
	WatchStateWatched    WatchState = "watched"
)

// ClassifyWatch must stay consistent with storage's watchCondition SQL; nil means no progress.
func ClassifyWatch(progress *Progress) WatchState {
	switch {
	case progress == nil:
		return WatchStateUnwatched
	case progress.Completed:
		return WatchStateWatched
	case progress.PositionMs == 0:
		return WatchStateUnwatched
	default:
		return WatchStateInProgress
	}
}

// Empty WatchFilter means WatchAll.
type WatchFilter string

const (
	WatchAll        WatchFilter = "all"
	WatchUnwatched  WatchFilter = "unwatched"
	WatchInProgress WatchFilter = "inProgress"
	WatchWatched    WatchFilter = "watched"
)

func (f WatchFilter) Valid() bool {
	switch f {
	case WatchAll, WatchUnwatched, WatchInProgress, WatchWatched:
		return true
	default:
		return false
	}
}

func (f WatchFilter) Matches(state WatchState) bool {
	switch f {
	case "", WatchAll:
		return true
	default:
		return string(f) == string(state)
	}
}
