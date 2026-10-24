package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Keep columns aligned with scanVideo; representativeJoin supplies the smallest audience-visible location path.
var videoColumnsTemplate = `videos.id, ` + locationColumns("rep.path", "rep") + `,
	videos.added_at, videos.indexed_at, videos.content_key, {userKey} as user_key, videos.duration_ms, videos.width,
	videos.height, videos.display_aspect_ratio, videos.container, videos.video_codec, videos.audio_codec, videos.playable,
	videos.unplayable_reason, videos.probe_state, videos.probe_error, videos.probe_error_code, videos.thumbnail_state, videos.seek_thumbnail_state, videos.preview_state,
	{public} as public, {favorite} as favorite, ` + overrideColumns + `, ` + editedAtColumn + `, ` + buildColumns

// Column order matches scanVideo.
const buildColumns = `videos.thumbnail_build, videos.seek_thumbnail_build, videos.preview_build`

// Windows recognizes both slash styles; other systems treat backslash as filename data, shared by listing and search-key rules.
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

func videoColumns(audience domain.Audience) string {
	return strings.NewReplacer(
		"{public}", publicColumn,
		"{favorite}", favoriteColumn,
		"{userKey}", userKeyExpr("videos"),
	).Replace(videoColumnsTemplate)
}

func (s *LibraryStore) VideoLocations(ctx context.Context, videoID int64) ([]domain.VideoLocation, error) {
	rows, err := s.db.sql.QueryContext(ctx, `select id, video_id, path, version, title, size_bytes, mtime, file_created_at, created_at, updated_at
		from video_locations where video_id = ? order by path`, videoID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	locations := []domain.VideoLocation{}
	for rows.Next() {
		var item domain.VideoLocation
		var mtime, createdAt, updatedAt int64
		var fileCreatedAt sql.NullInt64
		if err := rows.Scan(&item.ID, &item.VideoID, &item.Path, &item.Version, &item.Title,
			&item.SizeBytes, &mtime, &fileCreatedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.MTime = time.Unix(mtime, 0)
		item.FileCreatedAt = unixOrZero(fileCreatedAt)
		item.CreatedAt = time.Unix(createdAt, 0)
		item.UpdatedAt = time.Unix(updatedAt, 0)
		locations = append(locations, item)
	}
	return locations, rows.Err()
}

func (s *IngestStore) GetVideo(ctx context.Context, id int64) (domain.Video, error) {
	return getVideo(ctx, s.db.sql, domain.AudienceOwner, id)
}

// GetVideo returns domain.ErrNotFound for both hidden and missing videos.
func (s *LibraryStore) GetVideo(ctx context.Context, audience domain.Audience, id int64) (domain.Video, error) {
	return getVideo(ctx, s.db.sql, audience, id)
}

type videoQueryer interface {
	rowQueryer
	rowsQueryer
}

// Select the representative once for all columns; a virtual video reads its own row instead.
func representativeJoin(audience domain.Audience) string {
	return ` left join video_locations rep on rep.id = (select l.id from video_locations l
		where l.video_id = videos.id and ` + visibleLocationCondition("l", audience) + ` order by l.path limit 1)` +
		virtualVideoJoin("videos")
}

func getVideo(ctx context.Context, q videoQueryer, audience domain.Audience, id int64) (domain.Video, error) {
	row := q.QueryRowContext(ctx, `select `+videoColumns(audience)+` from videos`+representativeJoin(audience)+` where videos.id = ? and `+
		visibleVideoCondition("videos", audience), id)

	video, err := scanVideo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Video{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot read the video (id=%d): %w", id, err)
	}
	videos := []domain.Video{video}
	if err := attachVersions(ctx, q, audience, videos); err != nil {
		return domain.Video{}, err
	}
	video = videos[0]
	return video, nil
}

func syncRepresentativeContainer(ctx context.Context, tx *sql.Tx, videoID int64) error {
	query := `select l.path, v.container, v.probe_state, coalesce(v.video_codec, ''), coalesce(v.audio_codec, '')
		from videos v join video_locations l on l.video_id = v.id
		where v.id = ? and ` + registeredLocationCondition("l") + ` order by l.path limit 1`
	var path, probeState, videoCodec, audioCodec string
	var oldContainer sql.NullString
	err := tx.QueryRowContext(ctx, query, videoID).Scan(&path, &oldContainer, &probeState, &videoCodec, &audioCodec)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the primary location (video=%d): %w", videoID, err)
	}
	container := domain.ContainerFromPath(path)
	if oldContainer.String == container {
		return nil
	}
	if probeState == string(domain.ProbeStateDone) {
		play := domain.EvaluatePlayability(container, domain.Probe{VideoCodec: videoCodec, AudioCodec: audioCodec})
		_, err = tx.ExecContext(ctx, `update videos set container = ?, playable = ?, unplayable_reason = ?, indexed_at = ? where id = ?`,
			nullableString(container), boolToInt(play.Playable), nullableString(string(play.Reason)), time.Now().Unix(), videoID)
	} else {
		_, err = tx.ExecContext(ctx, `update videos set container = ?, playable = 0, unplayable_reason = null, indexed_at = ? where id = ?`,
			nullableString(container), time.Now().Unix(), videoID)
	}
	if err != nil {
		return fmt.Errorf("cannot update the primary location's container (video=%d): %w", videoID, err)
	}
	return nil
}

// ContentKeyReferenced prevents publishing artifacts whose content lost its final video during generation.
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
		return false, fmt.Errorf("cannot check references to the content key: %w", err)
	}
	return referenced == 1, nil
}

// Bind IDs as one JSON array to avoid SQLite limits; IN deduplicates IDs while excluding unidentified or unregistered content.
func registeredContentKeysForVideoIDs(ctx context.Context, q queryExecer, videoIDs []int64) ([]string, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}

	query := `select v.content_key from videos v
		where v.id in (select value from json_each(?)) and v.content_key <> '' and ` +
		registeredVideoCondition("v")
	rows, err := q.QueryContext(ctx, query, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot map video ids to content_key values: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("cannot map video ids to content_key values: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot map video ids to content_key values: %w", err)
	}
	return keys, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanVideo(row rowScanner) (domain.Video, error) {
	var (
		video                                    domain.Video
		mtime, fileCreatedAt, addedAt, indexedAt int64
		durationMs                               sql.NullInt64
		width, height                            sql.NullInt64
		displayAspectRatio                       sql.NullFloat64
		container, videoCodec, audioCodec        sql.NullString
		unplayableReason, probeError             sql.NullString
		probeErrorCode                           sql.NullString
		playable                                 int
		probeState, thumbnailState, previewState string
		seekThumbnailState                       string
		public, favorite                         bool
		displayName                              sql.NullString
		thumbnailPositionMs, thumbnailRevision   sql.NullInt64
		thumbnailImageRevision                   sql.NullInt64
		thumbnailImageOrigin                     sql.NullString
		editedAt                                 int64
	)

	err := row.Scan(
		&video.ID, &video.Path, &video.Title, &video.SizeBytes, &mtime, &fileCreatedAt, &addedAt, &indexedAt,
		&video.ContentKey, &video.UserKey, &durationMs, &width, &height, &displayAspectRatio, &container, &videoCodec, &audioCodec,
		&playable, &unplayableReason, &probeState, &probeError, &probeErrorCode, &thumbnailState, &seekThumbnailState, &previewState,
		&public, &favorite, &displayName, &thumbnailPositionMs, &thumbnailRevision, &thumbnailImageRevision,
		&thumbnailImageOrigin, &editedAt, &video.Builds.Thumbnail, &video.Builds.SeekThumbnail, &video.Builds.Preview,
	)
	if err != nil {
		return domain.Video{}, err
	}

	video.MTime = time.Unix(0, mtime)
	video.FileCreatedAt = time.Unix(fileCreatedAt, 0)
	video.AddedAt = time.Unix(addedAt, 0)
	video.IndexedAt = time.Unix(indexedAt, 0)
	video.EditedAt = time.Unix(editedAt, 0)
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
	if displayAspectRatio.Valid && displayAspectRatio.Float64 > 0 {
		value := displayAspectRatio.Float64
		video.DisplayAspectRatio = &value
	}
	video.Container = container.String
	video.VideoCodec = videoCodec.String
	video.AudioCodec = audioCodec.String
	video.Playable = playable != 0
	video.UnplayableReason = domain.UnplayableReason(unplayableReason.String)
	video.ProbeState = domain.ProbeState(probeState)
	video.ProbeError = probeError.String
	video.ProbeErrorCode = domain.ProbeErrorCode(probeErrorCode.String)
	video.ThumbnailState = domain.ThumbnailState(thumbnailState)
	video.SeekThumbnailState = domain.SeekThumbnailState(seekThumbnailState)
	video.PreviewState = domain.PreviewState(previewState)
	video.Public = public
	video.Favorite = favorite
	video.FileTitle = video.Title
	if displayName.Valid && displayName.String != "" {
		video.DisplayName = displayName.String
		video.Title = displayName.String
	}
	if thumbnailPositionMs.Valid {
		value := thumbnailPositionMs.Int64
		video.ThumbnailPositionMs = &value
	}
	video.ThumbnailRevision = thumbnailRevision.Int64
	video.ThumbnailImageRevision = thumbnailImageRevision.Int64
	video.ThumbnailImageOrigin = domain.ThumbnailImageOrigin(thumbnailImageOrigin.String)

	return video, nil
}

// The modification time in nanoseconds; rows scanned before mtime_ns existed have whole seconds only.
func mtimeNsExpr(alias string) string {
	return `coalesce(` + alias + `.mtime_ns, ` + alias + `.mtime * 1000000000)`
}

// Use modification time when file creation time is unknown.
func fileCreatedAtExpr(alias string) string {
	return `coalesce(` + alias + `.file_created_at, ` + alias + `.mtime)`
}

// A virtual video has no location; readers join its row as vv and take its location values from the expressions below.
func virtualVideoJoin(video string) string {
	return ` left join virtual_videos vv on vv.user_key = ` + video + `.content_key`
}

// The path is NULL for a chosen virtual video, so the location join finds nothing and vv supplies the values.
func chosenLocationJoin(path, video string) string {
	return ` left join video_locations loc on loc.path = ` + path + virtualVideoJoin(video)
}

// A virtual video's creation time is its modified and created sort value.
func locationMTimeExpr(loc string) string {
	return `coalesce(` + loc + `.mtime, vv.created_at)`
}

// The scanned modification time in nanoseconds; a virtual video reports its creation time.
func locationMTimeNsExpr(loc string) string {
	return `coalesce(` + mtimeNsExpr(loc) + `, vv.created_at * 1000000000)`
}

func locationCreatedAtExpr(loc string) string {
	return `coalesce(` + fileCreatedAtExpr(loc) + `, vv.created_at)`
}

func locationTitleKeyExpr(loc string) string {
	return `coalesce(` + loc + `.title_key, vv.title_key)`
}

// Columns in scanVideo order from path to file creation time; a virtual video has an empty path and size 0.
func locationColumns(path, loc string) string {
	return `coalesce(` + path + `, '') as path, coalesce(` + loc + `.title, vv.title) as title, ` +
		`coalesce(` + loc + `.size_bytes, 0) as size_bytes, ` + locationMTimeNsExpr(loc) + ` as mtime, ` +
		locationCreatedAtExpr(loc) + ` as file_created_at`
}

func nullableUnix(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}

func unixOrZero(value sql.NullInt64) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return time.Unix(value.Int64, 0)
}

// Treat an empty string as absent rather than preserving a separate empty-column value.
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// Zero becomes NULL; unknown duration must not be represented as zero.
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

func nullableFloat64(value float64) any {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
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
