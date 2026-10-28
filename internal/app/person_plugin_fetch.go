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
	images     []domain.WorkInfoCover
	expiresAt  time.Time
}

func (f *personPluginFetch) imageBytes() int {
	total := 0
	for _, image := range f.images {
		total += len(image.Data)
	}
	return total
}

func (f *personPluginFetch) expiry() time.Time { return f.expiresAt }

// FetchPersonWithPlugin runs a person plugin's fetch for a person by the key's source and id, which come together, or
// by its link. A key without either uses the person's first stored ID the plugin accepts, else the person's only
// folded link under the plugin's domains. A missing person returns ErrPersonNotFound; no accepted ID and no such link
// ErrPersonExternalIDMissing; several such links a PersonLinkAmbiguousError; a given link no domain matches
// ErrPersonLinkUnsupported; an undeclared source, an id outside 1-200 characters, only one of the two, a link that is
// not an http or https URL or a link with source or id ErrWorkInfoRunInvalid, all before the run; other errors follow
// run.
func (p *WorkInfoPlugins) FetchPersonWithPlugin(
	ctx context.Context, personID, pluginID int64, key domain.PersonFetchKey,
) (domain.PersonPluginFetchResult, error) {
	if p.people == nil {
		return domain.PersonPluginFetchResult{}, errors.New("person plugin fetches are not configured")
	}
	if (key.Source == nil) != (key.ID == nil) || (key.Link != nil && key.Source != nil) {
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
	req, err := personFetchRequest(person, plugin.Manifest, key)
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
		person: result.Person, images: fetchedImages(result.PersonPluginImages),
		expiresAt: p.now().Add(domain.WorkInfoFetchLifetime),
	}
	return domain.PersonPluginFetchResult{
		ID: p.fetches.addPerson(fetch, p.now()), ExpiresAt: fetch.expiresAt, Person: fetch.person,
		ImageCount: len(fetch.images), Link: req.URL,
	}, nil
}

// personFetchRequest takes the candidate's ID or the given link, otherwise the stored ID the plugin accepts, then the
// one stored link it handles.
func personFetchRequest(
	person domain.Credit, manifest domain.WorkInfoPluginManifest, key domain.PersonFetchKey,
) (domain.WorkInfoRunRequest, error) {
	req := domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch}
	switch {
	case key.Link != nil:
		link, err := domain.NormalizeFetchLink(*key.Link)
		if err != nil {
			return req, err
		}
		if !domain.LinkMatchesDomains(link, manifest.Domains) {
			return req, domain.ErrPersonLinkUnsupported
		}
		req.URL = link
		return req, nil
	case key.Source != nil:
		if !manifest.DeclaresSource(*key.Source) {
			return req, domain.ErrWorkInfoRunInvalid
		}
		normalized, err := domain.NormalizeWorkInfoSearchQuery(*key.ID)
		if err != nil {
			return req, err
		}
		req.Source, req.ID = *key.Source, normalized
		return req, nil
	}
	if stored, ok := domain.PersonFetchID(person.ExternalIDs, manifest.Sources); ok {
		req.Source, req.ID = stored.Source, stored.Value
		return req, nil
	}
	switch links := domain.PersonFetchLinks(person.Profile.Links, manifest.Domains); len(links) {
	case 0:
		return req, domain.ErrPersonExternalIDMissing
	case 1:
		req.URL = links[0]
		return req, nil
	default:
		return req, &domain.PersonLinkAmbiguousError{Links: links}
	}
}

// PersonPluginFetchImage returns ErrNotFound for an unknown, expired or other person's result and
// ErrPersonImageNotFound for an index outside the result's images.
func (p *WorkInfoPlugins) PersonPluginFetchImage(personID int64, fetchID string, index int) (domain.WorkInfoCover, error) {
	f := p.fetches
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(p.now())
	fetch, ok := f.people[fetchID]
	switch {
	case !ok || fetch.personID != personID:
		return domain.WorkInfoCover{}, domain.ErrNotFound
	case index < 0 || index >= len(fetch.images):
		return domain.WorkInfoCover{}, domain.ErrPersonImageNotFound
	}
	return fetch.images[index], nil
}

// ApplyPersonPluginFetch applies a selection of a kept result once and makes image imageIndex the person's face after
// the commit, replacing any. An unknown, expired or other person's result, or a plugin changed since the fetch,
// returns ErrWorkInfoFetchStale; an empty selection, an item or value the result lacks or an image index outside it
// ErrWorkInfoApplyInvalid; a person deleted or merged away ErrPersonNotFound.
func (p *WorkInfoPlugins) ApplyPersonPluginFetch(
	ctx context.Context, personID int64, fetchID string, selection domain.PersonPluginSelection, imageIndex int,
) (domain.PersonPluginApplied, error) {
	if p.people == nil {
		return domain.PersonPluginApplied{}, errors.New("person plugin fetches are not configured")
	}
	fetch, fetched, items, err := p.fetches.claimPerson(fetchID, personID, selection, imageIndex, p.now())
	if err != nil {
		return domain.PersonPluginApplied{}, err
	}
	applied, err := p.people.ApplyPersonPluginFetch(ctx, domain.PersonPluginApply{
		PersonID: personID, PluginID: fetch.pluginID, Digest: fetch.digest, Generation: fetch.generation,
		Fetched: fetched, Items: items,
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
		if revision, err := p.personImages.ReplacePersonImage(ctx, personID, fetch.images[imageIndex].Data); err == nil && revision != 0 {
			applied.Person.ImageRevision = revision
		}
	}
	return applied, nil
}

// FetchPeerPersonWithPlugin fetches like FetchPersonWithPlugin for a person of the paired node, whose remote id is
// personID: this node's plugin runs, and the peer gives the person's IDs and links.
func (p *WorkInfoPlugins) FetchPeerPersonWithPlugin(
	ctx context.Context, peer domain.PersonPluginPeer, personID, pluginID int64, key domain.PersonFetchKey,
) (domain.PersonPluginFetchResult, error) {
	return p.onPersonPeer(peer).FetchPersonWithPlugin(ctx, personID, pluginID, key)
}

// ApplyPeerPersonPluginFetch applies like ApplyPersonPluginFetch a result FetchPeerPersonWithPlugin kept: this node
// rechecks its plugin, then the peer writes the values and takes the image as the person's face.
func (p *WorkInfoPlugins) ApplyPeerPersonPluginFetch(
	ctx context.Context, peer domain.PersonPluginPeer, personID int64, fetchID string,
	selection domain.PersonPluginSelection, imageIndex int,
) (domain.PersonPluginApplied, error) {
	return p.onPersonPeer(peer).ApplyPersonPluginFetch(ctx, personID, fetchID, selection, imageIndex)
}

// onPersonPeer is p with the peer in place of this node's people and faces; it shares p's plugins and kept results.
func (p *WorkInfoPlugins) onPersonPeer(peer domain.PersonPluginPeer) *WorkInfoPlugins {
	q := *p
	node := peerPersonNode{PersonPluginPeer: peer, plugins: p}
	q.people, q.personImages = node, node
	return &q
}

type peerPersonNode struct {
	domain.PersonPluginPeer
	plugins *WorkInfoPlugins
}

// ApplyPersonPluginFetch rechecks the plugin here, since the peer has no copy of it, and sends the values without
// the image, which follows the commit as an upload.
func (n peerPersonNode) ApplyPersonPluginFetch(
	ctx context.Context, apply domain.PersonPluginApply,
) (domain.PersonPluginApplied, error) {
	if err := n.plugins.checkKeptPlugin(ctx, apply.PluginID, apply.Digest, apply.Generation); err != nil {
		return domain.PersonPluginApplied{}, err
	}
	apply.Items = slices.DeleteFunc(slices.Clone(apply.Items), func(item domain.PersonPluginItem) bool {
		return item == domain.PersonItemImage
	})
	return n.PersonPluginPeer.ApplyPersonPluginFetch(ctx, apply)
}

// FillPersonImage is never called: the person page's apply replaces the face.
func (peerPersonNode) FillPersonImage(context.Context, int64, []byte) (int64, error) { return 0, nil }

// ApplyPeerFetchedPerson writes what the paired node fetched for this node's person personID, as an apply of this
// node's own fetch writes it; the peer checked its plugin and sends the image apart. An item that is the image, is
// not valid or has no value in fetched returns ErrWorkInfoApplyInvalid, and a missing person ErrPersonNotFound, both
// without writes.
func (p *WorkInfoPlugins) ApplyPeerFetchedPerson(
	ctx context.Context, personID int64, fetched domain.FetchedPluginPerson, items []domain.PersonPluginItem,
) (domain.PersonPluginApplied, error) {
	if p.people == nil {
		return domain.PersonPluginApplied{}, errors.New("person plugin fetches are not configured")
	}
	if slices.ContainsFunc(items, func(item domain.PersonPluginItem) bool {
		return item == domain.PersonItemImage || !item.Valid() || !fetched.HasItem(item)
	}) {
		return domain.PersonPluginApplied{}, domain.ErrWorkInfoApplyInvalid
	}
	applied, err := p.people.ApplyPersonPluginFetch(ctx, domain.PersonPluginApply{
		PersonID: personID, FetchedByPeer: true, Fetched: fetched, Items: items,
	})
	if errors.Is(err, domain.ErrNotFound) {
		return domain.PersonPluginApplied{}, domain.ErrPersonNotFound
	}
	return applied, err
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

// claimPerson consumes a result only when it belongs to the person and the selection is valid, the image the one at
// imageIndex; it returns the result, the fetched person narrowed to the selection and the items to apply. Otherwise
// the result stays.
func (f *workInfoFetches) claimPerson(
	id string, personID int64, selection domain.PersonPluginSelection, imageIndex int, now time.Time,
) (*personPluginFetch, domain.FetchedPluginPerson, []domain.PersonPluginItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	fetch, ok := f.people[id]
	if !ok || fetch.personID != personID {
		return nil, domain.FetchedPluginPerson{}, nil, domain.ErrWorkInfoFetchStale
	}
	fetched, items, ok := fetch.person.Select(selection, imageIndex >= 0 && imageIndex < len(fetch.images))
	if !ok {
		return nil, domain.FetchedPluginPerson{}, nil, domain.ErrWorkInfoApplyInvalid
	}
	delete(f.people, id)
	return fetch, fetched, items, nil
}

func (f *workInfoFetches) restorePerson(id string, fetch *personPluginFetch) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.people[id] = fetch
}
