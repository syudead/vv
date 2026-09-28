package store

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// ListTags はタグを名前の自然順ですべて返す。本数 0 のタグも返す
// （data-model.md §5）。本数は video_tags を videos.content_key と結び、いま
// ライブラリにある動画だけを数える。3つの問い合わせ（タグ、シノニム、本数）に
// 分けるのは、タグごとに引き直すと N+1 になるためである。
func (s *TagStore) ListTags(ctx context.Context) ([]domain.Tag, error) {
	tags, index, err := listCanonicalTags(ctx, s.sql)
	if err != nil {
		return nil, err
	}
	if err := addSynonymsToTags(ctx, s.sql, tags, index); err != nil {
		return nil, err
	}
	if err := addVideoCountsToTags(ctx, s.sql, tags, index); err != nil {
		return nil, err
	}
	for i := range tags {
		domain.SortTagNames(tags[i].Synonyms)
	}
	domain.SortTags(tags)
	return tags, nil
}

// listCanonicalTags はタグごとの元の名前を読み、id から一覧内の位置への
// 対応も返す。
func listCanonicalTags(ctx context.Context, q queryExecer) ([]domain.Tag, map[int64]int, error) {
	rows, err := q.QueryContext(ctx, `
		select t.id, tn.name from tags t
		  join tag_names tn on tn.tag_id = t.id and tn.canonical = 1`)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tags := []domain.Tag{}
	index := make(map[int64]int)
	for rows.Next() {
		var tag domain.Tag
		if err := rows.Scan(&tag.ID, &tag.Name); err != nil {
			return nil, nil, fmt.Errorf("cannot read tags: %w", err)
		}
		tag.Synonyms = []string{}
		index[tag.ID] = len(tags)
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("cannot read tags: %w", err)
	}
	return tags, index, nil
}

// addSynonymsToTags は tags[index[tag_id]].Synonyms にシノニムを足す。
func addSynonymsToTags(ctx context.Context, q queryExecer, tags []domain.Tag, index map[int64]int) error {
	rows, err := q.QueryContext(ctx, `select tag_id, name from tag_names where canonical = 0`)
	if err != nil {
		return fmt.Errorf("cannot read synonyms: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var tagID int64
		var name string
		if err := rows.Scan(&tagID, &name); err != nil {
			return fmt.Errorf("cannot read synonyms: %w", err)
		}
		if i, ok := index[tagID]; ok {
			tags[i].Synonyms = append(tags[i].Synonyms, name)
		}
	}
	return rows.Err()
}

// addVideoCountsToTags は tags[index[tag_id]].VideoCount に、いまライブラリに
// ある動画だけを数えた本数を足す。手で付けた分とフォルダ名から付いている分の
// どちらかで付いていれば1本と数える（017 の data-model.md §4）。
func addVideoCountsToTags(ctx context.Context, q queryExecer, tags []domain.Tag, index map[int64]int) error {
	query := `select tag_id, count(*) from (` + taggedVideosSQL("") + `) group by tag_id`
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("cannot count tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var tagID int64
		var count int
		if err := rows.Scan(&tagID, &count); err != nil {
			return fmt.Errorf("cannot count tags: %w", err)
		}
		if i, ok := index[tagID]; ok {
			tags[i].VideoCount = count
		}
	}
	return rows.Err()
}
