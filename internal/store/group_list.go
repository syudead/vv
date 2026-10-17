package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// groupListCursorKind はグループの一覧のカーソルの最初の項目である。ほかの一覧のカーソルを
// 渡されたときに読めないカーソルとして断る。
const groupListCursorKind = "groups"

// groupListCandidate は一覧の候補のグループ 1 つである。
type groupListCandidate struct {
	id       int64
	titleKey string
}

// ListGroups は見る人（audience）に見せるグループを、どの深さのものも自然な名前の順（title_key、
// 同じなら id の順）に、cursor の次から limit 件返す（GET /api/groups、
// docs/design-docs/catalog-organization.md）。query は前後の空白を取り除き、名前が大文字小文字を
// 区別せずにそれを含むものだけに絞る（空なら全部）。見せる動画の無いグループは入らない。読めない cursor は domain.ErrInvalidCursor である。
func (s *LibraryStore) ListGroups(ctx context.Context, audience domain.Audience, query, cursor string, limit int) (domain.GroupListPage, error) {
	after, err := decodeGroupListCursor(cursor)
	if err != nil {
		return domain.GroupListPage{}, err
	}
	if limit <= 0 {
		limit = domain.GroupListDefaultLimit
	}
	limit = min(limit, domain.GroupListMaxLimit)

	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.GroupListPage{}, fmt.Errorf("cannot start reading the groups: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	candidates, err := groupListCandidates(ctx, tx, query, after)
	if err != nil {
		return domain.GroupListPage{}, err
	}
	// 見せる動画の無いグループを除くため、候補を limit+1 件ずつ読み、見せるものが limit+1 件
	// たまるまで続ける。
	var shown []groupListCandidate
	page := domain.GroupListPage{}
	for start := 0; start < len(candidates) && len(shown) <= limit; start += limit + 1 {
		chunk := candidates[start:min(start+limit+1, len(candidates))]
		ids := make([]int64, 0, len(chunk))
		for _, candidate := range chunk {
			ids = append(ids, candidate.id)
		}
		groups, err := loadGroups(ctx, tx, audience, ids)
		if err != nil {
			return domain.GroupListPage{}, err
		}
		for _, candidate := range chunk {
			group, ok := groups[candidate.id]
			if !ok || len(shown) > limit {
				continue
			}
			shown = append(shown, candidate)
			page.Groups = append(page.Groups, group)
		}
	}
	if len(shown) > limit {
		last := shown[limit-1]
		page.Groups = page.Groups[:limit]
		page.NextCursor = packCursor(groupListCursorKind, strconv.FormatInt(last.id, 10), last.titleKey)
	}
	if page.Roots, err = listMediaFolders(ctx, tx); err != nil {
		return domain.GroupListPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.GroupListPage{}, fmt.Errorf("cannot finish reading the groups: %w", err)
	}
	return page, nil
}

// decodeGroupListCursor は一覧のカーソルを、その位置のグループに戻す。空なら nil である。
func decodeGroupListCursor(cursor string) (*groupListCandidate, error) {
	if cursor == "" {
		return nil, nil
	}
	parts, err := unpackCursor(cursor, 3)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if parts[0] != groupListCursorKind || err != nil || id < 1 {
		return nil, fmt.Errorf("%w: not a group list cursor", domain.ErrInvalidCursor)
	}
	return &groupListCandidate{id: id, titleKey: parts[2]}, nil
}

// groupListCandidates は、after より後ろ（nil なら最初から）のグループのうち名前が query を含む
// ものを、一覧の順に返す。大文字小文字の区別は SQLite の lower() では ASCII にしか効かないので、
// 名前を読んでから domain.FoldKey で比べる（strings.ToLower では `Σ` と `ς` が同じにならない）。
func groupListCandidates(ctx context.Context, tx *sql.Tx, query string, after *groupListCandidate) ([]groupListCandidate, error) {
	where, args := "", []any{}
	if after != nil {
		where = ` where title_key > ?1 or (title_key = ?1 and id > ?2)`
		args = append(args, after.titleKey, after.id)
	}
	rows, err := tx.QueryContext(ctx, `select id, name, title_key from groups`+where+` order by title_key, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	needle := domain.FoldKey(strings.TrimSpace(query))
	var out []groupListCandidate
	for rows.Next() {
		var candidate groupListCandidate
		var name string
		if err := rows.Scan(&candidate.id, &name, &candidate.titleKey); err != nil {
			return nil, fmt.Errorf("cannot read the groups: %w", err)
		}
		if needle == "" || strings.Contains(domain.FoldKey(name), needle) {
			out = append(out, candidate)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the groups: %w", err)
	}
	return out, nil
}
