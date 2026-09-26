package store

// ライブ変換用の解析情報（video_transcode_probes）の読み出し。LibraryStore が受け持つ。
// 書き込みは IngestStore（ingest_results.go）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// TranscodeProbe は動画 1 件分の保存済みの解析情報を、解釈せずにそのまま返す。行が
// 無ければ nil。使ってよいかは domain.TranscodeProbeUsable が決める
// （specs/018-live-transcode-seek/data-model.md §3）。videos の一覧・詳細には載せない。
func (s *LibraryStore) TranscodeProbe(ctx context.Context, videoID int64) (*domain.StoredTranscodeProbe, error) {
	var stored domain.StoredTranscodeProbe
	err := s.db.sql.QueryRowContext(ctx, `
		select version, size_bytes, mtime_ns, probe from video_transcode_probes where video_id = ?`, videoID,
	).Scan(&stored.Version, &stored.Source.SizeBytes, &stored.Source.ModTimeNs, &stored.Probe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ライブ変換用の解析情報を読めません (id=%d): %w", videoID, err)
	}
	return &stored, nil
}
