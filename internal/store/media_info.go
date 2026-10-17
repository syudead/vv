package store

// 動画の追加の情報（video_media_info）の書き込みと読み出し。書き込みは取り込みの解析
// （ApplyProbe と ApplyProbeForJob）だけが upsertMediaInfo で行う。読み出しは
// LibraryStore が受け持つ。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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

// MediaInfo は動画 1 件の追加の情報を返す。行が無い、版が domain.MediaInfoVersion と違う、
// JSON が読めない、のいずれかなら nil（無い）である。詳細の読み出しだけが使い、一覧には
// 載せない。
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
