package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Summary reads the editable values of the listed registered videos in one snapshot, counting shared user keys once.
func (s *VideoEditStore) Summary(ctx context.Context, videoIDs []int64) (domain.VideoEditSummary, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VideoEditSummary{}, fmt.Errorf("cannot read the edit summary: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := userKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return domain.VideoEditSummary{}, err
	}
	summary := domain.VideoEditSummary{
		Total: len(keys), Tags: []domain.TagSummaryItem{}, People: []domain.VideoEditPersonCount{},
		Organizations: []domain.VideoEditOrganizationCount{}, Dates: []domain.VideoEditDateCount{},
	}
	if len(keys) == 0 {
		return summary, nil
	}
	if summary.Public, err = countPublicUserKeys(ctx, tx, keys); err != nil {
		return domain.VideoEditSummary{}, err
	}
	if summary.Tags, err = tagSummaryItems(ctx, tx, videoIDs); err != nil {
		return domain.VideoEditSummary{}, err
	}
	infos, err := workInfoByUserKeys(ctx, tx, keys)
	if err != nil {
		return domain.VideoEditSummary{}, err
	}
	values := make([]domain.WorkInfo, 0, len(infos))
	for _, key := range keys {
		if info, ok := infos[key]; ok {
			values = append(values, info)
		}
	}
	summary.People, summary.Organizations, summary.Dates = domain.SummarizeWorkInfos(values)
	if err := tx.Commit(); err != nil {
		return domain.VideoEditSummary{}, fmt.Errorf("cannot read the edit summary: %w", err)
	}
	return summary, nil
}

func countPublicUserKeys(ctx context.Context, tx *sql.Tx, keys []string) (int, error) {
	encoded, err := json.Marshal(keys)
	if err != nil {
		return 0, fmt.Errorf("cannot build user keys: %w", err)
	}
	var count int
	if err := tx.QueryRowContext(ctx,
		`select count(*) from public_videos where content_key in (select value from json_each(?))`, string(encoded),
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("cannot count the public videos: %w", err)
	}
	return count, nil
}

// Apply normalizes edit and applies every part to the listed registered videos in one transaction, recording the
// same activity and publishing the same events as the single-part writes. A missing tag or credit, or a role over
// its limit, writes nothing; missing videos are skipped.
func (s *VideoEditStore) Apply(ctx context.Context, videoIDs []int64, edit domain.VideoEdit) (domain.VideoEditResult, error) {
	edit, err := domain.NormalizeVideoEdit(edit)
	if err != nil {
		return domain.VideoEditResult{}, err
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.VideoEditResult{}, fmt.Errorf("cannot apply the edit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	added, err := resolveEditTags(ctx, tx, edit)
	if err != nil {
		return domain.VideoEditResult{}, err
	}
	if err := checkCreditEditRefs(ctx, tx, edit.Credits); err != nil {
		return domain.VideoEditResult{}, err
	}
	keyByVideo, err := userKeysByVideoID(ctx, tx, videoIDs, true)
	if err != nil {
		return domain.VideoEditResult{}, err
	}
	w := &videoEditWrite{tx: tx, c: &changes{}, keys: distinctUserKeys(videoIDs, keyByVideo), now: time.Now(), changed: map[string]bool{}}
	events, err := w.workInfo(ctx, edit)
	if err != nil {
		return domain.VideoEditResult{}, err
	}
	if err := w.tags(ctx, added, edit.TagsRemove); err != nil {
		return domain.VideoEditResult{}, err
	}
	result := domain.VideoEditResult{Tags: added}
	if result.ContentKeys, err = w.visibility(ctx, edit.Public); err != nil {
		return domain.VideoEditResult{}, err
	}
	if err := s.db.commitWith(ctx, tx, w.c, events...); err != nil {
		return domain.VideoEditResult{}, fmt.Errorf("cannot apply the edit: %w", err)
	}
	for _, key := range keyByVideo {
		if w.changed[key] {
			result.Applied++
		}
	}
	return result, nil
}

// videoEditWrite applies the parts of one bulk edit to the target user keys, collecting the keys any part changed.
type videoEditWrite struct {
	tx      *sql.Tx
	c       *changes
	keys    []string
	now     time.Time
	changed map[string]bool
}

func (w *videoEditWrite) mark(keys []string) {
	for _, key := range keys {
		w.changed[key] = true
	}
}

func (w *videoEditWrite) workInfo(ctx context.Context, edit domain.VideoEdit) ([]domain.Event, error) {
	if !edit.ChangesWorkInfo() {
		return nil, nil
	}
	before, err := workInfoByUserKeys(ctx, w.tx, w.keys)
	if err != nil {
		return nil, err
	}
	final := make(map[string]domain.WorkInfo, len(w.keys))
	for _, key := range w.keys {
		if final[key], err = edit.ApplyToWorkInfo(before[key]); err != nil {
			return nil, err
		}
	}
	edited, err := applyWorkInfo(ctx, w.tx, w.keys, final)
	if err != nil {
		return nil, err
	}
	if err := touchEditedAtForUserKeys(ctx, w.tx, edited, w.now); err != nil {
		return nil, err
	}
	w.mark(edited)
	return workInfoChangedEvents(ctx, w.tx, edited)
}

func (w *videoEditWrite) tags(ctx context.Context, added []domain.TagRef, removed []int64) error {
	for _, ref := range added {
		changed, err := attachTagToUserKeys(ctx, w.tx, w.keys, ref.ID, w.now)
		if err != nil {
			return err
		}
		if err := recordTagChange(ctx, w.tx, w.c, domain.ActivityTagAttach, changed, ref.ID, w.now); err != nil {
			return err
		}
		w.mark(changed)
	}
	for _, id := range removed {
		changed, err := detachTagFromUserKeys(ctx, w.tx, w.keys, id, w.now)
		if err != nil {
			return err
		}
		if err := recordTagChange(ctx, w.tx, w.c, domain.ActivityTagDetach, changed, id, w.now); err != nil {
			return err
		}
		w.mark(changed)
	}
	return nil
}

// visibility returns every content key of the targets when it makes them private, so callers can end guest streams.
func (w *videoEditWrite) visibility(ctx context.Context, public *bool) ([]string, error) {
	if public == nil {
		return nil, nil
	}
	changed, err := setPublicForUserKeys(ctx, w.tx, w.keys, *public, w.now)
	if err != nil {
		return nil, err
	}
	w.mark(changed)
	w.c.matchesChanged()
	if *public {
		return nil, nil
	}
	return contentKeysForUserKeys(ctx, w.tx, w.keys)
}

// Resolve tags.add in request order without repeats, creating tags named but missing, and check that every
// removed tag exists and none of them is also added.
func resolveEditTags(ctx context.Context, tx *sql.Tx, edit domain.VideoEdit) ([]domain.TagRef, error) {
	refs := []domain.TagRef{}
	added := make(map[int64]bool, len(edit.TagsAdd))
	for _, input := range edit.TagsAdd {
		var ref domain.TagRef
		var err error
		if input.ID != nil {
			ref, err = tagRefByID(ctx, tx, *input.ID)
		} else {
			ref, _, err = findOrCreateTag(ctx, tx, *input.Name)
		}
		if err != nil {
			return nil, err
		}
		if !added[ref.ID] {
			added[ref.ID] = true
			refs = append(refs, ref)
		}
	}
	if err := domain.CheckTagRemovals(added, edit.TagsRemove); err != nil {
		return nil, err
	}
	for _, id := range edit.TagsRemove {
		if _, err := tagRefByID(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	return refs, nil
}

// Report the first missing credit at its credits[i].add[j] or credits[i].remove[j] field, in request order.
func checkCreditEditRefs(ctx context.Context, tx *sql.Tx, edits []domain.CreditEdit) error {
	byKind := map[domain.CreditKind][]int64{}
	_ = domain.EachCreditEditID(edits, func(kind domain.CreditKind, id int64, _ string) error {
		byKind[kind] = append(byKind[kind], id)
		return nil
	})
	existing := make(map[domain.CreditKind]map[int64]bool, len(byKind))
	for kind, ids := range byKind {
		found, err := existingCreditIDs(ctx, tx, creditTablesByKind[kind], ids)
		if err != nil {
			return err
		}
		existing[kind] = found
	}
	return domain.EachCreditEditID(edits, func(kind domain.CreditKind, id int64, field string) error {
		if !existing[kind][id] {
			return &domain.CreditNotFoundError{Kind: kind, ID: id, Field: field}
		}
		return nil
	})
}

// Keep the targets' first-occurrence order so writes and activity follow the request.
func distinctUserKeys(videoIDs []int64, keyByVideo map[int64]string) []string {
	seen := make(map[string]bool, len(keyByVideo))
	keys := make([]string, 0, len(keyByVideo))
	for _, id := range videoIDs {
		if key, ok := keyByVideo[id]; ok && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}
