package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// Summary は videoIDs のうちいまライブラリにある動画の数（Total）と、1本以上に
// 付いているタグごとの本数（Items）を、名前の自然順で返す
// （contracts/tags-api.md §4 の summary）。
func (s *TagStore) Summary(ctx context.Context, videoIDs []int64) (domain.TagSummary, error) {
	// videoIDs → 利用者データの鍵の解決と本数の集計を同じ読み取りスナップショットで
	// 行う。別々に読むと、その間の付け外しで Total と Items の本数が食い違いうる。
	tx, err := s.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.TagSummary{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// 動画の数は利用者データの鍵の数で、同じ集まりの動画は 1 本と数える
	// （specs/030-video-versions/data-model.md §3）。
	keys, err := userKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return domain.TagSummary{}, err
	}
	summary := domain.TagSummary{Total: len(keys), Items: []domain.TagSummaryItem{}}
	if len(keys) == 0 {
		return summary, nil
	}

	encodedIDs, err := json.Marshal(videoIDs)
	if err != nil {
		return domain.TagSummary{}, fmt.Errorf("cannot build video ids: %w", err)
	}

	// count はどちらかの出所で、manualCount は手で付けた分だけで数える（017 の
	// data-model.md §4）。1本の動画に同じタグが複数のフォルダ名（元の名前と
	// シノニムなど）から当たりうるので、content_key の重複を除いて数える。
	rows, err := tx.QueryContext(ctx, `
		with selected(video_id, content_key) as (
			select v.id, `+userKeyExpr("v")+` from videos v
			 where v.id in (select value from json_each(?)) and v.content_key <> '' and `+
		registeredVideoCondition("v")+`),
		tagged(content_key, tag_id, manual) as (
			select vt.content_key, vt.tag_id, 1 from video_tags vt
			 where vt.content_key in (select content_key from selected)
			union all
			select s.content_key, folder_tn.tag_id, 0 from selected s
			  join video_folder_names vfn on vfn.video_id = s.video_id
			  join tag_names folder_tn on folder_tn.name = vfn.name
		)
		select t.tag_id, tn.name, tg.tentative, count(distinct t.content_key),
		       count(distinct case when t.manual = 1 then t.content_key end)
		  from tagged t
		  join tag_names tn on tn.tag_id = t.tag_id and tn.canonical = 1
		  join tags tg on tg.id = t.tag_id
		 group by t.tag_id`, string(encodedIDs),
	)
	if err != nil {
		return domain.TagSummary{}, fmt.Errorf("cannot read tag summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var item domain.TagSummaryItem
		if err := rows.Scan(&item.Tag.ID, &item.Tag.Name, &item.Tag.Tentative, &item.Count, &item.ManualCount); err != nil {
			return domain.TagSummary{}, fmt.Errorf("cannot read tag summaries: %w", err)
		}
		summary.Items = append(summary.Items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.TagSummary{}, fmt.Errorf("cannot read tag summaries: %w", err)
	}
	if err := rows.Close(); err != nil {
		return domain.TagSummary{}, fmt.Errorf("cannot read tag summaries: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.TagSummary{}, fmt.Errorf("cannot read tag summaries: %w", err)
	}
	domain.SortTagSummaryItems(summary.Items)
	return summary, nil
}
