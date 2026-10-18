package store

// LibraryStore reads transcode probes; IngestStore writes them.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// TranscodeProbe returns raw stored data or nil; the domain decides usability, and ordinary video responses omit it.
func (s *LibraryStore) TranscodeProbe(ctx context.Context, videoID int64) (*domain.StoredTranscodeProbe, error) {
	var stored domain.StoredTranscodeProbe
	err := s.db.sql.QueryRowContext(ctx, `
		select version, size_bytes, mtime_ns, probe from video_transcode_probes where video_id = ?`, videoID,
	).Scan(&stored.Version, &stored.Source.SizeBytes, &stored.Source.ModTimeNs, &stored.Probe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the transcode probe (id=%d): %w", videoID, err)
	}
	return &stored, nil
}
