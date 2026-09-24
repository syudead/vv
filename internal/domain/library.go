package domain

import (
	"errors"
	"time"
)

// 一覧の件数。上限を設けるのは、1回の応答が青天井に大きくならないように
// するためである。
const (
	// DefaultLimit は limit が指定されなかったときの件数。
	DefaultLimit = 60
	// MaxLimit は1ページで返す上限。越える指定は上限に丸める。
	MaxLimit = 200
)

// ErrNotFound は対象が存在しないことを表す。
var ErrNotFound = errors.New("対象が見つかりません")

var (
	ErrNoMediaFolders         = errors.New("メディアフォルダが登録されていません")
	ErrScanRunning            = errors.New("取り込みの実行中です")
	ErrFolderConflict         = errors.New("メディアフォルダが重複または包含しています")
	ErrVersionConflict        = errors.New("メディアフォルダが別の操作で変更されています")
	ErrInvalidMediaFolder     = errors.New("メディアフォルダとして登録できません")
	ErrUnsupportedMediaFolder = errors.New("対応していないメディアフォルダです")
)

// ErrProbeNotFailed は読み取りに失敗していない動画へ読み取りのやり直しを
// 求めたことを表す。読み取り中・読み取り済みの動画がこれに当たる。
var ErrProbeNotFailed = errors.New("読み取りに失敗した動画ではありません")

// ErrInvalidCursor はカーソルが解釈できないことを表す。
//
// 黙って先頭から返さないのは、無限スクロールが巻き戻って同じ内容を延々と
// 表示することになるためである。
var ErrInvalidCursor = errors.New("カーソルを解釈できません")

// VideoSort は一覧の並び順である。値は api/openapi.yaml の VideoSort に対応する。
type VideoSort string

const (
	// SortAddedAsc は追加が古い順。
	SortAddedAsc VideoSort = "addedAsc"
	// SortAddedDesc は追加が新しい順（既定）。
	SortAddedDesc VideoSort = "addedDesc"
	// SortModifiedAsc は一覧に出す所在の更新日時が古い順。
	SortModifiedAsc VideoSort = "modifiedAsc"
	// SortModifiedDesc は一覧に出す所在の更新日時が新しい順。
	SortModifiedDesc VideoSort = "modifiedDesc"
	// SortTitleAsc は題名の自然順（NaturalSortKey の昇順）。
	SortTitleAsc VideoSort = "titleAsc"
	// SortTitleDesc は題名の自然順の逆。
	SortTitleDesc VideoSort = "titleDesc"
	// SortDurationAsc は長さが短い順。長さの無い動画は末尾。
	SortDurationAsc VideoSort = "durationAsc"
	// SortDurationDesc は長さが長い順。長さの無い動画は末尾。
	SortDurationDesc VideoSort = "durationDesc"
	// SortSizeAsc は一覧に出す所在のファイルサイズが小さい順。
	SortSizeAsc VideoSort = "sizeAsc"
	// SortSizeDesc は一覧に出す所在のファイルサイズが大きい順。
	SortSizeDesc VideoSort = "sizeDesc"
	// SortPlayedAsc は最後に再生した時刻が古い順。再生の記録が無い動画は末尾。
	SortPlayedAsc VideoSort = "playedAsc"
	// SortPlayedDesc は最後に再生した時刻が新しい順。再生の記録が無い動画は末尾。
	SortPlayedDesc VideoSort = "playedDesc"
	// SortRandom は Seed と id から ShuffleKey で作る値の順。向きを持たない。
	SortRandom VideoSort = "random"
)

// Valid は既知の並び順かどうかを返す。
func (s VideoSort) Valid() bool {
	switch s {
	case SortAddedAsc, SortAddedDesc, SortModifiedAsc, SortModifiedDesc,
		SortTitleAsc, SortTitleDesc, SortDurationAsc, SortDurationDesc,
		SortSizeAsc, SortSizeDesc, SortPlayedAsc, SortPlayedDesc, SortRandom:
		return true
	default:
		return false
	}
}

// MaxShuffleSeed は SortRandom の Seed の上限である（0 以上この値以下）。
const MaxShuffleSeed = 2147483647

// ShuffleKey は SortRandom で並べる値を、seed と id だけから作る。0 以上の値を
// 返す。同じ seed なら同じ id に同じ値を返すので、ページをまたいでも、行が
// 増減しても並びは変わらない。混ぜ合わせは splitmix64 の仕上げの手順で、
// seed ごとに id の並びがよく散らばる。値がぶつかったときは呼び出し側が id で
// 決着させる。
func ShuffleKey(seed, id int64) int64 {
	z := uint64(seed)*0x9e3779b97f4a7c15 + uint64(id)
	z += 0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	// 符号付きの 64 ビットで負にならないよう、最上位の1ビットを落とす。
	return int64(z >> 1)
}

// VideoQuery は一覧の問い合わせ条件である。
type VideoQuery struct {
	// Query は検索欄の入力である。ParseSearchQuery の書き方
	// （specs/013-library-search/contracts/list-api.md §1）で解釈する。
	// 語が残らなければ絞り込まない。
	Query string
	// Watch は視聴状態の絞り込み。空は WatchAll と同じ。
	Watch WatchFilter
	// PlayableOnly はブラウザで再生できると確定した動画だけにする。
	PlayableOnly bool
	Sort         VideoSort
	// Seed は SortRandom の並びを決める値（0 以上 MaxShuffleSeed 以下）。
	// ほかの並び順では使わない。
	Seed int64
	// Cursor は前回の応答が返した NextCursor。空なら先頭から。
	Cursor string
	Limit  int
}

// VideoPage は一覧1ページ分である。
type VideoPage struct {
	Items []Video
	// Total は絞り込み後の総件数。ページングとは独立に返る。
	Total int
	// NextCursor は次のページの取得に渡す。これ以上無ければ空。
	NextCursor string
	// Limit は実際に使われた件数。指定の丸めが効いたかを呼び出し側が見られる。
	Limit int
}

// ScanState は走査の状態である。値は api/openapi.yaml の Scan.state に対応する。
type ScanState string

const (
	// ScanRunning は走査中。同時に1件だけ存在できる。
	ScanRunning ScanState = "running"
	// ScanDone は最後まで走った。個別のファイルの失敗は Failed に数える。
	ScanDone ScanState = "done"
	// ScanFailed は走査そのものが失敗した（対象ディレクトリが読めない等）。
	ScanFailed ScanState = "failed"
)

// Scan は走査1回の記録である。進捗として API に出る。
type Scan struct {
	ID         int64
	State      ScanState
	StartedAt  time.Time
	FinishedAt time.Time
	Total      int
	Completed  int
	Failed     int
	// Error は走査そのものが失敗した理由。個別のファイルの失敗は含まない。
	Error string
}

// ScanProgress は進捗の値である。
type ScanProgress struct {
	Total     int
	Completed int
	Failed    int
}
