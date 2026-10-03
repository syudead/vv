package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 仮のタグの確定・却下と、却下した名前（specs/031-tentative-tags/data-model.md §3・§5）。
// 仮のタグを作るのは tentative が真の一括操作（external_video_tags.go の resolveTagNames）で、
// 名前を tag_names に書く入口（insertTagName・RenameTag）が却下した名前から外す。

// ConfirmTag はタグ id を確定したタグにする。既に確定していれば何も変えない。名前と付与は
// 変えない。id が無ければ domain.ErrTagNotFound を返す。
func (s *TagStore) ConfirmTag(ctx context.Context, id int64) (domain.Tag, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tagRefByID(ctx, tx, id); err != nil {
		return domain.Tag{}, err
	}
	if err := confirmTagInTx(ctx, tx, id); err != nil {
		return domain.Tag{}, err
	}
	tag, err := tagByID(ctx, tx, id)
	if err != nil {
		return domain.Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Tag{}, fmt.Errorf("cannot confirm the tag: %w", err)
	}
	return tag, nil
}

// RejectTag は仮のタグ id を消し、その元の名前を却下した名前として覚え、その名前を返す。
// tag_names と video_tags は外部キーの ON DELETE CASCADE で連鎖して消える。id が無ければ
// domain.ErrTagNotFound、仮でなければ domain.ErrTagNotTentative を返し、何も変えない
// （specs/031-tentative-tags/research.md R-5）。
func (s *TagStore) RejectTag(ctx context.Context, id int64) (string, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	ref, err := tagRefByID(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if !ref.Tentative {
		return "", domain.ErrTagNotTentative
	}
	if _, err := tx.ExecContext(ctx, `delete from tags where id = ?`, id); err != nil {
		return "", fmt.Errorf("cannot reject the tag (id=%d): %w", id, err)
	}
	if _, err := tx.ExecContext(ctx,
		`insert or ignore into rejected_tag_names (name, sort_key, search_version, created_at) values (?, ?, ?, ?)`,
		ref.Name, domain.NaturalSortKey(ref.Name), domain.SearchKeyVersion, time.Now().Unix(),
	); err != nil {
		return "", fmt.Errorf("cannot remember the rejected tag name (%s): %w", ref.Name, err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("cannot reject the tag: %w", err)
	}
	return ref.Name, nil
}

// rejectedTagNameCursorSort はカーソルの並び順の項目に入れる名前で、ほかの一覧のカーソルと取り違えない。
const rejectedTagNameCursorSort = "rejectedTagName"

// ListRejectedTagNames は却下した名前を名前の自然順（sort_key、同じなら name のバイト順）で、
// cursor より後ろから limit 件返す（specs/036-tag-admin-scale/data-model.md §2）。limit が 0 以下なら
// cursor より後ろをすべて返し、NextCursor は空にする。Total は却下した名前の全部の数。
// 解釈できないカーソルや別の一覧のカーソルは domain.ErrInvalidCursor を返す。
func (s *TagStore) ListRejectedTagNames(ctx context.Context, cursor string, limit int) (domain.RejectedTagNamePage, error) {
	where, args := "", []any{}
	if cursor != "" {
		sortKey, name, err := decodeRejectedTagNameCursor(cursor)
		if err != nil {
			return domain.RejectedTagNamePage{}, err
		}
		where = ` where (sort_key, name) > (?, ?)`
		args = append(args, sortKey, name)
	}
	query := `select name, sort_key from rejected_tag_names` + where + ` order by sort_key, name`
	if limit > 0 {
		query += ` limit ?`
		args = append(args, limit+1)
	}

	// 件数とページを同じ取引で読み、間の却下・取り外しで Total と Items が食い違わないようにする。
	// 読むだけなので最後は rollback で閉じる。
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.RejectedTagNamePage{}, fmt.Errorf("cannot read rejected tag names: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	page := domain.RejectedTagNamePage{Items: []string{}}
	if err := tx.QueryRowContext(ctx, `select count(*) from rejected_tag_names`).Scan(&page.Total); err != nil {
		return domain.RejectedTagNamePage{}, fmt.Errorf("cannot count rejected tag names: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return domain.RejectedTagNamePage{}, fmt.Errorf("cannot read rejected tag names: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sortKeys []string
	for rows.Next() {
		var name, sortKey string
		if err := rows.Scan(&name, &sortKey); err != nil {
			return domain.RejectedTagNamePage{}, fmt.Errorf("cannot read rejected tag names: %w", err)
		}
		page.Items = append(page.Items, name)
		sortKeys = append(sortKeys, sortKey)
	}
	if err := rows.Err(); err != nil {
		return domain.RejectedTagNamePage{}, fmt.Errorf("cannot read rejected tag names: %w", err)
	}
	if limit > 0 && len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = encodeRejectedTagNameCursor(sortKeys[limit-1], page.Items[limit-1])
	}
	return page, nil
}

// encodeRejectedTagNameCursor は sort_key と name を listing.go と同じ包み方で包む。値の項目に
// 「sort_key、区切り、name」を入れる。sort_key は制御文字を含まない名前（domain.NormalizeTagName）から
// 作るので、区切りの文字を含まない。
func encodeRejectedTagNameCursor(sortKey, name string) string {
	return encodeCursorFields(cursorFields{sort: rejectedTagNameCursorSort, value: sortKey + cursorSeparator + name})
}

// decodeRejectedTagNameCursor は encodeRejectedTagNameCursor の包みを解く。
func decodeRejectedTagNameCursor(cursor string) (sortKey, name string, err error) {
	c, err := decodeCursor(cursor)
	if err != nil {
		return "", "", err
	}
	if c.sort != rejectedTagNameCursorSort || c.seed != "" || c.isNull || c.id != 0 {
		return "", "", fmt.Errorf("%w: cursor is not for rejected tag names", domain.ErrInvalidCursor)
	}
	sortKey, name, ok := strings.Cut(c.value, cursorSeparator)
	if !ok {
		return "", "", fmt.Errorf("%w: cursor has no name", domain.ErrInvalidCursor)
	}
	return sortKey, name, nil
}

// ForgetRejectedTagName は name を却下した名前から外す。無ければ何も変えない。name は
// domain.NormalizeTagName で整えてから照合し、整えられない入力はそのまま照合する
// （RemoveSynonym と同じ扱い）。
func (s *TagStore) ForgetRejectedTagName(ctx context.Context, name string) error {
	if normalized, err := domain.NormalizeTagName(name); err == nil {
		name = normalized
	}
	if _, err := s.sql.ExecContext(ctx, `delete from rejected_tag_names where name = ?`, name); err != nil {
		return fmt.Errorf("cannot forget the rejected tag name: %w", err)
	}
	return nil
}
