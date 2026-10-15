package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// タグ管理画面のまとめての確定・却下・削除と、確認に出す数
// （specs/036-tag-admin-scale/data-model.md §2、research.md R-4・R-6）。
//
// どちらも ids を json_each に 1 つの引数で渡し、id の数によらず決まった数の文で行う
// （external_video_tags.go の applyManualTags と同じ）。ドメインイベントは発行しない。

// BatchTags は ids のタグに action を 1 つの取引で行う。ids の重複は 1 つとして扱い、無い id は
// NotFoundIDs、domain.TagBatchApplies が偽の id は何も変えずに NotApplicableIDs、処理した id は
// AppliedIDs に、それぞれ ids に現れた順で入れる。確定は仮のタグを確定にし
// （specs/031-tentative-tags/data-model.md §3）、却下は元の名前を却下した名前に入れてから消し、
// 削除は消す（tag_names と video_tags は ON DELETE CASCADE で消える）。種類を付ける・外すのは
// tags.kind_id だけを書き、仮かどうかは変えない（specs/051-tag-kinds/data-model.md「Rules」）。
// 種類を付けるときは同じ取引で先に種類を読み、無ければ domain.ErrTagKindNotFound で何も
// 変えない。取引が失敗したら何も変えない。
func (s *TagStore) BatchTags(ctx context.Context, req domain.TagBatchRequest, ids []int64) (domain.TagBatchOutcome, error) {
	action := req.Action
	if !action.Valid() {
		return domain.TagBatchOutcome{}, fmt.Errorf("unknown tag batch action %q", action)
	}
	if (action == domain.TagBatchSetKind) != (req.KindID != 0) {
		return domain.TagBatchOutcome{}, fmt.Errorf("tag batch action %q with kind id %d", action, req.KindID)
	}
	outcome := domain.TagBatchOutcome{AppliedIDs: []int64{}, NotFoundIDs: []int64{}, NotApplicableIDs: []int64{}}
	unique := uniqueTagIDs(ids)
	if len(unique) == 0 {
		return outcome, nil
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagBatchOutcome{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if action == domain.TagBatchSetKind {
		if err := requireTagKind(ctx, tx, req.KindID); err != nil {
			return domain.TagBatchOutcome{}, err
		}
	}
	targets, err := tagBatchTargets(ctx, tx, unique)
	if err != nil {
		return domain.TagBatchOutcome{}, err
	}
	for _, id := range unique {
		target, found := targets[id]
		switch {
		case !found:
			outcome.NotFoundIDs = append(outcome.NotFoundIDs, id)
		case !domain.TagBatchApplies(req, target):
			outcome.NotApplicableIDs = append(outcome.NotApplicableIDs, id)
		default:
			outcome.AppliedIDs = append(outcome.AppliedIDs, id)
		}
	}
	if len(outcome.AppliedIDs) == 0 {
		return outcome, nil
	}

	encoded, err := json.Marshal(outcome.AppliedIDs)
	if err != nil {
		return domain.TagBatchOutcome{}, fmt.Errorf("cannot build tag ids: %w", err)
	}
	if err := applyTagBatch(ctx, tx, req, string(encoded)); err != nil {
		return domain.TagBatchOutcome{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.TagBatchOutcome{}, fmt.Errorf("cannot apply the tag batch: %w", err)
	}
	return outcome, nil
}

// applyTagBatch は JSON の配列 encodedIDs のタグに操作 req を書く。どの id も操作が働くタグである。
func applyTagBatch(ctx context.Context, tx *sql.Tx, req domain.TagBatchRequest, encodedIDs string) error {
	switch req.Action {
	case domain.TagBatchConfirm:
		if _, err := tx.ExecContext(ctx,
			`update tags set tentative = 0 where id in (select value from json_each(?))`, encodedIDs,
		); err != nil {
			return fmt.Errorf("cannot confirm the tags: %w", err)
		}
	case domain.TagBatchReject:
		// 名前は消す前に写す。消したあとでは tag_names の行が連鎖して消えている。
		if err := rememberRejectedNames(ctx, tx, encodedIDs); err != nil {
			return err
		}
		return deleteTagsByIDs(ctx, tx, encodedIDs)
	case domain.TagBatchDelete:
		return deleteTagsByIDs(ctx, tx, encodedIDs)
	case domain.TagBatchSetKind, domain.TagBatchClearKind:
		// clearKind では req.KindID が 0 なので null を書く。
		var kindID any
		if req.KindID != 0 {
			kindID = req.KindID
		}
		if _, err := tx.ExecContext(ctx,
			`update tags set kind_id = ? where id in (select value from json_each(?))`, kindID, encodedIDs,
		); err != nil {
			return fmt.Errorf("cannot set the kind of the tags: %w", err)
		}
	}
	return nil
}

// requireTagKind は id の種類があることを確かめ、無ければ domain.ErrTagKindNotFound を返す。
func requireTagKind(ctx context.Context, tx *sql.Tx, id int64) error {
	var found int
	err := tx.QueryRowContext(ctx, `select 1 from tag_kinds where id = ?`, id).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrTagKindNotFound
	}
	if err != nil {
		return fmt.Errorf("cannot read the tag kind: %w", err)
	}
	return nil
}

// TagImpact は、ids のうち今あり domain.TagImpactApplies が真のタグの数と、そのどれかが付いた
// いまライブラリにある動画の本数を返す。動画は手で付けた分とフォルダ名から付いている分の
// どちらでも 1 本で、id で重複を除いて数える（specs/014-video-tags/data-model.md §5）。
// 何も変えない。
func (s *TagStore) TagImpact(ctx context.Context, action domain.TagImpactAction, ids []int64) (domain.TagImpact, error) {
	if !action.Valid() {
		return domain.TagImpact{}, fmt.Errorf("unknown tag impact action %q", action)
	}
	unique := uniqueTagIDs(ids)
	if len(unique) == 0 {
		return domain.TagImpact{}, nil
	}

	// タグの種類と本数を同じ読み取りスナップショットで読む（Summary と同じ）。
	tx, err := s.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.TagImpact{}, err
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := tagBatchTargets(ctx, tx, unique)
	if err != nil {
		return domain.TagImpact{}, err
	}
	counted := make([]int64, 0, len(unique))
	for _, id := range unique {
		if target, found := targets[id]; found && domain.TagImpactApplies(action, target.Tentative) {
			counted = append(counted, id)
		}
	}
	if len(counted) == 0 {
		return domain.TagImpact{}, nil
	}

	encoded, err := json.Marshal(counted)
	if err != nil {
		return domain.TagImpact{}, fmt.Errorf("cannot build tag ids: %w", err)
	}
	query := `select count(distinct video_id) from (` +
		taggedVideosSQL(` and tag_id in (select value from json_each(?))`) + `)`
	var videoCount int
	if err := tx.QueryRowContext(ctx, query, string(encoded), string(encoded)).Scan(&videoCount); err != nil {
		return domain.TagImpact{}, fmt.Errorf("cannot count videos with the tags: %w", err)
	}
	return domain.TagImpact{TagCount: len(counted), VideoCount: videoCount}, nil
}

// uniqueTagIDs は ids の重複を除き、最初に現れた順に並べて返す。
func uniqueTagIDs(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	return unique
}

// tagBatchTargets は ids のうち今あるタグの仮かどうかと種類を、id ごとに返す。無い id は入らない。
func tagBatchTargets(ctx context.Context, q queryExecer, ids []int64) (map[int64]domain.TagBatchTarget, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build tag ids: %w", err)
	}
	rows, err := q.QueryContext(ctx,
		`select id, tentative, coalesce(kind_id, 0) from tags where id in (select value from json_each(?))`,
		string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	targets := make(map[int64]domain.TagBatchTarget, len(ids))
	for rows.Next() {
		var id int64
		var target domain.TagBatchTarget
		if err := rows.Scan(&id, &target.Tentative, &target.KindID); err != nil {
			return nil, fmt.Errorf("cannot read the tags: %w", err)
		}
		targets[id] = target
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the tags: %w", err)
	}
	return targets, nil
}

// deleteTagsByIDs は JSON の配列 encodedIDs のタグを消す。tag_names と video_tags は外部キーの
// ON DELETE CASCADE で連鎖して消える。
func deleteTagsByIDs(ctx context.Context, tx *sql.Tx, encodedIDs string) error {
	if _, err := tx.ExecContext(ctx,
		`delete from tags where id in (select value from json_each(?))`, encodedIDs,
	); err != nil {
		return fmt.Errorf("cannot delete the tags: %w", err)
	}
	return nil
}

// rememberRejectedNames は encodedIDs（JSON の id の配列）のタグの元の名前を、名前の自然順の鍵
// （domain.NaturalSortKey）と現在の版を添えて却下した名前に insert or ignore する
// （specs/036-tag-admin-scale/data-model.md §0・§2）。鍵は SQL で作れないので、名前を 1 回で読んで
// Go で鍵を作り、名前と鍵の組を json_each に 1 つの引数で渡して 1 つの文で書く。
func rememberRejectedNames(ctx context.Context, tx *sql.Tx, encodedIDs string) error {
	pairs, err := rejectedNamePairs(ctx, tx, encodedIDs)
	if err != nil {
		return fmt.Errorf("cannot read the rejected tag names: %w", err)
	}
	if len(pairs) == 0 {
		return nil
	}

	encoded, err := json.Marshal(pairs)
	if err != nil {
		return fmt.Errorf("cannot build the rejected tag names: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		insert or ignore into rejected_tag_names (name, sort_key, search_version, created_at)
		select json_extract(value, '$[0]'), json_extract(value, '$[1]'), ?, ?
		  from json_each(?)`,
		domain.SearchKeyVersion, time.Now().Unix(), string(encoded),
	); err != nil {
		return fmt.Errorf("cannot remember the rejected tag names: %w", err)
	}
	return nil
}

// rejectedNamePairs は encodedIDs のタグの元の名前と、その名前の自然順の鍵の組を返す。
func rejectedNamePairs(ctx context.Context, tx *sql.Tx, encodedIDs string) ([][2]string, error) {
	rows, err := tx.QueryContext(ctx, `
		select name from tag_names
		 where canonical = 1 and tag_id in (select value from json_each(?))`, encodedIDs)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var pairs [][2]string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		pairs = append(pairs, [2]string{name, domain.NaturalSortKey(name)})
	}
	return pairs, rows.Err()
}
