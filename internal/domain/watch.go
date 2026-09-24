package domain

// WatchState は再生の記録から導く視聴状態である。保存はせず、playback_progress
// を content_key で引いて導く（specs/013-library-search/data-model.md §6）。
// 値は一覧画面の watchState（web/src/lib/format.ts）と同じ定義である。
type WatchState string

const (
	// WatchStateUnwatched は記録が無いか、完了しておらず再生位置が 0 の状態。
	WatchStateUnwatched WatchState = "unwatched"
	// WatchStateInProgress は未視聴でも視聴済みでもない状態。
	WatchStateInProgress WatchState = "inProgress"
	// WatchStateWatched は完了扱いの状態。
	WatchStateWatched WatchState = "watched"
)

// ClassifyWatch は再生の記録を視聴状態に畳む。記録が無ければ nil を渡す。
//
// internal/store の一覧は、同じ定義を SQL の条件（watchCondition）として持つ。
// 定義を変えるときは両方を揃える。
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

// WatchFilter は一覧を視聴状態で絞る条件である。値は api/openapi.yaml の
// watch パラメータに対応する。空は WatchAll と同じに扱う。
type WatchFilter string

const (
	// WatchAll は絞り込まない（既定）。
	WatchAll WatchFilter = "all"
	// WatchUnwatched は未視聴だけにする。
	WatchUnwatched WatchFilter = "unwatched"
	// WatchInProgress は視聴途中だけにする。
	WatchInProgress WatchFilter = "inProgress"
	// WatchWatched は視聴済みだけにする。
	WatchWatched WatchFilter = "watched"
)

// Valid は既知の値かどうかを返す。
func (f WatchFilter) Valid() bool {
	switch f {
	case WatchAll, WatchUnwatched, WatchInProgress, WatchWatched:
		return true
	default:
		return false
	}
}

// Matches は視聴状態がこの条件に合うかを返す。空と WatchAll はすべてに合う。
func (f WatchFilter) Matches(state WatchState) bool {
	switch f {
	case "", WatchAll:
		return true
	default:
		return string(f) == string(state)
	}
}
