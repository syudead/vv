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

// UpsertVideo は走査で分かった1件を索引に反映する。
//
// 突き合わせは content_key を先に見る。内容が同じ別pathは同じvideoの
// locationとして追加する。論理videoを重複させないことが要求であり、
// 再生位置とサムネイルを引き継ぐ前提でもある。
//
// 内容が変わったとき（サイズか mtime が変わる）は、解析結果を捨てて
// probe_state を pending へ戻す。
func (db *DB) UpsertVideo(ctx context.Context, file domain.VideoFile) (domain.UpsertResult, error) {
	now := time.Now().Unix()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.UpsertResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var locationID, oldVideoID, oldVersion, oldSize, oldMtime int64
	var oldKey string
	err = tx.QueryRowContext(ctx, `
		select l.id, l.video_id, l.version, l.size_bytes, l.mtime, v.content_key
		from video_locations l join videos v on v.id = l.video_id where l.path = ?`, file.Path).
		Scan(&locationID, &oldVideoID, &oldVersion, &oldSize, &oldMtime, &oldKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.UpsertResult{}, err
	}
	locationExists := err == nil
	if locationExists && oldKey == file.ContentKey && oldSize == file.SizeBytes && oldMtime == file.MTime.Unix() {
		return domain.UpsertResult{ID: oldVideoID, Outcome: domain.OutcomeUnchanged}, tx.Commit()
	}

	var videoID int64
	var probeState, thumbnailState, previewState string
	err = tx.QueryRowContext(ctx, `select id, probe_state, thumbnail_state, preview_state from videos where content_key = ?`, file.ContentKey).
		Scan(&videoID, &probeState, &thumbnailState, &previewState)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.UpsertResult{}, err
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
			return domain.UpsertResult{}, fmt.Errorf("動画を取り込めません (%s): %w", file.Path, err)
		}
		videoID, err = res.LastInsertId()
		if err != nil {
			return domain.UpsertResult{}, err
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
		return domain.UpsertResult{}, fmt.Errorf("動画の場所を保存できません (%s): %w", file.Path, err)
	}
	// 題名とパスが変わりうるので、照合用の鍵も同じ書き込みの中で作り直す。
	if err := refreshSearchKeysByPath(ctx, tx, file.Path); err != nil {
		return domain.UpsertResult{}, err
	}
	if !locationExists {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id = ?`, videoID); err != nil {
			return domain.UpsertResult{}, err
		}
	} else if oldVideoID != videoID {
		if _, err := tx.ExecContext(ctx, `update videos set location_generation = location_generation + 1 where id in (?, ?)`, oldVideoID, videoID); err != nil {
			return domain.UpsertResult{}, err
		}
	}
	if err := syncRepresentativeContainer(ctx, tx, videoID); err != nil {
		return domain.UpsertResult{}, err
	}
	var released []domain.DeletedVideo
	if locationExists && oldVideoID != videoID {
		if err := syncRepresentativeContainer(ctx, tx, oldVideoID); err != nil {
			return domain.UpsertResult{}, err
		}
		released, err = collectDeletedVideos(tx.QueryContext(ctx, `delete from videos where id = ? and not exists (select 1 from video_locations where video_id = ?)
			returning id, content_key`, oldVideoID, oldVideoID))
		if err != nil {
			return domain.UpsertResult{}, err
		}
	}
	// 内容が変わって前の動画が消えたら、前の内容の生成物を片付けさせる。
	var c changes
	c.videosDeleted(released)
	if err := db.commit(tx, &c); err != nil {
		return domain.UpsertResult{}, err
	}
	outcome := domain.OutcomeMoved
	if locationExists {
		outcome = domain.OutcomeUpdated
	} else if newVideo {
		outcome = domain.OutcomeAdded
	}
	return domain.UpsertResult{
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

// RequeueMissingPreview は、プレビューを作り終えた記録があるのにファイルが
// 無い動画を、1つの取引の中で作り直す状態へ戻し、プレビューのジョブを積む。
// ファイルの有無はファイルの事実なので、呼び出し側が確かめて呼ぶ。
//
// preview_state が done で、内容の識別子が今も同じときだけ変える。すでに戻って
// いる、内容が変わった、動画が消えた場合は何もせず false を返す。同じ動画を
// 何度見つけても、積むのは1回である。
func (db *DB) RequeueMissingPreview(ctx context.Context, id int64, contentKey string) (bool, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("プレビューの作り直しを開始できません (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `update videos set preview_state = 'pending', updated_at = ?
		where id = ? and content_key = ? and preview_state = 'done'`, now, id, contentKey)
	if err != nil {
		return false, fmt.Errorf("プレビューの状態を戻せません (id=%d): %w", id, err)
	}
	reset, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("プレビューの状態の更新件数を確認できません (id=%d): %w", id, err)
	}
	if reset == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `delete from jobs where kind = 'preview' and video_id = ? and state in ('done', 'failed')`,
		id); err != nil {
		return false, fmt.Errorf("古いプレビューのジョブを掃除できません (id=%d): %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, `insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
		values ('preview', ?, 'queued', 0, ?, ?)
		on conflict (kind, video_id) where state in ('queued', 'running') do nothing`,
		id, now, now); err != nil {
		return false, fmt.Errorf("プレビューのジョブを積めません (id=%d): %w", id, err)
	}
	var c changes
	c.jobsQueued(domain.JobPreview)
	if err := db.commit(tx, &c); err != nil {
		return false, fmt.Errorf("プレビューの作り直しを確定できません (id=%d): %w", id, err)
	}
	return true, nil
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
// （app.Ingest.Thumbnail は代表サムネイルがあればシーク用プレビューだけを作る）。
// 一覧用プレビューのジョブは、読み取りの成功後に app.Ingest.Probe が積む。
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
			return domain.ErrNotFound
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

	kinds := []domain.JobKind{domain.JobProbe}
	if thumbnailReset > 0 || seekThumbnailMissing {
		kinds = append(kinds, domain.JobThumbnail)
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
	var c changes
	c.jobsQueued(kinds...)
	if err := db.commit(tx, &c); err != nil {
		return fmt.Errorf("読み取りのやり直しを確定できません (id=%d): %w", id, err)
	}
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
		return domain.Video{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Video{}, fmt.Errorf("動画を読み出せません (id=%d): %w", id, err)
	}
	return video, nil
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
	released, err := collectDeletedVideos(db.sql.QueryContext(ctx,
		`delete from videos where id in (`+placeholders+`) returning id, content_key`, args...))
	if err != nil {
		return fmt.Errorf("動画を削除できません: %w", err)
	}
	var c changes
	c.videosDeleted(released)
	db.publish(&c)
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
	var c changes
	c.videosDeleted(released)
	return db.commit(tx, &c)
}

// IndexedVideosByPath は索引に入っているものをパスで引ける形で返す。
// 走査はこれと実際のファイルを突き合わせて差分を出す。
func (db *DB) IndexedVideosByPath(ctx context.Context) (map[string]domain.IndexedVideo, error) {
	rows, err := db.sql.QueryContext(ctx, `select v.id, l.id, l.version, l.path, v.content_key, l.size_bytes, l.mtime, v.probe_state, v.thumbnail_state, v.preview_state from video_locations l join videos v on v.id = l.video_id`)
	if err != nil {
		return nil, fmt.Errorf("索引を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]domain.IndexedVideo{}
	for rows.Next() {
		var path string
		var video domain.IndexedVideo
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
