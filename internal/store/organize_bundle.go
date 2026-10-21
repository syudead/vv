package store

// Drag bundles share version planning and capture all affected groups, playlists, copied user data and candidates for undo.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// Bundle expands existing bundles, makes the drop target representative, and rejects missing videos or expanded size limits atomically.
func (s *OrganizeStore) Bundle(ctx context.Context, videoIDs []int64, targetID int64) ([]byte, error) {
	ids := uniqueIDs(append([]int64{targetID}, videoIDs...))
	if len(ids) < 2 {
		return nil, domain.ErrTooFewVersions
	}
	var encoded []byte
	err := writeWithEvents(ctx, s.db, "bundle the videos", func(tx *sql.Tx) ([]domain.Event, error) {
		plan, err := planBundle(ctx, tx, ids, targetID)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrGroupTargetNotFound
		}
		if err != nil {
			return nil, err
		}
		if len(plan.members) > domain.MaxOrganizeBundleVideos {
			return nil, domain.ErrTooManyVersions
		}
		plan.representativeByOwner = true
		image, err := bundleImage(ctx, tx, plan)
		if err != nil {
			return nil, err
		}
		if err := image.captureBefore(ctx, tx); err != nil {
			return nil, err
		}
		lastGroup, err := maxGroupID(ctx, tx)
		if err != nil {
			return nil, err
		}
		outcome, err := applyBundle(ctx, tx, plan)
		if err != nil {
			return nil, err
		}
		if err := image.addCreatedBundle(ctx, tx, outcome, lastGroup); err != nil {
			return nil, err
		}
		if err := image.captureAfter(ctx, tx); err != nil {
			return nil, err
		}
		image.finishGroupsImage(outcome.changedGroups)
		image.RebuildFolders = true
		image.Videos = outcome.changedVideos
		if encoded, err = image.encode(); err != nil {
			return nil, err
		}
		c := &changes{}
		if err := recordOwnerBundle(ctx, tx, c, plan.members, plan.joined); err != nil {
			return nil, err
		}
		return append(outcome.events(), c.events()...), nil
	})
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// Capture parent tables first and whole affected groups/playlists; sorted candidate keys ensure all pruned pairs are covered.
func bundleImage(ctx context.Context, tx *sql.Tx, plan bundlePlan) (undoImage, error) {
	keys, err := json.Marshal(plan.absorbedKeys)
	if err != nil {
		return undoImage{}, fmt.Errorf("cannot build content keys: %w", err)
	}
	groups, err := queryInt64s(ctx, tx, `select group_id from group_items where content_key in (select value from json_each(?1))
		union select group_id from group_exclusions where content_key in (select value from json_each(?1))
		order by 1`, string(keys))
	if err != nil {
		return undoImage{}, fmt.Errorf("cannot read the groups: %w", err)
	}
	playlists, err := queryInt64s(ctx, tx, `select distinct playlist_id from playlist_items
		where content_key in (select value from json_each(?)) order by playlist_id`, string(keys))
	if err != nil {
		return undoImage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	var specs []imageScopeSpec
	for _, spec := range []imageScopeSpec{{"video_bundles", "id", nil}, {"video_bundle_members", "bundle_id", nil}} {
		for _, id := range plan.absorbedBundles {
			specs = append(specs, imageScopeSpec{spec.table, spec.column, id})
		}
	}
	for _, spec := range []imageScopeSpec{{"group_items", "group_id", nil}, {"group_exclusions", "group_id", nil}} {
		for _, id := range groups {
			specs = append(specs, imageScopeSpec{spec.table, spec.column, id})
		}
	}
	for _, id := range playlists {
		specs = append(specs, imageScopeSpec{"playlist_items", "playlist_id", id})
	}
	for _, key := range plan.members {
		specs = append(specs, imageScopeSpec{"video_version_candidates", "key_a", key})
	}
	var image undoImage
	if err := image.addScopes(specs); err != nil {
		return undoImage{}, err
	}
	return image, nil
}

// New rows have empty before images; include new-key references so later attachments cause undo conflicts instead of silent deletion.
func (image *undoImage) addCreatedBundle(ctx context.Context, tx *sql.Tx, outcome bundleOutcome, lastGroup int64) error {
	created, err := queryInt64s(ctx, tx, `select id from groups where id > ? order by id`, lastGroup)
	if err != nil {
		return fmt.Errorf("cannot read the created groups: %w", err)
	}
	for _, id := range created {
		scopes, err := groupReferenceScopes(id)
		if err != nil {
			return err
		}
		image.Scopes = append(image.Scopes, scopes...)
		if err := image.addScopes([]imageScopeSpec{{"group_exclusions", "group_id", id}}); err != nil {
			return err
		}
	}
	specs := []imageScopeSpec{{"video_bundles", "id", outcome.bundleID}, {"video_bundle_members", "bundle_id", outcome.bundleID}}
	for _, t := range userDataTables {
		specs = append(specs, imageScopeSpec{t.table, "content_key", outcome.userKey})
	}
	for _, table := range []string{"group_items", "group_exclusions", "playlist_items"} {
		specs = append(specs, imageScopeSpec{table, "content_key", outcome.userKey})
	}
	return image.addScopes(specs)
}

// AUTOINCREMENT makes every subsequently created group ID greater than this maximum.
func maxGroupID(ctx context.Context, tx *sql.Tx) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, `select coalesce(max(id), 0) from groups`).Scan(&id); err != nil {
		return 0, fmt.Errorf("cannot read the groups: %w", err)
	}
	return id, nil
}

func (image *undoImage) addScopes(specs []imageScopeSpec) error {
	for _, spec := range specs {
		scope, err := newImageScope(spec.table, spec.column, spec.key)
		if err != nil {
			return err
		}
		image.Scopes = append(image.Scopes, scope)
	}
	return nil
}
