package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// lookupApply is what the work-info apply's lookups need from its merge: the person each fetched entry links and
// the people the apply created.
type lookupApply struct {
	people    []domain.FetchedPerson
	personIDs []int64
	created   map[int64]bool
}

// applyPersonLookups fills, for each lookup, the person its entries link when that is the person they matched at
// fetch time, or one this apply created for entries that matched none. A lookup whose plugin changed is skipped as
// stale and one linking another person as person_changed; a lookup whose entries link no one writes nothing, since
// the entries' own skips explain it. The result aligns with lookups: the person filled, or 0.
func applyPersonLookups(
	ctx context.Context, tx *sql.Tx, lookups []domain.FetchedPersonLookupApply, in lookupApply,
) ([]int64, []domain.SkippedWorkInfoValue, error) {
	filled := make([]int64, len(lookups))
	var skipped []domain.SkippedWorkInfoValue
	now := time.Now()
	for i, lookup := range lookups {
		field, linked, changed := lookupTarget(lookup, in)
		skip := func(reason domain.SkippedWorkInfoReason) {
			skipped = append(skipped, domain.SkippedWorkInfoValue{
				Field: field, Value: lookup.Fetched.Name, Reason: reason, Lookup: &lookup.Index,
			})
		}
		if linked == 0 {
			continue
		}
		err := checkFetchedPlugin(ctx, tx, lookup.PluginID, lookup.Digest, lookup.Generation)
		switch {
		case errors.Is(err, domain.ErrWorkInfoFetchStale):
			skip(domain.SkippedWorkInfoStale)
			continue
		case err != nil:
			return nil, nil, err
		case changed:
			skip(domain.SkippedWorkInfoPersonChanged)
			continue
		}
		values, _, err := fillPersonFromPlugin(ctx, tx, linked, lookup.Fetched, lookup.Items, false, now)
		if err != nil {
			return nil, nil, err
		}
		for _, value := range values {
			skipped = append(skipped, domain.SkippedWorkInfoValue{
				Field: field, Value: value.Value, Reason: value.Reason, Lookup: &lookup.Index, Item: value.Item,
				HolderName: value.HolderName,
			})
		}
		filled[i] = linked
	}
	return filled, skipped, nil
}

// lookupTarget returns the role field of the lookup's first linked entry, the person it links, and whether that is
// another person than the entries matched at fetch time.
func lookupTarget(lookup domain.FetchedPersonLookupApply, in lookupApply) (domain.WorkInfoField, int64, bool) {
	var field domain.WorkInfoField
	var linked int64
	changed := false
	for _, entry := range lookup.Entries {
		if entry < 0 || entry >= len(in.personIDs) || in.personIDs[entry] == 0 {
			continue
		}
		id := in.personIDs[entry]
		if linked == 0 {
			field, linked = domain.PersonField(in.people[entry].Role), id
		}
		if id != linked || lookup.MatchedID != 0 && id != lookup.MatchedID || lookup.MatchedID == 0 && !in.created[id] {
			changed = true
		}
	}
	return field, linked, changed
}
