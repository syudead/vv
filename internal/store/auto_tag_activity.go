package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 自動タグ付けのアクティビティの記録（specs/055-activity/data-model.md「Kinds」の auto_tag）。待ち行列に積むと
// running/waiting の記録を開いて行の activity_id に持たせ、取り出すと processing、終えると done にして付けた
// タグを activity_entry_tags に残し、失敗は attention/failed にする。どれも待ち行列を変える取引の中で書く
// （research.md R-2）。タグごとの取り消しは ActivityStore.UndoAutoTag の 1 つの取引である（R-7）。

// contentLabelExpr は内容の鍵の式 keyExpr の、書いた時点の表示名の式である。表示名があればそれ、無ければ
// 登録フォルダの下の代表の所在の題名、それも無ければ鍵そのもの（research.md R-10）。
func contentLabelExpr(keyExpr string) string {
	return `coalesce((select nullif(ov.display_name, '') from video_overrides ov where ov.content_key = ` + keyExpr + `),
		(select l.title from videos lv join video_locations l on l.video_id = lv.id
			where lv.content_key = ` + keyExpr + ` and ` + registeredLocationCondition("l") + ` order by l.path limit 1),
		` + keyExpr + `)`
}

// autoTagActivityEntry は内容 key の auto_tag の記録を running の reason で書く形にする。
func autoTagActivityEntry(key, label string, reason domain.ActivityReason, decider domain.ActivityDecider) domain.ActivityEntry {
	return domain.ActivityEntry{
		Kind:    domain.ActivityAutoTag,
		Subject: domain.ActivitySubject{Kind: domain.ActivitySubjectVideo, Key: key, Label: label},
		Status:  domain.ActivityRunning,
		Reason:  reason,
		Decider: decider,
	}
}

// claimAutoTagActivity は取り出した内容 key の行が属する記録 activityID を processing にする。記録が無いか
// running でなければ（起動時に interrupted になったなど）、新しい processing の記録を開いて行に持たせる。
// 新しい記録の判断したものは前の記録のもので、前の記録が無ければ rule である（data-model.md「Rules」）。
func claimAutoTagActivity(ctx context.Context, tx *sql.Tx, c *changes, key string, activityID int64, now time.Time) error {
	decider := domain.ActivityByRule
	if activityID != 0 {
		var status, stored string
		err := tx.QueryRowContext(ctx, `select status, decider from activity_entries where id = ?`, activityID).
			Scan(&status, &stored)
		switch {
		case err == nil && domain.ActivityStatus(status) == domain.ActivityRunning:
			return setActivityState(ctx, tx, c, activityID,
				domain.ActivityState{Status: domain.ActivityRunning, Reason: domain.ActivityProcessing}, now)
		case err == nil:
			decider = domain.ActivityDecider(stored)
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("cannot read the auto-tagging activity (id=%d): %w", activityID, err)
		}
	}
	var label string
	if err := tx.QueryRowContext(ctx, `select `+contentLabelExpr("?1"), key).Scan(&label); err != nil {
		return fmt.Errorf("cannot read the label of the auto-tagging subject: %w", err)
	}
	id, err := recordActivity(ctx, tx, c, autoTagActivityEntry(key, label, domain.ActivityProcessing, decider), now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update auto_tag_queue set activity_id = ? where content_key = ?`, id, key); err != nil {
		return fmt.Errorf("cannot link the auto-tagging job to its activity: %w", err)
	}
	return nil
}

// closeAutoTagActivity は記録 id を state にし、decider と detail が空でなければそれも書く。id が 0（この機能より
// 前に積んだ行）なら何もしない。
func closeAutoTagActivity(
	ctx context.Context, tx *sql.Tx, c *changes, id int64, state domain.ActivityState, decider domain.ActivityDecider,
	detail string, now time.Time,
) error {
	if id == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `update activity_entries set status = ?, reason = ?, outcome = ?,
		decider = coalesce(nullif(?, ''), decider), detail = coalesce(nullif(?, ''), detail), updated_at = ?
		where id = ?`, string(state.Status), nullString(string(state.Reason)), nullString(string(state.Outcome)),
		string(decider), detail, now.UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("cannot update the auto-tagging activity (id=%d): %w", id, err)
	}
	c.activityChanged()
	return nil
}

// autoTagWrite は判定が実際に書いた video_tags の行 1 つである。
type autoTagWrite struct {
	tagID     int64
	suggested bool
}

// writeAutoTags は selection のタグ（今もあり、却下されていないもの）を利用者データの鍵 userKey に now で
// 書き、実際に書いた行を返す。Confirmed は確定した付与として書き、仮に付いていれば確定して時刻を now に
// する。Tentative は仮に書き、既に付いていれば（手で付けた分も）そのままにする。書いた時刻は取り消しが
// 同じ行かを確かめるのに使う（research.md R-7）。
func writeAutoTags(
	ctx context.Context, tx *sql.Tx, userKey string, selection domain.AutoTagSelection, now time.Time,
) ([]autoTagWrite, error) {
	groups := []struct {
		ids       []int64
		suggested bool
		statement string
	}{
		{selection.Confirmed, false, `insert into video_tags (content_key, tag_id, created_at, tentative) values (?, ?, ?, 0)
			on conflict (content_key, tag_id) do update set tentative = 0, created_at = excluded.created_at
			where video_tags.tentative = 1`},
		{selection.Tentative, true, `insert or ignore into video_tags (content_key, tag_id, created_at, tentative)
			values (?, ?, ?, 1)`},
	}
	var written []autoTagWrite
	for _, group := range groups {
		ids, err := attachableAutoTags(ctx, tx, userKey, group.ids)
		if err != nil {
			return nil, err
		}
		for _, tagID := range ids {
			result, err := tx.ExecContext(ctx, group.statement, userKey, tagID, now.Unix())
			if err != nil {
				return nil, fmt.Errorf("cannot add an auto-tagging tag (tag=%d): %w", tagID, err)
			}
			if affected, err := result.RowsAffected(); err != nil {
				return nil, fmt.Errorf("cannot add an auto-tagging tag (tag=%d): %w", tagID, err)
			} else if affected > 0 {
				written = append(written, autoTagWrite{tagID: tagID, suggested: group.suggested})
			}
		}
	}
	return written, nil
}

// recordAutoTagActivityTags は記録 entryID に、判定が書いたタグを名前・書いた鍵と時刻・確率とともに残す
// （data-model.md「activity_entry_tags」）。
func recordAutoTagActivityTags(
	ctx context.Context, tx *sql.Tx, entryID int64, userKey string, written []autoTagWrite,
	probabilities map[int64]float64, now time.Time,
) error {
	for _, write := range written {
		ref, err := tagRefByID(ctx, tx, write.tagID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `insert into activity_entry_tags (entry_id, tag_id, tag_name, suggested,
			user_key, attached_at, probability) values (?, ?, ?, ?, ?, ?, ?)`,
			entryID, write.tagID, ref.Name, boolToInt(write.suggested), userKey, now.Unix(),
			probabilities[write.tagID]); err != nil {
			return fmt.Errorf("cannot record the auto-tagging tag (tag=%d): %w", write.tagID, err)
		}
	}
	return nil
}

// autoTagUndoTarget は取り消す記録と、そのタグの行である。
type autoTagUndoTarget struct {
	entry      domain.ActivityEntry
	tag        domain.ActivityEntryTag
	userKey    string
	attachedAt int64
}

// readAutoTagUndoTarget は記録 entryID とそのタグ tagID を取引の中で読み直す。記録か、記録が付けたタグが
// 無ければ domain.ErrNotFound。
func readAutoTagUndoTarget(ctx context.Context, tx *sql.Tx, entryID, tagID int64) (autoTagUndoTarget, error) {
	var (
		target                     autoTagUndoTarget
		kind, subjectKind, decider string
	)
	err := tx.QueryRowContext(ctx, `select e.kind, e.subject_kind, e.subject_key, e.subject_label, e.decider,
		t.tag_name, t.suggested, t.user_key, t.attached_at, t.probability, coalesce(t.undone_by, 0)
		from activity_entries e join activity_entry_tags t on t.entry_id = e.id
		where e.id = ? and t.tag_id = ?`, entryID, tagID).Scan(&kind, &subjectKind, &target.entry.Subject.Key,
		&target.entry.Subject.Label, &decider, &target.tag.Name, &target.tag.Suggested, &target.userKey,
		&target.attachedAt, &target.tag.Probability, &target.tag.UndoneBy)
	if errors.Is(err, sql.ErrNoRows) {
		return autoTagUndoTarget{}, fmt.Errorf("tag %d of activity entry %d: %w", tagID, entryID, domain.ErrNotFound)
	}
	if err != nil {
		return autoTagUndoTarget{}, fmt.Errorf("cannot read the activity entry to undo (id=%d): %w", entryID, err)
	}
	target.entry.ID = entryID
	target.entry.Kind = domain.ActivityKind(kind)
	target.entry.Subject.Kind = domain.ActivitySubjectKind(subjectKind)
	target.entry.Decider = domain.ActivityDecider(decider)
	target.tag.TagID = tagID
	return target, nil
}

// currentUserKey は内容 contentKey の動画の今の利用者データの鍵を返す。動画が無ければ空。
func currentUserKey(ctx context.Context, tx *sql.Tx, contentKey string) (string, error) {
	var key string
	err := tx.QueryRowContext(ctx, `select `+userKeyExpr("v")+` from videos v
		where v.content_key = ? and v.content_key <> '' limit 1`, contentKey).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the user key of the video: %w", err)
	}
	return key, nil
}

// autoTagRemoval は取り消しが video_tags に対して行ったことである。removed は行を消した鍵、reverted は
// 今の鍵（無ければ記録した鍵）の行を消したか、rejectAt は除外を書く鍵である。
type autoTagRemoval struct {
	removed  []string
	reverted bool
	rejectAt []string
}

// removeAutoTagAttachment は判定が書いた行がまだそのままなら消す。行は対象の動画の今の利用者データの鍵と、
// 記録した鍵の両方で探す。集まりに入ると代表のタグは時刻を保って集まりの鍵へ写り、記録した鍵にも隠れて残るので、
// 今の鍵を先に見て、どちらの鍵でも書いた時刻が同じで、提案のタグならまだ提案のままの行だけを消す。所有者が外して
// 付け直したか確定した行は時刻か状態が違うので残る（research.md R-7）。今の鍵の行を消したときが取り消しの完了で、
// 除外は今の鍵（無ければ記録した鍵）と、行を消した鍵に書く。
func removeAutoTagAttachment(ctx context.Context, tx *sql.Tx, target autoTagUndoTarget) (autoTagRemoval, error) {
	primary, err := currentUserKey(ctx, tx, target.entry.Subject.Key)
	if err != nil {
		return autoTagRemoval{}, err
	}
	if primary == "" {
		primary = target.userKey
	}
	keys := []string{primary}
	if target.userKey != primary {
		keys = append(keys, target.userKey)
	}
	removal := autoTagRemoval{rejectAt: []string{primary}}
	for _, key := range keys {
		result, err := tx.ExecContext(ctx, `delete from video_tags where content_key = ? and tag_id = ? and created_at = ?
			and (? = 0 or tentative = 1)`, key, target.tag.TagID, target.attachedAt, boolToInt(target.tag.Suggested))
		if err != nil {
			return autoTagRemoval{}, fmt.Errorf("cannot remove the auto-tagging tag (tag=%d): %w", target.tag.TagID, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return autoTagRemoval{}, fmt.Errorf("cannot remove the auto-tagging tag (tag=%d): %w", target.tag.TagID, err)
		}
		if affected == 0 {
			continue
		}
		removal.removed = append(removal.removed, key)
		if key == primary {
			removal.reverted = true
		} else {
			removal.rejectAt = append(removal.rejectAt, key)
		}
	}
	return removal, nil
}

// UndoAutoTag は auto_tag の記録 entryID が付けたタグ tagID を取り消し、所有者の undo の記録を返す
// （contracts/screen-api.md「POST /api/activity/{id}/undo」）。1 つの取引で、判定が書いた行がまだそのままなら
// 消し（集まりの鍵ならすべてのメンバーから外れる）、どちらの場合も auto_tag_rejections に除外を書いて、
// undo の記録を書く。行がもう違えば何も消さず、undo の結果は already_reverted である（research.md R-7）。
// 記録かタグが無ければ domain.ErrNotFound、取り消しを出さないか取り消し済みなら domain.ErrActivityNotUndoable。
func (s *ActivityStore) UndoAutoTag(ctx context.Context, entryID, tagID int64) (domain.ActivityEntry, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	defer func() { _ = tx.Rollback() }()
	target, err := readAutoTagUndoTarget(ctx, tx, entryID, tagID)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	if !target.entry.TagUndoOffered(target.tag) {
		return domain.ActivityEntry{}, domain.ErrActivityNotUndoable
	}
	removal, err := removeAutoTagAttachment(ctx, tx, target)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	now := time.Now()
	for _, key := range removal.rejectAt {
		if _, err := tx.ExecContext(ctx, `insert into auto_tag_rejections (content_key, tag_id, rejected_at)
			values (?, ?, ?) on conflict (content_key, tag_id) do update set rejected_at = excluded.rejected_at`,
			key, tagID, now.Unix()); err != nil {
			return domain.ActivityEntry{}, fmt.Errorf("cannot remember the rejected tag (tag=%d): %w", tagID, err)
		}
	}
	outcome := domain.ActivityAlreadyReverted
	if removal.reverted {
		outcome = domain.ActivityCompleted
	}
	c := &changes{}
	undoID, err := recordActivity(ctx, tx, c, domain.ActivityEntry{
		Kind: domain.ActivityUndo, Subject: target.entry.Subject, Status: domain.ActivityDone, Outcome: outcome,
		Detail: target.tag.Name, UndoesID: entryID, Decider: domain.ActivityByOwner,
	}, now)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	if _, err := tx.ExecContext(ctx, `update activity_entry_tags set undone_by = ? where entry_id = ? and tag_id = ?`,
		undoID, entryID, tagID); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot mark the auto-tagging tag undone (tag=%d): %w", tagID, err)
	}
	affected, err := touchAutoTagUndoneKeys(ctx, tx, removal.removed, now)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot undo the auto-tagging tag (tag=%d): %w", tagID, err)
	}
	if len(affected) > 0 {
		s.db.publishEvents(domain.AutoTagApplied{VideoIDs: affected})
	}
	return s.ActivityEntry(ctx, undoID)
}

// touchAutoTagUndoneKeys は取り消しで行を消した鍵 keys の編集日時を now に進め、その鍵の今の動画の id を返す。
func touchAutoTagUndoneKeys(ctx context.Context, tx *sql.Tx, keys []string, now time.Time) ([]int64, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if err := touchEditedAtForUserKeys(ctx, tx, keys, now); err != nil {
		return nil, err
	}
	var affected []int64
	for _, key := range keys {
		ids, err := videoIDsForUserKey(ctx, tx, key)
		if err != nil {
			return nil, err
		}
		affected = append(affected, ids...)
	}
	slices.Sort(affected)
	return slices.Compact(affected), nil
}
