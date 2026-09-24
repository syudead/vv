package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"runtime"
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
	// OutcomeMoved は既知の内容を新しいpathで発見した。
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
const videoColumnsTemplate = `videos.id,
	(select path from video_locations l where video_id = videos.id and {registered} order by path limit 1) as path,
	(select title from video_locations l where video_id = videos.id and {registered} order by path limit 1) as title,
	(select size_bytes from video_locations l where video_id = videos.id and {registered} order by path limit 1) as size_bytes,
	(select mtime from video_locations l where video_id = videos.id and {registered} order by path limit 1) as mtime,
	videos.added_at, videos.updated_at, videos.content_key, videos.duration_ms, videos.width,
	videos.height, videos.container, videos.video_codec, videos.audio_codec, videos.playable,
	videos.unplayable_reason, videos.probe_state, videos.probe_error, videos.thumbnail_state, videos.preview_state`

func registeredLocationCondition(alias string) string {
	separator := strconv.Itoa(int(os.PathSeparator))
	pathExpr := alias + `.path`
	rootExpr := `mf.path`
	if runtime.GOOS == "windows" {
		pathExpr = `lower(` + pathExpr + `)`
		rootExpr = `lower(` + rootExpr + `)`
	}
	trimmedRoot := `rtrim(` + rootExpr + `, char(47) || char(92))`
	return `exists (select 1 from media_folders mf where ` + pathExpr + ` = ` + rootExpr +
		` or instr(` + pathExpr + `, ` + trimmedRoot + ` || char(` + separator + `)) = 1` +
		` or instr(` + pathExpr + `, ` + trimmedRoot + ` || char(47)) = 1` +
		` or instr(` + pathExpr + `, ` + trimmedRoot + ` || char(92)) = 1)`
}

func registeredVideoCondition(alias string) string {
	return `exists (select 1 from video_locations l where l.video_id = ` + alias + `.id and ` +
		registeredLocationCondition("l") + `)`
}

func videoColumns() string {
	return strings.ReplaceAll(videoColumnsTemplate, "{registered}", registeredLocationCondition("l"))
}

// UpsertVideo は走査で分かった1件を索引に反映する。
//
// 突き合わせは content_key を先に見る。内容が同じ別pathは同じvideoの
// locationとして追加する。論理videoを重複させないことが要求であり、
// 再生位置とサムネイルを引き継ぐ前提でもある。
//
// 内容が変わったとき（サイズか mtime が変わる）は、解析結果を捨てて
// probe_state を pending へ戻す。
func (db *DB) UpsertVideo(ctx context.Context, file VideoFile) (UpsertResult, error) {
	now := time.Now().Unix()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return UpsertResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var locationID, oldVideoID, oldVersion, oldSize, oldMtime int64
	var oldKey string
	err = tx.QueryRowContext(ctx, `
		select l.id, l.video_id, l.version, l.size_bytes, l.mtime, v.content_key
		from video_locations l join videos v on v.id = l.video_id where l.path = ?`, file.Path).
		Scan(&locationID, &oldVideoID, &oldVersion, &oldSize, &oldMtime, &oldKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return UpsertResult{}, err
	}
	locationExists := err == nil
	if locationExists && oldKey == file.ContentKey && oldSize == file.SizeBytes && oldMtime == file.MTime.Unix() {
		return UpsertResult{ID: oldVideoID, Outcome: OutcomeUnchanged}, tx.Commit()
	}

	var videoID int64
	var probeState, thumbnailState, previewState string
	err = tx.QueryRowContext(ctx, `select id, probe_state, thumbnail_state, preview_state from videos where content_key = ?`, file.ContentKey).
		Scan(&videoID, &probeState, &thumbnailState, &previewState)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return UpsertResult{}, err
	}
	newVideo := errors.Is(err, sql.ErrNoRows)
	addedAt := file.AddedAt
	if addedAt.IsZero() {
		addedAt = time.Now()
	}
	if newVideo {
		probeState = string(domain.ProbeStatePending)
		thumbnailState = string(domain.ThumbnailStatePending)
		previewState = string(domain.PreviewStatePending)
		res, err := tx.ExecContext(ctx, `
		insert into videos
			(added_at, updated_at, content_key, container, playable, probe_state, thumbnail_state)
		values (?, ?, ?, ?, 0, 'pending', 'pending')`,
			addedAt.Unix(), now, file.ContentKey, nullableString(file.Container))
		if err != nil {
			return UpsertResult{}, fmt.Errorf("動画を取り込めません (%s): %w", file.Path, err)
		}
		videoID, err = res.LastInsertId()
		if err != nil {
			return UpsertResult{}, err
		}
	}

	if locationExists {
		_, err = tx.ExecContext(ctx, `update video_locations set video_id = ?, version = version + 1, title = ?, size_bytes = ?, mtime = ?, updated_at = ? where id = ?`,
			videoID, file.Title, file.SizeBytes, file.MTime.Unix(), now, locationID)
	} else {
		_, err = tx.ExecContext(ctx, `insert into video_locations(video_id, path, version, title, size_bytes, mtime, created_at, updated_at) values (?, ?, 1, ?, ?, ?, ?, ?)`,
			videoID, file.Path, file.Title, file.SizeBytes, file.MTime.Unix(), now, now)
	}
	if err != nil {
		return UpsertResult{}, fmt.Errorf("動画の場所を保存できません (%s): %w", file.Path, err)
	}
	if !locationExists {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id = ?`, videoID); err != nil {
			return UpsertResult{}, err
		}
	} else if oldVideoID != videoID {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id in (?, ?)`, oldVideoID, videoID); err != nil {
			return UpsertResult{}, err
		}
	}
	if err := syncRepresentativeContainer(ctx, tx, videoID); err != nil {
		return UpsertResult{}, err
	}
	var released []string
	if locationExists && oldVideoID != videoID {
		if err := syncRepresentativeContainer(ctx, tx, oldVideoID); err != nil {
			return UpsertResult{}, err
		}
		released, err = collectContentKeys(tx.QueryContext(ctx, `delete from videos where id = ? and not exists (select 1 from video_locations where video_id = ?)
			returning content_key`, oldVideoID, oldVideoID))
		if err != nil {
			return UpsertResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return UpsertResult{}, err
	}
	// 内容が変わって前の動画が消えたら、前の内容の生成物を片付けさせる。
	db.notifyContentReleased(released)
	outcome := OutcomeMoved
	if locationExists {
		outcome = OutcomeUpdated
	} else if newVideo {
		outcome = OutcomeAdded
	}
	return UpsertResult{
		ID: videoID, Outcome: outcome,
		NeedsProbe:     probeState != string(domain.ProbeStateDone),
		NeedsThumbnail: thumbnailState != string(domain.ThumbnailStateDone),
		NeedsPreview:   probeState == string(domain.ProbeStateDone) && previewState != string(domain.PreviewStateDone),
	}, nil
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

// ApplyProbeForJob writes only while the file identity captured at claim time is current.
func (db *DB) ApplyProbeForJob(
	ctx context.Context, job domain.Job, probe domain.Probe, play domain.Playability,
) (bool, error) {
	res, err := db.sql.ExecContext(ctx, `
		update videos set duration_ms = ?, width = ?, height = ?, video_codec = ?, audio_codec = ?,
		playable = ?, unplayable_reason = ?, probe_state = 'done', probe_error = null, updated_at = ?
		where id = ? and content_key = ? and exists (
			select 1 from video_locations where video_id = videos.id and id = ? and version = ? and path = ?)
		and location_generation = ?`,
		nullableInt64(probe.DurationMs), nullableInt(probe.Width), nullableInt(probe.Height),
		nullableString(probe.VideoCodec), nullableString(probe.AudioCodec), boolToInt(play.Playable),
		nullableString(string(play.Reason)), time.Now().Unix(), job.VideoID, job.ContentKey,
		job.LocationID, job.LocationVersion, job.LocationPath, job.LocationGeneration)
	if err != nil {
		return false, err
	}
	count, err := res.RowsAffected()
	return count == 1, err
}

// MarkProbeFailed は解析に失敗したことを記録する。行は残す。個別のファイルの
// 失敗で取り込み全体を止めないため、一覧には並んだままになる。
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

func (db *DB) SetThumbnailStateForJob(ctx context.Context, job domain.Job, state domain.ThumbnailState) (bool, error) {
	res, err := db.sql.ExecContext(ctx, `update videos set thumbnail_state = ?, updated_at = ?
		where id = ? and content_key = ? and exists (
			select 1 from video_locations where video_id = videos.id and id = ? and version = ? and path = ?)
		and location_generation = ?`,
		string(state), time.Now().Unix(), job.VideoID, job.ContentKey, job.LocationID,
		job.LocationVersion, job.LocationPath, job.LocationGeneration)
	if err != nil {
		return false, err
	}
	count, err := res.RowsAffected()
	return count == 1, err
}

// SetPreviewStateForJob applies only to the content and location generation
// captured when the preview job was claimed.
func (db *DB) SetPreviewStateForJob(ctx context.Context, job domain.Job, state domain.PreviewState) (bool, error) {
	res, err := db.sql.ExecContext(ctx, `update videos set preview_state = ?, updated_at = ?
		where id = ? and content_key = ? and exists (
			select 1 from video_locations where video_id = videos.id and id = ? and version = ? and path = ?)
		and location_generation = ?`, string(state), time.Now().Unix(), job.VideoID, job.ContentKey,
		job.LocationID, job.LocationVersion, job.LocationPath, job.LocationGeneration)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// SetPreviewStateForContent accepts a completed asset after a representative
// location changed, while still refusing a stale content identity.
func (db *DB) SetPreviewStateForContent(ctx context.Context, job domain.Job, state domain.PreviewState) (bool, error) {
	res, err := db.sql.ExecContext(ctx, `update videos set preview_state = ?, updated_at = ?
		where id = ? and content_key = ?`, string(state), time.Now().Unix(), job.VideoID, job.ContentKey)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// CompletePreviewForContent atomically marks the content-keyed asset and its
// claimed job complete. Location-only changes do not invalidate the asset.
func (db *DB) CompletePreviewForContent(ctx context.Context, job domain.Job) (bool, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `update videos set preview_state = 'done', updated_at = ?
		where id = ? and content_key = ?`, now, job.VideoID, job.ContentKey)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	res, err = tx.ExecContext(ctx, `update jobs set state = 'done', last_error = null, updated_at = ?
		where id = ? and kind = 'preview' and video_id = ? and state = 'running'`, now, job.ID, job.VideoID)
	if err != nil {
		return false, err
	}
	n, err = res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n != 1 {
		return false, fmt.Errorf("preview job is not running (id=%d)", job.ID)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) ContentKeyCurrent(ctx context.Context, videoID int64, key string) (bool, error) {
	var current int
	err := db.sql.QueryRowContext(ctx, `select exists(select 1 from videos where id = ? and content_key = ?)`, videoID, key).Scan(&current)
	return current != 0, err
}

// PreviewSourceCurrent accepts location-only churn while rejecting a claimed
// source path that was reassigned to different content during generation.
func (db *DB) PreviewSourceCurrent(ctx context.Context, job domain.Job) (bool, error) {
	var current int
	err := db.sql.QueryRowContext(ctx, `select exists (
		select 1 from videos where id = ? and content_key = ?
	) and not exists (
		select 1 from video_locations l join videos v on v.id = l.video_id
		where l.path = ? and v.content_key <> ?
	)`, job.VideoID, job.ContentKey, job.LocationPath, job.ContentKey).Scan(&current)
	return current != 0, err
}

func (db *DB) SetPreviewState(ctx context.Context, id int64, state domain.PreviewState) error {
	_, err := db.sql.ExecContext(ctx, `update videos set preview_state = ?, updated_at = ? where id = ?`, string(state), time.Now().Unix(), id)
	return err
}

// RetryProbe は読み取りに失敗した動画を、1つの取引の中で読み取り直す状態へ
// 戻し、スキャンが新しい内容に積むのと同じジョブを積む。
//
// probe_state が failed でなければ何も変えず domain.ErrProbeNotFailed を返す。
// 連打や別タブからの二度目はこれになり、ジョブは重複しない。failed は、その
// 動画の読み取りのジョブが終わっていることを意味する（FailClaimedJob が同じ
// 取引で記録する）ので、ここで積むジョブが running の古いジョブとの重複防止で
// 省かれることは無い。
//
// seekThumbnailMissing はシーク用プレビューの置き場が無いことを表す。置き場の
// 有無はファイルの事実なので、呼び出し側が確かめて渡す。thumbnail_state が
// done でも置き場が無ければ、状態はそのままでサムネイルのジョブを積む
// （thumbnailHandler は代表サムネイルがあればシーク用プレビューだけを作る）。
// 一覧用プレビューのジョブは、読み取りの成功後に probeHandler が積む。
func (db *DB) RetryProbe(ctx context.Context, id int64, seekThumbnailMissing bool) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("読み取りのやり直しを開始できません (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `update videos set probe_state = 'pending', probe_error = null, updated_at = ?
		where id = ? and probe_state = 'failed'`, now, id)
	if err != nil {
		return fmt.Errorf("読み取りの状態を戻せません (id=%d): %w", id, err)
	}
	reset, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("読み取りの状態の更新件数を確認できません (id=%d): %w", id, err)
	}
	if reset == 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from videos where id = ?)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("動画の有無を確かめられません (id=%d): %w", id, err)
		}
		if exists == 0 {
			return ErrNotFound
		}
		return domain.ErrProbeNotFailed
	}

	res, err = tx.ExecContext(ctx, `update videos set thumbnail_state = 'pending', updated_at = ?
		where id = ? and thumbnail_state <> 'done'`, now, id)
	if err != nil {
		return fmt.Errorf("サムネイルの状態を戻せません (id=%d): %w", id, err)
	}
	thumbnailReset, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("サムネイルの状態の更新件数を確認できません (id=%d): %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, `update videos set preview_state = 'pending', updated_at = ?
		where id = ? and preview_state = 'failed'`, now, id); err != nil {
		return fmt.Errorf("プレビューの状態を戻せません (id=%d): %w", id, err)
	}

	kinds := []JobKind{JobProbe}
	if thumbnailReset > 0 || seekThumbnailMissing {
		kinds = append(kinds, JobThumbnail)
	}
	for _, kind := range kinds {
		if _, err := tx.ExecContext(ctx, `delete from jobs where kind = ? and video_id = ? and state in ('done', 'failed')`,
			string(kind), id); err != nil {
			return fmt.Errorf("古いジョブを掃除できません (%s, video=%d): %w", kind, id, err)
		}
		if _, err := tx.ExecContext(ctx, `insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
			values (?, ?, 'queued', 0, ?, ?)
			on conflict (kind, video_id) where state in ('queued', 'running') do nothing`,
			string(kind), id, now, now); err != nil {
			return fmt.Errorf("ジョブを積めません (%s, video=%d): %w", kind, id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("読み取りのやり直しを確定できません (id=%d): %w", id, err)
	}
	db.notifyJobsQueued(kinds...)
	return nil
}

func (db *DB) VideoLocations(ctx context.Context, videoID int64) ([]domain.VideoLocation, error) {
	rows, err := db.sql.QueryContext(ctx, `select id, video_id, path, version, title, size_bytes, mtime, created_at, updated_at
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

// GetVideo は1件を返す。
func (db *DB) GetVideo(ctx context.Context, id int64) (domain.Video, error) {
	//nolint:gosec // videoColumns は定数で、利用者の入力は混ざらない。
	row := db.sql.QueryRowContext(ctx, `select `+videoColumns()+` from videos where videos.id = ? and `+registeredVideoCondition("videos"), id)

	video, err := scanVideo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Video{}, ErrNotFound
	}
	if err != nil {
		return domain.Video{}, fmt.Errorf("動画を読み出せません (id=%d): %w", id, err)
	}
	return video, nil
}

// ListVideos は一覧1ページを返す。
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

	// 総件数はカーソルに関係なく、絞り込み後の全件である。
	total, err := db.CountVideos(ctx, q.Query)
	if err != nil {
		return VideoPage{}, err
	}

	search, args := searchFilter(q.Query)
	conditions := []string{registeredVideoCondition("videos")}
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
	query := `select * from (select ` + videoColumns() + ` from videos) as videos`
	if len(conditions) > 0 {
		query += ` where ` + strings.Join(conditions, " and ")
	}

	// 並び順は検索の有無で変えない。関連度（bm25）にすると、LIKE 経路には
	// 関連度が無いため2つの経路で並びが変わり、利用者から見て不可解になる。
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

// CountVideos は絞り込み後の総件数を返す。1万件規模の count(*) は
// 索引走査で数 ms に収まる。
func (db *DB) CountVideos(ctx context.Context, search string) (int, error) {
	condition, args := searchFilter(search)
	availability := registeredVideoCondition("videos")

	query := `select count(*) from videos where ` + availability
	if condition != "" {
		query += ` and ` + condition
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
	released, err := collectContentKeys(db.sql.QueryContext(ctx,
		`delete from videos where id in (`+placeholders+`) returning content_key`, args...))
	if err != nil {
		return fmt.Errorf("動画を削除できません: %w", err)
	}
	db.notifyContentReleased(released)
	return nil
}

// DeleteVideoLocations removes filesystem facts and then only orphaned logical videos.
func (db *DB) DeleteVideoLocations(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	affected := map[int64]struct{}{}
	for _, id := range ids {
		var videoID int64
		if err := tx.QueryRowContext(ctx, `select video_id from video_locations where id = ?`, id).Scan(&videoID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		} else if err == nil {
			affected[videoID] = struct{}{}
		}
		if _, err := tx.ExecContext(ctx, `delete from video_locations where id = ?`, id); err != nil {
			return err
		}
	}
	for videoID := range affected {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id = ?`, videoID); err != nil {
			return err
		}
		if err := syncRepresentativeContainer(ctx, tx, videoID); err != nil {
			return err
		}
	}
	released, err := deleteOrphanVideos(ctx, tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	db.notifyContentReleased(released)
	return nil
}

// IndexedVideosByPath は索引に入っているものをパスで引ける形で返す。
// 走査はこれと実際のファイルを突き合わせて差分を出す。
func (db *DB) IndexedVideosByPath(ctx context.Context) (map[string]IndexedVideo, error) {
	rows, err := db.sql.QueryContext(ctx, `select v.id, l.id, l.version, l.path, v.content_key, l.size_bytes, l.mtime, v.probe_state, v.thumbnail_state, v.preview_state from video_locations l join videos v on v.id = l.video_id`)
	if err != nil {
		return nil, fmt.Errorf("索引を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]IndexedVideo{}
	for rows.Next() {
		var path string
		var video IndexedVideo
		var mtime int64
		if err := rows.Scan(&video.ID, &video.LocationID, &video.LocationVersion, &path, &video.ContentKey, &video.SizeBytes, &mtime, &video.ProbeState, &video.ThumbnailState, &video.PreviewState); err != nil {
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
// 動画が消えた場合に、書き終えた生成物を残さないために使う。
func (db *DB) ContentKeyReferenced(ctx context.Context, key string) (bool, error) {
	var referenced int
	if err := db.sql.QueryRowContext(ctx, `select exists(select 1 from videos where content_key = ?)`, key).
		Scan(&referenced); err != nil {
		return false, fmt.Errorf("識別子の参照を確かめられません: %w", err)
	}
	return referenced == 1, nil
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
// 中身を解釈しない。
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
		return "", 0, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
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
