package store

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// folderPrefix はフォルダ配下の所在の接頭辞（フォルダのパス + 区切り）を返す。
// `/` のように区切りで終わるフォルダには区切りを重ねない。
func folderPrefix(dir string) string {
	separator := string(os.PathSeparator)
	prefix := strings.TrimRight(dir, `/\`) + separator
	if runtime.GOOS == "windows" {
		prefix = strings.ToLower(prefix)
	}
	return prefix
}

// folderPathExpr は所在のパスを接頭辞と比べられる形にする。Windows では
// 大文字小文字を区別しない（registeredLocationCondition と同じ扱い）。
func folderPathExpr(alias string) string {
	if runtime.GOOS == "windows" {
		return `lower(` + alias + `.path)`
	}
	return alias + `.path`
}

// directChildCondition は、接頭辞の後ろに区切りが無い（直下にある）ことを表す。
func directChildCondition(alias string) string {
	rest := `substr(` + folderPathExpr(alias) + `, length(?) + 1)`
	condition := `instr(` + rest + `, char(` + strconv.Itoa(int(os.PathSeparator)) + `)) = 0`
	if runtime.GOOS == "windows" {
		condition += ` and instr(` + rest + `, char(47)) = 0`
	}
	return condition
}

// FolderLocations はフォルダ配下（深さを問わない）の、登録フォルダの下にある
// 所在をすべて返す。集計は internal/domain の SummarizeFolder が行う。
func (db *DB) FolderLocations(ctx context.Context, dir string) ([]domain.FolderLocation, error) {
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	rows, err := db.sql.QueryContext(ctx, `select l.path, l.video_id, videos.content_key, videos.thumbnail_state
		from video_locations l join videos on videos.id = l.video_id
		where instr(`+folderPathExpr("l")+`, ?) = 1 and `+registeredLocationCondition("l"),
		folderPrefix(dir))
	if err != nil {
		return nil, fmt.Errorf("フォルダの中身を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	locations := []domain.FolderLocation{}
	for rows.Next() {
		var item domain.FolderLocation
		var thumbnail string
		if err := rows.Scan(&item.Path, &item.VideoID, &item.ContentKey, &thumbnail); err != nil {
			return nil, fmt.Errorf("フォルダの中身を読み出せません: %w", err)
		}
		item.ThumbnailState = domain.ThumbnailState(thumbnail)
		locations = append(locations, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("フォルダの中身を読み出せません: %w", err)
	}
	return locations, nil
}

// HasFolderLocations はフォルダ配下（深さを問わない）に、登録フォルダの下に
// ある所在が1件でもあるかを返す。
func (db *DB) HasFolderLocations(ctx context.Context, dir string) (bool, error) {
	var found int
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	err := db.sql.QueryRowContext(ctx, `select exists (select 1 from video_locations l
		where instr(`+folderPathExpr("l")+`, ?) = 1 and `+registeredLocationCondition("l")+`)`,
		folderPrefix(dir)).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("フォルダの有無を確かめられません: %w", err)
	}
	return found == 1, nil
}

// folderVideoColumns は直下の動画1件を domain.Video に写す列である。並びは
// scanVideo と対応させる。題名・大きさ・更新時刻はそのフォルダにある所在の
// ものを使う（同じ内容がほかのフォルダにあっても、ここではこの名前で見える）。
const folderVideoColumns = `videos.id, direct.path, loc.title, loc.size_bytes, loc.mtime,
	videos.added_at, videos.updated_at, videos.content_key, videos.duration_ms, videos.width,
	videos.height, videos.container, videos.video_codec, videos.audio_codec, videos.playable,
	videos.unplayable_reason, videos.probe_state, videos.probe_error, videos.thumbnail_state`

// directVideosCTE は直下の所在を動画ごとに1件へまとめる。同じ動画の所在が
// 同じフォルダに2つ以上あるときは、パスの昇順で先のものを代表にする。
func directVideosCTE() string {
	return `with direct as (
		select l.video_id, min(l.path) as path from video_locations l
		where instr(` + folderPathExpr("l") + `, ?) = 1 and ` + directChildCondition("l") +
		` and ` + registeredLocationCondition("l") + `
		group by l.video_id)`
}

// ListFolderVideos はフォルダ直下の動画1ページを返す。並び順とカーソルの形は
// ListVideos と同じで、titleAsc の題名はそのフォルダにある所在の題名である。
func (db *DB) ListFolderVideos(ctx context.Context, q domain.FolderVideoQuery) (VideoPage, error) {
	limit := normalizeLimit(q.Limit)
	sort := q.Sort
	if sort == "" {
		sort = SortAddedDesc
	}
	prefix := folderPrefix(q.Dir)
	args := []any{prefix, prefix}

	var total int
	//nolint:gosec // 組み立てるのは定型の条件句だけで、値はすべて引数で渡す。
	if err := db.sql.QueryRowContext(ctx, directVideosCTE()+` select count(*) from direct`, args...).Scan(&total); err != nil {
		return VideoPage{}, fmt.Errorf("フォルダの動画を数えられません: %w", err)
	}

	query := directVideosCTE() + ` select * from (select ` + folderVideoColumns + `
		from direct join videos on videos.id = direct.video_id
		join video_locations loc on loc.path = direct.path) as videos`
	if q.Cursor != "" {
		condition, cursorArgs, err := cursorCondition(sort, q.Cursor)
		if err != nil {
			return VideoPage{}, err
		}
		query += ` where ` + condition
		args = append(args, cursorArgs...)
	}
	query += ` order by ` + orderBy(sort) + ` limit ?`

	rows, err := db.sql.QueryContext(ctx, query, append(args, limit+1)...)
	if err != nil {
		return VideoPage{}, fmt.Errorf("フォルダの動画を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	page := VideoPage{Total: total, Limit: limit, Items: []domain.Video{}}
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return VideoPage{}, fmt.Errorf("フォルダの動画を読み出せません: %w", err)
		}
		page.Items = append(page.Items, video)
	}
	if err := rows.Err(); err != nil {
		return VideoPage{}, fmt.Errorf("フォルダの動画を読み出せません: %w", err)
	}

	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = encodeCursor(sort, page.Items[len(page.Items)-1])
	}
	return page, nil
}
