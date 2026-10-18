package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// External reads always use owner visibility but return only videos and locations inside registered folders.

// A distinct cursor sort name prevents reuse of screen-list cursors.
const externalCursorSort = "external"

// ListExternalVideos pages by ascending (added_at, id), reading videos, locations and tags from one snapshot.
func (s *LibraryStore) ListExternalVideos(ctx context.Context, q domain.ExternalVideoQuery) (domain.ExternalVideoPage, error) {
	limit := q.Limit
	switch {
	case limit <= 0:
		limit = domain.ExternalVideoDefaultLimit
	case limit > domain.MaxLimit:
		limit = domain.MaxLimit
	}
	query := `select videos.id, videos.added_at from videos where ` + registeredVideoCondition("videos")
	var args []any
	if q.Cursor != "" {
		addedAt, id, err := decodeExternalCursor(q.Cursor)
		if err != nil {
			return domain.ExternalVideoPage{}, err
		}
		query += ` and (videos.added_at, videos.id) > (?, ?)`
		args = append(args, addedAt, id)
	}
	query += ` order by videos.added_at, videos.id limit ?`
	args = append(args, limit+1)

	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ExternalVideoPage{}, fmt.Errorf("cannot start reading the videos: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := readExternalVideoKeys(ctx, tx, query, args)
	if err != nil {
		return domain.ExternalVideoPage{}, err
	}

	page := domain.ExternalVideoPage{Items: []domain.ExternalVideo{}}
	if len(keys) > limit {
		keys = keys[:limit]
		last := keys[limit-1]
		page.NextCursor = encodeExternalCursor(last.addedAt, last.id)
	}
	ids := make([]int64, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, k.id)
	}
	if page.Items, err = loadExternalVideos(ctx, tx, ids); err != nil {
		return domain.ExternalVideoPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ExternalVideoPage{}, fmt.Errorf("cannot finish reading the videos: %w", err)
	}
	return page, nil
}

// LookupExternalVideo matches exactly one ID, content key or raw path; missing or unregistered references return domain.ErrNotFound.
func (s *LibraryStore) LookupExternalVideo(ctx context.Context, ref domain.VideoRef) (domain.ExternalVideo, error) {
	if !ref.Valid() {
		return domain.ExternalVideo{}, domain.ErrNotFound
	}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ExternalVideo{}, fmt.Errorf("cannot start reading the video: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := resolveVideoRef(ctx, tx, ref)
	if err != nil {
		return domain.ExternalVideo{}, err
	}
	videos, err := loadExternalVideos(ctx, tx, []int64{id})
	if err != nil {
		return domain.ExternalVideo{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ExternalVideo{}, fmt.Errorf("cannot finish reading the video: %w", err)
	}
	if len(videos) != 1 {
		return domain.ExternalVideo{}, domain.ErrNotFound
	}
	return videos[0], nil
}

func resolveVideoRef(ctx context.Context, q rowQueryer, ref domain.VideoRef) (int64, error) {
	var query string
	var arg any
	switch {
	case ref.ID != 0:
		query = `select videos.id from videos where videos.id = ? and ` + registeredVideoCondition("videos")
		arg = ref.ID
	case ref.ContentKey != "":
		query = `select videos.id from videos where videos.content_key = ? and ` + registeredVideoCondition("videos")
		arg = ref.ContentKey
	default:
		// Require the referenced path itself to be registered, matching paths exposed by the external API.
		query = `select l.video_id from video_locations l where l.path = ? and ` + registeredLocationCondition("l")
		arg = ref.Path
	}
	var id int64
	err := q.QueryRowContext(ctx, query, arg).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("cannot look up the video: %w", err)
	}
	return id, nil
}

// Preserve requested ID order and drop missing videos or videos without registered locations.
func loadExternalVideos(ctx context.Context, tx *sql.Tx, ids []int64) ([]domain.ExternalVideo, error) {
	out := make([]domain.ExternalVideo, 0, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build video id values: %w", err)
	}

	byID, err := readVideosByIDs(ctx, tx, string(encoded))
	if err != nil {
		return nil, err
	}
	locations, err := readRegisteredLocations(ctx, tx, string(encoded))
	if err != nil {
		return nil, err
	}

	userKeys := make([]string, 0, len(byID))
	for _, video := range byID {
		// Unidentified content has no tags; identified content uses its user-data key.
		if video.UserKey != "" {
			userKeys = append(userKeys, video.UserKey)
		}
	}
	tags, err := tagsByContentKeys(ctx, tx, userKeys)
	if err != nil {
		return nil, err
	}
	// Read work information from the same snapshot and user-data key; missing rows stay nil for the API to render empty.
	infos, err := workInfoByUserKeys(ctx, tx, userKeys)
	if err != nil {
		return nil, err
	}

	for _, id := range ids {
		video, ok := byID[id]
		if !ok || len(locations[id]) == 0 {
			continue
		}
		if info, ok := infos[video.UserKey]; ok && video.UserKey != "" {
			video.WorkInfo = &info
		}
		item := domain.ExternalVideo{Video: video, Locations: locations[id], Tags: []domain.VideoTag{}}
		if video.UserKey != "" && tags[video.UserKey] != nil {
			item.Tags = tags[video.UserKey]
		}
		out = append(out, item)
	}
	return out, nil
}

type externalVideoKey struct{ id, addedAt int64 }

func readExternalVideoKeys(ctx context.Context, tx *sql.Tx, query string, args []any) ([]externalVideoKey, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var keys []externalVideoKey
	for rows.Next() {
		var k externalVideoKey
		if err := rows.Scan(&k.id, &k.addedAt); err != nil {
			return nil, fmt.Errorf("cannot read the videos: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the videos: %w", err)
	}
	return keys, nil
}

// Choose representative locations from registered folders with owner visibility.
func readVideosByIDs(ctx context.Context, tx *sql.Tx, encodedIDs string) (map[int64]domain.Video, error) {
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(domain.AudienceOwner)+` from videos`+
		representativeJoin(domain.AudienceOwner)+`
		where videos.id in (select value from json_each(?)) and `+registeredVideoCondition("videos"), encodedIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot read the videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byID := map[int64]domain.Video{}
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read the videos: %w", err)
		}
		byID[video.ID] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the videos: %w", err)
	}
	return byID, nil
}

func readRegisteredLocations(ctx context.Context, tx *sql.Tx, encodedIDs string) (map[int64][]string, error) {
	rows, err := tx.QueryContext(ctx, `select l.video_id, l.path from video_locations l
		where l.video_id in (select value from json_each(?)) and `+registeredLocationCondition("l")+`
		order by l.video_id, l.path`, encodedIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot read the video locations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	locations := map[int64][]string{}
	for rows.Next() {
		var videoID int64
		var path string
		if err := rows.Scan(&videoID, &path); err != nil {
			return nil, fmt.Errorf("cannot read the video locations: %w", err)
		}
		locations[videoID] = append(locations[videoID], path)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the video locations: %w", err)
	}
	return locations, nil
}

func encodeExternalCursor(addedAt, id int64) string {
	fields := []string{externalCursorSort, "", "0", strconv.FormatInt(id, 10), strconv.FormatInt(addedAt, 10)}
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator)))
}

// Reject malformed cursors and cursors belonging to screen lists.
func decodeExternalCursor(cursor string) (addedAt, id int64, err error) {
	c, err := decodeCursor(cursor)
	if err != nil {
		return 0, 0, err
	}
	if c.sort != externalCursorSort || c.seed != "" || c.isNull {
		return 0, 0, fmt.Errorf("%w: cursor is not for the external video list", domain.ErrInvalidCursor)
	}
	addedAt, err = strconv.ParseInt(c.value, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: cannot parse the added time", domain.ErrInvalidCursor)
	}
	return addedAt, c.id, nil
}
