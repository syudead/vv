package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// Preview matching uses Match; applying matches reruns matchCredits inside the write transaction.

// Match prefers external IDs, then folded names and aliases; multiple owners are ambiguous and unmatched entries are new.
func (s *CreditStore) Match(ctx context.Context, kind domain.CreditKind, entries []domain.CreditMatchEntry) ([]domain.CreditMatch, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return nil, err
	}
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot start matching the %s entities: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	matches, err := matchCredits(ctx, tx, t, entries)
	if err != nil {
		return nil, err
	}
	return matches, tx.Commit()
}

// Batch each lookup once regardless of entry count, then resolve results in input order.
func matchCredits(ctx context.Context, tx *sql.Tx, t creditTables, entries []domain.CreditMatchEntry) ([]domain.CreditMatch, error) {
	idHolders, err := externalIDHolderMap(ctx, tx, t, entries)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(entries))
	for i, entry := range entries {
		keys[i] = domain.FoldForMatch(entry.Name)
	}
	keyHolders, err := foldedNameHolderMap(ctx, tx, t, keys)
	if err != nil {
		return nil, err
	}
	holders := make([][]int64, len(entries))
	var linked []int64
	for i, entry := range entries {
		holders[i] = entryHolders(entry, idHolders)
		if len(holders[i]) == 0 {
			holders[i] = keyHolders[keys[i]]
		}
		if len(holders[i]) == 1 {
			linked = append(linked, holders[i][0])
		}
	}
	names, err := canonicalCreditNames(ctx, tx, t, linked)
	if err != nil {
		return nil, err
	}
	matches := make([]domain.CreditMatch, len(entries))
	for i, found := range holders {
		switch len(found) {
		case 0:
			matches[i] = domain.CreditMatch{State: domain.CreditMatchNew}
		case 1:
			// Only a person can lack a canonical name, which makes it provisional.
			name, named := names[found[0]]
			ref := &domain.CreditRef{ID: found[0], Name: name, Provisional: !named}
			matches[i] = domain.CreditMatch{State: domain.CreditMatchExisting, Credit: ref}
		default:
			matches[i] = domain.CreditMatch{State: domain.CreditMatchAmbiguous}
		}
	}
	return matches, nil
}

func entryHolders(entry domain.CreditMatchEntry, idHolders map[domain.ExternalID]int64) []int64 {
	var holders []int64
	for _, id := range entry.ExternalIDs {
		holder, found := idHolders[id]
		if found && !slices.Contains(holders, holder) {
			holders = append(holders, holder)
		}
	}
	return holders
}

type matchExternalID struct {
	Source string `json:"s"`
	Value  string `json:"v"`
}

// External IDs are unique per kind and use the (source, value) index.
func externalIDHolderMap(ctx context.Context, tx *sql.Tx, t creditTables, entries []domain.CreditMatchEntry) (map[domain.ExternalID]int64, error) {
	var ids []matchExternalID
	for _, entry := range entries {
		for _, id := range entry.ExternalIDs {
			ids = append(ids, matchExternalID{Source: id.Source, Value: id.Value})
		}
	}
	holders := make(map[domain.ExternalID]int64)
	if len(ids) == 0 {
		return holders, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot encode the external IDs of %s entries: %w", t.kind, err)
	}
	err = eachCreditRow(ctx, tx, t.q(`select x.{id}, x.source, x.value from json_each(?) j
		 join {ext} x on x.source = json_extract(j.value, '$.s') and x.value = json_extract(j.value, '$.v')`),
		string(encoded), func(id int64, source, value string) {
			holders[domain.ExternalID{Source: source, Value: value}] = id
		})
	if err != nil {
		return nil, fmt.Errorf("cannot look up the external IDs of %s entries: %w", t.kind, err)
	}
	return holders, nil
}

// Empty folded names never match; return deduplicated entity IDs in order.
func foldedNameHolderMap(ctx context.Context, tx *sql.Tx, t creditTables, keys []string) (map[string][]int64, error) {
	wanted := make([]string, 0, len(keys))
	for _, key := range keys {
		if key != "" {
			wanted = append(wanted, key)
		}
	}
	holders := make(map[string][]int64)
	if len(wanted) == 0 {
		return holders, nil
	}
	encoded, err := json.Marshal(wanted)
	if err != nil {
		return nil, fmt.Errorf("cannot encode the names of %s entries: %w", t.kind, err)
	}
	err = eachCreditRow(ctx, tx, t.q(`select distinct {id}, search_key, '' from {names}
		 where search_key in (select value from json_each(?)) order by {id}`),
		string(encoded), func(id int64, key, _ string) {
			holders[key] = append(holders[key], id)
		})
	if err != nil {
		return nil, fmt.Errorf("cannot match the names of %s entries: %w", t.kind, err)
	}
	return holders, nil
}

func canonicalCreditNames(ctx context.Context, tx *sql.Tx, t creditTables, ids []int64) (map[int64]string, error) {
	names := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot encode %s ids: %w", t.kind, err)
	}
	err = eachCreditRow(ctx, tx, t.q(`select {id}, name, '' from {names}
		 where canonical = 1 and {id} in (select value from json_each(?))`),
		string(encoded), func(id int64, name, _ string) { names[id] = name })
	if err != nil {
		return nil, fmt.Errorf("cannot read the names of %s entities: %w", t.kind, err)
	}
	return names, nil
}
