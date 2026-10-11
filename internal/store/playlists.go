package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// プレイリスト（specs/044-playlists/data-model.md「Store operations」、research.md R-1〜R-5・R-11）。
// 項目は動画の利用者データの鍵（userKeyExpr）かグループのフォルダの鍵（domain.FolderKey）で持ち、
// ライブラリへの外部キーを持たない。鍵が今ライブラリの動画やグループに引けない項目は、読み出しから
// 外すだけで消さない（R-4）。位置はプレイリストの中で 0 から詰めた整数で、書き込みのたびに
// 詰め直す（R-3）。どの操作もドメインイベントを発行しない（R-10）。

// playlistNow は名前と項目の変更の時刻の元である。試験が時計を止めるために差し替える。
var playlistNow = time.Now

// playlistItemVideoExpr は、プレイリストの項目（別名 alias の playlist_items）の動画の項目が
// 所有者に見せる動画の id を返す式である。集まりの鍵なら集まりの実効の代表、そうでなければ
// 集まりに属さない同じ content_key の動画で、ライブラリに無ければ NULL になる。グループの
// 項目でも NULL になる。
func playlistItemVideoExpr(alias string) string {
	bundleID := `(select pb.id from video_bundles pb where pb.user_key = ` + alias + `.content_key)`
	return `(case when ` + alias + `.content_key is null then null
		when exists (select 1 from video_bundles pb where pb.user_key = ` + alias + `.content_key) then ` +
		effectiveRepresentativeExpr(bundleID, domain.AudienceOwner) + `
		else (select pv.id from videos pv where pv.content_key = ` + alias + `.content_key and pv.content_key <> ''
			and not exists (select 1 from video_bundle_members pm where pm.content_key = pv.content_key)
			and ` + visibleVideoCondition("pv", domain.AudienceOwner) + ` order by pv.id limit 1) end)`
}

// playlistItemShownCondition は、プレイリストの項目（別名 alias）を見せるかの条件句である。
// 動画の項目は playlistItemVideoExpr が引けるとき、グループの項目はそのフォルダの鍵が今
// グループ（folder_groups.path_key）にあり、所有者に見えるメンバーを持つとき見せる（R-4）。
// メンバーの条件は loadGroups と同じで、メンバーを失ったグループの行が索引の作り直しまで
// 残っても、件数と Playlist の項目が食い違わない。
func playlistItemShownCondition(alias string) string {
	return `((` + alias + `.folder_key is not null and exists (select 1 from folder_groups pg
		join folder_group_members pgm on pgm.group_id = pg.id join videos pgv on pgv.id = pgm.video_id
		where pg.path_key = ` + alias + `.folder_key and ` + visibleVideoCondition("pgv", domain.AudienceOwner) + `))
		or ` + playlistItemVideoExpr(alias) + ` is not null)`
}

// playlistSummaryColumns は playlists（別名 p）の要約の列で、scanPlaylist が読む。
var playlistSummaryColumns = `p.id, p.name, p.updated_at, (select count(*) from playlist_items i
	where i.playlist_id = p.id and ` + playlistItemShownCondition("i") + `)`

func scanPlaylist(row interface{ Scan(...any) error }) (domain.Playlist, error) {
	var playlist domain.Playlist
	var updatedAt int64
	if err := row.Scan(&playlist.ID, &playlist.Name, &updatedAt, &playlist.ItemCount); err != nil {
		return domain.Playlist{}, err
	}
	playlist.UpdatedAt = time.UnixMilli(updatedAt)
	return playlist, nil
}

// ListPlaylists はすべてのプレイリストを、更新日時の新しい順、同じなら id の大きい順に返す。
// 項目の数は見せる項目の数である。
func (s *PlaylistStore) ListPlaylists(ctx context.Context) ([]domain.Playlist, error) {
	rows, err := s.sql.QueryContext(ctx, `select `+playlistSummaryColumns+` from playlists p
		order by p.updated_at desc, p.id desc`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlists: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []domain.Playlist{}
	for rows.Next() {
		playlist, err := scanPlaylist(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read the playlists: %w", err)
		}
		out = append(out, playlist)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the playlists: %w", err)
	}
	return out, nil
}

// CreatePlaylist は名前 name（domain.ValidatePlaylistName で整える）の空のプレイリストを作る。
// 同じ名前のプレイリストがあってもよい（R-11）。
func (s *PlaylistStore) CreatePlaylist(ctx context.Context, name string) (domain.Playlist, error) {
	name, err := domain.ValidatePlaylistName(name)
	if err != nil {
		return domain.Playlist{}, err
	}
	now := playlistNow().UnixMilli()
	result, err := s.sql.ExecContext(ctx, `insert into playlists (name, created_at, updated_at) values (?, ?, ?)`,
		name, now, now)
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot create the playlist: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot create the playlist: %w", err)
	}
	return domain.Playlist{ID: id, Name: name, UpdatedAt: time.UnixMilli(now)}, nil
}

// RenamePlaylist はプレイリスト id の名前を name（domain.ValidatePlaylistName で整える）にし、
// 変えたあとの要約を返す。無ければ domain.ErrPlaylistNotFound。
func (s *PlaylistStore) RenamePlaylist(ctx context.Context, id int64, name string) (domain.Playlist, error) {
	name, err := domain.ValidatePlaylistName(name)
	if err != nil {
		return domain.Playlist{}, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot rename the playlist: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `update playlists set name = ?, updated_at = ? where id = ?`,
		name, playlistNow().UnixMilli(), id)
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot rename the playlist: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot rename the playlist: %w", err)
	} else if affected == 0 {
		return domain.Playlist{}, domain.ErrPlaylistNotFound
	}
	playlist, err := playlistSummary(ctx, tx, id)
	if err != nil {
		return domain.Playlist{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot rename the playlist: %w", err)
	}
	return playlist, nil
}

// DeletePlaylist はプレイリスト id とその項目を消す。動画・グループとその利用者データには
// 触れない。無ければ domain.ErrPlaylistNotFound。
func (s *PlaylistStore) DeletePlaylist(ctx context.Context, id int64) error {
	result, err := s.sql.ExecContext(ctx, `delete from playlists where id = ?`, id)
	if err != nil {
		return fmt.Errorf("cannot delete the playlist: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot delete the playlist: %w", err)
	}
	if affected == 0 {
		return domain.ErrPlaylistNotFound
	}
	return nil
}

// Playlist はプレイリスト id の要約と、見せる項目を位置の順に返す。動画とグループは所有者として
// 読む。要約と項目は読み取り用の接続の同じスナップショットから返す。無ければ
// domain.ErrPlaylistNotFound。
func (s *PlaylistStore) Playlist(ctx context.Context, id int64) (domain.Playlist, []domain.PlaylistItem, error) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Playlist{}, nil, fmt.Errorf("cannot read the playlist: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	playlist, err := playlistSummary(ctx, tx, id)
	if err != nil {
		return domain.Playlist{}, nil, err
	}
	items, err := shownPlaylistItems(ctx, tx, id)
	if err != nil {
		return domain.Playlist{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Playlist{}, nil, fmt.Errorf("cannot read the playlist: %w", err)
	}
	return playlist, items, nil
}

// PlaylistPlayback はプレイリスト id を項目 itemID の動画 videoID から再生しているときの、今の
// 位置と次に開く動画を、最新の並びから返す（data-model.md「Playback order」、research.md R-6）。
// 全項目（見せない項目も含む）と見せる項目の動画・グループを読み取り用の接続の同じ
// スナップショットから読み、domain.PlaylistPlaybackOf に渡す。プレイリストが無ければ
// domain.ErrPlaylistNotFound、項目がそのプレイリストに無ければ domain.ErrPlaylistItemNotFound。
func (s *PlaylistStore) PlaylistPlayback(ctx context.Context, id, itemID, videoID int64) (domain.PlaylistPlayback, error) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.PlaylistPlayback{}, fmt.Errorf("cannot read the playlist playback: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	playlist, err := playlistSummary(ctx, tx, id)
	if err != nil {
		return domain.PlaylistPlayback{}, err
	}
	order, err := playlistItemOrder(ctx, tx, id)
	if err != nil {
		return domain.PlaylistPlayback{}, err
	}
	shown, err := shownPlaylistItems(ctx, tx, id)
	if err != nil {
		return domain.PlaylistPlayback{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.PlaylistPlayback{}, fmt.Errorf("cannot read the playlist playback: %w", err)
	}

	byID := make(map[int64]domain.PlaylistItem, len(shown))
	for _, item := range shown {
		byID[item.ID] = item
	}
	items := make([]domain.PlaylistItem, len(order))
	for i, itemID := range order {
		item, ok := byID[itemID]
		if !ok {
			item = domain.PlaylistItem{ID: itemID}
		}
		items[i] = item
	}
	playback, err := domain.PlaylistPlaybackOf(items, domain.PlaylistCurrent{ItemID: itemID, VideoID: videoID})
	if err != nil {
		return domain.PlaylistPlayback{}, err
	}
	playback.Playlist = playlist
	return playback, nil
}

// AddPlaylistItems は add の動画とグループを、プレイリスト id の末尾に要求の順（動画、次に
// フォルダ）で足す（R-5）。動画の id は利用者データの鍵へ引き直し（同じ集まりの id は 1 つの
// 項目になる）、フォルダは domain.FolderKey で整えて今グループのものだけを使う（SetFavorites と
// 同じ）。引けた鍵が既にプレイリストにあれば Existing に数え、何も変えない。プレイリストが
// 無ければ domain.ErrPlaylistNotFound。足したときだけ更新日時を進める。
func (s *PlaylistStore) AddPlaylistItems(ctx context.Context, id int64, add domain.PlaylistAdd) (domain.PlaylistAdded, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.PlaylistAdded{}, fmt.Errorf("cannot add to the playlist: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensurePlaylist(ctx, tx, id); err != nil {
		return domain.PlaylistAdded{}, err
	}

	videoKeys, err := orderedUserKeys(ctx, tx, add.VideoIDs)
	if err != nil {
		return domain.PlaylistAdded{}, err
	}
	groups, err := groupFolderKeys(ctx, tx, add.FolderPaths)
	if err != nil {
		return domain.PlaylistAdded{}, err
	}
	var folderKeys []string
	for _, path := range add.FolderPaths {
		key := domain.FolderKey(path)
		if slices.Contains(groups, key) && !slices.Contains(folderKeys, key) {
			folderKeys = append(folderKeys, key)
		}
	}

	var next int
	if err := tx.QueryRowContext(ctx, `select count(*) from playlist_items where playlist_id = ?`, id).Scan(&next); err != nil {
		return domain.PlaylistAdded{}, fmt.Errorf("cannot add to the playlist: %w", err)
	}
	now := playlistNow().UnixMilli()
	var added domain.PlaylistAdded
	insert := func(column, key string) error {
		result, err := tx.ExecContext(ctx, `insert or ignore into playlist_items
			(playlist_id, `+column+`, position, added_at) values (?, ?, ?, ?)`, id, key, next, now)
		if err != nil {
			return fmt.Errorf("cannot add to the playlist: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot add to the playlist: %w", err)
		}
		if affected == 0 {
			added.Existing++
			return nil
		}
		added.Added++
		next++
		return nil
	}
	for _, key := range videoKeys {
		if err := insert("content_key", key); err != nil {
			return domain.PlaylistAdded{}, err
		}
	}
	for _, key := range folderKeys {
		if err := insert("folder_key", key); err != nil {
			return domain.PlaylistAdded{}, err
		}
	}
	if added.Added > 0 {
		if err := touchPlaylist(ctx, tx, id); err != nil {
			return domain.PlaylistAdded{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.PlaylistAdded{}, fmt.Errorf("cannot add to the playlist: %w", err)
	}
	return added, nil
}

// MovePlaylistItem はプレイリスト id の項目 itemID を、項目 afterItemID のすぐ後ろへ移し
// （nil なら先頭）、プレイリストを詰め直す（R-3）。見せない項目も位置を持ったまま並びに残る。
// プレイリストが無ければ domain.ErrPlaylistNotFound、どちらかの項目がそのプレイリストに
// 無ければ domain.ErrPlaylistItemNotFound。itemID の後ろへ自分を移すのは何も変えない。
func (s *PlaylistStore) MovePlaylistItem(ctx context.Context, id, itemID int64, afterItemID *int64) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot move the playlist item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensurePlaylist(ctx, tx, id); err != nil {
		return err
	}
	order, err := playlistItemOrder(ctx, tx, id)
	if err != nil {
		return err
	}
	from := slices.Index(order, itemID)
	if from < 0 {
		return domain.ErrPlaylistItemNotFound
	}
	if afterItemID != nil && !slices.Contains(order, *afterItemID) {
		return domain.ErrPlaylistItemNotFound
	}
	if afterItemID != nil && *afterItemID == itemID {
		return tx.Commit()
	}
	order = slices.Delete(order, from, from+1)
	to := 0
	if afterItemID != nil {
		to = slices.Index(order, *afterItemID) + 1
	}
	order = slices.Insert(order, to, itemID)
	if err := writePlaylistOrder(ctx, tx, order); err != nil {
		return err
	}
	if err := touchPlaylist(ctx, tx, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot move the playlist item: %w", err)
	}
	return nil
}

// RemovePlaylistItem はプレイリスト id の項目 itemID を消して詰め直す。動画・グループとその
// お気に入りや再生位置には触れない。プレイリストが無ければ domain.ErrPlaylistNotFound、項目が
// 無ければ domain.ErrPlaylistItemNotFound。
func (s *PlaylistStore) RemovePlaylistItem(ctx context.Context, id, itemID int64) error {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot remove the playlist item: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := ensurePlaylist(ctx, tx, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `delete from playlist_items where id = ? and playlist_id = ?`, itemID, id)
	if err != nil {
		return fmt.Errorf("cannot remove the playlist item: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("cannot remove the playlist item: %w", err)
	} else if affected == 0 {
		return domain.ErrPlaylistItemNotFound
	}
	if err := renumberPlaylists(ctx, tx, []int64{id}); err != nil {
		return err
	}
	if err := touchPlaylist(ctx, tx, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot remove the playlist item: %w", err)
	}
	return nil
}

// rekeyPlaylistItems は利用者データの鍵 from の動画の項目を鍵 to の項目へ付け替える
// （data-model.md「Succession and bundling」、R-2）。行は写さずに移す。1 つのプレイリストに
// 鍵 to の項目が 2 つになるときは位置の大きい方を消し、そのプレイリストを詰め直す。更新日時は
// 利用者の変更ではないので進めない。
func rekeyPlaylistItems(ctx context.Context, tx *sql.Tx, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	affected, err := queryInt64s(ctx, tx, `select distinct playlist_id from playlist_items where content_key = ?`, from)
	if err != nil {
		return fmt.Errorf("cannot move the playlist items: %w", err)
	}
	if len(affected) == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `delete from playlist_items where content_key in (?1, ?2)
		and exists (select 1 from playlist_items o where o.playlist_id = playlist_items.playlist_id
			and o.content_key in (?1, ?2) and o.id <> playlist_items.id
			and (o.position < playlist_items.position or (o.position = playlist_items.position and o.id < playlist_items.id)))`,
		from, to); err != nil {
		return fmt.Errorf("cannot move the playlist items: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `update playlist_items set content_key = ? where content_key = ?`, to, from); err != nil {
		return fmt.Errorf("cannot move the playlist items: %w", err)
	}
	return renumberPlaylists(ctx, tx, affected)
}

// renumberPlaylists は playlistIDs のプレイリストの項目の位置を、今の並び（位置、同じなら id）の
// まま 0 から詰め直す。並びを先に読み切ってから書く。位置を相関副問い合わせで数える 1 文の
// update は、同じ文で既に書き換えた行の位置を数えてしまい、位置が重なる。
func renumberPlaylists(ctx context.Context, tx *sql.Tx, playlistIDs []int64) error {
	for _, id := range playlistIDs {
		order, err := playlistItemOrder(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := writePlaylistOrder(ctx, tx, order); err != nil {
			return err
		}
	}
	return nil
}

// ensurePlaylist はプレイリスト id があるかを確かめ、無ければ domain.ErrPlaylistNotFound を返す。
func ensurePlaylist(ctx context.Context, tx *sql.Tx, id int64) error {
	var exists int
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from playlists where id = ?)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("cannot read the playlist: %w", err)
	}
	if exists == 0 {
		return domain.ErrPlaylistNotFound
	}
	return nil
}

// touchPlaylist はプレイリスト id の更新日時を今にする。
func touchPlaylist(ctx context.Context, tx *sql.Tx, id int64) error {
	if _, err := tx.ExecContext(ctx, `update playlists set updated_at = ? where id = ?`,
		playlistNow().UnixMilli(), id); err != nil {
		return fmt.Errorf("cannot update the playlist: %w", err)
	}
	return nil
}

// playlistSummary はプレイリスト id の要約を読む。無ければ domain.ErrPlaylistNotFound。
func playlistSummary(ctx context.Context, tx *sql.Tx, id int64) (domain.Playlist, error) {
	playlist, err := scanPlaylist(tx.QueryRowContext(ctx, `select `+playlistSummaryColumns+` from playlists p where p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Playlist{}, domain.ErrPlaylistNotFound
	}
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("cannot read the playlist: %w", err)
	}
	return playlist, nil
}

// playlistItemOrder はプレイリスト id の全項目（見せない項目も含む）の id を位置の順に返す。
func playlistItemOrder(ctx context.Context, tx *sql.Tx, id int64) ([]int64, error) {
	ids, err := queryInt64s(ctx, tx, `select id from playlist_items where playlist_id = ? order by position, id`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlist items: %w", err)
	}
	return ids, nil
}

// writePlaylistOrder は order の項目の位置を、その並びの 0 からの番号にする。
func writePlaylistOrder(ctx context.Context, tx *sql.Tx, order []int64) error {
	for position, itemID := range order {
		if _, err := tx.ExecContext(ctx, `update playlist_items set position = ? where id = ?`, position, itemID); err != nil {
			return fmt.Errorf("cannot reorder the playlist: %w", err)
		}
	}
	return nil
}

// orderedUserKeys は videoIDs を要求の順に利用者データの鍵へ引き直し、重複を除いて返す。
// 引けない id は除く（userKeysForVideoIDs と同じ条件）。
func orderedUserKeys(ctx context.Context, tx *sql.Tx, videoIDs []int64) ([]string, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select v.id, `+userKeyExpr("v")+` from videos v
		where v.id in (select value from json_each(?)) and v.content_key <> '' and `+
		registeredVideoCondition("v"), string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byID := map[int64]string{}
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
		}
		byID[id] = key
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
	}
	var keys []string
	for _, id := range videoIDs {
		if key, ok := byID[id]; ok && key != "" && !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// shownPlaylistItems はプレイリスト id の見せる項目を位置の順に、所有者として読んだ動画か
// グループと合わせて返す。
func shownPlaylistItems(ctx context.Context, tx *sql.Tx, id int64) ([]domain.PlaylistItem, error) {
	rows, err := tx.QueryContext(ctx, `select i.id, `+playlistItemVideoExpr("i")+`,
		(select pg.id from folder_groups pg where pg.path_key = i.folder_key)
		from playlist_items i where i.playlist_id = ? order by i.position, i.id`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlist items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	type entry struct {
		id      int64
		videoID sql.NullInt64
		groupID sql.NullInt64
	}
	var entries []entry
	var videoIDs, groupIDs []int64
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.videoID, &e.groupID); err != nil {
			return nil, fmt.Errorf("cannot read the playlist items: %w", err)
		}
		switch {
		case e.videoID.Valid:
			videoIDs = append(videoIDs, e.videoID.Int64)
		case e.groupID.Valid:
			groupIDs = append(groupIDs, e.groupID.Int64)
		default:
			continue
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the playlist items: %w", err)
	}
	_ = rows.Close()

	videos, err := ownerVideosByID(ctx, tx, videoIDs)
	if err != nil {
		return nil, err
	}
	groups, err := loadGroups(ctx, tx, domain.AudienceOwner, groupIDs)
	if err != nil {
		return nil, err
	}
	items := make([]domain.PlaylistItem, 0, len(entries))
	for _, e := range entries {
		item := domain.PlaylistItem{ID: e.id}
		if e.videoID.Valid {
			video, ok := videos[e.videoID.Int64]
			if !ok {
				continue
			}
			item.Video = &video
		} else {
			group, ok := groups[e.groupID.Int64]
			if !ok {
				continue
			}
			item.Group = &group
		}
		items = append(items, item)
	}
	return items, nil
}

// ownerVideosByID は ids の動画を所有者として読み、id ごとに返す。
func ownerVideosByID(ctx context.Context, tx *sql.Tx, ids []int64) (map[int64]domain.Video, error) {
	out := map[int64]domain.Video{}
	if len(ids) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(domain.AudienceOwner)+` from videos`+
		representativeJoin(domain.AudienceOwner)+` where videos.id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlist videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read the playlist videos: %w", err)
		}
		out[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the playlist videos: %w", err)
	}
	return out, nil
}

// queryInt64s は 1 列の整数を返す問い合わせの結果を読み切る。
func queryInt64s(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var value int64
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}
