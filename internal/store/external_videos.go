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

// 外部連携 API の動画の読み出し（specs/026-external-api/contracts/external-api.md §2、
// research.md R-6）。見る人は常に所有者で、非公開の動画も返す。どちらの読み出しも、
// 登録フォルダの下に所在を 1 つ以上持つ動画だけを対象にし、所在も登録フォルダの下の
// ものだけを返す。変更の追跡はしないので、表と列は足さない。

// externalCursorSort はカーソルの並び順の欄に入れる名前である。画面の一覧のカーソル
// （listing.go）と同じ包みを使い、この欄で画面の一覧のカーソルと取り違えないようにする。
const externalCursorSort = "external"

// ListExternalVideos は登録フォルダの下に所在を持つ動画を (added_at, id) の昇順で
// 1 ページ返す。ページングは画面の ListVideos と同じ keyset のカーソルで、途中で動画が
// 増える・消える・移動しても続きの要求は失敗せず、その時点の続きを返す。解釈できない
// カーソルは domain.ErrInvalidCursor を包む。
//
// ページの動画・所在・タグを同じ読み取りスナップショット（1 取引）から返す。
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
	// 次のページがあるかを知るために 1 件多く取る。
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

// LookupExternalVideo は ref が指す動画を 1 本返す。ID と ContentKey は動画を、Path は
// 登録フォルダの下の今の所在を、正規化せずバイト列の完全一致で引く。無い、または
// 登録フォルダの下に所在が無いときは domain.ErrNotFound。ref が ID・ContentKey・Path の
// ちょうど 1 つを持たないときも domain.ErrNotFound にする（形の検査は入口の仕事）。
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

// resolveVideoRef は ref を、登録フォルダの下に所在を持つ動画の id へ引き当てる。
// 引けなければ domain.ErrNotFound。
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
		// 所在そのものが登録フォルダの下にあることを求める。外部連携 API は登録外の所在を
		// 返さないので、一覧に出ないパスでは引けない。
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

// loadExternalVideos は ids の動画を、属性・登録フォルダの下の所在・タグを揃えて ids の
// 順に返す。登録フォルダの下に所在の無い動画と、行の無い id は落とす。
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

	contentKeys := make([]string, 0, len(byID))
	for _, video := range byID {
		// 内容を読めていない動画（空の content_key）はタグを持たない。
		if video.ContentKey != "" {
			contentKeys = append(contentKeys, video.ContentKey)
		}
	}
	tags, err := tagsByContentKeys(ctx, tx, contentKeys)
	if err != nil {
		return nil, err
	}

	for _, id := range ids {
		video, ok := byID[id]
		if !ok || len(locations[id]) == 0 {
			continue
		}
		item := domain.ExternalVideo{Video: video, Locations: locations[id], Tags: []domain.VideoTag{}}
		if video.ContentKey != "" && tags[video.ContentKey] != nil {
			item.Tags = tags[video.ContentKey]
		}
		out = append(out, item)
	}
	return out, nil
}

// externalVideoKey は一覧の並びの値である。
type externalVideoKey struct{ id, addedAt int64 }

// readExternalVideoKeys は一覧の 1 ページ分の (id, added_at) を並びの順に読む。
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

// readVideosByIDs は id の JSON 配列 encodedIDs のうち、登録フォルダの下に所在を持つ動画を
// id で引ける形で返す。代表の所在は登録フォルダの下のもの（所有者の見え方）。
func readVideosByIDs(ctx context.Context, tx *sql.Tx, encodedIDs string) (map[int64]domain.Video, error) {
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(domain.AudienceOwner)+` from videos
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

// readRegisteredLocations は id の JSON 配列 encodedIDs の動画の、登録フォルダの下の所在の
// パスを動画ごとにパスの順で返す。
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

// encodeExternalCursor は (added_at, id) を画面の一覧のカーソルと同じ包み
// （listing.go の encodeCursor・decodeCursor）で不透明な文字列にする。
func encodeExternalCursor(addedAt, id int64) string {
	fields := []string{externalCursorSort, "", "0", strconv.FormatInt(id, 10), strconv.FormatInt(addedAt, 10)}
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(fields, cursorSeparator)))
}

// decodeExternalCursor は encodeExternalCursor の包みを解く。解釈できないもの、画面の
// 一覧のカーソルは domain.ErrInvalidCursor を包む。
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
