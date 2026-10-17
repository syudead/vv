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
var ErrNotFound = errors.New("not found")

var (
	ErrNoMediaFolders         = errors.New("no media folders are configured")
	ErrScanRunning            = errors.New("a scan is in progress")
	ErrFolderConflict         = errors.New("media folders overlap or contain each other")
	ErrVersionConflict        = errors.New("media folders were changed by another operation")
	ErrInvalidMediaFolder     = errors.New("cannot be added as a media folder")
	ErrUnsupportedMediaFolder = errors.New("unsupported media folder")
)

// ErrProbeNotFailed は読み取りに失敗していない動画へ読み取りのやり直しを
// 求めたことを表す。読み取り中・読み取り済みの動画がこれに当たる。
var ErrProbeNotFailed = errors.New("the video has not failed to probe")

// ErrInvalidCursor はカーソルが解釈できないことを表す。
//
// 黙って先頭から返さないのは、無限スクロールが巻き戻って同じ内容を延々と
// 表示することになるためである。
var ErrInvalidCursor = errors.New("invalid cursor")

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
	// SortCreatedAsc は一覧に出す所在のファイルの作成日時が古い順。作成日時が取れなかった
	// 所在は更新日時を使う。
	SortCreatedAsc VideoSort = "createdAsc"
	// SortCreatedDesc は一覧に出す所在のファイルの作成日時が新しい順。
	SortCreatedDesc VideoSort = "createdDesc"
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
	// SortFavoritedAsc はお気に入りにした日時が古い順。お気に入りでない項目は末尾
	//
	SortFavoritedAsc VideoSort = "favoritedAsc"
	// SortFavoritedDesc はお気に入りにした日時が新しい順。お気に入りでない項目は末尾。
	SortFavoritedDesc VideoSort = "favoritedDesc"
	// SortRandom は Seed と id から ShuffleKey で作る値の順。向きを持たない。
	SortRandom VideoSort = "random"
)

// Valid は既知の並び順かどうかを返す。
func (s VideoSort) Valid() bool {
	switch s {
	case SortAddedAsc, SortAddedDesc, SortModifiedAsc, SortModifiedDesc,
		SortCreatedAsc, SortCreatedDesc, SortTitleAsc, SortTitleDesc, SortDurationAsc, SortDurationDesc,
		SortSizeAsc, SortSizeDesc, SortPlayedAsc, SortPlayedDesc, SortFavoritedAsc, SortFavoritedDesc, SortRandom:
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
	// で解釈する。
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
	// TagIDs は絞り込むタグの id の並び（すべてを持つ動画だけにする AND、
	// docs/design-docs/catalog-organization.md）。存在しない id は条件から落とし、
	// VideoPage.MissingTagIDs に返す。空ならタグで絞り込まない。
	TagIDs []int64
	// FavoriteOnly はお気に入りの項目だけにする。
	FavoriteOnly bool
	// Person は、どの役割でもこの人物に結び付いた動画だけにする人物の id である。
	// 0 は絞り込まない。実体の無い id は何にも当たらない
	//
	Person int64
	// Organization は Person と同じ形の、団体の id による絞り込みである。
	Organization int64
	// TagKindID は、手で付けたかフォルダ名から付いたかを問わず、この種類のタグを 1 つでも
	// 持つ動画だけにするタグの種類の id である。0 は絞り込まない。実体の無い id は何にも
	// 当たらない。
	TagKindID int64
	// AddedWithinDays は、いまからこの日数前の時刻以後に追加した動画だけにする
	// （1 以上 MaxAddedWithinDays 以下）。0 は絞り込まない
	//
	AddedWithinDays int
	// MinDurationSec と MaxDurationSec は長さの範囲（秒、両端を含む。0 以上
	// MaxDurationBoundSec 以下）である。nil はその側を限らない。どちらかがあれば、
	// 長さの分からない動画は当たらない（R-4）。
	MinDurationSec *int
	MaxDurationSec *int
}

// ListsSmartFolders は、この条件のライブラリの一番上にスマートフォルダの項目を出すかを返す。
// 検索が無く、お気に入りのみ以外の絞り込みも無いときだけ出す。お気に入りのみはお気に入りにした
// フォルダだけを出す。並べ替えは絞り込みではない。
func (q VideoQuery) ListsSmartFolders() bool {
	return ParseSearchQuery(q.Query).Empty() && (q.Watch == "" || q.Watch == WatchAll) && !q.PlayableOnly &&
		len(q.TagIDs) == 0 && q.Person == 0 && q.Organization == 0 && q.TagKindID == 0 &&
		q.AddedWithinDays == 0 && q.MinDurationSec == nil && q.MaxDurationSec == nil
}

// MaxAddedWithinDays は VideoQuery.AddedWithinDays の上限である（R-4）。
const MaxAddedWithinDays = 36500

// MaxDurationBoundSec は VideoQuery の長さの範囲の各端の上限（秒）である。
const MaxDurationBoundSec = 360000

var (
	// ErrInvalidAddedWithin は追加日の日数が範囲（1〜MaxAddedWithinDays）の外にあることを表す。
	ErrInvalidAddedWithin = errors.New("added within is out of range")
	// ErrInvalidDurationRange は長さの端が範囲（0〜MaxDurationBoundSec）の外にあるか、
	// 下限が上限を越えることを表す。
	ErrInvalidDurationRange = errors.New("duration range is invalid")
)

// CheckAddedAndLength は追加日と長さの絞り込みの値を確かめる。addedWithin・minSec・maxSec は
// 指定が無ければ nil である。ライブラリの一覧とスマートフォルダの条件が同じ範囲を使う
func CheckAddedAndLength(addedWithin, minSec, maxSec *int) error {
	if addedWithin != nil && (*addedWithin < 1 || *addedWithin > MaxAddedWithinDays) {
		return ErrInvalidAddedWithin
	}
	for _, bound := range []*int{minSec, maxSec} {
		if bound != nil && (*bound < 0 || *bound > MaxDurationBoundSec) {
			return ErrInvalidDurationRange
		}
	}
	if minSec != nil && maxSec != nil && *minSec > *maxSec {
		return ErrInvalidDurationRange
	}
	return nil
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
	// MissingTagIDs は VideoQuery.TagIDs のうちいまタグとして存在しなかった id。
	// 1つも無ければ空。
	MissingTagIDs []int64
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
	ID    int64
	State ScanState
	// Origin は走査を始めた主体である。アップグレード前の行は manual になる。
	Origin     ScanOrigin
	StartedAt  time.Time
	FinishedAt time.Time
	Total      int
	Completed  int
	Failed     int
	// Error は走査そのものが失敗した理由。個別のファイルの失敗は含まない。
	Error string
	// ErrorCode は State が failed のときの理由のコード。アップグレード前の行では空である
	//
	ErrorCode ScanErrorCode
	// ErrorPath は理由が結び付く場所（メディアフォルダ、または読めなかった場所）の
	// 絶対パス。特定の場所に結び付かなければ空である。
	ErrorPath string
	// Videos はこの走査の対象の動画の本数、SettledVideos はそのうち残りの仕事が
	// 無い本数である。
	Videos        int
	SettledVideos int
	// SettledAt は対象の動画がすべて済んだ時刻。済んでいなければゼロ値である。
	SettledAt time.Time
	// IssuesRevision は問題の記録（scan_issues）を変えるたびに増える番号である
	//
	IssuesRevision int64
	// Import は利用者に見せる取り込みの状態で、internal/app が組み立てる。
	// 保存側は埋めない。
	Import ImportProgress
	// Issues は問題の本数で、internal/app が組み立てる。保存側は埋めない。
	Issues ScanIssueCounts
	// Activity は今の処理で、internal/app がメモリの値から埋める。保存側は埋めない。
	Activity ScanActivity
}

// Tally は走査の記録から、取り込みの状態を決める入力を作る。問題の数は
// 呼び出し側が渡す。
func (s Scan) Tally(unregisteredIssues, failedIssues int) ImportTally {
	return ImportTally{
		State:              s.State,
		ScanTotal:          s.Total,
		ScanCompleted:      s.Completed,
		ScanFailed:         s.Failed,
		Videos:             s.Videos,
		SettledVideos:      s.SettledVideos,
		UnregisteredIssues: unregisteredIssues,
		FailedIssues:       failedIssues,
	}
}

// ScanProgress は進捗の値である。
type ScanProgress struct {
	Total     int
	Completed int
	Failed    int
}
