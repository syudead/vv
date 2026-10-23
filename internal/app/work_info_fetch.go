package app

import (
	"context"
	"crypto/rand"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Search/fetch through WorkInfoPluginRuntime and apply through FetchedWorkInfoApplier. Results stay in memory for WorkInfoFetchLifetime, bound to video and plugin digest; fetching never mutates videos.

// WorkInfoVideos checks video existence before running plugins; implemented by store.LibraryStore.
type WorkInfoVideos interface {
	// GetVideo returns the audience-visible video or ErrNotFound.
	GetVideo(ctx context.Context, audience domain.Audience, id int64) (domain.Video, error)
}

// FetchedWorkInfoApplier persists fetched values; implemented by store.OverrideStore.
type FetchedWorkInfoApplier interface {
	// Apply selected title, tentative tags and details atomically, returning the updated video, rejected tag names and skipped details. Recheck plugin existence, approval and digest in the transaction, returning ErrWorkInfoFetchStale without writes on mismatch or ErrNotFound for a missing video.
	ApplyFetchedWorkInfo(ctx context.Context, apply domain.FetchedWorkInfoApply) (domain.FetchedWorkInfoApplied, error)
	// RecordWorkInfoFetch records success for empty failure, otherwise preserves the plugin failure message.
	RecordWorkInfoFetch(ctx context.Context, videoID int64, failure string) error
}

// WorkInfoCreditMatcher matches fetched people/organizations against stored entities; implemented by store.CreditStore.
type WorkInfoCreditMatcher interface {
	// Match preserves entry order.
	Match(ctx context.Context, kind domain.CreditKind, entries []domain.CreditMatchEntry) ([]domain.CreditMatch, error)
}

// SearchWorkInfo uses an approved plugin; trimmed queries must contain 1-200 characters or return ErrWorkInfoRunInvalid. Other validation and execution errors follow run.
func (p *WorkInfoPlugins) SearchWorkInfo(ctx context.Context, id int64, query string) ([]domain.WorkInfoCandidate, error) {
	result, err := p.search(ctx, id, domain.WorkInfoPluginKindWork, query)
	if err != nil {
		return nil, err
	}
	candidates := result.Candidates
	if candidates == nil {
		candidates = []domain.WorkInfoCandidate{}
	}
	return candidates, nil
}

// SearchPeople runs an approved person plugin's search like SearchWorkInfo; a work-info plugin returns
// ErrWorkInfoPluginKindMismatch.
func (p *WorkInfoPlugins) SearchPeople(ctx context.Context, id int64, query string) ([]domain.PersonCandidate, error) {
	result, err := p.search(ctx, id, domain.WorkInfoPluginKindPerson, query)
	if err != nil {
		return nil, err
	}
	candidates := result.PersonCandidates
	if candidates == nil {
		candidates = []domain.PersonCandidate{}
	}
	return candidates, nil
}

func (p *WorkInfoPlugins) search(
	ctx context.Context, id int64, kind domain.WorkInfoPluginKind, query string,
) (domain.WorkInfoRunResult, error) {
	normalized, err := domain.NormalizeWorkInfoSearchQuery(query)
	if err != nil {
		return domain.WorkInfoRunResult{}, err
	}
	_, result, err := p.run(ctx, id, kind, domain.WorkInfoRunRequest{Operation: domain.WorkInfoSearch, Query: normalized})
	return result, err
}

// FetchWorkInfo validates a trimmed 1-200-character ID and video existence before running an approved plugin, then keeps fetched values and entity matches in memory without changing the video. Pass URL-shaped IDs unchanged: only the plugin interprets them; invalid IDs return ErrWorkInfoRunInvalid, missing videos ErrNotFound, and other errors follow run.
func (p *WorkInfoPlugins) FetchWorkInfo(
	ctx context.Context, videoID, pluginID int64, id string,
) (domain.WorkInfoFetchResult, error) {
	normalized, err := domain.NormalizeWorkInfoSearchQuery(id)
	if err != nil {
		return domain.WorkInfoFetchResult{}, err
	}
	return p.keepFetched(ctx, videoID, pluginID, domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch, ID: normalized})
}

// Check video existence before execution, then match and retain the result bound to that video.
func (p *WorkInfoPlugins) keepFetched(
	ctx context.Context, videoID, pluginID int64, req domain.WorkInfoRunRequest,
) (domain.WorkInfoFetchResult, error) {
	if p.videos == nil || p.applier == nil || p.matcher == nil {
		return domain.WorkInfoFetchResult{}, errors.New("work-info fetches are not configured")
	}
	if _, err := p.videos.GetVideo(ctx, domain.AudienceOwner, videoID); err != nil {
		return domain.WorkInfoFetchResult{}, err
	}
	plugin, result, err := p.run(ctx, pluginID, domain.WorkInfoPluginKindWork, req)
	if err := p.recordFetch(ctx, videoID, err); err != nil {
		return domain.WorkInfoFetchResult{}, err
	}
	if err != nil {
		return domain.WorkInfoFetchResult{}, err
	}
	// Invalid display names are absent and cannot be selected.
	title := domain.KeptWorkInfoTitle(result.WorkInfo.Title)
	tags := result.WorkInfo.Tags
	if tags == nil {
		tags = []string{}
	}
	keepPersonImages(result.WorkInfo.People, result.PersonImages)
	details := domain.OrderFetchedWorkDetails(result.WorkInfo)
	people, err := p.matchCredits(ctx, domain.CreditPerson, domain.PersonMatchEntries(details.People))
	if err != nil {
		return domain.WorkInfoFetchResult{}, err
	}
	organizations, err := p.matchCredits(ctx, domain.CreditOrganization,
		domain.OrganizationMatchEntries(details.Organizations))
	if err != nil {
		return domain.WorkInfoFetchResult{}, err
	}
	fetch := &workInfoFetch{
		videoID: videoID, pluginID: plugin.ID, digest: plugin.Digest, generation: plugin.Generation,
		title: title, tags: tags, details: details, personMatches: people, organizationMatches: organizations,
		expiresAt: p.now().Add(domain.WorkInfoFetchLifetime),
	}
	fetch.covers, fetch.coverURLs = keptCovers(result.WorkInfo.CoverURLs, result.CoverImages)
	id := p.fetches.add(fetch, p.now())
	return domain.WorkInfoFetchResult{
		ID: id, ExpiresAt: fetch.expiresAt, Title: title, Tags: tags,
		Details: details, PersonMatches: people, OrganizationMatches: organizations, Covers: fetch.covers,
	}, nil
}

// Record only fetches that executed a plugin; preserve the WorkInfoRunError message, falling back to its kind. Preflight rejections produce no activity.
func (p *WorkInfoPlugins) recordFetch(ctx context.Context, videoID int64, runErr error) error {
	failure := ""
	if runErr != nil {
		var failed *domain.WorkInfoRunError
		if !errors.As(runErr, &failed) {
			return nil
		}
		failure = failed.PluginMessage
		if failure == "" {
			failure = string(failed.Failure)
		}
	}
	return p.applier.RecordWorkInfoFetch(ctx, videoID, failure)
}

// Skip storage reads for empty entry lists.
func (p *WorkInfoPlugins) matchCredits(
	ctx context.Context, kind domain.CreditKind, entries []domain.CreditMatchEntry,
) ([]domain.CreditMatch, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	return p.matcher.Match(ctx, kind, entries)
}

// Validate existence, kind, approval, operation support and required settings before executing, returning ErrNotFound, ErrWorkInfoPluginKindMismatch, ErrWorkInfoRunInvalid for an undeclared source or a link outside the domains, ErrWorkInfoPluginNotApproved, ErrWorkInfoOperationUnsupported or WorkInfoSettingsMissingError respectively. Pass only declared settings with values; execution errors are WorkInfoRunError.
func (p *WorkInfoPlugins) run(
	ctx context.Context, id int64, kind domain.WorkInfoPluginKind, req domain.WorkInfoRunRequest,
) (domain.WorkInfoPlugin, domain.WorkInfoRunResult, error) {
	plugin, module, err := p.store.WorkInfoPluginModule(ctx, id)
	if err != nil {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, err
	}
	if plugin.Kind() != kind {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, domain.ErrWorkInfoPluginKindMismatch
	}
	// Recheck the source and the link on the snapshot that runs, since the plugin may have been replaced since the
	// caller chose them.
	if (req.Source != "" && !plugin.Manifest.DeclaresSource(req.Source)) ||
		(req.URL != "" && !domain.LinkMatchesDomains(req.URL, plugin.Manifest.Domains)) {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, domain.ErrWorkInfoRunInvalid
	}
	req.Kind, req.Sources = kind, plugin.Manifest.Sources
	if plugin.State() != domain.WorkInfoPluginApproved {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, domain.ErrWorkInfoPluginNotApproved
	}
	if !plugin.Manifest.Supports(req.Operation) {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, domain.ErrWorkInfoOperationUnsupported
	}
	req.Settings, err = domain.WorkInfoRunSettings(plugin.Manifest.Settings, plugin.Values)
	if err != nil {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, err
	}
	// Snapshot proxy settings per run so changes affect only later executions.
	network, err := p.runNetwork(ctx)
	if err != nil {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, err
	}
	result, _, err := p.runtime.Run(ctx, plugin.Digest, module, req, network,
		p.recordRun(ctx, plugin, domain.SecretWorkInfoPluginValues(plugin.Manifest.Settings, req.Settings)))
	if err != nil {
		return domain.WorkInfoPlugin{}, domain.WorkInfoRunResult{}, err
	}
	return plugin, result, nil
}

// Use no proxy when no network source is configured.
func (p *WorkInfoPlugins) runNetwork(ctx context.Context) (domain.WorkInfoRunNetwork, error) {
	if p.network == nil {
		return domain.WorkInfoRunNetwork{}, nil
	}
	return p.network.RunNetwork(ctx)
}

// ApplyWorkInfoFetch applies a selection of a result once; coverByPage checks and consumes a selected cover without
// applying it, for a page that uploads its own framing. Missing/expired or changed-plugin results return
// ErrWorkInfoFetchStale, wrong-video results ErrNotFound, and a selection the result cannot give
// ErrWorkInfoApplyInvalid; failed application writes nothing. The store rechecks the plugin in its transaction and
// reports invalid original tag names and normalized rejected names in fetched order.
func (p *WorkInfoPlugins) ApplyWorkInfoFetch(
	ctx context.Context, videoID int64, fetchID string, selection domain.WorkInfoSelection, coverByPage bool,
) (domain.WorkInfoApplied, error) {
	if p.applier == nil {
		return domain.WorkInfoApplied{}, errors.New("work-info fetches are not configured")
	}
	fetch, chosen, err := p.fetches.claim(fetchID, videoID, selection, p.now())
	if err != nil {
		return domain.WorkInfoApplied{}, err
	}
	applied, err := p.applyFetch(ctx, fetch, chosen, coverByPage)
	if err != nil && !errors.Is(err, domain.ErrWorkInfoFetchStale) && !errors.Is(err, domain.ErrNotFound) {
		// Restore results after transient writes fail so the owner can retry.
		p.fetches.restore(fetchID, fetch)
	}
	return applied, err
}

// chosenWork is a valid selection with the fetched details narrowed to it.
type chosenWork struct {
	selection    domain.WorkInfoSelection
	details      domain.FetchedWorkInfo
	detailFields []domain.WorkInfoField
}

func (p *WorkInfoPlugins) applyFetch(
	ctx context.Context, fetch *workInfoFetch, chosen chosenWork, coverByPage bool,
) (domain.WorkInfoApplied, error) {
	plugin, err := p.store.WorkInfoPlugin(ctx, fetch.pluginID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return domain.WorkInfoApplied{}, domain.ErrWorkInfoFetchStale
	case err != nil:
		return domain.WorkInfoApplied{}, err
	case plugin.State() != domain.WorkInfoPluginApproved || plugin.Digest != fetch.digest ||
		plugin.Generation != fetch.generation:
		return domain.WorkInfoApplied{}, domain.ErrWorkInfoFetchStale
	}
	fields := chosen.selection.Fields
	apply := domain.FetchedWorkInfoApply{
		VideoID: fetch.videoID, PluginID: fetch.pluginID, Digest: fetch.digest, Generation: fetch.generation,
		Details: chosen.details, DetailFields: chosen.detailFields,
	}
	if slices.Contains(fields, domain.WorkInfoFieldTitle) {
		title := fetch.title
		apply.Title = &title
	}
	names := selectedTagNames(fetch.tags, chosen.selection.Tags)
	var invalid map[string]bool
	if slices.Contains(fields, domain.WorkInfoFieldTags) {
		apply.Tags, invalid = splitTagNames(names)
	}
	applied, err := p.applier.ApplyFetchedWorkInfo(ctx, apply)
	if err != nil {
		return domain.WorkInfoApplied{}, err
	}
	video := applied.Video
	var coverSkipped []domain.SkippedWorkInfoValue
	if slices.Contains(fields, domain.WorkInfoFieldCover) && !coverByPage {
		// Apply covers only after the transaction revalidates the plugin and commits metadata.
		video, coverSkipped = p.applyCover(ctx, fetch, chosen.selection.CoverIndex, video)
	}
	// Like the cover, faces follow the commit so a plugin the transaction found stale stores none.
	video = p.applyPersonImages(ctx, video, appliedFaces(chosen.details, applied))
	skipped := append(coverSkipped, applied.Skipped...)
	if skipped == nil {
		skipped = []domain.SkippedWorkInfoValue{}
	}
	var linked []int64
	for _, id := range applied.PersonIDs {
		if id != 0 && !slices.Contains(linked, id) {
			linked = append(linked, id)
		}
	}
	return domain.WorkInfoApplied{
		Video: video, SkippedTags: skippedTags(names, invalid, applied.RejectedTags), Skipped: skipped,
		LinkedPeople: linked,
	}, nil
}

// Preserve fetched order; nil selected returns all fetched names.
func selectedTagNames(fetched, selected []string) []string {
	if selected == nil {
		return fetched
	}
	out := make([]string, 0, len(selected))
	for _, name := range fetched {
		if slices.Contains(selected, name) {
			out = append(out, name)
		}
	}
	return out
}

// Normalize and deduplicate valid names, retaining a set of invalid original names.
func splitTagNames(names []string) ([]string, map[string]bool) {
	valid := make([]string, 0, len(names))
	invalid := map[string]bool{}
	for _, name := range names {
		normalized, err := domain.NormalizeTagName(name)
		if err != nil {
			invalid[name] = true
			continue
		}
		if !slices.Contains(valid, normalized) {
			valid = append(valid, normalized)
		}
	}
	return valid, invalid
}

// Report unique skipped names in fetched order: original invalid names and normalized rejected names.
func skippedTags(names []string, invalid map[string]bool, rejected []string) []domain.SkippedTag {
	out := []domain.SkippedTag{}
	seen := map[string]bool{}
	add := func(name string, reason domain.SkippedTagReason) {
		if !seen[name] {
			seen[name] = true
			out = append(out, domain.SkippedTag{Name: name, Reason: reason})
		}
	}
	for _, name := range names {
		if invalid[name] {
			add(name, domain.SkippedTagInvalid)
			continue
		}
		normalized, err := domain.NormalizeTagName(name)
		if err == nil && slices.Contains(rejected, normalized) {
			add(normalized, domain.SkippedTagRejected)
		}
	}
	return out
}

// Retain validated title, plugin digest/generation and fetched details for the result lifetime. Entity matches are display snapshots only; application rechecks them transactionally.
type workInfoFetch struct {
	videoID             int64
	pluginID            int64
	digest              string
	generation          int64
	title               string
	tags                []string
	details             domain.FetchedWorkInfo
	personMatches       []domain.CreditMatch
	organizationMatches []domain.CreditMatch

	covers []domain.WorkInfoCover
	// coverURLs aligns with covers, for a skipped cover's text.
	coverURLs []string
	expiresAt time.Time
}

// Unknown fields have no value.
func (f *workInfoFetch) hasValue(field domain.WorkInfoField) bool {
	switch field {
	case domain.WorkInfoFieldTitle:
		return f.title != ""
	case domain.WorkInfoFieldTags:
		return len(f.tags) > 0
	case domain.WorkInfoFieldCover:
		return len(f.covers) > 0
	}
	return f.details.HasDetail(field)
}

// Nil means all tags; otherwise require tags in fields and a nonempty exact-spelling subset of fetched tags.
func (f *workInfoFetch) validTagSelection(fields []domain.WorkInfoField, tags []string) bool {
	if tags == nil {
		return true
	}
	if len(tags) == 0 || !slices.Contains(fields, domain.WorkInfoFieldTags) {
		return false
	}
	for _, name := range tags {
		if !slices.Contains(f.tags, name) {
			return false
		}
	}
	return true
}

// Index results by ID and expire them on insert or retrieval.
// People holds person page results apart, under the same lifetime and image budget.
type workInfoFetches struct {
	mu      sync.Mutex
	entries map[string]*workInfoFetch
	people  map[string]*personPluginFetch
}

func newWorkInfoFetches() *workInfoFetches {
	return &workInfoFetches{entries: map[string]*workInfoFetch{}, people: map[string]*personPluginFetch{}}
}

// Assign a random 128-bit ID; evict the earliest-expiring image-bearing results when their images exceed WorkInfoFetchCoverBudget.
func (f *workInfoFetches) add(fetch *workInfoFetch, now time.Time) string {
	id := rand.Text()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	f.entries[id] = fetch
	f.trimCovers()
	return id
}

// Caller holds mu; evict earliest-expiring image-bearing results of either kind until within budget.
func (f *workInfoFetches) trimCovers() {
	total := 0
	for _, entry := range f.entries {
		total += entry.imageBytes()
	}
	for _, entry := range f.people {
		total += entry.imageBytes()
	}
	for total > domain.WorkInfoFetchCoverBudget {
		var oldest keptImages
		for _, entry := range f.entries {
			oldest = earlierImages(oldest, entry)
		}
		for _, entry := range f.people {
			oldest = earlierImages(oldest, entry)
		}
		total -= oldest.imageBytes()
		f.evict(oldest)
	}
}

// keptImages is a kept result of either kind as the image budget sees it.
type keptImages interface {
	imageBytes() int
	expiry() time.Time
}

func (f *workInfoFetch) expiry() time.Time { return f.expiresAt }

// earlierImages returns the earlier-expiring of oldest and entry among those holding images.
func earlierImages(oldest, entry keptImages) keptImages {
	if entry.imageBytes() > 0 && (oldest == nil || entry.expiry().Before(oldest.expiry())) {
		return entry
	}
	return oldest
}

// Caller holds mu.
func (f *workInfoFetches) evict(entry keptImages) {
	maps.DeleteFunc(f.entries, func(_ string, kept *workInfoFetch) bool { return keptImages(kept) == entry })
	maps.DeleteFunc(f.people, func(_ string, kept *personPluginFetch) bool { return keptImages(kept) == entry })
}

// Return ErrNotFound for a cover index outside the result, missing results or video mismatches.
func (f *workInfoFetches) cover(id string, videoID int64, index int, now time.Time) (domain.WorkInfoCover, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	fetch, ok := f.entries[id]
	if !ok || fetch.videoID != videoID || index < 0 || index >= len(fetch.covers) {
		return domain.WorkInfoCover{}, domain.ErrNotFound
	}
	return fetch.covers[index], nil
}

// choose checks a selection against the result and narrows its details; false for an empty selection, a whole field
// without a value, a tag or cover the result lacks, or an entry or value Select refuses.
func (f *workInfoFetch) choose(selection domain.WorkInfoSelection) (chosenWork, bool) {
	details, fields, ok := f.details.Select(selection)
	cover := slices.Contains(selection.Fields, domain.WorkInfoFieldCover)
	valid := ok && f.validTagSelection(selection.Fields, selection.Tags) &&
		(!cover || selection.CoverIndex >= 0 && selection.CoverIndex < len(f.covers)) &&
		!slices.ContainsFunc(selection.Fields, func(field domain.WorkInfoField) bool { return !f.hasValue(field) }) &&
		(len(selection.Fields) > 0 || len(fields) > 0)
	return chosenWork{selection: selection, details: details, detailFields: fields}, valid
}

// Validate existence/expiry, video and selection before consuming a result once. ErrWorkInfoFetchStale, ErrNotFound and ErrWorkInfoApplyInvalid preserve it unless expired.
func (f *workInfoFetches) claim(
	id string, videoID int64, selection domain.WorkInfoSelection, now time.Time,
) (*workInfoFetch, chosenWork, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prune(now)
	fetch, ok := f.entries[id]
	if !ok {
		return nil, chosenWork{}, domain.ErrWorkInfoFetchStale
	}
	if fetch.videoID != videoID {
		return nil, chosenWork{}, domain.ErrNotFound
	}
	chosen, valid := fetch.choose(selection)
	if !valid {
		return nil, chosenWork{}, domain.ErrWorkInfoApplyInvalid
	}
	delete(f.entries, id)
	return fetch, chosen, nil
}

// Restore a claimed result; expiry is enforced on the next insert or retrieval.
func (f *workInfoFetches) restore(id string, fetch *workInfoFetch) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[id] = fetch
}

// Caller holds mu.
func (f *workInfoFetches) prune(now time.Time) {
	maps.DeleteFunc(f.entries, func(_ string, fetch *workInfoFetch) bool { return !now.Before(fetch.expiresAt) })
	maps.DeleteFunc(f.people, func(_ string, fetch *personPluginFetch) bool { return !now.Before(fetch.expiresAt) })
}
