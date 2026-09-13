package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 走査と保存の間でやり取りする値は internal/domain が持つ。ここでは別名を
// 置いて、store を使う側が domain を直接 import しなくても読めるようにする。
type (
	// VideoFile は走査で分かる事実である。
	VideoFile = domain.VideoFile
	// IndexedVideo は差分判定に要る最小限の値である。
	IndexedVideo = domain.IndexedVideo
	// UpsertOutcome は取り込み1件の結果である。
	UpsertOutcome = domain.UpsertOutcome
	// UpsertResult は取り込み1件の結果である。
	UpsertResult = domain.UpsertResult
	// VideoSort は一覧の並び順である。
	VideoSort = domain.VideoSort
	// VideoQuery は一覧の問い合わせ条件である。
	VideoQuery = domain.VideoQuery
	// VideoPage は一覧1ページ分である。
	VideoPage = domain.VideoPage
)

const (
	// OutcomeAdded は新しく取り込んだ。
	OutcomeAdded = domain.OutcomeAdded
	// OutcomeUpdated は既存の行の内容が変わった。
	OutcomeUpdated = domain.OutcomeUpdated
	// OutcomeMoved は内容が同じままパスだけが変わった（移動・改名）。
	OutcomeMoved = domain.OutcomeMoved
	// OutcomeUnchanged は何も変わらなかった。
	OutcomeUnchanged = domain.OutcomeUnchanged
	// SortAddedDesc は追加が新しい順（既定）。
	SortAddedDesc = domain.SortAddedDesc
	// SortTitleAsc は題名順。
	SortTitleAsc = domain.SortTitleAsc
	// DefaultLimit は limit が指定されなかったときの件数。
	DefaultLimit = domain.DefaultLimit
	// MaxLimit は1ページで返す上限。
	MaxLimit = domain.MaxLimit
)

// 対象が無い・カーソルが壊れているといった判断は、保存層の都合ではなく
// 呼び出し側が扱う種類の誤りなので internal/domain が持つ。
var (
	// ErrNotFound は対象の行が無いことを表す。
	ErrNotFound = domain.ErrNotFound
	// ErrInvalidCursor はカーソルが解釈できないことを表す。
	ErrInvalidCursor = domain.ErrInvalidCursor
)

// videoColumns は domain.Video を組み立てるのに要る列である。
// 並びは scanVideo と対応させる。
const videoColumns = `id, path, title, size_bytes, mtime, added_at, updated_at,
	content_key, duration_ms, width, height, container, video_codec, audio_codec,
	playable, unplayable_reason, probe_state, probe_error, thumbnail_state`

// UpsertVideo は走査で分かった1件を索引に反映する（R-107 / R-109）。
//
// 突き合わせは content_key を先に見る。内容が同じでパスだけが違うものは
// 移動・改名なので、行を作り直さずパスを更新する。重複を作らないことが
// FR-004 の要求であり、再生位置とサムネイルを引き継ぐ前提でもある。
//
// 内容が変わったとき（サイズか mtime が変わる）は、解析結果を捨てて
// probe_state を pending へ戻す（data-model.md）。
func (db *DB) UpsertVideo(ctx context.Context, file VideoFile) (UpsertResult, error) {
	now := time.Now().Unix()

	// 1. 内容が一致する行。パスが違えば移動・改名である。
	existing, err := db.videoByContentKey(ctx, file.ContentKey)
	switch {
	case err != nil && !errors.Is(err, ErrNotFound):
		return UpsertResult{}, err
	case err == nil:
		return db.updateExisting(ctx, existing, file, now)
	}

	// 2. パスが一致する行。内容が差し替わった場合はここに来る。
	existing, err = db.videoByPath(ctx, file.Path)
	switch {
	case err != nil && !errors.Is(err, ErrNotFound):
		return UpsertResult{}, err
	case err == nil:
		return db.updateExisting(ctx, existing, file, now)
	}

	// 3. 新規。解析はこれからなので、再生できない側に倒した状態で入れる。
	addedAt := file.AddedAt
	if addedAt.IsZero() {
		addedAt = time.Now()
	}
	res, err := db.sql.ExecContext(ctx, `
		insert into videos
			(path, title, size_bytes, mtime, added_at, updated_at, content_key, container,
			 playable, probe_state, thumbnail_state)
		values (?, ?, ?, ?, ?, ?, ?, ?, 0, 'pending', 'pending')`,
		file.Path, file.Title, file.SizeBytes, file.MTime.Unix(), addedAt.Unix(), now,
		file.ContentKey, nullableString(file.Container),
	)
	if err != nil {
		return UpsertResult{}, fmt.Errorf("動画を取り込めません (%s): %w", file.Path, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return UpsertResult{}, fmt.Errorf("動画を取り込めません (%s): %w", file.Path, err)
	}
	return UpsertResult{ID: id, Outcome: OutcomeAdded}, nil
}

// updateExisting は既存の行を実際のファイルに合わせる。
func (db *DB) updateExisting(
	ctx context.Context, existing IndexedVideo, file VideoFile, now int64,
) (UpsertResult, error) {
	contentChanged := existing.SizeBytes != file.SizeBytes ||
		!existing.MTime.Equal(file.MTime) ||
		existing.ContentKey != file.ContentKey

	pathRow, err := db.pathAndTitle(ctx, existing.ID)
	if err != nil {
		return UpsertResult{}, err
	}
	pathChanged := pathRow.path != file.Path || pathRow.title != file.Title

	if !contentChanged && !pathChanged {
		return UpsertResult{ID: existing.ID, Outcome: OutcomeUnchanged}, nil
	}

	// 内容が変わったら解析をやり直す。古い尺やコーデックを残すと、一覧が
	// 実体と食い違ったまま表示され続ける。
	query := `
		update videos
		   set path = ?, title = ?, size_bytes = ?, mtime = ?, content_key = ?,
		       container = ?, updated_at = ?`
	args := []any{
		file.Path, file.Title, file.SizeBytes, file.MTime.Unix(), file.ContentKey,
		nullableString(file.Container), now,
	}
	if contentChanged {
		query += `,
		       duration_ms = null, width = null, height = null,
		       video_codec = null, audio_codec = null,
		       playable = 0, unplayable_reason = null,
		       probe_state = 'pending', probe_error = null,
		       thumbnail_state = 'pending'`
	}
	query += ` where id = ?`
	args = append(args, existing.ID)

	if _, err := db.sql.ExecContext(ctx, query, args...); err != nil {
		return UpsertResult{}, fmt.Errorf("動画を更新できません (%s): %w", file.Path, err)
	}

	outcome := OutcomeUpdated
	if !contentChanged {
		outcome = OutcomeMoved
	}
	return UpsertResult{ID: existing.ID, Outcome: outcome}, nil
}

// videoByContentKey は内容が一致する行を返す。
func (db *DB) videoByContentKey(ctx context.Context, key string) (IndexedVideo, error) {
	return db.indexedVideo(ctx,
		`select id, content_key, size_bytes, mtime from videos where content_key = ?`, key)
}

// videoByPath はパスが一致する行を返す。
func (db *DB) videoByPath(ctx context.Context, path string) (IndexedVideo, error) {
	return db.indexedVideo(ctx,
		`select id, content_key, size_bytes, mtime from videos where path = ?`, path)
}

func (db *DB) indexedVideo(ctx context.Context, query string, arg any) (IndexedVideo, error) {
	var out IndexedVideo
	var mtime int64
	err := db.sql.QueryRowContext(ctx, query, arg).Scan(&out.ID, &out.ContentKey, &out.SizeBytes, &mtime)
	if errors.Is(err, sql.ErrNoRows) {
		return IndexedVideo{}, ErrNotFound
	}
	if err != nil {
		return IndexedVideo{}, fmt.Errorf("動画を読み出せません: %w", err)
	}
	out.MTime = time.Unix(mtime, 0)
	return out, nil
}

type pathTitle struct {
	path  string
	title string
}

func (db *DB) pathAndTitle(ctx context.Context, id int64) (pathTitle, error) {
	var out pathTitle
	err := db.sql.QueryRowContext(ctx, `select path, title from videos where id = ?`, id).
		Scan(&out.path, &out.title)
	if errors.Is(err, sql.ErrNoRows) {
		return pathTitle{}, ErrNotFound
	}
	if err != nil {
		return pathTitle{}, fmt.Errorf("動画を読み出せません: %w", err)
	}
	return out, nil
}

// ApplyProbe は解析の結果を反映する。再生可否の判定は domain が行い、
// ここはその結果を書き込むだけである。
//
// 取得できなかった値は null のままにする。0 で代用すると、一覧で
// 「尺が 0 の動画」と「尺が分からない動画」を区別できなくなる。
func (db *DB) ApplyProbe(
	ctx context.Context, id int64, probe domain.Probe, play domain.Playability,
) error {
	_, err := db.sql.ExecContext(ctx, `
		update videos
		   set duration_ms = ?, width = ?, height = ?,
		       video_codec = ?, audio_codec = ?,
		       playable = ?, unplayable_reason = ?,
		       probe_state = 'done', probe_error = null, updated_at = ?
		 where id = ?`,
		nullableInt64(probe.DurationMs), nullableInt(probe.Width), nullableInt(probe.Height),
		nullableString(probe.VideoCodec), nullableString(probe.AudioCodec),
		boolToInt(play.Playable), nullableString(string(play.Reason)),
		time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("解析の結果を反映できません (id=%d): %w", id, err)
	}
	return nil
}

// MarkProbeFailed は解析に失敗したことを記録する。行は残す。個別のファイルの
// 失敗で取り込み全体を止めないため、一覧には並んだままになる（FR-008）。
func (db *DB) MarkProbeFailed(ctx context.Context, id int64, reason string) error {
	_, err := db.sql.ExecContext(ctx, `
		update videos
		   set probe_state = 'failed', probe_error = ?, playable = 0, updated_at = ?
		 where id = ?`,
		reason, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("解析の失敗を記録できません (id=%d): %w", id, err)
	}
	return nil
}

// SetThumbnailState はサムネイル生成の状態を記録する。
func (db *DB) SetThumbnailState(ctx context.Context, id int64, state domain.ThumbnailState) error {
	_, err := db.sql.ExecContext(ctx,
		`update videos set thumbnail_state = ?, updated_at = ? where id = ?`,
		string(state), time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("サムネイルの状態を記録できません (id=%d): %w", id, err)
	}
	return nil
}

// GetVideo は1件を返す。
func (db *DB) GetVideo(ctx context.Context, id int64) (domain.Video, error) {
	//nolint:gosec // videoColumns は定数で、利用者の入力は混ざらない。
	row := db.sql.QueryRowContext(ctx, `select `+videoColumns+` from videos where id = ?`, id)

	video, err := scanVideo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Video{}, ErrNotFound
	}
	if err != nil {
		return domain.Video{}, fmt.Errorf("動画を読み出せません (id=%d): %w", id, err)
	}
	return video, nil
}

// ListVideos は一覧1ページを返す（R-109）。
//
// ページングは keyset（カーソル）方式である。offset を使うと、取り込みで行が
// 増減した瞬間に取りこぼしと重複が起きる。並び順の値と id を境界に使うので、
// 途中で行が動いても続きが安定して取れる。
func (db *DB) ListVideos(ctx context.Context, q VideoQuery) (VideoPage, error) {
	limit := normalizeLimit(q.Limit)
	sort := q.Sort
	if sort == "" {
		sort = SortAddedDesc
	}

	// 総件数はカーソルに関係なく、絞り込み後の全件である（FR-012）。
	total, err := db.CountVideos(ctx, q.Query)
	if err != nil {
		return VideoPage{}, err
	}

	search, args := searchFilter(q.Query)
	conditions := []string{}
	if search != "" {
		conditions = append(conditions, search)
	}

	if q.Cursor != "" {
		condition, cursorArgs, err := cursorCondition(sort, q.Cursor)
		if err != nil {
			return VideoPage{}, err
		}
		conditions = append(conditions, condition)
		args = append(args, cursorArgs...)
	}

	//nolint:gosec // 組み立てるのは列名と定型の条件句だけで、値はすべて引数で渡す。
	query := `select ` + videoColumns + ` from videos`
	if len(conditions) > 0 {
		query += ` where ` + strings.Join(conditions, " and ")
	}

	// 並び順は検索の有無で変えない。関連度（bm25）にすると、LIKE 経路には
	// 関連度が無いため2つの経路で並びが変わり、利用者から見て不可解になる
	// （R-110）。
	query += ` order by ` + orderBy(sort) + ` limit ?`

	// 次のページがあるかを知るために1件多く取る。件数を数え直すより安い。
	rows, err := db.sql.QueryContext(ctx, query, append(args, limit+1)...)
	if err != nil {
		return VideoPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	page := VideoPage{Total: total, Limit: limit, Items: []domain.Video{}}
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return VideoPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
		}
		page.Items = append(page.Items, video)
	}
	if err := rows.Err(); err != nil {
		return VideoPage{}, fmt.Errorf("一覧を読み出せません: %w", err)
	}

	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = encodeCursor(sort, page.Items[len(page.Items)-1])
	}
	return page, nil
}

// CountVideos は絞り込み後の総件数を返す（FR-012）。1万件規模の count(*) は
// 索引走査で数 ms に収まる。
func (db *DB) CountVideos(ctx context.Context, search string) (int, error) {
	condition, args := searchFilter(search)

	query := `select count(*) from videos`
	if condition != "" {
		query += ` where ` + condition
	}

	var total int
	//nolint:gosec // condition は組み立て済みの定型句で、値はすべて引数で渡す。
	if err := db.sql.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("件数を数えられません: %w", err)
	}
	return total, nil
}

// DeleteVideos は指定した行を消す。連鎖してジョブも消える。
// 空の指定で全件消さないよう、何も渡されなければ何もしない。
func (db *DB) DeleteVideos(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	//nolint:gosec // 組み立てるのはプレースホルダの数だけで、値は引数で渡す。
	if _, err := db.sql.ExecContext(ctx,
		`delete from videos where id in (`+placeholders+`)`, args...); err != nil {
		return fmt.Errorf("動画を削除できません: %w", err)
	}
	return nil
}

// IndexedVideosByPath は索引に入っているものをパスで引ける形で返す。
// 走査はこれと実際のファイルを突き合わせて差分を出す（R-107）。
func (db *DB) IndexedVideosByPath(ctx context.Context) (map[string]IndexedVideo, error) {
	rows, err := db.sql.QueryContext(ctx, `select id, path, content_key, size_bytes, mtime from videos`)
	if err != nil {
		return nil, fmt.Errorf("索引を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]IndexedVideo{}
	for rows.Next() {
		var path string
		var video IndexedVideo
		var mtime int64
		if err := rows.Scan(&video.ID, &path, &video.ContentKey, &video.SizeBytes, &mtime); err != nil {
			return nil, fmt.Errorf("索引を読み出せません: %w", err)
		}
		video.MTime = time.Unix(mtime, 0)
		out[path] = video
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("索引を読み出せません: %w", err)
	}
	return out, nil
}

// ContentKeys は参照されている内容の識別子を集合で返す。
// スキャン完了時の孤児サムネイルの掃除に使う（data-model.md 2 節）。
func (db *DB) ContentKeys(ctx context.Context) (map[string]struct{}, error) {
	rows, err := db.sql.QueryContext(ctx, `select content_key from videos`)
	if err != nil {
		return nil, fmt.Errorf("識別子を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]struct{}{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("識別子を読み出せません: %w", err)
		}
		out[key] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("識別子を読み出せません: %w", err)
	}
	return out, nil
}

// orderBy は並び順の SQL 句を返す。索引（videos_added_at_desc_idx /
// videos_title_asc_idx）と同じ並びにする。
func orderBy(sort VideoSort) string {
	if sort == SortTitleAsc {
		return `title asc, id asc`
	}
	return `added_at desc, id desc`
}

// cursorCondition はカーソルより後ろだけを取る条件を返す。
// 並び順の値が同じ行は id で決着させる。
func cursorCondition(sort VideoSort, cursor string) (string, []any, error) {
	value, id, err := decodeCursor(cursor)
	if err != nil {
		return "", nil, err
	}

	if sort == SortTitleAsc {
		return `(title, id) > (?, ?)`, []any{value, id}, nil
	}

	addedAt, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return "", nil, fmt.Errorf("%w: 追加時刻として解釈できません", ErrInvalidCursor)
	}
	return `(added_at, id) < (?, ?)`, []any{addedAt, id}, nil
}

// cursorSeparator はカーソルの中で並び順の値と id を区切る。題名に現れない
// 制御文字を選ぶ。
const cursorSeparator = "\x1f"

// encodeCursor は「並び順の値 + id」を不透明な文字列に包む。クライアントは
// 中身を解釈しない（R-109）。
func encodeCursor(sort VideoSort, last domain.Video) string {
	value := strconv.FormatInt(last.AddedAt.Unix(), 10)
	if sort == SortTitleAsc {
		value = last.Title
	}
	return base64.RawURLEncoding.EncodeToString(
		[]byte(value + cursorSeparator + strconv.FormatInt(last.ID, 10)))
}

// decodeCursor は包みを解く。解釈できないものは誤りとして返す。
func decodeCursor(cursor string) (value string, id int64, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}

	parts := strings.SplitN(string(raw), cursorSeparator, 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("%w: 区切りがありません", ErrInvalidCursor)
	}

	id, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("%w: 識別子として解釈できません", ErrInvalidCursor)
	}
	return parts[0], id, nil
}

// normalizeLimit は件数を既定値と上限に丸める。
func normalizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultLimit
	case limit > MaxLimit:
		return MaxLimit
	default:
		return limit
	}
}

// rowScanner は *sql.Row と *sql.Rows の共通部分である。
type rowScanner interface {
	Scan(dest ...any) error
}

// scanVideo は1行を domain.Video へ写す。列の並びは videoColumns と対応する。
func scanVideo(row rowScanner) (domain.Video, error) {
	var (
		video                             domain.Video
		mtime, addedAt, updatedAt         int64
		durationMs                        sql.NullInt64
		width, height                     sql.NullInt64
		container, videoCodec, audioCodec sql.NullString
		unplayableReason, probeError      sql.NullString
		playable                          int
		probeState, thumbnailState        string
	)

	err := row.Scan(
		&video.ID, &video.Path, &video.Title, &video.SizeBytes, &mtime, &addedAt, &updatedAt,
		&video.ContentKey, &durationMs, &width, &height, &container, &videoCodec, &audioCodec,
		&playable, &unplayableReason, &probeState, &probeError, &thumbnailState,
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
