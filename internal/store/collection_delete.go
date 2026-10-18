package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// DeleteCollections dissolves groups and deletes smart-folder trees in one transaction, ignoring missing IDs.
// Each removal shares the single DissolveGroup and DeleteSmartFolder path, so videos stay and the same events publish after commit.
func (s *CollectionStore) DeleteCollections(ctx context.Context, del domain.CollectionDelete) (domain.CollectionsDeleted, error) {
	var out domain.CollectionsDeleted
	err := writeWithEvents(ctx, s.db, "delete the collections", func(tx *sql.Tx) ([]domain.Event, error) {
		groups, err := existingGroupIDs(ctx, tx, normalizeIDs(slices.Clone(del.GroupIDs)))
		if err != nil {
			return nil, err
		}
		folders, err := existingIDs(ctx, tx, "smart_folders", normalizeIDs(slices.Clone(del.SmartFolderIDs)))
		if err != nil {
			return nil, err
		}
		changedGroups, err := dissolveGroupsAndRebuild(ctx, tx, groups)
		if err != nil {
			return nil, err
		}
		named, err := deleteSmartFolderTrees(ctx, tx, folders)
		if err != nil {
			return nil, err
		}
		out = domain.CollectionsDeleted{DissolvedGroups: len(groups), DeletedSmartFolders: len(folders)}
		return append(groupsChangedEvents(changedGroups), smartFoldersChangedEvents(named)...), nil
	})
	if err != nil {
		return domain.CollectionsDeleted{}, err
	}
	return out, nil
}

// Skip folders an earlier requested ancestor already removed by cascade.
func deleteSmartFolderTrees(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	var named []int64
	for _, id := range ids {
		folderNamed, err := deleteSmartFolderTree(ctx, tx, id)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		named = append(named, folderNamed...)
	}
	return named, nil
}
