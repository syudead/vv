package domain

// タグ管理画面の一覧のページ読み（specs/036-tag-admin-scale/data-model.md §1、research.md R-1）。

// TagSort はタグの一覧の並び順である。API の TagSort と同じ文字列を持つ。値が同じ
// タグは名前の自然順（tag_names.sort_key）、それも同じなら id で並べる。
type TagSort string

const (
	// TagSortName は名前の自然順（既定）。向きは無い。
	TagSortName TagSort = "name"
	// TagSortCountDesc は本数の多い順。
	TagSortCountDesc TagSort = "countDesc"
	// TagSortCountAsc は本数の少ない順。
	TagSortCountAsc TagSort = "countAsc"
	// TagSortCreatedDesc は作った日の新しい順。
	TagSortCreatedDesc TagSort = "createdDesc"
	// TagSortCreatedAsc は作った日の古い順。
	TagSortCreatedAsc TagSort = "createdAsc"
)

// Valid は s が既知の並び順であることを返す。
func (s TagSort) Valid() bool {
	switch s {
	case TagSortName, TagSortCountDesc, TagSortCountAsc, TagSortCreatedDesc, TagSortCreatedAsc:
		return true
	default:
		return false
	}
}

// MaxTagPageLimit はタグの一覧の 1 ページの件数の上限である（GET /api/library と同じ）。
const MaxTagPageLimit = 200

// TagListQuery はタグの一覧の条件である。
type TagListQuery struct {
	// Search は呼び手が受けた生の検索語。店が FoldForMatch を掛けて前後の空白を落とし、
	// 空でなければ元の名前かシノニムの照合形に部分一致するタグだけにする。
	Search string
	// TentativeOnly は仮のタグだけにする。
	TentativeOnly bool
	// UnusedOnly は本数 0 のタグだけにする。
	UnusedOnly bool
	// Sort は並び順。空なら TagSortName。
	Sort TagSort
	// Cursor は前のページの TagPage.NextCursor。解釈できないものや別の並び順の
	// ものは ErrInvalidCursor。
	Cursor string
	// Limit は 1 ページの件数。0 なら全件を返し、Cursor は無視する。
	Limit int
}

// TagPage はタグの一覧の 1 ページである。
type TagPage struct {
	Items []Tag
	// Total は条件（検索・絞り込み）に合うタグの数。
	Total int
	// TotalAll は全部のタグの数。
	TotalAll int
	// NextCursor は続きの取得に渡す。続きが無ければ空。
	NextCursor string
	// Exact は検索語と元の名前かシノニムの綴りが完全に一致し、絞り込みにも合うタグ。
	// Limit を付けた 1 ページ目（Cursor が空）で、検索語が空でないときだけ引く。
	// 無ければ nil。
	Exact *Tag
}
