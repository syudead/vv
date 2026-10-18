package domain

// TagSort breaks ties by natural name, then ID, matching API values.
type TagSort string

const (
	TagSortName        TagSort = "name"
	TagSortCountDesc   TagSort = "countDesc"
	TagSortCountAsc    TagSort = "countAsc"
	TagSortCreatedDesc TagSort = "createdDesc"
	TagSortCreatedAsc  TagSort = "createdAsc"
)

func (s TagSort) Valid() bool {
	switch s {
	case TagSortName, TagSortCountDesc, TagSortCountAsc, TagSortCreatedDesc, TagSortCreatedAsc:
		return true
	default:
		return false
	}
}

const MaxTagPageLimit = 200

type TagListQuery struct {
	// Storage folds and trims Search, then substring-matches canonical names and synonyms.
	Search        string
	TentativeOnly bool
	UnusedOnly    bool
	Kind          TagKindFilter
	Sort          TagSort
	// Cursor must match the sort order or yields ErrInvalidCursor.
	Cursor string
	// Limit zero returns all rows and ignores Cursor.
	Limit int
}

// TagKindFilter's zero value applies no filter.
type TagKindFilter struct {
	ID int64
	// None and ID are mutually exclusive.
	None bool
}

func (f TagKindFilter) Matches(kind *TagKindRef) bool {
	switch {
	case f.None:
		return kind == nil
	case f.ID != 0:
		return kind != nil && kind.ID == f.ID
	}
	return true
}

type TagPage struct {
	Items      []Tag
	Total      int
	TotalAll   int
	NextCursor string
	// Exact matches original spelling plus filters and is queried only on a limited first page with nonempty search.
	Exact *Tag
}
