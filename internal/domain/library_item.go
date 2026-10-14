package domain

import "time"

// ライブラリの項目（specs/017-folder-groups/data-model.md §5・§6）。ライブラリの
// 一覧は、動画とグループを「項目」として混ぜて返す。

// LibraryItem は一覧の項目1件である。Video と Group のちょうど一方を持つ。
type LibraryItem struct {
	Video *Video
	Group *LibraryGroup
}

// LibraryPage はライブラリの一覧1ページ分である。
type LibraryPage struct {
	Items []LibraryItem
	// Total は絞り込み後の項目（カード）の数。ページングとは独立に返る。
	Total int
	// NextCursor は次のページの取得に渡す。これ以上無ければ空。
	NextCursor string
	// Limit は実際に使われた件数。
	Limit int
	// MissingTagIDs は VideoQuery.TagIDs のうちいまタグとして存在しなかった id。
	MissingTagIDs []int64
	// Roots は登録フォルダの一覧である。項目と同じ読み取りスナップショットから読む。
	// 応答の VideoFolder（動画の所在と自動グループのフォルダ）はこれから作る。
	Roots []MediaFolder
}

// LibrarySelection は一覧の条件に合う項目の、ページングしない選択である
// （GET /api/library/ids、specs/035-favorites/data-model.md §5）。動画の項目の id と、
// グループの項目ごとの id と再生順の動画の id を分けて持つ（specs/048-groups/data-model.md
// 「Library items」）。
type LibrarySelection struct {
	// VideoIDs は動画の項目の id。
	VideoIDs []int64
	// Groups はグループの項目ごとの id と動画。
	Groups []LibraryGroupSelection
}

// LibraryGroupSelection はグループの項目 1 件の id と動画である。
type LibraryGroupSelection struct {
	// ID はグループの id。
	ID int64
	// VideoIDs はグループの再生順（GroupPlayOrder）の、見せてよい動画の id。
	VideoIDs []int64
}

// AllVideoIDs は動画の項目の id とグループのメンバーの id の和を、この順に返す。
func (s LibrarySelection) AllVideoIDs() []int64 {
	ids := append([]int64{}, s.VideoIDs...)
	for _, group := range s.Groups {
		ids = append(ids, group.VideoIDs...)
	}
	return ids
}

// LibraryGroup はグループの項目である。値はどれも、絞り込みに関係なく、見る人に
// 見せてよい子孫の動画の全部から、再生順で作る（specs/048-groups/research.md R-7）。
type LibraryGroup struct {
	// ID はグループの id。
	ID int64
	// Path は自動グループのフォルダ（origin_folder、FolderKey の形）。手で作ったグループでは
	// 空である。応答の VideoFolder は、登録フォルダの下にあるときだけ LocateFolder で作る。
	Path string
	// Name はグループの名前。
	Name string
	// Members は見せてよい子孫の動画を再生順（子グループは深さ優先、同じ動画は最初の 1 回）で
	// 持つ。
	Members []Video
	// WatchState と WatchedCount は GroupWatch の結果である。
	WatchState   WatchState
	WatchedCount int
	// OpenVideoID は押したときに開くメンバー（GroupOpenIndex）。
	OpenVideoID int64
	// DurationMs は長さの分かっているメンバーの合計。1本も分からなければ nil。
	DurationMs *int64
	// SizeBytes はメンバーの代表の所在の大きさの合計。
	SizeBytes int64
	// AddedAt はメンバーの追加日時の最大。
	AddedAt time.Time
	// LastPlayedAt はメンバーの最後に再生した時刻の最大。記録が無ければ nil。
	LastPlayedAt *time.Time
	// Favorite は所有者がグループをお気に入りにしたか（specs/035-favorites/data-model.md §2）。
	// メンバーの Video.Favorite とは独立である。保存層が埋める。
	Favorite bool
}

// NewLibraryGroup は並んだメンバーと、それぞれの再生の記録（無ければ nil）から
// グループの項目を作る。progress は members と同じ長さで同じ並びである。
// ゲストには再生の記録を渡さない（全部 nil）ので、開くメンバーは最初のメンバーになる。
func NewLibraryGroup(id int64, path, name string, members []Video, progress []*Progress) LibraryGroup {
	group := LibraryGroup{ID: id, Path: path, Name: name, Members: members}
	group.WatchState, group.WatchedCount = GroupWatch(progress)
	if index := GroupOpenIndex(progress); index < len(members) {
		group.OpenVideoID = members[index].ID
	}
	for i, member := range members {
		group.SizeBytes += member.SizeBytes
		if member.DurationMs != nil {
			total := *member.DurationMs
			if group.DurationMs != nil {
				total += *group.DurationMs
			}
			group.DurationMs = &total
		}
		if member.AddedAt.After(group.AddedAt) {
			group.AddedAt = member.AddedAt
		}
		if i < len(progress) && progress[i] != nil {
			played := progress[i].UpdatedAt
			if group.LastPlayedAt == nil || played.After(*group.LastPlayedAt) {
				group.LastPlayedAt = &played
			}
		}
	}
	return group
}

// memberStarted は、メンバーを見始めた（位置が 0 より大きいか完了した）かを返す。
// 1本の定義は ClassifyWatch と同じで、見始めていれば unwatched ではない。
func memberStarted(progress *Progress) bool {
	return ClassifyWatch(progress) != WatchStateUnwatched
}

// GroupWatch は並んだメンバーの再生の記録から、グループの視聴状態と見終えた本数を
// 決める（data-model.md §6、親 Issue 要件 17）。見始めたメンバーが無ければ
// unwatched、全メンバーが完了なら watched、それ以外は inProgress である。
//
// internal/store の一覧は、同じ定義を SQL（libraryItemsCTE の視聴状態）として持つ。
// 定義を変えるときは両方を揃える。
func GroupWatch(progress []*Progress) (WatchState, int) {
	started, watched := 0, 0
	for _, p := range progress {
		if memberStarted(p) {
			started++
		}
		if ClassifyWatch(p) == WatchStateWatched {
			watched++
		}
	}
	switch {
	case started == 0:
		return WatchStateUnwatched, watched
	case watched == len(progress):
		return WatchStateWatched, watched
	default:
		return WatchStateInProgress, watched
	}
}

// GroupOpenIndex は、グループを押したときに開くメンバーの位置を返す（data-model.md §6、
// 親 Issue 要件 23）。並びの順で、位置が 0 より大きく完了していない最初のメンバー。
// 無ければ最初の未完了のメンバー。全部完了なら（メンバーが無いときも）0 である。
func GroupOpenIndex(progress []*Progress) int {
	for i, p := range progress {
		if ClassifyWatch(p) == WatchStateInProgress {
			return i
		}
	}
	for i, p := range progress {
		if ClassifyWatch(p) != WatchStateWatched {
			return i
		}
	}
	return 0
}
