package store

// 動画の追加の情報（video_media_info）の書き込みと読み出し
// （specs/056-media-info/data-model.md「video_media_info」）。書き込みは取り込みの解析
// （ApplyProbeForJob）と、のちの再解析の仕事が upsertMediaInfo を共有する。読み出しは
// LibraryStore が受け持つ。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// upsertMediaInfo は video_media_info の 1 行を今の版で置き換える。呼び手は動画の身元を
// 確かめた取引の中で呼ぶ。
func upsertMediaInfo(ctx context.Context, db queryExecer, videoID int64, info domain.MediaInfo, now time.Time) error {
	encoded, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("cannot encode the media information as JSON (id=%d): %w", videoID, err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into video_media_info (video_id, version, info, updated_at)
		values (?, ?, ?, ?)
		on conflict (video_id) do update
		   set version = excluded.version, info = excluded.info, updated_at = excluded.updated_at`,
		videoID, domain.MediaInfoVersion, string(encoded), now.Unix(),
	); err != nil {
		return fmt.Errorf("cannot save the media information (id=%d): %w", videoID, err)
	}
	return nil
}

// mediaInfoMissingCondition は、別名 alias の videos の行の解析が done なのに、今の
// domain.MediaInfoVersion の追加の情報の行が無い条件である。走査が解析し直しの仕事を積む
// 判断（domain.IndexedVideo.MediaInfoMissing と EnsureJob）に使う。
func mediaInfoMissingCondition(alias string) string {
	return alias + `.probe_state = '` + string(domain.ProbeStateDone) + `' and not exists (
		select 1 from video_media_info m where m.video_id = ` + alias + `.id and m.version = ` +
		strconv.Itoa(domain.MediaInfoVersion) + `)`
}

// ApplyMediaInfoForJob は解析し直しの仕事（domain.JobMetadata）の結果を書き、書いたかを返す。
// 専有した時点の内容鍵・所在・所在の世代が今も同じときだけ video_media_info の行を置き換え、
// 同じ取引でその動画の metadata_failed を直近の取り込みの問題から消す。videos の行には
// 触れない。身元が変わっていれば何も書かずに false を返し、仕事はそのまま完了させる
// （specs/056-media-info/data-model.md「Rules」）。
func (s *IngestStore) ApplyMediaInfoForJob(ctx context.Context, job domain.Job, info domain.MediaInfo) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot start saving the media information (id=%d): %w", job.VideoID, err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := jobIdentityCurrent(ctx, tx, job)
	if err != nil {
		return false, fmt.Errorf("cannot check the metadata job location (id=%d): %w", job.VideoID, err)
	}
	if !current {
		return false, nil
	}
	if err := upsertMediaInfo(ctx, tx, job.VideoID, info, time.Now()); err != nil {
		return false, err
	}
	if err := clearFailedIssue(ctx, tx, domain.JobMetadata, job.VideoID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("cannot commit the media information (id=%d): %w", job.VideoID, err)
	}
	return true, nil
}

// MediaInfo は動画 1 件の追加の情報を返す。行が無い、版が domain.MediaInfoVersion と違う、
// JSON が読めない、のいずれかなら nil（無い）である。詳細の読み出しだけが使い、一覧には
// 載せない（specs/056-media-info/research.md R-6）。
func (s *LibraryStore) MediaInfo(ctx context.Context, videoID int64) (*domain.MediaInfo, error) {
	var version int
	var encoded string
	err := s.db.sql.QueryRowContext(ctx,
		`select version, info from video_media_info where video_id = ?`, videoID,
	).Scan(&version, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the media information (id=%d): %w", videoID, err)
	}
	if version != domain.MediaInfoVersion {
		return nil, nil
	}
	var info domain.MediaInfo
	if json.Unmarshal([]byte(encoded), &info) != nil {
		return nil, nil
	}
	return &info, nil
}
