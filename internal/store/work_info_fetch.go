package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Apply fetched title, tentative tags and details in one transaction through shared private helpers.

// ApplyFetchedWorkInfo revalidates plugin approval, digest and generation, rematches credits inside the transaction, and creates only entities retained by the merge.
func (s *OverrideStore) ApplyFetchedWorkInfo(
	ctx context.Context, apply domain.FetchedWorkInfoApply,
) (domain.FetchedWorkInfoApplied, error) {
	var title string
	if apply.Title != nil {
		normalized, _, err := domain.NormalizeDisplayName(*apply.Title)
		if err != nil {
			return domain.FetchedWorkInfoApplied{}, err
		}
		title = normalized
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, fmt.Errorf("cannot apply the fetched work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := checkFetchedPlugin(ctx, tx, apply); err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	contentKeys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{apply.VideoID})
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	userKeys, err := userKeysForVideoIDs(ctx, tx, []int64{apply.VideoID})
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	if len(contentKeys) == 0 || len(userKeys) == 0 {
		return domain.FetchedWorkInfoApplied{}, domain.ErrNotFound
	}

	now := time.Now()
	var touched []string
	if apply.Title != nil {
		if touched, _, err = writeChangedDisplayName(ctx, tx, contentKeys[0], title, now); err != nil {
			return domain.FetchedWorkInfoApplied{}, err
		}
	}
	out := domain.FetchedWorkInfoApplied{RejectedTags: []string{}, Skipped: []domain.SkippedWorkInfoValue{}}
	changed := []int64{apply.VideoID}
	if len(apply.Tags) > 0 {
		// Fetched tags are tentative and must not promote automatic suggestions.
		out.RejectedTags, err = writeVideoTagsByName(ctx, tx, userKeys[:1], domain.VideoTagsAdd, apply.Tags,
			tagWrite{tentative: true, keepSuggested: true}, now)
		if err != nil {
			return domain.FetchedWorkInfoApplied{}, err
		}
		// Shared user keys make tag edits visible to every bundle member.
		if changed, err = videoIDsForUserKeys(ctx, tx, userKeys[:1]); err != nil {
			return domain.FetchedWorkInfoApplied{}, err
		}
	}
	details, err := applyFetchedDetails(ctx, tx, userKeys[0], apply)
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	edited := details.edited
	out.Skipped = append(out.Skipped, details.skipped...)
	out.PersonIDs = details.personIDs
	members, err := contentKeysForUserKeys(ctx, tx, edited)
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	touched = append(touched, members...)
	slices.Sort(touched)
	if err := touchEditedAt(ctx, tx, slices.Compact(touched), now); err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	if out.Video, err = ownerVideoWithWorkInfo(ctx, tx, apply.VideoID); err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	workInfoEvents, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	c := &changes{}
	if err := recordOwnerVideoActivity(ctx, tx, c, domain.ActivityWorkInfoApply,
		ownerVideoChanges(contentKeys[:1], ""), now); err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	c.matchesChanged()
	events := append(appliedWorkInfoEvents(apply.VideoID, changed), workInfoEvents...)
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.FetchedWorkInfoApplied{}, fmt.Errorf("cannot apply the fetched work information: %w", err)
	}
	return out, nil
}

type appliedDetails struct {
	edited    []string
	skipped   []domain.SkippedWorkInfoValue
	personIDs []int64
}

// Rematch credits in the write transaction so entities created since fetch can be reused, then fill the linked
// people's empty profile fields.
func applyFetchedDetails(
	ctx context.Context, tx *sql.Tx, userKey string, apply domain.FetchedWorkInfoApply,
) (appliedDetails, error) {
	if len(apply.DetailFields) == 0 {
		return appliedDetails{}, nil
	}
	merge, err := mergeFetchedDetails(ctx, tx, userKey, apply)
	if err != nil {
		return appliedDetails{}, err
	}
	final, _, err := domain.NormalizeWorkInfo(merge.Info)
	if err != nil {
		return appliedDetails{}, fmt.Errorf("cannot merge the fetched work information: %w", err)
	}
	out := appliedDetails{skipped: merge.Skipped, personIDs: make([]int64, len(apply.Details.People))}
	for i, at := range merge.PersonLinks {
		if at >= 0 {
			out.personIDs[i] = merge.Info.People[at].Person.ID
		}
	}
	profileSkipped, err := fillFetchedPersonProfiles(ctx, tx, apply.Details.People, out.personIDs)
	if err != nil {
		return appliedDetails{}, err
	}
	out.skipped = append(out.skipped, profileSkipped...)
	if out.edited, err = applyWorkInfo(ctx, tx, []string{userKey}, map[string]domain.WorkInfo{userKey: final}); err != nil {
		return appliedDetails{}, err
	}
	return out, nil
}

// mergeFetchedDetails lays the fetched details over the stored ones and creates the new entities the merge kept,
// filling their IDs into the merged work information.
func mergeFetchedDetails(
	ctx context.Context, tx *sql.Tx, userKey string, apply domain.FetchedWorkInfoApply,
) (domain.WorkInfoMerge, error) {
	stored, err := workInfoByUserKeys(ctx, tx, []string{userKey})
	if err != nil {
		return domain.WorkInfoMerge{}, err
	}
	people := creditTablesByKind[domain.CreditPerson]
	organizations := creditTablesByKind[domain.CreditOrganization]
	personMatches, err := matchCredits(ctx, tx, people, domain.PersonMatchEntries(apply.Details.People))
	if err != nil {
		return domain.WorkInfoMerge{}, err
	}
	organizationMatches, err := matchCredits(ctx, tx, organizations,
		domain.OrganizationMatchEntries(apply.Details.Organizations))
	if err != nil {
		return domain.WorkInfoMerge{}, err
	}
	merge := domain.MergeFetchedWorkInfo(domain.WorkInfoMergeInput{
		Stored: stored[userKey], Fetched: apply.Details, Fields: apply.DetailFields,
		PersonMatches: personMatches, OrganizationMatches: organizationMatches,
	})
	err = createMergedCredits(ctx, tx, people, merge.NewPeople, func(at int, id int64) {
		merge.Info.People[at].Person.ID = id
	})
	if err != nil {
		return domain.WorkInfoMerge{}, err
	}
	err = createMergedCredits(ctx, tx, organizations, merge.NewOrganizations, func(at int, id int64) {
		merge.Info.Organizations[at].Organization.ID = id
	})
	if err != nil {
		return domain.WorkInfoMerge{}, err
	}
	return merge, nil
}

// fillFetchedPersonProfiles fills each linked person's empty profile fields in fetched order, so a person named twice
// keeps the first valid value, and returns the values that would have made a profile invalid.
func fillFetchedPersonProfiles(
	ctx context.Context, tx *sql.Tx, fetched []domain.FetchedPerson, personIDs []int64,
) ([]domain.SkippedWorkInfoValue, error) {
	index := map[int64]int{}
	var ids []int64
	for i, id := range personIDs {
		if _, seen := index[id]; id != 0 && !seen && !fetched[i].Profile.Equal(domain.PersonProfile{}) {
			index[id] = len(ids)
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build person ids: %w", err)
	}
	people := make([]domain.Credit, len(ids))
	if err := addPersonProfiles(ctx, tx, people, index, string(encoded)); err != nil {
		return nil, err
	}
	stored := make([]domain.PersonProfile, len(ids))
	for i := range people {
		stored[i] = people[i].Profile
	}
	var skipped []domain.SkippedWorkInfoValue
	now := time.Now()
	for i, id := range personIDs {
		at, ok := index[id]
		if !ok {
			continue
		}
		var invalid []domain.SkippedWorkInfoValue
		people[at].Profile, invalid = domain.FillFetchedPersonProfile(people[at].Profile, fetched[i], now)
		skipped = append(skipped, invalid...)
	}
	for i, id := range ids {
		if people[i].Profile.Equal(stored[i]) {
			continue
		}
		if err := writePersonProfile(ctx, tx, id, people[i].Profile); err != nil {
			return nil, err
		}
	}
	return skipped, nil
}

// Names and external IDs are normalized and ownership checked before creating retained merged credits.
func createMergedCredits(
	ctx context.Context, tx *sql.Tx, t creditTables, created []domain.NewCredit, link func(at int, id int64),
) error {
	for _, credit := range created {
		id, err := insertCredit(ctx, tx, t, credit.Input, nil)
		if err != nil {
			return err
		}
		for _, at := range credit.Links {
			link(at, id)
		}
	}
	return nil
}

// Notify the target first, then each other video sharing changed tag keys exactly once.
func appliedWorkInfoEvents(videoID int64, changed []int64) []domain.Event {
	events := []domain.Event{domain.VideoOverrideChanged{VideoID: videoID}}
	for _, id := range changed {
		if id != videoID {
			events = append(events, domain.VideoOverrideChanged{VideoID: id})
		}
	}
	return events
}

// Generation detects replacement or revoke-and-reapprove even when module bytes and digest are unchanged.
func checkFetchedPlugin(ctx context.Context, tx *sql.Tx, apply domain.FetchedWorkInfoApply) error {
	var (
		current           string
		problem, approved sql.NullString
		generation        int64
	)
	err := tx.QueryRowContext(ctx,
		`select digest, problem, approved_digest, generation from work_info_plugins where id = ?`, apply.PluginID,
	).Scan(&current, &problem, &approved, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrWorkInfoFetchStale
	}
	if err != nil {
		return fmt.Errorf("cannot read the work-info plugin: %w", err)
	}
	state := domain.WorkInfoPluginStateOf(domain.WorkInfoPluginProblem(problem.String), current, approved.String)
	if state != domain.WorkInfoPluginApproved || current != apply.Digest || generation != apply.Generation {
		return domain.ErrWorkInfoFetchStale
	}
	return nil
}
