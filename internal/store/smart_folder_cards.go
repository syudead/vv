package store

// Card counts and previews come from matching videos without group collapsing, ordered by the folder's playback order.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// smartCardBatch bounds the folders one card statement reads.
const smartCardBatch = 50

// SmartFolderCards deduplicates in input order and reads visible cards in few queries; missing predicate references
// match nothing.
func (s *SmartFolderStore) SmartFolderCards(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.SmartFolderCard, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot start reading the smart folder cards: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := shownSmartFolderTargets(ctx, tx, audience, ids)
	if err != nil {
		return nil, err
	}
	cards := make([]domain.SmartFolderCard, len(targets))
	for i, target := range targets {
		cards[i] = domain.SmartFolderCard{SmartFolderSummary: target.summary, WatchState: domain.WatchStateUnwatched,
			Previews: []domain.Video{}, Tags: []domain.VideoTag{}}
	}
	for start, end := range smartCardBatches(targets) {
		if err := readSmartFolderCardValues(ctx, tx, audience, targets[start:end], cards[start:end]); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot finish reading the smart folder cards: %w", err)
	}
	return cards, nil
}

// smartCardBatches splits targets into statements holding at most one search folder: SQLite rejects a statement with
// two search CTEs as nested too deep.
func smartCardBatches(targets []smartFolderTarget) iter.Seq2[int, int] {
	return func(yield func(int, int) bool) {
		start, searches := 0, 0
		for i, target := range targets {
			search := !target.missing && smartMatchFor(target.spec, `s`).search != ""
			if i > start && (i-start == smartCardBatch || search && searches > 0) {
				if !yield(start, i) {
					return
				}
				start, searches = i, 0
			}
			if search {
				searches++
			}
		}
		if start < len(targets) {
			yield(start, len(targets))
		}
	}
}

// ListSmartFolders lists every folder for the owner and none for guests, at every depth in natural-name order.
func (s *SmartFolderStore) ListSmartFolders(ctx context.Context, audience domain.Audience) ([]domain.SmartFolderSummary, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot start reading the smart folders: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ids, err := queryInt64s(ctx, tx, `select id from smart_folders order by title_key, id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folders: %w", err)
	}
	targets, err := shownSmartFolderTargets(ctx, tx, audience, ids)
	if err != nil {
		return nil, err
	}
	folders := make([]domain.SmartFolderSummary, 0, len(targets))
	for _, target := range targets {
		folders = append(folders, target.summary)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot finish reading the smart folders: %w", err)
	}
	return folders, nil
}

type smartCardRow struct {
	index                         int
	count, watched, started, size int64
	duration, added, played       sql.NullInt64
	open                          sql.NullInt64
	previews                      string
	tags                          sql.NullString
}

func (r *smartCardRow) dest() []any {
	return []any{&r.index, &r.count, &r.size, &r.duration, &r.added, &r.played, &r.watched, &r.started,
		&r.open, &r.previews, &r.tags}
}

// Filter matching videos once per folder and batch-load preview videos.
func readSmartFolderCardValues(ctx context.Context, tx *sql.Tx, audience domain.Audience, targets []smartFolderTarget, cards []domain.SmartFolderCard) error {
	query := smartFolderBranches(targets, func(query *smartFolderQuery, index int, target smartFolderTarget, match smartMatch) {
		order := itemOrder(target.sort)
		query.ctes = append(query.ctes, `m`+strconv.Itoa(index)+` as materialized (select videos.*, `+order.value+
			` as sort_value from smart_base videos where `+match.condition+`)`)
		query.cteArgs = append(query.cteArgs, match.args...)
		query.parts = append(query.parts, smartCardSelect(index, order.orderBy(), audience.IsOwner()))
	})
	rows, err := query.run(ctx, tx, audience)
	if err != nil {
		return fmt.Errorf("cannot read the smart folder cards: %w", err)
	}
	if rows == nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	previews := map[int][]int64{}
	for rows.Next() {
		var row smartCardRow
		if err := rows.Scan(row.dest()...); err != nil {
			return fmt.Errorf("cannot read the smart folder cards: %w", err)
		}
		if previews[row.index], err = row.apply(&cards[row.index]); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the smart folder cards: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("cannot read the smart folder cards: %w", err)
	}
	for index, earlier := range query.same {
		summary := cards[index].SmartFolderSummary
		cards[index] = cards[earlier]
		cards[index].SmartFolderSummary = summary
		previews[index] = previews[earlier]
	}
	return loadSmartCardPreviews(ctx, tx, audience, previews, cards)
}

// Choose the first in-progress video, then first unwatched, then first overall; guests always open the first without reading history.
func smartCardSelect(index int, orderBy string, owner bool) string {
	m := `m` + strconv.Itoa(index)
	open := orderBy
	tags := `null`
	if owner {
		open = `case watch_state when 'inProgress' then 0 when 'unwatched' then 1 else 2 end, ` + orderBy
		tags = smartCardTagsSQL(m)
	}
	return `select ` + strconv.Itoa(index) + `, count(*), coalesce(sum(size_bytes), 0), sum(duration_ms), max(added_at),
		max(played_at), coalesce(sum(watch_state = 'watched'), 0), coalesce(sum(watch_state <> 'unwatched'), 0),
		(select id from ` + m + ` order by ` + open + ` limit 1),
		(select json_group_array(id) from (select id from ` + m + ` where thumbnail_state = '` +
		string(domain.ThumbnailStateDone) + `' order by ` + orderBy + ` limit ` + strconv.Itoa(domain.MaxFolderPreviews) + `)), ` +
		tags + ` from ` + m
}

// Union tags across matching videos; tentative is true only if every attachment is tentative, and folder tags use fixed join order.
func smartCardTagsSQL(m string) string {
	return `(select json_group_array(json_array(t.tag_id, tn.name, tg.tentative, t.manual, t.from_folder, t.unconfirmed))
		from (select tag_id, max(manual) as manual, max(from_folder) as from_folder, min(unconfirmed) as unconfirmed from (
				select vt.tag_id, 1 as manual, 0 as from_folder, vt.tentative as unconfirmed from video_tags vt
				where vt.content_key in (select user_key from ` + m + ` where content_key <> '')
				union all
				select ftn.tag_id, 0, 1, 0 from tag_names ftn cross join video_folder_names vfn on vfn.name = ftn.name
				where vfn.video_id in (select id from ` + m + ` where content_key <> ''))
			group by tag_id) t
		join tag_names tn on tn.tag_id = t.tag_id and tn.canonical = 1
		join tags tg on tg.id = t.tag_id)`
}

func (r smartCardRow) apply(card *domain.SmartFolderCard) ([]int64, error) {
	card.VideoCount = int(r.count)
	card.SizeBytes = r.size
	card.WatchedCount = int(r.watched)
	card.WatchState = domain.WatchStateOf(card.VideoCount, int(r.started), card.WatchedCount)
	if r.duration.Valid {
		card.DurationMs = &r.duration.Int64
	}
	if r.added.Valid {
		card.AddedAt = time.Unix(r.added.Int64, 0)
	}
	if r.played.Valid {
		played := time.Unix(r.played.Int64, 0)
		card.LastPlayedAt = &played
	}
	card.OpenVideoID = r.open.Int64
	if r.tags.Valid {
		tags, err := decodeSmartCardTags(r.tags.String)
		if err != nil {
			return nil, err
		}
		card.Tags = tags
	}
	var previews []int64
	if err := json.Unmarshal([]byte(r.previews), &previews); err != nil {
		return nil, fmt.Errorf("cannot read the smart folder previews: %w", err)
	}
	return previews, nil
}

func decodeSmartCardTags(encoded string) ([]domain.VideoTag, error) {
	var rows [][]json.RawMessage
	if err := json.Unmarshal([]byte(encoded), &rows); err != nil {
		return nil, fmt.Errorf("cannot read the smart folder tags: %w", err)
	}
	tags := make([]domain.VideoTag, 0, len(rows))
	for _, row := range rows {
		var tag domain.VideoTag
		var tentative, manual, fromFolder, unconfirmed int
		fields := []any{&tag.ID, &tag.Name, &tentative, &manual, &fromFolder, &unconfirmed}
		if len(row) != len(fields) {
			return nil, fmt.Errorf("cannot read the smart folder tags: %d fields", len(row))
		}
		for i, field := range fields {
			if err := json.Unmarshal(row[i], field); err != nil {
				return nil, fmt.Errorf("cannot read the smart folder tags: %w", err)
			}
		}
		tag.Tentative, tag.Manual, tag.FromFolder, tag.Unconfirmed = tentative == 1, manual == 1, fromFolder == 1, unconfirmed == 1
		tags = append(tags, tag)
	}
	domain.SortVideoTags(tags)
	return tags, nil
}

func loadSmartCardPreviews(ctx context.Context, tx *sql.Tx, audience domain.Audience, previews map[int][]int64, cards []domain.SmartFolderCard) error {
	var ids []int64
	for _, list := range previews {
		ids = append(ids, list...)
	}
	videos, _, err := playOrderMembers(ctx, tx, audience, ids)
	if err != nil {
		return err
	}
	byID := make(map[int64]domain.Video, len(videos))
	for _, video := range videos {
		byID[video.ID] = video
	}
	for index, list := range previews {
		for _, id := range list {
			cards[index].Previews = append(cards[index].Previews, byID[id])
		}
	}
	return nil
}
