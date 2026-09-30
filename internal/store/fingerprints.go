package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 映像の指紋（specs/030-video-versions/data-model.md §1・§6、research.md R-6）。
// video_fingerprints は索引で、内容の参照が無くなるときに releaseContentIndex が消す。

// fingerprintMissingCondition は、別名 alias の videos の行のシーク用スプライトが完成して
// いるのに、今の domain.FingerprintVersion の指紋が無い条件である。走査が指紋の仕事を
// 積み直す判断（domain.IndexedVideo.FingerprintMissing と EnsureJob）に使う。
func fingerprintMissingCondition(alias string) string {
	return alias + `.seek_thumbnail_state = '` + string(domain.SeekThumbnailDone) + `' and ` +
		alias + `.content_key <> '' and not exists (
		select 1 from video_fingerprints f where f.content_key = ` + alias + `.content_key and f.version = ` +
		strconv.Itoa(domain.FingerprintVersion) + `)`
}

// ApplyFingerprintForJob は専有した時点の内容鍵・所在・所在の世代が今も同じときだけ、
// その内容の指紋を置き換え、反映したかを返す。同じ取引でその動画の fingerprint_failed を
// 直近の取り込みの問題から消す。候補の算出は次の単位がこの取引に足す（data-model.md §7）。
func (s *IngestStore) ApplyFingerprintForJob(
	ctx context.Context, job domain.Job, fingerprint domain.Fingerprint,
) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot start saving the fingerprint (id=%d): %w", job.VideoID, err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := jobIdentityCurrent(ctx, tx, job)
	if err != nil {
		return false, fmt.Errorf("cannot check the fingerprint job location (id=%d): %w", job.VideoID, err)
	}
	if !current {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `insert into video_fingerprints (content_key, version, interval_ms, hashes, updated_at)
		values (?, ?, ?, ?, ?)
		on conflict (content_key) do update
		   set version = excluded.version, interval_ms = excluded.interval_ms,
		       hashes = excluded.hashes, updated_at = excluded.updated_at`,
		job.ContentKey, fingerprint.Version, fingerprint.IntervalMs, fingerprint.Encode(), time.Now().Unix(),
	); err != nil {
		return false, fmt.Errorf("cannot save the fingerprint (id=%d): %w", job.VideoID, err)
	}
	if err := clearFailedIssue(ctx, tx, domain.JobFingerprint, job.VideoID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("cannot commit the fingerprint (id=%d): %w", job.VideoID, err)
	}
	return true, nil
}
