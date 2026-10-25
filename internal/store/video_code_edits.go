package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// CodeSummary returns one row per user key of the listed library videos, in the order the videos were first
// listed: the key's shown video (a bundle's representative) and its current code.
func (s *VideoEditStore) CodeSummary(ctx context.Context, videoIDs []int64) ([]domain.VideoCodeRow, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the code rows: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keyByVideo, err := userKeysByVideoID(ctx, tx, videoIDs, true)
	if err != nil {
		return nil, err
	}
	keys := distinctUserKeys(videoIDs, keyByVideo)
	shown, err := shownVideosForUserKeys(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	infos, err := workInfoByUserKeys(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	rows := make([]domain.VideoCodeRow, 0, len(keys))
	for _, key := range keys {
		if video, ok := shown[key]; ok {
			rows = append(rows, domain.VideoCodeRow{Video: video, Code: infos[key].Code})
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot read the code rows: %w", err)
	}
	return rows, nil
}

// Read the owner list shape of each key's shown video in one statement.
func shownVideosForUserKeys(ctx context.Context, tx *sql.Tx, keys []string) (map[string]domain.Video, error) {
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build user keys: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select `+videoColumns(domain.AudienceOwner)+` from videos`+
		representativeJoin(domain.AudienceOwner)+` where videos.id in (select `+
		shownVideoForKeyExpr("k.value", domain.AudienceOwner)+` from json_each(?) k)`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the code row videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var videos []domain.Video
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read the code row videos: %w", err)
		}
		videos = append(videos, video)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the code row videos: %w", err)
	}
	if err := attachVersions(ctx, tx, domain.AudienceOwner, videos); err != nil {
		return nil, err
	}
	byKey := make(map[string]domain.Video, len(videos))
	for _, video := range videos {
		byKey[video.UserKey] = video
	}
	return byKey, nil
}

// ApplyCodes replaces only the code of each item's user key in one transaction and returns how many keys changed.
// A missing video, then a changed contentVersion, then a second item on one key fails as a VideoCodeEditAtError
// and writes nothing. Events match WorkInfoStore.Set; no activity is written.
func (s *VideoEditStore) ApplyCodes(ctx context.Context, items []domain.VideoCodeEdit) (int, error) {
	items, err := domain.NormalizeVideoCodeEdits(items)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot write the codes: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := codeEditKeys(ctx, tx, items)
	if err != nil {
		return 0, err
	}
	before, err := workInfoByUserKeys(ctx, tx, keys)
	if err != nil {
		return 0, err
	}
	final := make(map[string]domain.WorkInfo, len(keys))
	for i, key := range keys {
		info := before[key]
		info.Code = items[i].Code
		final[key] = info
	}
	edited, err := applyWorkInfo(ctx, tx, keys, final)
	if err != nil {
		return 0, err
	}
	if err := touchEditedAtForUserKeys(ctx, tx, edited, time.Now()); err != nil {
		return 0, err
	}
	events, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return 0, err
	}
	if err := s.db.commitEvents(ctx, tx, events...); err != nil {
		return 0, fmt.Errorf("cannot write the codes: %w", err)
	}
	return len(edited), nil
}

// codeEditKeys returns each item's user key in item order after checking every video exists, then every
// contentVersion, then that no two items share a key.
func codeEditKeys(ctx context.Context, tx *sql.Tx, items []domain.VideoCodeEdit) ([]string, error) {
	ids := make([]int64, len(items))
	for i, item := range items {
		ids[i] = item.VideoID
	}
	targets, err := videoKeysByID(ctx, tx, ids, true)
	if err != nil {
		return nil, err
	}
	for i, item := range items {
		if _, ok := targets[item.VideoID]; !ok {
			return nil, &domain.VideoCodeEditAtError{Index: i, Err: domain.ErrNotFound}
		}
	}
	for i, item := range items {
		if domain.ContentVersion(targets[item.VideoID].contentKey) != item.ContentVersion {
			return nil, &domain.VideoCodeEditAtError{Index: i, Err: domain.ErrContentChanged}
		}
	}
	keys := make([]string, len(items))
	seen := make(map[string]bool, len(items))
	for i, item := range items {
		key := targets[item.VideoID].userKey
		if seen[key] {
			return nil, &domain.VideoCodeEditAtError{Index: i, Err: domain.ErrDuplicateVideoCodeEdit}
		}
		seen[key] = true
		keys[i] = key
	}
	return keys, nil
}
