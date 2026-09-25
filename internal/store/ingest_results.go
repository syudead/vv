package store

// 取り込みの段階の結果を動画の行へ反映する操作と、作り直しの予約。IngestStore が受け持つ。

import (
	"context"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ApplyProbe は解析の結果を反映する。再生可否の判定は domain が行い、
// ここはその結果を書き込むだけである。
//
// 取得できなかった値は null のままにする。0 で代用すると、一覧で
// 「尺が 0 の動画」と「尺が分からない動画」を区別できなくなる。
func (s *IngestStore) ApplyProbe(
	ctx context.Context, id int64, probe domain.Probe, play domain.Playability,
) error {
	_, err := s.db.sql.ExecContext(ctx, `
		update videos
		   set duration_ms = ?, width = ?, height = ?, display_aspect_ratio = ?,
		       video_codec = ?, audio_codec = ?,
		       playable = ?, unplayable_reason = ?,
		       probe_state = 'done', probe_error = null, updated_at = ?
		 where id = ?`,
		nullableInt64(probe.DurationMs), nullableInt(probe.Width), nullableInt(probe.Height), nullableFloat64(probe.DisplayAspectRatio),
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
func (s *IngestStore) ApplyProbeForJob(
	ctx context.Context, job domain.Job, probe domain.Probe, play domain.Playability,
) (bool, error) {
	res, err := s.db.sql.ExecContext(ctx, `
		update videos set duration_ms = ?, width = ?, height = ?, display_aspect_ratio = ?, video_codec = ?, audio_codec = ?,
		playable = ?, unplayable_reason = ?, probe_state = 'done', probe_error = null, updated_at = ?
		where id = ? and content_key = ? and exists (
			select 1 from video_locations where video_id = videos.id and id = ? and version = ? and path = ?)
		and location_generation = ?`,
		nullableInt64(probe.DurationMs), nullableInt(probe.Width), nullableInt(probe.Height), nullableFloat64(probe.DisplayAspectRatio),
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
func (s *IngestStore) MarkProbeFailed(ctx context.Context, id int64, reason string) error {
	_, err := s.db.sql.ExecContext(ctx, `
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
func (s *IngestStore) SetThumbnailState(ctx context.Context, id int64, state domain.ThumbnailState) error {
	_, err := s.db.sql.ExecContext(ctx,
		`update videos set thumbnail_state = ?, updated_at = ? where id = ?`,
		string(state), time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("サムネイルの状態を記録できません (id=%d): %w", id, err)
	}
	return nil
}

func (s *IngestStore) SetThumbnailStateForJob(ctx context.Context, job domain.Job, state domain.ThumbnailState) (bool, error) {
	res, err := s.db.sql.ExecContext(ctx, `update videos set thumbnail_state = ?, updated_at = ?
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
func (s *IngestStore) SetPreviewStateForJob(ctx context.Context, job domain.Job, state domain.PreviewState) (bool, error) {
	res, err := s.db.sql.ExecContext(ctx, `update videos set preview_state = ?, updated_at = ?
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
func (s *IngestStore) SetPreviewStateForContent(ctx context.Context, job domain.Job, state domain.PreviewState) (bool, error) {
	res, err := s.db.sql.ExecContext(ctx, `update videos set preview_state = ?, updated_at = ?
		where id = ? and content_key = ?`, string(state), time.Now().Unix(), job.VideoID, job.ContentKey)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// CompletePreviewForContent atomically marks the content-keyed asset and its
// claimed job complete. Location-only changes do not invalidate the asset.
func (s *IngestStore) CompletePreviewForContent(ctx context.Context, job domain.Job) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
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

func (s *IngestStore) ContentKeyCurrent(ctx context.Context, videoID int64, key string) (bool, error) {
	var current int
	err := s.db.sql.QueryRowContext(ctx, `select exists(select 1 from videos where id = ? and content_key = ?)`, videoID, key).Scan(&current)
	return current != 0, err
}

// PreviewSourceCurrent accepts location-only churn while rejecting a claimed
// source path that was reassigned to different content during generation.
func (s *IngestStore) PreviewSourceCurrent(ctx context.Context, job domain.Job) (bool, error) {
	var current int
	err := s.db.sql.QueryRowContext(ctx, `select exists (
		select 1 from videos where id = ? and content_key = ?
	) and not exists (
		select 1 from video_locations l join videos v on v.id = l.video_id
		where l.path = ? and v.content_key <> ?
	)`, job.VideoID, job.ContentKey, job.LocationPath, job.ContentKey).Scan(&current)
	return current != 0, err
}

func (s *IngestStore) SetPreviewState(ctx context.Context, id int64, state domain.PreviewState) error {
	_, err := s.db.sql.ExecContext(ctx, `update videos set preview_state = ?, updated_at = ? where id = ?`, string(state), time.Now().Unix(), id)
	return err
}

// RequeueMissingPreview は、プレビューを作り終えた記録があるのにファイルが
// 無い動画を、1つの取引の中で作り直す状態へ戻し、プレビューのジョブを積む。
// ファイルの有無はファイルの事実なので、呼び出し側が確かめて呼ぶ。
//
// preview_state が done で、内容の識別子が今も同じときだけ変える。すでに戻って
// いる、内容が変わった、動画が消えた場合は何もせず false を返す。同じ動画を
// 何度見つけても、積むのは1回である。
func (s *IngestStore) RequeueMissingPreview(ctx context.Context, id int64, contentKey string) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
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
	if err := s.db.commit(tx, &c); err != nil {
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
func (s *IngestStore) RetryProbe(ctx context.Context, id int64, seekThumbnailMissing bool) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
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
	if err := s.db.commit(tx, &c); err != nil {
		return fmt.Errorf("読み取りのやり直しを確定できません (id=%d): %w", id, err)
	}
	return nil
}
