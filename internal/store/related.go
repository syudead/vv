package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// DirectVideoPaths はディレクトリ直下の動画を、id とパスだけで全件返す。
// 同じ動画の所在が直下に2つ以上あるときは、ListFolderVideos と同じく
// パスの小さい方を1件だけ返す（directVideosCTE）。並べ方は internal/domain の
// OrderRelated が決める。
func (db *DB) DirectVideoPaths(ctx context.Context, dir string) ([]domain.RelatedSibling, error) {
	prefix := folderPrefix(dir)
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	rows, err := db.sql.QueryContext(ctx, directVideosCTE()+` select video_id, path from direct`, prefix, prefix)
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
// 返す。登録フォルダの下に所在の無い動画と、この動画自身は含めない。
//
// 片側の並びは、追加日時の差が小さい順、差が同じなら id の大きい順で、
// OrderRelated の補い方と同じである。そのため、同じフォルダの動画を除いたあとに
// 要る件数（limit から同じフォルダの件数を引いた数）は、両側の先頭 limit 件の
// 中に必ず入っている。
func (db *DB) VideosAddedNear(ctx context.Context, id int64, addedAt time.Time, limit int) ([]domain.RelatedNeighbor, error) {
	at := addedAt.Unix()
	registered := registeredVideoCondition("videos")
	before := `select id, added_at from videos
		where (added_at < ? or (added_at = ? and id < ?)) and ` + registered + `
		order by added_at desc, id desc limit ?`
	after := `select id, added_at from videos
		where (added_at > ? or (added_at = ? and id > ?)) and ` + registered + `
		order by added_at asc, id desc limit ?`

	neighbors := []domain.RelatedNeighbor{}
	for _, query := range []string{before, after} {
		side, err := db.relatedNeighbors(ctx, query, at, at, id, limit)
		if err != nil {
			return nil, fmt.Errorf("追加日時の近い動画を読み出せません: %w", err)
		}
		neighbors = append(neighbors, side...)
	}
	return neighbors, nil
}

func (db *DB) relatedNeighbors(ctx context.Context, query string, args ...any) ([]domain.RelatedNeighbor, error) {
	rows, err := db.sql.QueryContext(ctx, query, args...)
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

// VideosByIDs は指定した動画の本体を返す。並びは ids と同じで、登録フォルダの
// 下に所在が無くなった動画は抜ける。
func (db *DB) VideosByIDs(ctx context.Context, ids []int64) ([]domain.Video, error) {
	if len(ids) == 0 {
		return []domain.Video{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	//nolint:gosec // 組み立てるのは列名・定型の条件句・プレースホルダの数だけで、値は引数で渡す。
	rows, err := db.sql.QueryContext(ctx, `select `+videoColumns()+` from videos where videos.id in (`+
		placeholders+`) and `+registeredVideoCondition("videos"), args...)
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
