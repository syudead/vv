package domain

// WatchState is derived from playback_progress and matches the UI definition in web/src/lib/format.ts.
type WatchState string

const (
	WatchStateUnwatched  WatchState = "unwatched"
	WatchStateInProgress WatchState = "inProgress"
	WatchStateWatched    WatchState = "watched"
)

// ClassifyWatch is the rule internal/store's watch-state SQL restates, compared in its rule_equivalence_test.go; nil means no progress.
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
