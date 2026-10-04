package store

// 取り込みの段階の結果を動画の行へ反映する操作と、作り直しの予約。IngestStore が受け持つ。

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ApplyProbe は解析の結果を反映する。再生可否の判定は domain が行い、
// ここはその結果を書き込むだけである。ライブ変換用の解析情報（probe.Transcode）が
// あれば、同じ取引で video_transcode_probes へ upsert する。
//
// 取得できなかった値は null のままにする。0 で代用すると、一覧で
// 「尺が 0 の動画」と「尺が分からない動画」を区別できなくなる。
func (s *IngestStore) ApplyProbe(
	ctx context.Context, id int64, probe domain.Probe, play domain.Playability,
) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot apply the probe result (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	res, err := tx.ExecContext(ctx, `
		update videos
		   set duration_ms = ?, width = ?, height = ?, display_aspect_ratio = ?,
		       video_codec = ?, audio_codec = ?,
		       playable = ?, unplayable_reason = ?,
		       probe_state = 'done', probe_error = null, probe_error_code = null, indexed_at = ?
		 where id = ?`,
		nullableInt64(probe.DurationMs), nullableInt(probe.Width), nullableInt(probe.Height), nullableFloat64(probe.DisplayAspectRatio),
		nullableString(probe.VideoCodec), nullableString(probe.AudioCodec),
		boolToInt(play.Playable), nullableString(string(play.Reason)),
		now.Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("cannot apply the probe result (id=%d): %w", id, err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot apply the probe result (id=%d): %w", id, err)
	}
	var succeeded []int64
	if count == 1 {
		if probe.Transcode != nil {
			if err := upsertTranscodeProbe(ctx, tx, id, probe.Source, *probe.Transcode, now); err != nil {
				return err
			}
		}
		var contentKey string
		if err := tx.QueryRowContext(ctx, `select content_key from videos where id = ?`, id).Scan(&contentKey); err != nil {
			return fmt.Errorf("cannot apply the probe result (id=%d): %w", id, err)
		}
		if succeeded, err = applySuccession(ctx, tx, contentKey, probe.DurationMs); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot apply the probe result (id=%d): %w", id, err)
	}
	s.db.publishEvents(successionEvents(succeeded)...)
	return nil
}

// ApplyProbeForJob writes only while the file identity captured at claim time is current.
// The live-transcode probe is upserted in the same transaction, only when the video row was written.
//
// 結果を書いたら、同じ取引で一覧用プレビューの仕事を積む。解析の仕事はこの時点で
// まだ running なので、その動画が一瞬だけ「済み」に数えられることが無い
// （specs/024-import-progress/research.md R-2）。
func (s *IngestStore) ApplyProbeForJob(
	ctx context.Context, job domain.Job, probe domain.Probe, play domain.Playability,
) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	res, err := tx.ExecContext(ctx, `
		update videos set duration_ms = ?, width = ?, height = ?, display_aspect_ratio = ?, video_codec = ?, audio_codec = ?,
		playable = ?, unplayable_reason = ?, probe_state = 'done', probe_error = null, probe_error_code = null, indexed_at = ?
		where id = ? and content_key = ? and exists (
			select 1 from video_locations where video_id = videos.id and id = ? and version = ? and path = ?)
		and location_generation = ?`,
		nullableInt64(probe.DurationMs), nullableInt(probe.Width), nullableInt(probe.Height), nullableFloat64(probe.DisplayAspectRatio),
		nullableString(probe.VideoCodec), nullableString(probe.AudioCodec), boolToInt(play.Playable),
		nullableString(string(play.Reason)), now.Unix(), job.VideoID, job.ContentKey,
		job.LocationID, job.LocationVersion, job.LocationPath, job.LocationGeneration)
	if err != nil {
		return false, err
	}
	count, err := res.RowsAffected()
	if err != nil || count != 1 {
		return false, err
	}
	if probe.Transcode != nil {
		if err := upsertTranscodeProbe(ctx, tx, job.VideoID, probe.Source, *probe.Transcode, now); err != nil {
			return false, err
		}
	}
	if err := clearFailedIssue(ctx, tx, domain.JobProbe, job.VideoID); err != nil {
		return false, err
	}
	if err := requeueJob(ctx, tx, domain.JobPreview, job.VideoID, now.Unix()); err != nil {
		return false, err
	}
	// 走査が閉じたあとに解析が終わったなら、同じパスの前の中身を引き継ぐかをここで決める
	// （specs/030-video-versions/data-model.md §5）。
	succeeded, err := applySuccession(ctx, tx, job.ContentKey, probe.DurationMs)
	if err != nil {
		return false, err
	}
	var c changes
	c.jobsQueued(domain.JobPreview)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return false, err
	}
	s.db.publishEvents(successionEvents(succeeded)...)
	return true, nil
}

// SaveTranscodeProbe は、ライブ変換の要求時にその場で解析した結果を保存する。
// source は変換で開いたファイルの大きさと更新時刻である。upsert 1 文なので、取り込みと
// 変換が同時に書いても後に書いた行が残る（specs/018-live-transcode-seek/data-model.md §4）。
func (s *IngestStore) SaveTranscodeProbe(
	ctx context.Context, videoID int64, source domain.FileStamp, probe domain.TranscodeProbe,
) error {
	return upsertTranscodeProbe(ctx, s.db.sql, videoID, source, probe, time.Now())
}

// upsertTranscodeProbe は video_transcode_probes の 1 行を今の版で置き換える。
func upsertTranscodeProbe(
	ctx context.Context, db queryExecer, videoID int64, source domain.FileStamp, probe domain.TranscodeProbe, now time.Time,
) error {
	encoded, err := json.Marshal(probe)
	if err != nil {
		return fmt.Errorf("cannot encode the transcode probe as JSON (id=%d): %w", videoID, err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into video_transcode_probes (video_id, version, size_bytes, mtime_ns, probe, updated_at)
		values (?, ?, ?, ?, ?, ?)
		on conflict (video_id) do update
		   set version = excluded.version, size_bytes = excluded.size_bytes, mtime_ns = excluded.mtime_ns,
		       probe = excluded.probe, updated_at = excluded.updated_at`,
		videoID, domain.TranscodeProbeVersion, source.SizeBytes, source.ModTimeNs, string(encoded), now.Unix(),
	); err != nil {
		return fmt.Errorf("cannot save the transcode probe (id=%d): %w", videoID, err)
	}
	return nil
}

// MarkProbeFailed は解析に失敗したことを記録する。行は残す。個別のファイルの
// 失敗で取り込み全体を止めないため、一覧には並んだままになる。cause の文を
// probe_error に、domain.ProbeFailure で包まれた理由のコードを probe_error_code に
// 書く（包まれていなければ internal）。
func (s *IngestStore) MarkProbeFailed(ctx context.Context, id int64, cause error) error {
	_, err := s.db.sql.ExecContext(ctx, `
		update videos
		   set probe_state = 'failed', probe_error = ?, probe_error_code = ?, playable = 0, indexed_at = ?
		 where id = ?`,
		cause.Error(), string(domain.ProbeErrorCodeOf(cause)), time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("cannot record the probe failure (id=%d): %w", id, err)
	}
	return nil
}

// SetThumbnailState はサムネイル生成の状態を記録する。
func (s *IngestStore) SetThumbnailState(ctx context.Context, id int64, state domain.ThumbnailState) error {
	_, err := s.db.sql.ExecContext(ctx,
		`update videos set thumbnail_state = ?, indexed_at = ? where id = ?`,
		string(state), time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("cannot record the thumbnail state (id=%d): %w", id, err)
	}
	return nil
}

// SetThumbnailStateForJob は代表サムネイルの状態を記録する。専有したときの内容鍵・
// 所在・所在の世代が今も同じときだけ反映し、反映したかを返す。done を書いたら、同じ
// 取引でその動画の thumbnail_failed を問題から消し、substitution に従って
// thumbnail_first_frame を入れる・消す。
func (s *IngestStore) SetThumbnailStateForJob(
	ctx context.Context, job domain.Job, state domain.ThumbnailState, substitution domain.Substitution,
) (bool, error) {
	return s.setStageStateForJob(ctx, job, domain.JobThumbnail, "thumbnail_state", string(state),
		state == domain.ThumbnailStateDone, substitution, nil)
}

// setStageStateForJob は段階 kind の状態列 column に state を書く。専有したときの
// 内容鍵・所在・所在の世代が今も同じときだけ反映し、反映したかを返す。succeeded なら、
// 同じ取引でその段階の *_failed を直近の取り込みの問題から消し、substitution に従って
// 代用の行を入れる・消す（specs/024-import-progress/data-model.md §3）。succeeded なら、
// 続く段階 next の仕事も同じ取引で積む。
func (s *IngestStore) setStageStateForJob(
	ctx context.Context, job domain.Job, kind domain.JobKind, column, state string, succeeded bool,
	substitution domain.Substitution, next []domain.JobKind,
) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `update videos set `+column+` = ?, indexed_at = ?
		where id = ? and content_key = ? and exists (
			select 1 from video_locations where video_id = videos.id and id = ? and version = ? and path = ?)
		and location_generation = ?`,
		state, time.Now().Unix(), job.VideoID, job.ContentKey, job.LocationID,
		job.LocationVersion, job.LocationPath, job.LocationGeneration)
	if err != nil {
		return false, err
	}
	count, err := res.RowsAffected()
	if err != nil || count != 1 {
		return false, err
	}
	if succeeded {
		if err := clearFailedIssue(ctx, tx, kind, job.VideoID); err != nil {
			return false, err
		}
		if err := applySubstitution(ctx, tx, kind, job, substitution); err != nil {
			return false, err
		}
	}
	var c changes
	if succeeded && len(next) > 0 {
		for _, following := range next {
			if err := requeueJob(ctx, tx, following, job.VideoID, time.Now().Unix()); err != nil {
				return false, err
			}
		}
		c.jobsQueued(next...)
	}
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return false, err
	}
	return true, nil
}

// SetSeekThumbnailStateForJob はシーク用サムネイルの状態を記録する。専有した
// ときの内容鍵・所在・所在の世代が今も同じときだけ反映し、反映したかを返す。done を
// 書いたら、substitution に従って seek_thumbnail_full_decode を入れる・消す。
//
// done を書いたら、同じ取引で指紋の仕事を積む。指紋は完成したスプライトから作るので、
// 作り直し（RequeueMissingSeekThumbnails）のあとの完了でも積み直す
// （specs/030-video-versions/data-model.md §6）。
func (s *IngestStore) SetSeekThumbnailStateForJob(
	ctx context.Context, job domain.Job, state domain.SeekThumbnailState, substitution domain.Substitution,
) (bool, error) {
	applied, err := s.setStageStateForJob(ctx, job, domain.JobSeekThumbnail, "seek_thumbnail_state", string(state),
		state == domain.SeekThumbnailDone, substitution, []domain.JobKind{domain.JobFingerprint})
	if err != nil {
		return false, fmt.Errorf("cannot record the seek thumbnail state (id=%d): %w", job.VideoID, err)
	}
	return applied, nil
}

// SetPreviewStateForJob applies only to the content and location generation
// captured when the preview job was claimed.
func (s *IngestStore) SetPreviewStateForJob(ctx context.Context, job domain.Job, state domain.PreviewState) (bool, error) {
	return s.setStageStateForJob(ctx, job, domain.JobPreview, "preview_state", string(state),
		state == domain.PreviewStateDone, domain.SubstitutionUnknown, nil)
}

// SetPreviewStateForContent accepts a completed asset after a representative
// location changed, while still refusing a stale content identity.
func (s *IngestStore) SetPreviewStateForContent(ctx context.Context, job domain.Job, state domain.PreviewState) (bool, error) {
	res, err := s.db.sql.ExecContext(ctx, `update videos set preview_state = ?, indexed_at = ?
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
	res, err := tx.ExecContext(ctx, `update videos set preview_state = 'done', indexed_at = ?
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
	if err := clearFailedIssue(ctx, tx, domain.JobPreview, job.VideoID); err != nil {
		return false, err
	}
	var c changes
	c.remainingChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
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

// ThumbnailSourceCurrent は、動画 videoID の内容が key のままで、所在 locationPath が今も
// 内容 key の動画の所在かを返す。代表サムネイルの位置の指定で、要求が所在を決めてから
// 生成するまでの間に、走査がその所在を別の内容へ付け替えていないかを確かめる
// （specs/029-video-overrides/research.md R-4）。所在が消えていても偽を返す。
func (s *IngestStore) ThumbnailSourceCurrent(
	ctx context.Context, videoID int64, key, locationPath string,
) (bool, error) {
	var current int
	err := s.db.sql.QueryRowContext(ctx, `select exists (
		select 1 from videos where id = ? and content_key = ?
	) and exists (
		select 1 from video_locations l join videos v on v.id = l.video_id
		where l.path = ? and v.content_key = ?
	)`, videoID, key, locationPath, key).Scan(&current)
	return current != 0, err
}

func (s *IngestStore) SetPreviewState(ctx context.Context, id int64, state domain.PreviewState) error {
	_, err := s.db.sql.ExecContext(ctx, `update videos set preview_state = ?, indexed_at = ? where id = ?`, string(state), time.Now().Unix(), id)
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
		return false, fmt.Errorf("cannot start regenerating the preview (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `update videos set preview_state = 'pending', indexed_at = ?
		where id = ? and content_key = ? and preview_state = 'done'`, now, id, contentKey)
	if err != nil {
		return false, fmt.Errorf("cannot reset the preview state (id=%d): %w", id, err)
	}
	reset, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot check the preview state update count (id=%d): %w", id, err)
	}
	if reset == 0 {
		return false, nil
	}
	if err := requeueJob(ctx, tx, domain.JobPreview, id, now); err != nil {
		return false, err
	}
	var c changes
	c.jobsQueued(domain.JobPreview)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return false, fmt.Errorf("cannot commit regenerating the preview (id=%d): %w", id, err)
	}
	return true, nil
}

// RequeueMissingSeekThumbnails は、シーク用サムネイルを作り終えた記録があるのに
// 置き場が無い動画を、1つの取引の中で作り直す状態へ戻し、シーク用サムネイルの
// ジョブを積む。置き場の有無はファイルの事実なので、呼び出し側が確かめて呼ぶ。
//
// seek_thumbnail_state が done で、内容の識別子が今も同じときだけ変える。
// そうでなければ何もせず false を返す。同じ動画を何度見つけても、積むのは
// 1回である（RequeueMissingPreview と同じ形）。
func (s *IngestStore) RequeueMissingSeekThumbnails(ctx context.Context, id int64, contentKey string) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot start regenerating seek thumbnails (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `update videos set seek_thumbnail_state = 'pending', indexed_at = ?
		where id = ? and content_key = ? and seek_thumbnail_state = 'done'`, now, id, contentKey)
	if err != nil {
		return false, fmt.Errorf("cannot reset the seek thumbnail state (id=%d): %w", id, err)
	}
	reset, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot check the seek thumbnail state update count (id=%d): %w", id, err)
	}
	if reset == 0 {
		return false, nil
	}
	if err := requeueJob(ctx, tx, domain.JobSeekThumbnail, id, now); err != nil {
		return false, err
	}
	// 待っている指紋の仕事は、スプライトが完成するまで取り出せない。作り直しが上限まで
	// 失敗するとスプライトは完成しないので、残すと残りの仕事に数え続ける。捨てて、作り直しの
	// 完了（SetSeekThumbnailStateForJob）で積み直す。
	if _, err := tx.ExecContext(ctx, `delete from jobs where kind = ? and video_id = ? and state = 'queued'`,
		string(domain.JobFingerprint), id); err != nil {
		return false, fmt.Errorf("cannot drop the waiting fingerprint job (id=%d): %w", id, err)
	}
	var c changes
	c.jobsQueued(domain.JobSeekThumbnail)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return false, fmt.Errorf("cannot commit regenerating seek thumbnails (id=%d): %w", id, err)
	}
	return true, nil
}

// requeueJob は終わった行を捨ててから kind の仕事を queued で積み、その動画を直近の
// 走査の対象に加える（addScanVideos）。未完了の行が既にあれば積まない。
//
// 呼び出し側は changes.jobsQueued を記録し、commit で完了の時刻を決め直させる。
func requeueJob(ctx context.Context, tx *sql.Tx, kind domain.JobKind, id, now int64) error {
	if _, err := tx.ExecContext(ctx, `delete from jobs where kind = ? and video_id = ? and state in ('done', 'failed')`,
		string(kind), id); err != nil {
		return fmt.Errorf("cannot clean up old jobs (%s, video=%d): %w", kind, id, err)
	}
	if _, err := tx.ExecContext(ctx, `insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
		values (?, ?, 'queued', 0, ?, ?)
		on conflict (kind, video_id) where state in ('queued', 'running') do nothing`,
		string(kind), id, now, now); err != nil {
		return fmt.Errorf("cannot queue the job (%s, video=%d): %w", kind, id, err)
	}
	return addScanVideos(ctx, tx, id)
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
// thumbnail_state は done でなければ、seek_thumbnail_state は failed なら
// pending に戻し、戻したときだけそれぞれのジョブを積む。seek_thumbnail_state が done なのに
// 置き場が無い動画は、動画の応答を組み立てるときに
// RequeueMissingSeekThumbnails が積み直す。
// 一覧用プレビューのジョブは、読み取りの結果を書く取引で ApplyProbeForJob が積む。
func (s *IngestStore) RetryProbe(ctx context.Context, id int64) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start re-reading (id=%d): %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `update videos set probe_state = 'pending', probe_error = null, probe_error_code = null, indexed_at = ?
		where id = ? and probe_state = 'failed'`, now, id)
	if err != nil {
		return fmt.Errorf("cannot reset the probe state (id=%d): %w", id, err)
	}
	reset, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot check the probe state update count (id=%d): %w", id, err)
	}
	if reset == 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from videos where id = ?)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("cannot check whether the video exists (id=%d): %w", id, err)
		}
		if exists == 0 {
			return domain.ErrNotFound
		}
		return domain.ErrProbeNotFailed
	}

	res, err = tx.ExecContext(ctx, `update videos set thumbnail_state = 'pending', indexed_at = ?
		where id = ? and thumbnail_state <> 'done'`, now, id)
	if err != nil {
		return fmt.Errorf("cannot reset the thumbnail state (id=%d): %w", id, err)
	}
	thumbnailReset, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot check the thumbnail state update count (id=%d): %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, `update videos set preview_state = 'pending', indexed_at = ?
		where id = ? and preview_state = 'failed'`, now, id); err != nil {
		return fmt.Errorf("cannot reset the preview state (id=%d): %w", id, err)
	}
	res, err = tx.ExecContext(ctx, `update videos set seek_thumbnail_state = 'pending', indexed_at = ?
		where id = ? and seek_thumbnail_state = 'failed'`, now, id)
	if err != nil {
		return fmt.Errorf("cannot reset the seek thumbnail state (id=%d): %w", id, err)
	}
	seekThumbnailReset, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot check the seek thumbnail state update count (id=%d): %w", id, err)
	}

	kinds := []domain.JobKind{domain.JobProbe}
	if thumbnailReset > 0 {
		kinds = append(kinds, domain.JobThumbnail)
	}
	if seekThumbnailReset > 0 {
		kinds = append(kinds, domain.JobSeekThumbnail)
	}
	for _, kind := range kinds {
		if err := requeueJob(ctx, tx, kind, id, now); err != nil {
			return err
		}
	}
	var c changes
	c.jobsQueued(kinds...)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot commit re-reading (id=%d): %w", id, err)
	}
	return nil
}
