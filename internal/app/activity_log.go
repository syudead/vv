package app

import (
	"context"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// アクティビティの記録の一覧・やり直し・取り消し（specs/055-activity/plan.md「How it works」）。記録の
// 書き込みは保存層が変える内容と同じ取引で行い、ここは一覧を渡し、やり直しと取り消しを種類ごとに既存の
// use case へ振り分ける。activity.go は取り込み中の今の処理（domain.ScanActivity）で、別物である。

// ActivityLogStore はアクティビティの記録の読み出しと起動時の回復である。internal/store の
// *ActivityStore が満たす。
type ActivityLogStore interface {
	// ListActivity は query を満たす記録を id の降順に cursor の次から limit 件返す。読めない cursor は
	// domain.ErrInvalidCursor。
	ListActivity(
		ctx context.Context, audience domain.Audience, query domain.ActivityQuery, cursor string, limit int,
	) (domain.ActivityPage, error)
	// ActivityEntry は記録 1 件を返す。無ければ domain.ErrNotFound。
	ActivityEntry(ctx context.Context, id int64) (domain.ActivityEntry, error)
	// RecoverInterruptedActivity は processing の running の記録を attention/interrupted にする。
	RecoverInterruptedActivity(ctx context.Context) (int, error)
}

// ActivityRetrier は 1 つの種類の記録の処理を始め直す（contracts/screen-api.md
// 「POST /api/activity/{id}/retry」）。種類ごとの既存の use case（読み取りのやり直し・手動の走査・
// 自動タグ付けの待ち行列・判定の再試行）がこれを満たす。
type ActivityRetrier interface {
	RetryActivity(ctx context.Context, entry domain.ActivityEntry) error
}

// ActivityUndoer は 1 つの種類の自動の判断を取り消し、所有者の undo の記録を返す（contracts/screen-api.md
// 「POST /api/activity/{id}/undo」）。tagID は auto_tag のタグで、ほかの種類では 0。
type ActivityUndoer interface {
	UndoActivity(ctx context.Context, entry domain.ActivityEntry, tagID int64) (domain.ActivityEntry, error)
}

// ActivityOptions はアクティビティの記録に必要な依存である。
type ActivityOptions struct {
	Store ActivityLogStore
	// Retriers は種類ごとのやり直しの振り分け表である。無い種類はやり直しを出さない。
	Retriers map[domain.ActivityKind]ActivityRetrier
	// Undoers は種類ごとの取り消しの振り分け表である。無い種類は取り消しを出さない。
	Undoers map[domain.ActivityKind]ActivityUndoer
}

// Activity はアクティビティの記録の一覧と、やり直しと取り消しの振り分けを持つ。
type Activity struct {
	store    ActivityLogStore
	retriers map[domain.ActivityKind]ActivityRetrier
	undoers  map[domain.ActivityKind]ActivityUndoer
}

// NewActivity は振り分け表の写しを持つ Activity を返す。
func NewActivity(opts ActivityOptions) *Activity {
	a := &Activity{
		store:    opts.Store,
		retriers: map[domain.ActivityKind]ActivityRetrier{},
		undoers:  map[domain.ActivityKind]ActivityUndoer{},
	}
	for kind, retrier := range opts.Retriers {
		if retrier != nil {
			a.retriers[kind] = retrier
		}
	}
	for kind, undoer := range opts.Undoers {
		if undoer != nil {
			a.undoers[kind] = undoer
		}
	}
	return a
}

// ListActivity は記録の 1 ページを返す。
func (a *Activity) ListActivity(
	ctx context.Context, audience domain.Audience, query domain.ActivityQuery, cursor string, limit int,
) (domain.ActivityPage, error) {
	return a.store.ListActivity(ctx, audience, query, cursor, limit)
}

// RecoverInterrupted は、前回の停止で processing のまま残った記録を attention/interrupted にする。起動時に、
// ワーカーを動かす前に呼ぶ（docs/design-docs/process-lifecycle.md）。
func (a *Activity) RecoverInterrupted(ctx context.Context) (int, error) {
	recovered, err := a.store.RecoverInterruptedActivity(ctx)
	if err != nil {
		return 0, fmt.Errorf("cannot recover the interrupted activity: %w", err)
	}
	return recovered, nil
}

// RetryActivity は記録 id の処理を始め直す。無ければ domain.ErrNotFound、記録がやり直しを出さないか、その
// 種類の振り分け先が無ければ domain.ErrActivityNotRetryable。振り分け先の誤り（走査中など）はそのまま返す。
func (a *Activity) RetryActivity(ctx context.Context, id int64) error {
	entry, err := a.store.ActivityEntry(ctx, id)
	if err != nil {
		return err
	}
	retrier, ok := a.retriers[entry.Kind]
	if !ok || !entry.RetryOffered() {
		return domain.ErrActivityNotRetryable
	}
	return retrier.RetryActivity(ctx, entry)
}

// UndoActivity は記録 id の自動の判断を取り消し、所有者の undo の記録を返す。無い記録と、auto_tag の記録が
// 付けていないタグは domain.ErrNotFound。記録かタグが取り消しを出さないか、その種類の振り分け先が無ければ
// domain.ErrActivityNotUndoable。
func (a *Activity) UndoActivity(ctx context.Context, id, tagID int64) (domain.ActivityEntry, error) {
	entry, err := a.store.ActivityEntry(ctx, id)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	if !undoTargetOffered(entry, tagID) {
		if entry.Kind == domain.ActivityAutoTag && !hasActivityTag(entry, tagID) {
			return domain.ActivityEntry{}, fmt.Errorf("tag %d of activity entry %d: %w", tagID, id, domain.ErrNotFound)
		}
		return domain.ActivityEntry{}, domain.ErrActivityNotUndoable
	}
	undoer, ok := a.undoers[entry.Kind]
	if !ok {
		return domain.ActivityEntry{}, domain.ErrActivityNotUndoable
	}
	return undoer.UndoActivity(ctx, entry, tagID)
}

// undoTargetOffered は、記録 entry（auto_tag ならそのタグ tagID）が取り消しを出すかを返す。
func undoTargetOffered(entry domain.ActivityEntry, tagID int64) bool {
	if entry.Kind != domain.ActivityAutoTag {
		return entry.UndoOffered()
	}
	index := slices.IndexFunc(entry.Tags, func(tag domain.ActivityEntryTag) bool { return tag.TagID == tagID })
	return index >= 0 && entry.TagUndoOffered(entry.Tags[index])
}

// hasActivityTag は auto_tag の記録 entry がタグ tagID を付けたかを返す。
func hasActivityTag(entry domain.ActivityEntry, tagID int64) bool {
	return slices.ContainsFunc(entry.Tags, func(tag domain.ActivityEntryTag) bool { return tag.TagID == tagID })
}
