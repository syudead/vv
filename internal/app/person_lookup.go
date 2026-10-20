package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A work-info fetch plans person plugin lookups; the page runs each one and its result stays with the fetch until
// the apply (docs/design-docs/person-plugins.md).

// keptLookup is a lookup's result; the profile keeps only the values the comparison shows.
type keptLookup struct {
	person domain.FetchedPluginPerson
	image  *domain.WorkInfoCover
}

func (k *keptLookup) hasItem(item domain.PersonPluginItem) bool {
	if item == domain.PersonItemImage {
		return k.image != nil
	}
	return k.person.HasItem(item)
}

func (p *WorkInfoPlugins) planPersonLookups(
	ctx context.Context, people []domain.FetchedPerson, matches []domain.CreditMatch,
) (domain.PersonLookupPlan, error) {
	if len(people) == 0 {
		return domain.PlanPersonLookups(nil, nil, nil), nil
	}
	plugins, err := p.store.ListWorkInfoPlugins(ctx)
	if err != nil {
		return domain.PersonLookupPlan{}, err
	}
	return domain.PlanPersonLookups(people, matches, plugins), nil
}

// RunPersonLookup runs planned lookup index of a fetch once and keeps its result with the fetch; a lookup that has a
// result answers it without running. An unknown or expired fetch returns ErrWorkInfoFetchStale, another video's fetch
// or an index outside the plan ErrNotFound, and a plugin replaced, revoked or deleted since the fetch
// ErrPersonLookupPluginChanged; other errors follow run, and a failed lookup keeps nothing.
func (p *WorkInfoPlugins) RunPersonLookup(
	ctx context.Context, videoID int64, fetchID string, index int,
) (domain.PersonLookupResult, error) {
	if p.people == nil {
		return domain.PersonLookupResult{}, errors.New("person plugin lookups are not configured")
	}
	lookup, kept, err := p.fetches.lookup(fetchID, videoID, index, p.now())
	if err != nil {
		return domain.PersonLookupResult{}, err
	}
	if kept == nil {
		if kept, err = p.runLookup(ctx, lookup); err != nil {
			return domain.PersonLookupResult{}, err
		}
		if kept, err = p.fetches.keepLookup(fetchID, index, kept, p.now()); err != nil {
			return domain.PersonLookupResult{}, err
		}
	}
	result := domain.PersonLookupResult{Person: kept.person, HasImage: kept.image != nil}
	if lookup.MatchedID == 0 {
		return result, nil
	}
	current, err := p.people.Get(ctx, domain.CreditPerson, lookup.MatchedID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
	case err != nil:
		return domain.PersonLookupResult{}, err
	default:
		result.Current = &current
	}
	return result, nil
}

// runLookup runs the lookup only on the plugin snapshot the plan saw, checked before the run and again after it since
// the owner may replace, revoke or delete the plugin while it runs; turning automatic lookups off since does not stop
// it.
func (p *WorkInfoPlugins) runLookup(ctx context.Context, lookup domain.PersonLookup) (*keptLookup, error) {
	if err := p.checkLookupPlugin(ctx, lookup); err != nil {
		return nil, err
	}
	// A lookup keeps one image under the fetch's budget, so it fetches only the first.
	ran, result, err := p.run(ctx, lookup.PluginID, domain.WorkInfoPluginKindPerson, domain.WorkInfoRunRequest{
		Operation: domain.WorkInfoFetch, Source: lookup.Source, ID: lookup.ID, ImageLimit: 1,
	})
	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrWorkInfoPluginNotApproved),
		errors.Is(err, domain.ErrWorkInfoPluginKindMismatch), errors.Is(err, domain.ErrWorkInfoRunInvalid):
		// The plugin changed between the check and the run.
		return nil, domain.ErrPersonLookupPluginChanged
	case err != nil:
		return nil, err
	case !lookupPluginUnchanged(ran, lookup):
		return nil, domain.ErrPersonLookupPluginChanged
	}
	if err := p.checkLookupPlugin(ctx, lookup); err != nil {
		return nil, err
	}
	result.Person.Profile = domain.ShownFetchedProfile(result.Person.Profile, p.now())
	kept := &keptLookup{person: result.Person}
	if images := fetchedImages(result.PersonPluginImages); len(images) > 0 {
		kept.image = &images[0]
	}
	return kept, nil
}

// checkLookupPlugin returns ErrPersonLookupPluginChanged unless the stored plugin is the approved snapshot the plan
// saw.
func (p *WorkInfoPlugins) checkLookupPlugin(ctx context.Context, lookup domain.PersonLookup) error {
	plugin, err := p.store.WorkInfoPlugin(ctx, lookup.PluginID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return domain.ErrPersonLookupPluginChanged
	case err != nil:
		return err
	case !lookupPluginUnchanged(plugin, lookup):
		return domain.ErrPersonLookupPluginChanged
	}
	return nil
}

func lookupPluginUnchanged(plugin domain.WorkInfoPlugin, lookup domain.PersonLookup) bool {
	return plugin.State() == domain.WorkInfoPluginApproved && plugin.Digest == lookup.Digest &&
		plugin.Generation == lookup.Generation
}

// PersonLookupImage returns ErrNotFound for an unknown or expired fetch, another video's fetch or an index outside
// the plan, and ErrPersonImageNotFound for a lookup without a kept image.
func (p *WorkInfoPlugins) PersonLookupImage(videoID int64, fetchID string, index int) (domain.WorkInfoCover, error) {
	f := p.fetches
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(p.now())
	fetch, ok := f.entries[fetchID]
	switch {
	case !ok || fetch.videoID != videoID || index < 0 || index >= len(fetch.lookupRuns):
		return domain.WorkInfoCover{}, domain.ErrNotFound
	case fetch.lookupRuns[index] == nil || fetch.lookupRuns[index].image == nil:
		return domain.WorkInfoCover{}, domain.ErrPersonImageNotFound
	}
	return *fetch.lookupRuns[index].image, nil
}

func (f *workInfoFetches) lookup(
	id string, videoID int64, index int, now time.Time,
) (domain.PersonLookup, *keptLookup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	fetch, ok := f.entries[id]
	switch {
	case !ok:
		return domain.PersonLookup{}, nil, domain.ErrWorkInfoFetchStale
	case fetch.videoID != videoID, index < 0, index >= len(fetch.lookups.Lookups):
		return domain.PersonLookup{}, nil, domain.ErrNotFound
	}
	return fetch.lookups.Lookups[index], fetch.lookupRuns[index], nil
}

// keepLookup keeps the first result of a lookup, so concurrent runs answer the same one, and returns
// ErrWorkInfoFetchStale when the fetch was applied, expired or evicted during the run, or when the result's image
// evicts its own fetch from the budget.
func (f *workInfoFetches) keepLookup(id string, index int, kept *keptLookup, now time.Time) (*keptLookup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	fetch, ok := f.entries[id]
	if !ok {
		return nil, domain.ErrWorkInfoFetchStale
	}
	if fetch.lookupRuns[index] == nil {
		fetch.lookupRuns[index] = kept
		f.trimCovers()
		if f.entries[id] != fetch {
			return nil, domain.ErrWorkInfoFetchStale
		}
	}
	return fetch.lookupRuns[index], nil
}

// validLookups requires distinct planned lookups that ran, each with nonempty items that have values and one of its
// entries' roles among fields.
func (f *workInfoFetch) validLookups(fields []domain.WorkInfoField, lookups []domain.PersonLookupSelection) bool {
	seen := map[int]bool{}
	for _, selection := range lookups {
		if selection.Index < 0 || selection.Index >= len(f.lookupRuns) || seen[selection.Index] ||
			len(selection.Items) == 0 {
			return false
		}
		seen[selection.Index] = true
		kept := f.lookupRuns[selection.Index]
		roleSelected := slices.ContainsFunc(f.lookups.Lookups[selection.Index].Entries, func(entry int) bool {
			return slices.Contains(fields, domain.PersonField(f.details.People[entry].Role))
		})
		if kept == nil || !roleSelected {
			return false
		}
		for _, item := range selection.Items {
			if !kept.hasItem(item) {
				return false
			}
		}
	}
	return true
}

func (f *workInfoFetch) lookupApplies(lookups []domain.PersonLookupSelection) []domain.FetchedPersonLookupApply {
	out := make([]domain.FetchedPersonLookupApply, 0, len(lookups))
	for _, selection := range lookups {
		lookup := f.lookups.Lookups[selection.Index]
		out = append(out, domain.FetchedPersonLookupApply{
			Index: selection.Index, PluginID: lookup.PluginID, Digest: lookup.Digest, Generation: lookup.Generation,
			Entries: lookup.Entries, MatchedID: lookup.MatchedID, Fetched: f.lookupRuns[selection.Index].person,
			Items: selection.Items,
		})
	}
	return out
}
