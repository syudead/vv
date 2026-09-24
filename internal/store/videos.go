package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// videoColumns は domain.Video を組み立てるのに要る列である。
// 並びは scanVideo と対応させる。
const videoColumnsTemplate = `videos.id,
	(select path from video_locations l where video_id = videos.id and {registered} order by path limit 1) as path,
	(select title from video_locations l where video_id = videos.id and {registered} order by path limit 1) as title,
	(select size_bytes from video_locations l where video_id = videos.id and {registered} order by path limit 1) as size_bytes,
	(select mtime from video_locations l where video_id = videos.id and {registered} order by path limit 1) as mtime,
	videos.added_at, videos.updated_at, videos.content_key, videos.duration_ms, videos.width,
	videos.height, videos.container, videos.video_codec, videos.audio_codec, videos.playable,
	videos.unplayable_reason, videos.probe_state, videos.probe_error, videos.thumbnail_state, videos.preview_state`

// registrationSeparators は、登録フォルダの下かどうかを調べるときに区切りとして
// 扱う文字である。Windows では `/` と `\` の両方、それ以外の OS では `/` だけで、
// `\` はファイル名の一部である。一覧の判定（registeredLocationCondition）と
// 照合用の鍵（registeredRelativePath）は、この同じ規則を使う。
func registrationSeparators() []rune {
	if runtime.GOOS == "windows" {
		return []rune{'/', '\\'}
	}
	return []rune{os.PathSeparator}
}

func registeredLocationCondition(alias string) string {
	pathExpr := alias + `.path`
	rootExpr := `mf.path`
	if runtime.GOOS == "windows" {
		pathExpr = `lower(` + pathExpr + `)`
		rootExpr = `lower(` + rootExpr + `)`
	}
	separators := registrationSeparators()
	chars := make([]string, 0, len(separators))
	for _, separator := range separators {
		chars = append(chars, `char(`+strconv.Itoa(int(separator))+`)`)
	}
	trimmedRoot := `rtrim(` + rootExpr + `, ` + strings.Join(chars, ` || `) + `)`
	condition := `exists (select 1 from media_folders mf where ` + pathExpr + ` = ` + rootExpr
	for _, char := range chars {
		condition += ` or instr(` + pathExpr + `, ` + trimmedRoot + ` || ` + char + `) = 1`
	}
	return condition + `)`
}

func registeredVideoCondition(alias string) string {
	return `exists (select 1 from video_locations l where l.video_id = ` + alias + `.id and ` +
		registeredLocationCondition("l") + `)`
}

func videoColumns() string {
	return strings.ReplaceAll(videoColumnsTemplate, "{registered}", registeredLocationCondition("l"))
}

// VideoLocations は動画の所在をパスの順にすべて返す。配信と既定アプリで開く
// 操作が、登録フォルダの内側にある実体を選ぶのに使う。
func (s *LibraryStore) VideoLocations(ctx context.Context, videoID int64) ([]domain.VideoLocation, error) {
	rows, err := s.db.sql.QueryContext(ctx, `select id, video_id, path, version, title, size_bytes, mtime, created_at, updated_at
		from video_locations where video_id = ? order by path`, videoID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	locations := []domain.VideoLocation{}
	for rows.Next() {
		var item domain.VideoLocation
		var mtime, createdAt, updatedAt int64
		if err := rows.Scan(&item.ID, &item.VideoID, &item.Path, &item.Version, &item.Title,
			&item.SizeBytes, &mtime, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.MTime = time.Unix(mtime, 0)
		item.CreatedAt = time.Unix(createdAt, 0)
		item.UpdatedAt = time.Unix(updatedAt, 0)
		locations = append(locations, item)
	}
	return locations, rows.Err()
}

// GetVideo は1件を返す。取り込みと閲覧が同じものを読むので、SQL は getVideo の
// 1か所に置く。
func (s *IngestStore) GetVideo(ctx context.Context, id int64) (domain.Video, error) {
	return getVideo(ctx, s.db.sql, id)
}

func (s *LibraryStore) GetVideo(ctx context.Context, id int64) (domain.Video, error) {
	return getVideo(ctx, s.db.sql, id)
}

func getVideo(ctx context.Context, q rowQueryer, id int64) (domain.Video, error) {
	//nolint:gosec // videoColumns は定数で、利用者の入力は混ざらない。
	row := q.QueryRowContext(ctx, `select `+videoColumns()+` from videos where videos.id = ? and `+registeredVideoCondition("videos"), id)

	video, err := scanVideo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Video{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Video{}, fmt.Errorf("動画を読み出せません (id=%d): %w", id, err)
	}
	return video, nil
}

func syncRepresentativeContainer(ctx context.Context, tx *sql.Tx, videoID int64) error {
	query := `select l.path, v.container, v.probe_state, coalesce(v.video_codec, ''), coalesce(v.audio_codec, '')
		from videos v join video_locations l on l.video_id = v.id
		where v.id = ? and ` + registeredLocationCondition("l") + ` order by l.path limit 1`
	var path, probeState, videoCodec, audioCodec string
	var oldContainer sql.NullString
	//nolint:gosec // registeredLocationCondition は定型SQLだけを返す。
	err := tx.QueryRowContext(ctx, query, videoID).Scan(&path, &oldContainer, &probeState, &videoCodec, &audioCodec)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("代表場所を読み出せません (video=%d): %w", videoID, err)
	}
	container := domain.ContainerFromPath(path)
	if oldContainer.String == container {
		return nil
	}
	if probeState == string(domain.ProbeStateDone) {
		play := domain.EvaluatePlayability(container, domain.Probe{VideoCodec: videoCodec, AudioCodec: audioCodec})
		_, err = tx.ExecContext(ctx, `update videos set container = ?, playable = ?, unplayable_reason = ?, updated_at = ? where id = ?`,
			nullableString(container), boolToInt(play.Playable), nullableString(string(play.Reason)), time.Now().Unix(), videoID)
	} else {
		_, err = tx.ExecContext(ctx, `update videos set container = ?, playable = 0, unplayable_reason = null, updated_at = ? where id = ?`,
			nullableString(container), time.Now().Unix(), videoID)
	}
	if err != nil {
		return fmt.Errorf("代表場所のcontainerを更新できません (video=%d): %w", videoID, err)
	}
	return nil
}

// ContentKeyReferenced は内容の識別子を持つ動画が今もあるかを返す。生成中に
// 動画が消えた場合に、書き終えた生成物を残さないために使う。取り込みと閲覧が
// 同じものを確かめるので、SQL は contentKeyReferenced の1か所に置く。
func (s *IngestStore) ContentKeyReferenced(ctx context.Context, key string) (bool, error) {
	return contentKeyReferenced(ctx, s.db.sql, key)
}

func (s *LibraryStore) ContentKeyReferenced(ctx context.Context, key string) (bool, error) {
	return contentKeyReferenced(ctx, s.db.sql, key)
}

func contentKeyReferenced(ctx context.Context, q rowQueryer, key string) (bool, error) {
	var referenced int
	if err := q.QueryRowContext(ctx, `select exists(select 1 from videos where content_key = ?)`, key).
		Scan(&referenced); err != nil {
		return false, fmt.Errorf("識別子の参照を確かめられません: %w", err)
	}
	return referenced == 1, nil
}

// rowScanner は *sql.Row と *sql.Rows の共通部分である。
type rowScanner interface {
	Scan(dest ...any) error
}

// scanVideo は1行を domain.Video へ写す。列の並びは videoColumns と対応する。
func scanVideo(row rowScanner) (domain.Video, error) {
	var (
		video                                    domain.Video
		mtime, addedAt, updatedAt                int64
		durationMs                               sql.NullInt64
		width, height                            sql.NullInt64
		container, videoCodec, audioCodec        sql.NullString
		unplayableReason, probeError             sql.NullString
		playable                                 int
		probeState, thumbnailState, previewState string
	)

	err := row.Scan(
		&video.ID, &video.Path, &video.Title, &video.SizeBytes, &mtime, &addedAt, &updatedAt,
		&video.ContentKey, &durationMs, &width, &height, &container, &videoCodec, &audioCodec,
		&playable, &unplayableReason, &probeState, &probeError, &thumbnailState, &previewState,
	)
	if err != nil {
		return domain.Video{}, err
	}

	video.MTime = time.Unix(mtime, 0)
	video.AddedAt = time.Unix(addedAt, 0)
	video.UpdatedAt = time.Unix(updatedAt, 0)
	if durationMs.Valid {
		value := durationMs.Int64
		video.DurationMs = &value
	}
	if width.Valid {
		value := int(width.Int64)
		video.Width = &value
	}
	if height.Valid {
		value := int(height.Int64)
		video.Height = &value
	}
	video.Container = container.String
	video.VideoCodec = videoCodec.String
	video.AudioCodec = audioCodec.String
	video.Playable = playable != 0
	video.UnplayableReason = domain.UnplayableReason(unplayableReason.String)
	video.ProbeState = domain.ProbeState(probeState)
	video.ProbeError = probeError.String
	video.ThumbnailState = domain.ThumbnailState(thumbnailState)
	video.PreviewState = domain.PreviewState(previewState)

	return video, nil
}

// nullableString は空文字を null として書き込む。「値が無い」と「空文字」を
// 列の上で区別しないための単純化である。
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// nullableInt64 は 0 を null として書き込む。尺は 0 で代用しない。
func nullableInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
