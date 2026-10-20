package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// Summary reports selected library videos and tag counts in natural-name order.
func (s *TagStore) Summary(ctx context.Context, videoIDs []int64) (domain.TagSummary, error) {
	// Resolve user keys and count attachments in one snapshot so totals cannot disagree during concurrent edits.
	tx, err := s.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.TagSummary{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Count shared bundle user keys as one video.
	keys, err := userKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return domain.TagSummary{}, err
	}
	summary := domain.TagSummary{Total: len(keys), Items: []domain.TagSummaryItem{}}
	if len(keys) == 0 {
		return summary, nil
	}

	if summary.Items, err = tagSummaryItems(ctx, tx, videoIDs); err != nil {
		return domain.TagSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TagSummary{}, fmt.Errorf("cannot read tag summaries: %w", err)
	}
	return summary, nil
}

// Count tags of the selected registered videos by user key in the caller's snapshot, in natural-name order.
func tagSummaryItems(ctx context.Context, q queryExecer, videoIDs []int64) ([]domain.TagSummaryItem, error) {
	encodedIDs, err := json.Marshal(videoIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}

	// Deduplicate content keys across matching folder names and synonyms; manualCount includes manual attachments only.
	rows, err := q.QueryContext(ctx, `
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
		       count(distinct case when t.manual = 1 then t.content_key end), `+tagKindColumns+`
		  from tagged t
		  join tag_names tn on tn.tag_id = t.tag_id and tn.canonical = 1
		  join tags tg on tg.id = t.tag_id`+tagKindJoin+`
		 group by t.tag_id`, string(encodedIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot read tag summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []domain.TagSummaryItem{}
	for rows.Next() {
		var item domain.TagSummaryItem
		var kind scannedTagKind
		if err := rows.Scan(append([]any{&item.Tag.ID, &item.Tag.Name, &item.Tag.Tentative, &item.Count, &item.ManualCount},
			kind.targets()...)...); err != nil {
			return nil, fmt.Errorf("cannot read tag summaries: %w", err)
		}
		item.Tag.Kind = kind.ref()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read tag summaries: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("cannot read tag summaries: %w", err)
	}
	domain.SortTagSummaryItems(items)
	return items, nil
}
