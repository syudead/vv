package store

import (
	"context"
	"database/sql"
	"encoding/json"
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
// 削除は消す（tag_names と video_tags は ON DELETE CASCADE で消える）。取引が失敗したら
// 何も変えない。
func (s *TagStore) BatchTags(ctx context.Context, action domain.TagBatchAction, ids []int64) (domain.TagBatchOutcome, error) {
	if !action.Valid() {
		return domain.TagBatchOutcome{}, fmt.Errorf("unknown tag batch action %q", action)
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

	tentative, err := tentativeByTagID(ctx, tx, unique)
	if err != nil {
		return domain.TagBatchOutcome{}, err
	}
	for _, id := range unique {
		isTentative, found := tentative[id]
		switch {
		case !found:
			outcome.NotFoundIDs = append(outcome.NotFoundIDs, id)
		case !domain.TagBatchApplies(action, isTentative):
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
	switch action {
	case domain.TagBatchConfirm:
		if _, err := tx.ExecContext(ctx,
			`update tags set tentative = 0 where id in (select value from json_each(?))`, string(encoded),
		); err != nil {
			return domain.TagBatchOutcome{}, fmt.Errorf("cannot confirm the tags: %w", err)
		}
	case domain.TagBatchReject:
		// 名前は消す前に写す。消したあとでは tag_names の行が連鎖して消えている。
		if _, err := tx.ExecContext(ctx, `
			insert or ignore into rejected_tag_names (name, created_at)
			select name, ? from tag_names
			 where canonical = 1 and tag_id in (select value from json_each(?))`,
			time.Now().Unix(), string(encoded),
		); err != nil {
			return domain.TagBatchOutcome{}, fmt.Errorf("cannot remember the rejected tag names: %w", err)
		}
		if err := deleteTagsByIDs(ctx, tx, string(encoded)); err != nil {
			return domain.TagBatchOutcome{}, err
		}
	case domain.TagBatchDelete:
		if err := deleteTagsByIDs(ctx, tx, string(encoded)); err != nil {
			return domain.TagBatchOutcome{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.TagBatchOutcome{}, fmt.Errorf("cannot apply the tag batch: %w", err)
	}
	return outcome, nil
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

	tentative, err := tentativeByTagID(ctx, tx, unique)
	if err != nil {
		return domain.TagImpact{}, err
	}
	counted := make([]int64, 0, len(unique))
	for _, id := range unique {
		if isTentative, found := tentative[id]; found && domain.TagImpactApplies(action, isTentative) {
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

// tentativeByTagID は ids のうち今あるタグの仮かどうかを、id ごとに返す。無い id は入らない。
func tentativeByTagID(ctx context.Context, q queryExecer, ids []int64) (map[int64]bool, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build tag ids: %w", err)
	}
	rows, err := q.QueryContext(ctx,
		`select id, tentative from tags where id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tentative := make(map[int64]bool, len(ids))
	for rows.Next() {
		var id int64
		var isTentative bool
		if err := rows.Scan(&id, &isTentative); err != nil {
			return nil, fmt.Errorf("cannot read the tags: %w", err)
		}
		tentative[id] = isTentative
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the tags: %w", err)
	}
	return tentative, nil
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
