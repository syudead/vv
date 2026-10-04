package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// DirectVideoPaths はディレクトリ直下の動画を、id とパスだけで全件返す。
// 同じ動画の所在が直下に2つ以上あるときは、ListFolderVideos と同じく
// パスの小さい方を1件だけ返す（chosenLocationsCTE）。並べ方は internal/domain の
// OrderRelated が決める。ゲストには公開の動画だけを返す。
func (s *LibraryStore) DirectVideoPaths(ctx context.Context, audience domain.Audience, dir string) ([]domain.RelatedSibling, error) {
	cte, args := chosenLocationsCTE(folderScope(dir, domain.FolderScopeDirect, audience), domain.SearchExpr{})
	rows, err := s.db.sql.QueryContext(ctx, cte+` select video_id, path from chosen`, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read videos in the same folder: %w", err)
	}
	defer func() { _ = rows.Close() }()

	siblings := []domain.RelatedSibling{}
	for rows.Next() {
		var item domain.RelatedSibling
		if err := rows.Scan(&item.VideoID, &item.Path); err != nil {
			return nil, fmt.Errorf("cannot read videos in the same folder: %w", err)
		}
		siblings = append(siblings, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read videos in the same folder: %w", err)
	}
	return siblings, nil
}

// VideosAddedNear は追加日時がこの動画に近い動画を、前後それぞれ最大 limit 件
// 返す。見る人に見せてよい所在の無い動画（登録フォルダの下に所在の無い動画と、
// ゲストには非公開の動画）、見せる動画でないもの（集まりの代表以外のバージョン、
// shownVideoCondition）と、この動画自身は含めない。
//
// 片側の並びは、追加日時の差が小さい順、差が同じなら id の大きい順で、
// OrderRelated の補い方と同じである。そのため、同じフォルダの動画を除いたあとに
// 要る件数（limit から同じフォルダの件数を引いた数）は、両側の先頭 limit 件の
// 中に必ず入っている。
func (s *LibraryStore) VideosAddedNear(ctx context.Context, audience domain.Audience, id int64, addedAt time.Time, limit int) ([]domain.RelatedNeighbor, error) {
	at := addedAt.Unix()
	visible := visibleVideoCondition("videos", audience) + ` and ` + shownVideoCondition("videos", audience)
	before := `select id, added_at from videos
		where (added_at < ? or (added_at = ? and id < ?)) and ` + visible + `
		order by added_at desc, id desc limit ?`
	after := `select id, added_at from videos
		where (added_at > ? or (added_at = ? and id > ?)) and ` + visible + `
		order by added_at asc, id desc limit ?`

	neighbors := []domain.RelatedNeighbor{}
	for _, query := range []string{before, after} {
		side, err := s.relatedNeighbors(ctx, query, at, at, id, limit)
		if err != nil {
			return nil, fmt.Errorf("cannot read videos added around the same time: %w", err)
		}
		neighbors = append(neighbors, side...)
	}
	return neighbors, nil
}

func (s *LibraryStore) relatedNeighbors(ctx context.Context, query string, args ...any) ([]domain.RelatedNeighbor, error) {
	rows, err := s.db.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var neighbors []domain.RelatedNeighbor
	for rows.Next() {
		var item domain.RelatedNeighbor
		var addedAt int64
		if err := rows.Scan(&item.VideoID, &addedAt); err != nil {
			return nil, err
		}
		item.AddedAt = time.Unix(addedAt, 0)
		neighbors = append(neighbors, item)
	}
	return neighbors, rows.Err()
}

// VideosByIDs は指定した動画の本体を返す。並びは ids と同じで、見る人に見せて
// よい所在が無い動画（登録フォルダの下に所在が無くなった動画と、ゲストには
// 非公開の動画）と、集まりの代表以外のバージョン（shownVideoCondition）は抜ける。
func (s *LibraryStore) VideosByIDs(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error) {
	if len(ids) == 0 {
		return []domain.Video{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := s.db.sql.QueryContext(ctx, `select `+videoColumns(audience)+` from videos where videos.id in (`+
		placeholders+`) and `+visibleVideoCondition("videos", audience)+` and `+shownVideoCondition("videos", audience), args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read related videos: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byID := make(map[int64]domain.Video, len(ids))
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read related videos: %w", err)
		}
		byID[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read related videos: %w", err)
	}

	videos := make([]domain.Video, 0, len(ids))
	for _, id := range ids {
		if video, ok := byID[id]; ok {
			videos = append(videos, video)
		}
	}
	return videos, nil
}

// VideoGroup は動画 id が属するグループを、見る人に見せてよいメンバーだけで返す
// （GET /api/videos/{id} の group、関連動画の group、GET /api/videos/{id}/group-members。
// specs/017-folder-groups/contracts/folder-groups-api.md §3）。メンバーの id はすべてを
// グループの中の並びで返し、詳細は window の範囲だけを読む。グループが大きくても、読む
// 詳細と応答はメンバーの本数に比例しない（issue 674）。見せ方はライブラリの項目と同じで
// ある（minGroupMembers）。グループに属さない動画、見せてよいメンバーが足りないグループ、
// この動画自身を見せられないときは false を返す。
//
// グループのフォルダは、メンバーと同じ読み取りのスナップショットの登録フォルダから求める。
// 索引は登録フォルダの変更と同じ取引で作り直されるので、求められなければ索引の不整合として
// 失敗を返す。
func (s *LibraryStore) VideoGroup(ctx context.Context, audience domain.Audience, id int64, window domain.GroupWindow) (domain.VideoGroup, bool, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("cannot start reading the video's group: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var groupID int64
	var path, name string
	err = tx.QueryRowContext(ctx, `select g.id, g.path, g.name from folder_group_members m
		join folder_groups g on g.id = m.group_id where m.video_id = ?`, id).Scan(&groupID, &path, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.VideoGroup{}, false, nil
	}
	if err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("cannot read the video's group: %w", err)
	}
	memberIDs, err := groupMemberIDs(ctx, tx, audience, groupID)
	if err != nil {
		return domain.VideoGroup{}, false, err
	}
	group := domain.VideoGroup{Name: name, MemberIDs: memberIDs}
	position := group.Position(id)
	if len(memberIDs) < minGroupMembers(audience) || position == 0 {
		return domain.VideoGroup{}, false, nil
	}
	if window.Limit > 0 {
		group.Offset = window.Start(len(memberIDs), position)
		end := min(group.Offset+window.Limit, len(memberIDs))
		group.Members, err = groupMembers(ctx, tx, audience, memberIDs[group.Offset:end])
		if err != nil {
			return domain.VideoGroup{}, false, err
		}
	}
	roots, err := listMediaFolders(ctx, tx)
	if err != nil {
		return domain.VideoGroup{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("cannot finish reading the video's group: %w", err)
	}

	folder, located := domain.LocateFolder(roots, path)
	if !located {
		return domain.VideoGroup{}, false, fmt.Errorf("the group folder is not under a media folder: %s", path)
	}
	group.Folder = folder
	return group, true, nil
}

// groupMemberIDs はグループの見せてよいメンバーの id を、グループの中の並びで返す
// （loadGroups と同じ条件）。
func groupMemberIDs(ctx context.Context, tx *sql.Tx, audience domain.Audience, groupID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `select m.video_id from folder_group_members m
		join videos on videos.id = m.video_id
		where m.group_id = ? and `+visibleVideoCondition("videos", audience)+`
		order by m.position`, groupID)
	if err != nil {
		return nil, fmt.Errorf("cannot read group members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read group members: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read group members: %w", err)
	}
	return ids, nil
}

// groupMembers はメンバーの詳細を ids の並びで返す。ids は同じ取引の groupMemberIDs から
// 取ったものなので、どれも見せてよい。
func groupMembers(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64) ([]domain.Video, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build group member ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(audience)+` from videos
		where videos.id in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read group members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byID := make(map[int64]domain.Video, len(ids))
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read group members: %w", err)
		}
		byID[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read group members: %w", err)
	}
	members := make([]domain.Video, 0, len(ids))
	for _, id := range ids {
		if video, ok := byID[id]; ok {
			members = append(members, video)
		}
	}
	return members, nil
}
