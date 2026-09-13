package domain

import (
	"errors"
	"time"
)

// 一覧の件数。上限を設けるのは、1回の応答が青天井に大きくならないように
// するためである（R-109）。
const (
	// DefaultLimit は limit が指定されなかったときの件数。
	DefaultLimit = 60
	// MaxLimit は1ページで返す上限。越える指定は上限に丸める。
	MaxLimit = 200
)

// ErrNotFound は対象が存在しないことを表す。
var ErrNotFound = errors.New("対象が見つかりません")

// ErrInvalidCursor はカーソルが解釈できないことを表す。
//
// 黙って先頭から返さないのは、無限スクロールが巻き戻って同じ内容を延々と
// 表示することになるためである（contracts/http-routes.md）。
var ErrInvalidCursor = errors.New("カーソルを解釈できません")

// VideoSort は一覧の並び順である。値は api/openapi.yaml の VideoSort に対応する。
type VideoSort string

const (
	// SortAddedDesc は追加が新しい順（既定）。
	SortAddedDesc VideoSort = "addedDesc"
	// SortTitleAsc は題名順。
	SortTitleAsc VideoSort = "titleAsc"
)

// Valid は既知の並び順かどうかを返す。
func (s VideoSort) Valid() bool {
	switch s {
	case SortAddedDesc, SortTitleAsc:
		return true
	default:
		return false
	}
}

// VideoQuery は一覧の問い合わせ条件である。
type VideoQuery struct {
	// Query は題名の部分一致。空なら絞り込まない。1文字から指定できる。
	Query string
	Sort  VideoSort
	// Cursor は前回の応答が返した NextCursor。空なら先頭から。
	Cursor string
	Limit  int
}

// VideoPage は一覧1ページ分である。
type VideoPage struct {
	Items []Video
	// Total は絞り込み後の総件数。ページングとは独立に返る（FR-012）。
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

// Scan は走査1回の記録である。進捗として API に出る（R-108）。
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
