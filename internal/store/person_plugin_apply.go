package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ApplyPersonPluginFetch rechecks the plugin's approval, digest and generation, then fills the person's empty profile
// fields and appends missing aliases and external IDs in one transaction. A plugin changed since the fetch returns
// ErrWorkInfoFetchStale and a missing person ErrNotFound, both without writes. The face is the caller's, after commit.
func (s *CreditStore) ApplyPersonPluginFetch(
	ctx context.Context, apply domain.PersonPluginApply,
) (domain.PersonPluginApplied, error) {
	t := creditTablesByKind[domain.CreditPerson]
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.PersonPluginApplied{}, fmt.Errorf("cannot apply the fetched person: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := checkFetchedPlugin(ctx, tx, apply.PluginID, apply.Digest, apply.Generation); err != nil {
		return domain.PersonPluginApplied{}, err
	}
	skipped, matched, err := fillPersonFromPlugin(ctx, tx, apply.PersonID, apply.Fetched, apply.Items, time.Now())
	if err != nil {
		return domain.PersonPluginApplied{}, err
	}
	person, err := creditByID(ctx, tx, t, apply.PersonID)
	if err != nil {
		return domain.PersonPluginApplied{}, err
	}
	if err := s.db.commitMatchChangeIf(ctx, tx, matched); err != nil {
		return domain.PersonPluginApplied{}, fmt.Errorf("cannot apply the fetched person: %w", err)
	}
	if skipped == nil {
		skipped = []domain.SkippedPersonValue{}
	}
	return domain.PersonPluginApplied{Person: person, Skipped: skipped}, nil
}

// fillPersonFromPlugin writes the selected items of a fetched person to the stored person under FillPersonItems'
// rules, returning the values left out and whether the person's matching keys changed; a missing person is
// ErrNotFound. The person page's apply and the work-info apply's lookups share it.
func fillPersonFromPlugin(
	ctx context.Context, tx *sql.Tx, personID int64, fetched domain.FetchedPluginPerson,
	items []domain.PersonPluginItem, today time.Time,
) ([]domain.SkippedPersonValue, bool, error) {
	t := creditTablesByKind[domain.CreditPerson]
	current, err := creditByID(ctx, tx, t, personID)
	if err != nil {
		return nil, false, err
	}
	holders, err := externalIDHolderNames(ctx, tx, t, personID, fetched.ExternalIDs)
	if err != nil {
		return nil, false, err
	}
	filled := domain.FillPersonItems(domain.PersonItemFill{
		Current: current, Fetched: fetched, Items: items, Holders: holders, Today: today,
	})
	next := domain.CreditInput{
		Name: current.Name, Aliases: filled.Aliases, ExternalIDs: filled.ExternalIDs, Profile: filled.Profile,
	}
	changes := creditChanges(t, current, next)
	patch := domain.CreditPatch{ExternalIDs: &next.ExternalIDs}
	if err := applyCreditChanges(ctx, tx, t, personID, next, patch, changes); err != nil {
		return nil, false, err
	}
	return filled.Skipped, changes.matched(), nil
}

// externalIDHolderNames maps each normalized ID that an entity other than selfID holds to that entity's name.
func externalIDHolderNames(
	ctx context.Context, tx *sql.Tx, t creditTables, selfID int64, ids []domain.ExternalID,
) (map[domain.ExternalID]string, error) {
	out := map[domain.ExternalID]string{}
	for _, raw := range ids {
		id, ok := domain.NormalizeExternalID(raw)
		if !ok {
			continue
		}
		holder, found, err := externalIDHolder(ctx, tx, t, id, selfID)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		var name string
		if err := tx.QueryRowContext(ctx, t.q(`select name from {names} where {id} = ? and canonical = 1`),
			holder).Scan(&name); err != nil {
			return nil, fmt.Errorf("cannot read the name of %s %d: %w", t.kind, holder, err)
		}
		out[id] = name
	}
	return out, nil
}
