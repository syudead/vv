package store

import (
	"context"
	"database/sql"
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
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	rows, err := s.db.sql.QueryContext(ctx, cte+` select video_id, path from chosen`, args...)
	if err != nil {
		return nil, fmt.Errorf("同じフォルダの動画を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	siblings := []domain.RelatedSibling{}
	for rows.Next() {
		var item domain.RelatedSibling
		if err := rows.Scan(&item.VideoID, &item.Path); err != nil {
			return nil, fmt.Errorf("同じフォルダの動画を読み出せません: %w", err)
		}
		siblings = append(siblings, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("同じフォルダの動画を読み出せません: %w", err)
	}
	return siblings, nil
}

// VideosAddedNear は追加日時がこの動画に近い動画を、前後それぞれ最大 limit 件
// 返す。見る人に見せてよい所在の無い動画（登録フォルダの下に所在の無い動画と、
// ゲストには非公開の動画）と、この動画自身は含めない。
//
// 片側の並びは、追加日時の差が小さい順、差が同じなら id の大きい順で、
// OrderRelated の補い方と同じである。そのため、同じフォルダの動画を除いたあとに
// 要る件数（limit から同じフォルダの件数を引いた数）は、両側の先頭 limit 件の
// 中に必ず入っている。
func (s *LibraryStore) VideosAddedNear(ctx context.Context, audience domain.Audience, id int64, addedAt time.Time, limit int) ([]domain.RelatedNeighbor, error) {
	at := addedAt.Unix()
	visible := visibleVideoCondition("videos", audience)
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
			return nil, fmt.Errorf("追加日時の近い動画を読み出せません: %w", err)
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
// 非公開の動画）は抜ける。
func (s *LibraryStore) VideosByIDs(ctx context.Context, audience domain.Audience, ids []int64) ([]domain.Video, error) {
	if len(ids) == 0 {
		return []domain.Video{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	//nolint:gosec // 組み立てるのは列名・定型の条件句・プレースホルダの数だけで、値は引数で渡す。
	rows, err := s.db.sql.QueryContext(ctx, `select `+videoColumns(audience)+` from videos where videos.id in (`+
		placeholders+`) and `+visibleVideoCondition("videos", audience), args...)
	if err != nil {
		return nil, fmt.Errorf("関連動画を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byID := make(map[int64]domain.Video, len(ids))
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("関連動画を読み出せません: %w", err)
		}
		byID[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("関連動画を読み出せません: %w", err)
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
// （GET /api/videos/{id} の group と関連動画の group、specs/017-folder-groups/contracts/folder-groups-api.md §3）。
// メンバーは loadGroups と同じくグループの中の並びで、見せ方はライブラリの項目と同じである
// （minGroupMembers）。グループに属さない動画、見せてよいメンバーが足りないグループ、
// この動画自身を見せられないときは false を返す。
//
// グループのフォルダは、メンバーと同じ読み取りのスナップショットの登録フォルダから求める。
// 索引は登録フォルダの変更と同じ取引で作り直されるので、求められなければ索引の不整合として
// 失敗を返す。
func (s *LibraryStore) VideoGroup(ctx context.Context, audience domain.Audience, id int64) (domain.VideoGroup, bool, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("動画のグループの読み取りを始められません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var groupID int64
	err = tx.QueryRowContext(ctx, `select group_id from folder_group_members where video_id = ?`, id).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.VideoGroup{}, false, nil
	}
	if err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("動画のグループを読み出せません: %w", err)
	}
	groups, err := loadGroups(ctx, tx, audience, []int64{groupID})
	if err != nil {
		return domain.VideoGroup{}, false, err
	}
	roots, err := listMediaFolders(ctx, tx)
	if err != nil {
		return domain.VideoGroup{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.VideoGroup{}, false, fmt.Errorf("動画のグループの読み取りを終えられません: %w", err)
	}

	group, ok := groups[groupID]
	if !ok || len(group.Members) < minGroupMembers(audience) {
		return domain.VideoGroup{}, false, nil
	}
	out := domain.VideoGroup{Name: group.Name, Members: group.Members}
	if out.Position(id) == 0 {
		return domain.VideoGroup{}, false, nil
	}
	folder, located := domain.LocateFolder(roots, group.Path)
	if !located {
		return domain.VideoGroup{}, false, fmt.Errorf("グループのフォルダが登録フォルダの下にありません: %s", group.Path)
	}
	out.Folder = folder
	return out, true, nil
}
