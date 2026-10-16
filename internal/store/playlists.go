package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// プレイリスト（specs/044-playlists/data-model.md「Store operations」、research.md R-1〜R-5・R-11）。
// 項目は動画の利用者データの鍵（userKeyExpr）、グループの id（specs/048-groups/research.md R-10）、
// スマートフォルダの id（specs/052-smart-folders/contracts/screen-api.md）のどれかで持つ。
// スマートフォルダの項目は中身を持たず、読むたびにフォルダの再生順を求める。フォルダの項目は
// フォルダの削除で外部キーにより消え、削除が詰め直す（DeleteSmartFolder）。動画への外部キーは持たず、鍵が今ライブラリの動画に引けない項目や、見せる動画を持たない
// グループの項目は、読み出しから外すだけで消さない（R-4）。グループの項目はグループの解散で外部キーの
// on delete cascade により消え、解散が詰め直す（dissolveGroup）。位置はプレイリストの中で 0 から詰めた整数で、書き込みのたびに
// 詰め直す（R-3）。どの操作もドメインイベントを発行しない（R-10）。

// playlistNow は名前と項目の変更の時刻の元である。試験が時計を止めるために差し替える。
var playlistNow = time.Now

// playlistItemVideoExpr は、プレイリストの項目（別名 alias の playlist_items）の動画の項目が
// 所有者に見せる動画の id を返す式である（shownVideoForKeyExpr）。グループの項目では NULL になる。
func playlistItemVideoExpr(alias string) string {
	return `(case when ` + alias + `.content_key is null then null
		else ` + shownVideoForKeyExpr(alias+`.content_key`, domain.AudienceOwner) + ` end)`
}

// playlistGroupVideos は、プレイリストの項目（別名 alias）のグループの、所有者に見せる子孫の動画を
// 再生順の鍵とともに返す副問い合わせの頭（`with ... gdesc`）である。呼び出し側は gdesc
// （root_id, video_id, sort_key）を読む select 文を続ける。グループの読み出し（loadGroups）と同じ
// groupDescendantsCTE を使うので、見せるかと再生順がライブラリのグループと食い違わない。
func playlistGroupVideos(alias string) string {
	return `with ` + groupDescendantsCTE(`select `+alias+`.group_id as id`, domain.AudienceOwner)
}

// playlistItemShownCondition は、プレイリストの項目（別名 alias）を見せるかの条件句である。
// 動画の項目は playlistItemVideoExpr が引けるとき、グループの項目はそのグループが所有者に見せる
// 子孫の動画を持つとき見せる（R-4、specs/048-groups/data-model.md「Play order」）。スマートフォルダの
// 項目は、空のフォルダを所有者に見せるのと同じく（specs/052-smart-folders/research.md R-8）、
// フォルダがある限り見せる。shownPlaylistItems も同じ条件で項目を選ぶので、件数と Playlist の項目が
// 食い違わない。
func playlistItemShownCondition(alias string) string {
	return `(` + alias + `.smart_folder_id is not null
		or (` + alias + `.group_id is not null and exists (` + playlistGroupVideos(alias) + ` select 1 from gdesc))
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

// ListPlaylists はプレイリストを、更新日時の新しい順、同じなら id の大きい順に、cursor の次から
// limit 件返す。cursor が空なら先頭から。読めない cursor は domain.ErrInvalidCursor。項目の数は
// 見せる項目の数で、Covers は返したページのプレイリストにだけ作る一覧のカードの絵柄の
// サムネイル（playlistCovers）である。Total は全部のプレイリストの数、Version は一覧の版
// （playlistListVersion）で、要約・絵柄・Total・Version は読み取り用の接続の同じスナップショットから
// 読む。並びとカーソルは playlists_order_idx（00039）で引く。
func (s *PlaylistStore) ListPlaylists(ctx context.Context, cursor string, limit int) (domain.PlaylistPage, error) {
	limit = max(1, limit)
	var conditions []string
	var args []any
	if cursor != "" {
		after, err := domain.DecodePlaylistCursor(cursor)
		if err != nil {
			return domain.PlaylistPage{}, err
		}
		// 行の値の比較（listing.go の cursorCondition と同じ形）。or で書くと SQLite は
		// playlists_order_idx を先頭から読むが、この形なら索引をカーソルの位置から引く。
		conditions = append(conditions, `(p.updated_at, p.id) < (?, ?)`)
		args = append(args, after.UpdatedAtMs, after.ID)
	}
	// 1 件多く読んで、続きがあるかを知る。
	args = append(args, limit+1)

	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	page := domain.PlaylistPage{Items: []domain.Playlist{}}
	var updatedAtSum int64
	if err := tx.QueryRowContext(ctx, `select count(*), coalesce(sum(updated_at), 0) from playlists`).Scan(
		&page.Total, &updatedAtSum); err != nil {
		return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	page.Version = playlistListVersion(page.Total, updatedAtSum)
	rows, err := tx.QueryContext(ctx, `select `+playlistSummaryColumns+` from playlists p`+whereClause(conditions)+`
		order by p.updated_at desc, p.id desc limit ?`, args...)
	if err != nil {
		return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		playlist, err := scanPlaylist(rows)
		if err != nil {
			return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
		}
		page.Items = append(page.Items, playlist)
	}
	if err := rows.Err(); err != nil {
		return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	_ = rows.Close()
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.NextCursor = domain.EncodePlaylistCursor(domain.PlaylistCursor{
			UpdatedAtMs: last.UpdatedAt.UnixMilli(), ID: last.ID,
		})
	}
	for i := range page.Items {
		covers, err := playlistCovers(ctx, tx, page.Items[i].ID)
		if err != nil {
			return domain.PlaylistPage{}, err
		}
		page.Items[i].Covers = covers
	}
	if err := tx.Commit(); err != nil {
		return domain.PlaylistPage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	return page, nil
}

// playlistListVersion は一覧の版である（domain.PlaylistPage.Version）。プレイリストの数と全部の
// updated_at の和から作る。名前か項目の変更は nextUpdatedAt でその行の updated_at を必ず進めるので
// 和が変わり、作る・消すは数が変わる。同じミリ秒の変更も、最大値では見分けられないが和では
// 見分けられる。中身はクライアントが比べるだけで、解釈しない。
func playlistListVersion(total int, updatedAtSum int64) string {
	return strconv.Itoa(total) + "." + strconv.FormatInt(updatedAtSum, 10)
}

// playlistCoverVideoExpr は、プレイリストの項目（別名 alias）の絵柄に差し込む動画の id を返す式である。
// 動画の項目は playlistItemVideoExpr の動画、グループの項目はそのグループの所有者に見せる子孫の
// 動画のうち再生順で最初のサムネイル生成済みのもの。グループの項目は playlistItemShownCondition と
// 同じ子孫（playlistGroupVideos）から引くので、見せない項目は NULL になる。スマートフォルダの項目は
// 再生順を SQL の式では求められないので NULL で、絵柄に差し込まない。
func playlistCoverVideoExpr(alias string) string {
	return `(case when ` + alias + `.content_key is not null then ` + playlistItemVideoExpr(alias) + `
		else (` + playlistGroupVideos(alias) + ` select d.video_id from gdesc d join videos pgv on pgv.id = d.video_id
			where pgv.thumbnail_state = ? order by d.sort_key limit 1) end)`
}

// playlistCovers はプレイリスト id の一覧のカードの絵柄に差し込むサムネイルを返す（domain.Playlist.Covers）。
// 見せる項目を位置の順にたどり、項目ごとの動画（playlistCoverVideoExpr）のうちサムネイル生成済みの
// ものだけを domain.MaxFolderPreviews 件まで取る。サムネイルの URL の版に使う位置と改版は
// FolderLocations と同じく video_overrides から読む。
func playlistCovers(ctx context.Context, tx *sql.Tx, id int64) ([]domain.FolderPreview, error) {
	rows, err := tx.QueryContext(ctx, `select v.id, v.content_key, v.preview_state,
			(select ov.thumbnail_position_ms from video_overrides ov where ov.content_key = v.content_key and v.content_key <> ''),
			(select ov.thumbnail_revision from video_overrides ov where ov.content_key = v.content_key and v.content_key <> '')
		from playlist_items i join videos v on v.id = `+playlistCoverVideoExpr("i")+`
		where i.playlist_id = ? and v.thumbnail_state = ?
		order by i.position, i.id limit ?`,
		string(domain.ThumbnailStateDone), id, string(domain.ThumbnailStateDone), domain.MaxFolderPreviews)
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlist covers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	covers := []domain.FolderPreview{}
	for rows.Next() {
		var cover domain.FolderPreview
		var preview string
		var position, revision sql.NullInt64
		if err := rows.Scan(&cover.VideoID, &cover.ContentKey, &preview, &position, &revision); err != nil {
			return nil, fmt.Errorf("cannot read the playlist covers: %w", err)
		}
		cover.PreviewState = domain.PreviewState(preview)
		if position.Valid {
			ms := position.Int64
			cover.ThumbnailPositionMs = &ms
			cover.ThumbnailRevision = revision.Int64
		}
		covers = append(covers, cover)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the playlist covers: %w", err)
	}
	return covers, nil
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
	result, err := tx.ExecContext(ctx, `update playlists set name = ?, updated_at = `+nextUpdatedAt+` where id = ?`,
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
// 読む。要約・項目・登録フォルダの一覧は読み取り用の接続の同じスナップショットから返すので、
// グループのフォルダはどれも返す登録フォルダの下にあり、件数と項目が食い違わない
// （domain.LibraryPage.Roots と同じ）。無ければ domain.ErrPlaylistNotFound。
func (s *PlaylistStore) Playlist(ctx context.Context, id int64) (domain.Playlist, []domain.PlaylistItem, []domain.MediaFolder, error) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Playlist{}, nil, nil, fmt.Errorf("cannot read the playlist: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	playlist, err := playlistSummary(ctx, tx, id)
	if err != nil {
		return domain.Playlist{}, nil, nil, err
	}
	items, err := shownPlaylistItems(ctx, tx, id)
	if err != nil {
		return domain.Playlist{}, nil, nil, err
	}
	roots, err := listMediaFolders(ctx, tx)
	if err != nil {
		return domain.Playlist{}, nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Playlist{}, nil, nil, fmt.Errorf("cannot read the playlist: %w", err)
	}
	return playlist, items, roots, nil
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

// AddPlaylistItems は add の動画・グループ・スマートフォルダを、プレイリスト id の末尾に要求の順
// （動画、グループ、スマートフォルダ）で足す（R-5）。動画の id は利用者データの鍵へ引き直し（同じ
// 集まりの id は 1 つの項目になる）、グループとスマートフォルダは今あるもの（existingIDs）だけを
// 使う（SetFavorites と同じ）。引けた鍵・グループ・フォルダが既にプレイリストにあれば Existing に
// 数え、何も変えない。プレイリストが無ければ domain.ErrPlaylistNotFound。足したときだけ更新日時を
// 進める。
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
	groupIDs, err := existingGroupIDs(ctx, tx, add.GroupIDs)
	if err != nil {
		return domain.PlaylistAdded{}, err
	}
	folderIDs, err := existingIDs(ctx, tx, "smart_folders", add.SmartFolderIDs)
	if err != nil {
		return domain.PlaylistAdded{}, err
	}

	var next int
	if err := tx.QueryRowContext(ctx, `select count(*) from playlist_items where playlist_id = ?`, id).Scan(&next); err != nil {
		return domain.PlaylistAdded{}, fmt.Errorf("cannot add to the playlist: %w", err)
	}
	now := playlistNow().UnixMilli()
	var added domain.PlaylistAdded
	insert := func(column string, key any) error {
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
	for _, groupID := range groupIDs {
		if err := insert("group_id", groupID); err != nil {
			return domain.PlaylistAdded{}, err
		}
	}
	for _, folderID := range folderIDs {
		if err := insert("smart_folder_id", folderID); err != nil {
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
	moved, err := moveItemAfter(ctx, tx, playlistPositions, id, itemID, afterItemID)
	if errors.Is(err, errPositionItemNotFound) {
		return domain.ErrPlaylistItemNotFound
	}
	if err != nil {
		return err
	}
	if !moved {
		return tx.Commit()
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
// 鍵 to の項目が 2 つになるときは位置の大きい方を消し、そのプレイリストを詰め直す。
//
// 更新日時はここでは進めない。付け替えと同じ取引で集まりや動画も変わり、見せる項目はその後で
// 決まるので、呼び出し側が取引の初めに snapshotPlaylistShapes で形を読み、変更を終えてから
// touchReshapedPlaylists で比べる。
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
	if err := deleteRekeyDuplicates(ctx, tx, playlistPositions, from, to); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update playlist_items set content_key = ? where content_key = ?`, to, from); err != nil {
		return fmt.Errorf("cannot move the playlist items: %w", err)
	}
	return renumberPlaylists(ctx, tx, affected)
}

// playlistShapes は、プレイリストの id ごとに、その全項目を位置の順に「項目の id と見せるか」で
// 並べた形である。形が同じなら、見せる項目もその並びも同じである。
type playlistShapes map[int64]string

// snapshotPlaylistShapes は、鍵 keys のどれかの項目を持つプレイリストの今の形を読む。
// 付け替えを含む取引の初め、集まりや動画を変える前に呼ぶ。
func snapshotPlaylistShapes(ctx context.Context, tx *sql.Tx, keys []string) (playlistShapes, error) {
	keys = slices.DeleteFunc(slices.Clone(keys), func(key string) bool { return key == "" })
	shapes := playlistShapes{}
	if len(keys) == 0 {
		return shapes, nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlists: %w", err)
	}
	ids, err := queryInt64s(ctx, tx, `select distinct playlist_id from playlist_items
		where content_key in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlists: %w", err)
	}
	for _, id := range ids {
		shape, err := playlistShape(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		shapes[id] = shape
	}
	return shapes, nil
}

// touchReshapedPlaylists は、before に読んだプレイリストのうち形が変わったものの更新日時を
// 進める。付け替えで隠れていた項目が見えた、見えていた項目が隠れた、重なった項目を消したときが
// それにあたる。再生の応答とプレイリストの応答は更新日時が同じときだけ同じ並びとして使われる
// （ui-design.md「One order」）。鍵を付け替えただけで形が変わらないプレイリストは進めず、一覧の
// 順を動かさない。取引の変更を終えてから呼ぶ。
func touchReshapedPlaylists(ctx context.Context, tx *sql.Tx, before playlistShapes) error {
	for _, id := range slices.Sorted(maps.Keys(before)) {
		after, err := playlistShape(ctx, tx, id)
		if err != nil {
			return err
		}
		if after == before[id] {
			continue
		}
		if err := touchPlaylist(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// playlistShape はプレイリスト id の形（playlistShapes）を読む。
func playlistShape(ctx context.Context, tx *sql.Tx, id int64) (string, error) {
	rows, err := tx.QueryContext(ctx, `select i.id, `+playlistItemShownCondition("i")+`
		from playlist_items i where i.playlist_id = ? order by i.position, i.id`, id)
	if err != nil {
		return "", fmt.Errorf("cannot read the playlist items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var shape strings.Builder
	for rows.Next() {
		var itemID int64
		var shown bool
		if err := rows.Scan(&itemID, &shown); err != nil {
			return "", fmt.Errorf("cannot read the playlist items: %w", err)
		}
		fmt.Fprintf(&shape, "%d:%t,", itemID, shown)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("cannot read the playlist items: %w", err)
	}
	return shape.String(), nil
}

// renumberPlaylists は playlistIDs のプレイリストの項目の位置を、今の並び（位置、同じなら id）の
// まま 0 から詰め直す。
func renumberPlaylists(ctx context.Context, tx *sql.Tx, playlistIDs []int64) error {
	return renumberPositions(ctx, tx, playlistPositions, playlistIDs)
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

// nextUpdatedAt は名前か項目を変えた後の playlists.updated_at である。今の時刻で、前の値より
// 必ず進める。同じミリ秒に 2 度変わっても、時計が戻っても、変わる前と後の更新日時は違うので、
// 応答の更新日時が同じなら同じ並びである（ui-design.md「One order」）。引数は今の Unix ms。
const nextUpdatedAt = `max(?, updated_at + 1)`

// touchPlaylist はプレイリスト id の更新日時を今に、前の値より必ず後へ進める。
func touchPlaylist(ctx context.Context, tx *sql.Tx, id int64) error {
	if _, err := tx.ExecContext(ctx, `update playlists set updated_at = `+nextUpdatedAt+` where id = ?`,
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
	return positionOrder(ctx, tx, playlistPositions, id)
}

// orderedUserKeys は videoIDs を要求の順に利用者データの鍵へ引き直し、重複を除いて返す。
// 引けない id は除く（userKeysForVideoIDs と同じ条件）。
func orderedUserKeys(ctx context.Context, tx *sql.Tx, videoIDs []int64) ([]string, error) {
	byID, err := userKeysByVideoID(ctx, tx, videoIDs, true)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, id := range videoIDs {
		if key, ok := byID[id]; ok && key != "" && !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// playlistEntry はプレイリストの見せる項目 1 つの、動画・グループ・スマートフォルダの id である。
// どれか 1 つだけが有効である。
type playlistEntry struct {
	id       int64
	videoID  sql.NullInt64
	groupID  sql.NullInt64
	folderID sql.NullInt64
}

// shownPlaylistItems はプレイリスト id の見せる項目（playlistItemShownCondition）を位置の順に、
// 所有者として読んだ動画・グループ・スマートフォルダと合わせて返す。グループの値とメンバーの並びは
// loadGroups の再生順（specs/048-groups/data-model.md「Play order」）、スマートフォルダの値と
// メンバーの並びは読んだ時点のフォルダの再生順（specs/052-smart-folders/data-model.md「Contents」）
// である。
func shownPlaylistItems(ctx context.Context, tx *sql.Tx, id int64) ([]domain.PlaylistItem, error) {
	entries, err := shownPlaylistEntries(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	var videoIDs, groupIDs []int64
	for _, e := range entries {
		if e.videoID.Valid {
			videoIDs = append(videoIDs, e.videoID.Int64)
		} else if e.groupID.Valid {
			groupIDs = append(groupIDs, e.groupID.Int64)
		}
	}
	videos, err := ownerVideosByID(ctx, tx, videoIDs)
	if err != nil {
		return nil, err
	}
	groups, err := loadGroups(ctx, tx, domain.AudienceOwner, groupIDs)
	if err != nil {
		return nil, err
	}
	now := listNow().Unix()
	items := make([]domain.PlaylistItem, 0, len(entries))
	for _, e := range entries {
		item := domain.PlaylistItem{ID: e.id}
		switch {
		case e.videoID.Valid:
			video, ok := videos[e.videoID.Int64]
			if !ok {
				continue
			}
			item.Video = &video
		case e.groupID.Valid:
			group, ok := groups[e.groupID.Int64]
			if !ok {
				continue
			}
			item.Group = &group
		default:
			card, err := ownerSmartFolderCard(ctx, tx, e.folderID.Int64, now)
			if err != nil {
				return nil, err
			}
			item.SmartFolder = &card
		}
		items = append(items, item)
	}
	return items, nil
}

// shownPlaylistEntries はプレイリスト id の見せる項目の id を位置の順に読む。
func shownPlaylistEntries(ctx context.Context, tx *sql.Tx, id int64) ([]playlistEntry, error) {
	rows, err := tx.QueryContext(ctx, `select i.id, `+playlistItemVideoExpr("i")+`, i.group_id, i.smart_folder_id
		from playlist_items i where i.playlist_id = ? and `+playlistItemShownCondition("i")+`
		order by i.position, i.id`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlist items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var entries []playlistEntry
	for rows.Next() {
		var e playlistEntry
		if err := rows.Scan(&e.id, &e.videoID, &e.groupID, &e.folderID); err != nil {
			return nil, fmt.Errorf("cannot read the playlist items: %w", err)
		}
		if e.videoID.Valid || e.groupID.Valid || e.folderID.Valid {
			entries = append(entries, e)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the playlist items: %w", err)
	}
	return entries, nil
}

// ownerVideosByID は ids の動画を所有者として読み、id ごとに返す。
func ownerVideosByID(ctx context.Context, tx *sql.Tx, ids []int64) (map[int64]domain.Video, error) {
	return videosByID(ctx, tx, domain.AudienceOwner, ids)
}

// videosByID は ids の動画を見る人（audience）の代表の所在で読み、id ごとに返す。
func videosByID(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64) (map[int64]domain.Video, error) {
	out := map[int64]domain.Video{}
	if len(ids) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(audience)+` from videos`+
		representativeJoin(audience)+` where videos.id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read the videos: %w", err)
		}
		out[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the videos: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("cannot read the videos: %w", err)
	}
	if err := attachVersionsIn(ctx, tx, audience, out); err != nil {
		return nil, err
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
