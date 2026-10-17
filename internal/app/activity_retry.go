package app

import (
	"context"
	"errors"

	"github.com/syudead/vv/internal/domain"
)

// 取り込みと失敗した走査のやり直し。app.Activity の振り分け表に、import と scan の種類で登録する。

// ImportRetryStore は import の記録のやり直し先である。internal/store の *IngestStore が満たす。
type ImportRetryStore interface {
	// RetryImport は記録 entryID の、内容の鍵 contentKey の動画の失敗した段階を積み直す。積んだ仕事は所有者が
	// 始めた新しい import の記録になる。記録がもうやり直しを出さないか、動画が無いか、積み直す仕事が無ければ
	// domain.ErrActivityNotRetryable。
	RetryImport(ctx context.Context, entryID int64, contentKey string) error
}

// ImportRetrier は import の記録の動画の失敗した段階を、読み取りのやり直し（POST /api/videos/{id}/probe）と
// 同じく積み直す。積んだことは保存層の確定後の知らせ（domain.JobsQueued）が各段階のワーカーを起こす。
type ImportRetrier struct {
	store ImportRetryStore
}

// NewImportRetrier は store へ積み直す ImportRetrier を返す。
func NewImportRetrier(store ImportRetryStore) *ImportRetrier {
	return &ImportRetrier{store: store}
}

// RetryActivity は記録 entry の対象の動画の取り込みをやり直す。
func (r *ImportRetrier) RetryActivity(ctx context.Context, entry domain.ActivityEntry) error {
	return r.store.RetryImport(ctx, entry.ID, entry.Subject.Key)
}

// RetryActivity は失敗した走査の記録 entry をやり直し、POST /api/scans と同じくすべての登録フォルダの手動の
// 走査を始める。走査中なら domain.ErrScanRunning、登録フォルダが無ければ domain.ErrActivityNotRetryable を
// 返す。記録は、新しい走査が done で終わったときに閉じる。
func (s *Scans) RetryActivity(ctx context.Context, _ domain.ActivityEntry) error {
	_, started, err := s.StartScan(ctx)
	switch {
	case errors.Is(err, domain.ErrNoMediaFolders):
		return domain.ErrActivityNotRetryable
	case err != nil:
		return err
	case !started:
		return domain.ErrScanRunning
	}
	return nil
}
