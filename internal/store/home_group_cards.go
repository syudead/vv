package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ホームの棚のグループの項目は、カードに要る値だけで作る（docs/design-docs/home.md）。集計の値
// （本数、合計、追加日時、視聴の本数）は判定のときに読んだ根の値（group_root_values の行）から、開くメンバー
// とフォルダの絵柄に差し込むサムネイルは、ページのカードの全部をまとめた 1 つの問い合わせで、再生順の
// 先頭から選ぶ。メンバーの全部は読まない（ライブラリの項目の loadGroups と違い、Members は nil である）。

// groupCardPreviewKind と groupCardOpenKind は、カードの問い合わせの行の種類（開くメンバーか、絵柄の
// サムネイルか）である。
const (
	groupCardOpenKind    = 0
	groupCardPreviewKind = 1
)

// loadGroupCards はグループ ids のカードの値を、判定の済んだ根 known の値（開くメンバーと絵柄のサムネイルの
// id を含む）と、ページのカードの全部をまとめた 1 つの問い合わせ（グループの名前・フォルダ・お気に入りと、
// 選んだメンバーのサムネイルの版）から作る。見せる動画の無いグループは結果に入らない。
func (r *homeShelfReader) loadGroupCards(ids []int64, known map[int64]homeShelfRoot) (map[int64]domain.LibraryGroup, error) {
	out := map[int64]domain.LibraryGroup{}
	var picked [][4]int64
	for _, id := range ids {
		values := known[id].values
		if values.members == 0 {
			continue
		}
		picked = append(picked, [4]int64{id, groupCardOpenKind, 0, values.openMember.Int64})
		for i, preview := range values.previews {
			picked = append(picked, [4]int64{id, groupCardPreviewKind, int64(i), preview})
		}
	}
	if len(picked) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(picked)
	if err != nil {
		return nil, fmt.Errorf("cannot build the group cards: %w", err)
	}
	rows, err := r.tx.QueryContext(r.ctx, groupCardsSelect, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the group cards: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rootID int64
		var kind int
		var path, name string
		var favorite bool
		var video domain.Video
		var previewState string
		if err := rows.Scan(&rootID, &kind, &name, &path, &favorite, &video.ID, &video.ContentKey, &previewState,
			&video.ThumbnailPositionMs, &video.ThumbnailRevision, &video.ThumbnailImageRevision); err != nil {
			return nil, fmt.Errorf("cannot read the group cards: %w", err)
		}
		video.PreviewState = domain.PreviewState(previewState)
		card, ok := out[rootID]
		if !ok {
			card = r.groupCard(rootID, path, name, known[rootID].values)
			card.Favorite = favorite
		}
		if kind == groupCardOpenKind {
			card.OpenVideoID = video.ID
		} else {
			card.Previews = append(card.Previews, video.Preview())
		}
		out[rootID] = card
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the group cards: %w", err)
	}
	return out, nil
}

// groupCardsSelect はカードの問い合わせである。引数は選んだメンバーの `[根の id, 種類, 位置, 動画の id]` の
// JSON の配列 1 つで、行はその順に、根のグループの名前・フォルダ・お気に入りと、動画のサムネイルの版
// （overrideColumns のサムネイルの列）を添える。
const groupCardsSelect = `with picked(root_id, kind, position, video_id) as (
		select json_extract(value, '$[0]'), json_extract(value, '$[1]'), json_extract(value, '$[2]'), json_extract(value, '$[3]')
		from json_each(?))
	select picked.root_id, picked.kind, g.name, coalesce(g.origin_folder, ''), ` + "(select gf.favorited_at from group_favorites gf where gf.group_id = g.id) is not null" + `,
		videos.id, videos.content_key, videos.preview_state,
		(select ov.thumbnail_position_ms from video_overrides ov where ov.content_key = videos.content_key and videos.content_key <> ''),
		coalesce((select ov.thumbnail_revision from video_overrides ov where ov.content_key = videos.content_key and videos.content_key <> ''), 0),
		coalesce((select ti.revision from thumbnail_images ti where ti.content_key = videos.content_key and videos.content_key <> ''), 0)
	from picked join groups g on g.id = picked.root_id join videos on videos.id = picked.video_id
	order by picked.root_id, picked.kind, picked.position`

// groupCard は根 rootID のカードの集計の値を、根の値 values から作る（NewLibraryGroup と同じ値）。ゲストには
// 視聴の値と最後に再生した時刻を入れない（NewLibraryGroup に再生の記録を渡さないのと同じ）。
func (r *homeShelfReader) groupCard(rootID int64, path, name string, values rootValuesRow) domain.LibraryGroup {
	card := domain.LibraryGroup{ID: rootID, Path: path, Name: name, VideoCount: values.members,
		SizeBytes: values.sizeBytes.Int64, AddedAt: time.Unix(values.addedAt.Int64, 0), WatchState: domain.WatchStateUnwatched}
	if values.durationMs.Valid {
		duration := values.durationMs.Int64
		card.DurationMs = &duration
	}
	if !r.audience.IsOwner() {
		return card
	}
	card.WatchState, card.WatchedCount = domain.WatchStateOf(values.members, values.started, values.watched), values.watched
	card.LastPlayedAt = nullTime(values.playedAt)
	return card
}

// nullTime は NULL のことのある unix 秒を *time.Time にする。
func nullTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	at := time.Unix(value.Int64, 0)
	return &at
}
