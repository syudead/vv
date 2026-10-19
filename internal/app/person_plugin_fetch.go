package app

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// The person page fetches a person's information from a person plugin and applies selected items; results are kept
// like work-info fetches (docs/design-docs/person-plugins.md).

// PersonPluginPeople reads people and applies person fetches; implemented by store.CreditStore.
type PersonPluginPeople interface {
	// Get returns ErrNotFound for a missing person.
	Get(ctx context.Context, kind domain.CreditKind, id int64) (domain.Credit, error)
	// ApplyPersonPluginFetch rechecks the plugin in its transaction, returning ErrWorkInfoFetchStale or ErrNotFound
	// for a missing person, both without writes.
	ApplyPersonPluginFetch(ctx context.Context, apply domain.PersonPluginApply) (domain.PersonPluginApplied, error)
}

// personPluginFetch is bound to one person and to the plugin's digest and generation at the run.
type personPluginFetch struct {
	personID   int64
	pluginID   int64
	digest     string
	generation int64
	person     domain.FetchedPluginPerson
	image      *domain.WorkInfoCover
	expiresAt  time.Time
}

func (f *personPluginFetch) imageBytes() int {
	if f.image == nil {
		return 0
	}
	return len(f.image.Data)
}

func (f *personPluginFetch) expiry() time.Time { return f.expiresAt }

// FetchPersonWithPlugin runs a person plugin's fetch for a person by source and id, which come together or not at
// all; without them it uses the person's first stored ID the plugin accepts. A missing person returns
// ErrPersonNotFound, no accepted ID ErrPersonExternalIDMissing, an undeclared source, an id outside 1-200 characters
// or only one of the two ErrWorkInfoRunInvalid, all before the run; other errors follow run.
func (p *WorkInfoPlugins) FetchPersonWithPlugin(
	ctx context.Context, personID, pluginID int64, source, id *string,
) (domain.PersonPluginFetchResult, error) {
	if p.people == nil {
		return domain.PersonPluginFetchResult{}, errors.New("person plugin fetches are not configured")
	}
	if (source == nil) != (id == nil) {
		return domain.PersonPluginFetchResult{}, domain.ErrWorkInfoRunInvalid
	}
	person, err := p.people.Get(ctx, domain.CreditPerson, personID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.PersonPluginFetchResult{}, domain.ErrPersonNotFound
	}
	if err != nil {
		return domain.PersonPluginFetchResult{}, err
	}
	plugin, err := p.store.WorkInfoPlugin(ctx, pluginID)
	if err != nil {
		return domain.PersonPluginFetchResult{}, err
	}
	if plugin.Kind() != domain.WorkInfoPluginKindPerson {
		return domain.PersonPluginFetchResult{}, domain.ErrWorkInfoPluginKindMismatch
	}
	req, err := personFetchRequest(person, plugin.Manifest, source, id)
	if err != nil {
		return domain.PersonPluginFetchResult{}, err
	}
	ran, result, err := p.run(ctx, pluginID, domain.WorkInfoPluginKindPerson, req)
	if err != nil {
		return domain.PersonPluginFetchResult{}, err
	}
	// Keep the profile the comparison shows, so an apply can select and fill only the values the owner saw.
	result.Person.Profile = domain.ShownFetchedProfile(result.Person.Profile, p.now())
	fetch := &personPluginFetch{
		personID: personID, pluginID: ran.ID, digest: ran.Digest, generation: ran.Generation,
		person: result.Person, image: fetchedCover(result.PersonImage), expiresAt: p.now().Add(domain.WorkInfoFetchLifetime),
	}
	return domain.PersonPluginFetchResult{
		ID: p.fetches.addPerson(fetch, p.now()), ExpiresAt: fetch.expiresAt, Person: fetch.person,
		HasImage: fetch.image != nil,
	}, nil
}

// personFetchRequest takes the candidate's ID when given, otherwise the stored ID the plugin accepts.
func personFetchRequest(
	person domain.Credit, manifest domain.WorkInfoPluginManifest, source, id *string,
) (domain.WorkInfoRunRequest, error) {
	req := domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch}
	if source == nil {
		stored, ok := domain.PersonFetchID(person.ExternalIDs, manifest.Sources)
		if !ok {
			return req, domain.ErrPersonExternalIDMissing
		}
		req.Source, req.ID = stored.Source, stored.Value
		return req, nil
	}
	if !manifest.DeclaresSource(*source) {
		return req, domain.ErrWorkInfoRunInvalid
	}
	normalized, err := domain.NormalizeWorkInfoSearchQuery(*id)
	if err != nil {
		return req, err
	}
	req.Source, req.ID = *source, normalized
	return req, nil
}

// PersonPluginFetchImage returns ErrNotFound for an unknown, expired or other person's result and
// ErrPersonImageNotFound for a result without an image.
func (p *WorkInfoPlugins) PersonPluginFetchImage(personID int64, fetchID string) (domain.WorkInfoCover, error) {
	f := p.fetches
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(p.now())
	fetch, ok := f.people[fetchID]
	switch {
	case !ok || fetch.personID != personID:
		return domain.WorkInfoCover{}, domain.ErrNotFound
	case fetch.image == nil:
		return domain.WorkInfoCover{}, domain.ErrPersonImageNotFound
	}
	return *fetch.image, nil
}

// ApplyPersonPluginFetch applies the selected items of a kept result once and gives the person the image after the
// commit when it has no face. An unknown, expired or other person's result, or a plugin changed since the fetch,
// returns ErrWorkInfoFetchStale; no items or an item without a value ErrWorkInfoApplyInvalid; a person deleted or
// merged away ErrPersonNotFound.
func (p *WorkInfoPlugins) ApplyPersonPluginFetch(
	ctx context.Context, personID int64, fetchID string, items []domain.PersonPluginItem,
) (domain.PersonPluginApplied, error) {
	if p.people == nil {
		return domain.PersonPluginApplied{}, errors.New("person plugin fetches are not configured")
	}
	fetch, err := p.fetches.claimPerson(fetchID, personID, items, p.now())
	if err != nil {
		return domain.PersonPluginApplied{}, err
	}
	applied, err := p.people.ApplyPersonPluginFetch(ctx, domain.PersonPluginApply{
		PersonID: personID, PluginID: fetch.pluginID, Digest: fetch.digest, Generation: fetch.generation,
		Fetched: fetch.person, Items: items,
	})
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return domain.PersonPluginApplied{}, domain.ErrPersonNotFound
	case errors.Is(err, domain.ErrWorkInfoFetchStale):
		return domain.PersonPluginApplied{}, err
	case err != nil:
		// Restore the result after a transient failure so the owner can retry.
		p.fetches.restorePerson(fetchID, fetch)
		return domain.PersonPluginApplied{}, err
	}
	if slices.Contains(items, domain.PersonItemImage) && p.personImages != nil {
		// Like the work-info faces, the image follows the commit; a failure drops it without failing the apply.
		if revision, err := p.personImages.FillPersonImage(ctx, personID, fetch.image.Data); err == nil && revision != 0 {
			applied.Person.ImageRevision = revision
		}
	}
	return applied, nil
}

func (f *workInfoFetches) addPerson(fetch *personPluginFetch, now time.Time) string {
	id := rand.Text()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	f.people[id] = fetch
	f.trimCovers()
	return id
}

// claimPerson consumes a result only when it belongs to the person and every item has a value; otherwise the result
// stays.
func (f *workInfoFetches) claimPerson(
	id string, personID int64, items []domain.PersonPluginItem, now time.Time,
) (*personPluginFetch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	fetch, ok := f.people[id]
	if !ok || fetch.personID != personID {
		return nil, domain.ErrWorkInfoFetchStale
	}
	if len(items) == 0 {
		return nil, domain.ErrWorkInfoApplyInvalid
	}
	for _, item := range items {
		if item == domain.PersonItemImage && fetch.image == nil || item != domain.PersonItemImage && !fetch.person.HasItem(item) {
			return nil, domain.ErrWorkInfoApplyInvalid
		}
	}
	delete(f.people, id)
	return fetch, nil
}

func (f *workInfoFetches) restorePerson(id string, fetch *personPluginFetch) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.people[id] = fetch
}
