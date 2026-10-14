package store

import (
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 管理画面の一覧（タグ・人物・団体）が共有する並び順とカーソル（specs/036-tag-admin-scale/
// research.md R-1、specs/047-video-work-info/data-model.md「Entities」）。どの一覧も listed に
// id・sort_key・video_count・created_at の列を持ち、値が同じ行は名前の自然順（sort_key）、
// それも同じなら id で並べる。

// adminListOrder は管理画面の一覧の並び順 1 つ分である。値の向きと名前の向きが違うので、
// keyset の条件は行値の比較 1 つではなく展開した形で書く。
type adminListOrder struct {
	// value は名前の前に比べる listed の列。名前の順では空。
	value string
	// desc は value を降順に並べる。名前（sort_key, id）はいつも昇順。
	desc    bool
	orderBy string
}

func newAdminListOrder(value string, desc bool) adminListOrder {
	order := adminListOrder{value: value, desc: desc, orderBy: `sort_key asc, id asc`}
	if value != "" {
		direction := ` asc`
		if desc {
			direction = ` desc`
		}
		order.orderBy = value + direction + `, ` + order.orderBy
	}
	return order
}

var adminListOrders = map[domain.TagSort]adminListOrder{
	domain.TagSortName:        newAdminListOrder("", false),
	domain.TagSortCountDesc:   newAdminListOrder("video_count", true),
	domain.TagSortCountAsc:    newAdminListOrder("video_count", false),
	domain.TagSortCreatedDesc: newAdminListOrder("created_at", true),
	domain.TagSortCreatedAsc:  newAdminListOrder("created_at", false),
}

// adminListRow はカーソルに包む 1 行の値である。
type adminListRow struct {
	id         int64
	videoCount int
	createdAt  time.Time
	sortKey    string
}

// cursorValue は名前の前に比べる値（本数・作った日の Unix 秒）である。
func (o adminListOrder) cursorValue(row adminListRow) string {
	switch o.value {
	case "video_count":
		return strconv.Itoa(row.videoCount)
	case "created_at":
		return strconv.FormatInt(row.createdAt.Unix(), 10)
	default:
		return ""
	}
}

// encodeCursor は「一覧の種類・並び順の名前・値・id・名前の鍵」を包む。名前の鍵は最後に置く。
// 一覧の種類（tags・people・organizations）を先頭に置き、別の一覧のカーソルと取り違えないようにする。
func (o adminListOrder) encodeCursor(list string, sort domain.TagSort, last adminListRow) string {
	return packCursor(list, string(sort), o.cursorValue(last), strconv.FormatInt(last.id, 10), last.sortKey)
}

// cursorCondition はカーソルより後ろだけを取る条件を返す。別の一覧・別の並び順のカーソルや
// 解釈できないものは ErrInvalidCursor。
func (o adminListOrder) cursorCondition(list string, sort domain.TagSort, cursor string) (string, []any, error) {
	parts, err := unpackCursor(cursor, 5)
	if err != nil {
		return "", nil, err
	}
	if parts[0] != list {
		return "", nil, fmt.Errorf("%w: cursor is not for %s", domain.ErrInvalidCursor, list)
	}
	if parts[1] != string(sort) {
		return "", nil, fmt.Errorf("%w: cursor is for a different sort order", domain.ErrInvalidCursor)
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return "", nil, fmt.Errorf("%w: cannot parse the id", domain.ErrInvalidCursor)
	}
	sortKey := parts[4]
	nameAfter := `(sort_key, id) > (?, ?)`
	if o.value == "" {
		if parts[2] != "" {
			return "", nil, fmt.Errorf("%w: unexpected sort value", domain.ErrInvalidCursor)
		}
		return nameAfter, []any{sortKey, id}, nil
	}
	value, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", nil, fmt.Errorf("%w: cannot parse the sort value", domain.ErrInvalidCursor)
	}
	op := ` > `
	if o.desc {
		op = ` < `
	}
	clause := `(` + o.value + op + `? or (` + o.value + ` = ? and ` + nameAfter + `))`
	return clause, []any{value, value, sortKey, id}, nil
}

// pageStatement は listed（CTE）から columns と条件に合う数 total を 1 ページ読む文と引数を返す。
// total は窓関数でカーソルを掛ける前の listed 全体について数え、ページの行と同じ問い合わせで
// 返す（listed の本数の集計を 2 回走らせない）。limit が正なら、続きがあるかを知るために 1 件多く取る。
func (o adminListOrder) pageStatement(listed string, listedArgs []any, columns, cursorClause string, cursorArgs []any, limit int) (string, []any) {
	statement := listed + ` select ` + columns + `, total from (
		select ` + columns + `, count(*) over () as total from listed
	)`
	args := append([]any{}, listedArgs...)
	if cursorClause != "" {
		statement += ` where ` + cursorClause
		args = append(args, cursorArgs...)
	}
	statement += ` order by ` + o.orderBy
	if limit > 0 {
		statement += ` limit ?`
		args = append(args, limit+1)
	}
	return statement, args
}
