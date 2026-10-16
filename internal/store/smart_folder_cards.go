package store

// スマートフォルダのカードの読み出し（GET /api/smart-folders/cards、specs/052-smart-folders/
// data-model.md「Cards read」、research.md R-14）。画面に出たカードの値を、フォルダの一致する動画
// （smart_folder_matches.go）から読む。グループにまとめないので、本数は一致する動画の数で、絵柄と
// 開く動画はフォルダの並べ順の先頭から選ぶ。

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// SmartFolderCards は見る人（audience）に見せるスマートフォルダ ids のカードの値を、ids の順に
// 返す。無いフォルダと、ゲストに見せないフォルダ（再生順が空）の id は飛ばし、同じ id は 1 回だけ
// 返す。経路の条件が無い実体を名指すフォルダは何にも一致しない（research.md R-9）。フォルダによらない
// 部分は 1 度だけ求め、どのフォルダの値も同じ読み取りスナップショットの 1 つの問い合わせで読む
// （research.md R-13）。
func (s *SmartFolderStore) SmartFolderCards(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.SmartFolderCard, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot start reading the smart folder cards: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := shownSmartFolderTargets(ctx, tx, audience, ids, listNow().Unix())
	if err != nil {
		return nil, err
	}
	cards := make([]domain.SmartFolderCard, len(targets))
	for i, target := range targets {
		cards[i] = domain.SmartFolderCard{SmartFolderSummary: target.summary, WatchState: domain.WatchStateUnwatched,
			Previews: []domain.Video{}, Tags: []domain.VideoTag{}}
	}
	if err := readSmartFolderCardValues(ctx, tx, audience, targets, cards); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot finish reading the smart folder cards: %w", err)
	}
	return cards, nil
}

// smartCardRow はカードの読み出しの問い合わせの 1 行（フォルダ 1 つ）である。
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

// readSmartFolderCardValues はフォルダ targets のカードの値を 1 つの問い合わせで読み、同じ位置の
// cards[i] に入れる。フォルダごとに一致する動画を m<i> に 1 度だけ絞り、値をそこから集める。
// 絵柄の動画はまとめて読む。
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

// smartCardSelect はフォルダ index の一致する動画 m<index> からカードの値を集める select 文である。
// orderBy はフォルダの並べ順（sort_value と id）、owner は所有者の読み出しかで、所有者にはタグの
// 和集合も集める。開く動画は GroupOpenIndex と同じく、並べ順で見ている途中の最初、無ければ見ていない
// 最初、無ければ先頭の動画で、1 回の並べ替えで選ぶ。ゲストには再生の記録を見せないので、先頭の動画に
// する（NewLibraryGroup と同じ）。
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

// smartCardTagsSQL は一致する動画 m のタグの和集合を `[[id, 名前, タグの仮, 手, フォルダ名, 未確定], …]`
// の JSON にする副問い合わせである。出所は tagsByContentKeys と同じく手で付けた分（利用者データの鍵）
// とフォルダ名から付く分で、どれかの動画で真なら真、未確定はどの付与も未確定のときだけ真にする。
// フォルダ名の分はタグの名前から引く（cross join で結ぶ順を決める）。
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

// apply は行の値をカード card に入れ、絵柄の動画の id をフォルダの並べ順で返す。
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

// decodeSmartCardTags は smartCardTagsSQL の JSON をタグにし、SortVideoTags の順に並べる。
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

// loadSmartCardPreviews はカードごとの絵柄の動画 previews（カードの位置から id の並び）を 1 度に読み、
// cards[i].Previews に同じ順で入れる。
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
