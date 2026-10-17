package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// 重複の候補の判定の記録（version_judgement）のやり直しと取り消し。
// Activity の振り分け表に入り、保存層の 1 つの取引の後で判定のワーカーを起こす。

// VersionJudgementRetryStore は失敗か中断した組の判定をすぐ取り出せるようにする。internal/store の
// *VersionStore が満たす。
type VersionJudgementRetryStore interface {
	// RetryJudgement は記録 id の組の retry_at を今にし、記録を待ちに戻す。やり直しを出さなければ
	// domain.ErrActivityNotRetryable。
	RetryJudgement(ctx context.Context, id int64) error
}

// VersionJudgementUndoStore は自動の「同じ」と「違う」を 1 つの取引で取り消す。internal/store の
// *ActivityStore が満たす。
type VersionJudgementUndoStore interface {
	// UndoVersionJudgement は記録 id の判断を取り消し、所有者の undo の記録を返す。
	UndoVersionJudgement(ctx context.Context, id int64) (domain.ActivityEntry, error)
}

// judgeWaker は判定のワーカーを起こす。*VersionJudge が満たす。
type judgeWaker interface {
	Wake()
}

// VersionJudgementActivity は version_judgement の記録の ActivityRetrier と ActivityUndoer である。
type VersionJudgementActivity struct {
	retry VersionJudgementRetryStore
	undo  VersionJudgementUndoStore
	judge judgeWaker
}

// NewVersionJudgementActivity は判定の記録のやり直しと取り消しを組み立てる。judge は保存の後に起こす
// 判定のワーカーである。
func NewVersionJudgementActivity(
	retry VersionJudgementRetryStore, undo VersionJudgementUndoStore, judge judgeWaker,
) *VersionJudgementActivity {
	return &VersionJudgementActivity{retry: retry, undo: undo, judge: judge}
}

// RetryActivity は記録 entry の組をすぐ判定し直せるようにし、判定のワーカーを起こす。記録は running に戻り、
// 同じ記録が新しい結果になる。
func (a *VersionJudgementActivity) RetryActivity(ctx context.Context, entry domain.ActivityEntry) error {
	if err := a.retry.RetryJudgement(ctx, entry.ID); err != nil {
		return err
	}
	a.judge.Wake()
	return nil
}

// UndoActivity は記録 entry の自動の判断を取り消し、所有者の undo の記録を返す。組が候補に戻りうるので、
// 判定のワーカーを起こす（保留と外した組は取り出さない）。tagID は使わない。
func (a *VersionJudgementActivity) UndoActivity(
	ctx context.Context, entry domain.ActivityEntry, _ int64,
) (domain.ActivityEntry, error) {
	undo, err := a.undo.UndoVersionJudgement(ctx, entry.ID)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	a.judge.Wake()
	return undo, nil
}
