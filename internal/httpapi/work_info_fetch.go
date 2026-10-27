package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Work-info plugin routes are owner-only and excluded from accessRoutes. Keep vv's English error message separate from pluginMessage.

func (s *server) SearchWorkInfo(w http.ResponseWriter, r *http.Request, id gen.WorkInfoPluginId) {
	query, ok := s.readSearchQuery(w, r)
	if !ok {
		return
	}
	candidates, err := s.workInfoPlugins.SearchWorkInfo(r.Context(), id, query)
	if err != nil {
		s.workInfoRunError(w, err, "The work-info plugin was not found.")
		return
	}
	out := gen.WorkInfoSearchResult{Candidates: make([]gen.WorkInfoCandidate, 0, len(candidates))}
	for _, c := range candidates {
		out.Candidates = append(out.Candidates, gen.WorkInfoCandidate{Id: c.ID, Url: c.URL, Title: c.Title, Detail: optionalString(c.Detail)})
	}
	if match, ok := domain.MatchWorkInfoCandidate(candidates, query); ok {
		out.Match = &match
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// SearchPeopleWithPlugin answers a person plugin's search; its errors are SearchWorkInfo's.
func (s *server) SearchPeopleWithPlugin(w http.ResponseWriter, r *http.Request, id gen.WorkInfoPluginId) {
	query, ok := s.readSearchQuery(w, r)
	if !ok {
		return
	}
	candidates, err := s.workInfoPlugins.SearchPeople(r.Context(), id, query)
	if err != nil {
		s.workInfoRunError(w, err, "The person plugin was not found.")
		return
	}
	out := make([]gen.PersonPluginCandidate, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, gen.PersonPluginCandidate{
			Source: c.Source, Id: c.ID, Name: c.Name, Url: c.URL, Detail: optionalString(c.Detail),
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.PersonPluginSearchResult{Candidates: out}, s.logger)
}

func (s *server) readSearchQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Query *string `json:"query"`
	}
	if !s.readJSONBody(w, r, &body) {
		return "", false
	}
	if body.Query == nil {
		s.invalidRequest(w, "query is required.")
		return "", false
	}
	return *body.Query, s.workInfoPluginsConfigured(w)
}

func (s *server) writePluginKindMismatch(w http.ResponseWriter) {
	s.writeError(w, http.StatusConflict, codePluginKindMismatch, "The plugin is of the other kind.")
}

// FetchWorkInfo caches results without changing the video; pass IDs unchanged even when they look like URLs.
func (s *server) FetchWorkInfo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	var body struct {
		PluginID *int64  `json:"pluginId"`
		ID       *string `json:"id"`
		Link     *string `json:"link"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.PluginID == nil || (body.ID == nil) == (body.Link == nil) {
		s.invalidRequest(w, "pluginId and one of id and link are required.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	fetch, err := s.fetchWorkInfo(r, id, *body.PluginID, body.ID, body.Link)
	if errors.Is(err, domain.ErrWorkInfoLinkUnsupported) {
		s.writeError(w, http.StatusBadRequest, codeWorkInfoLinkUnsupported, "The plugin does not read links from this site.")
		return
	}
	if errors.Is(err, domain.ErrPeerUnreachable) {
		s.peerUnreachable(w)
		return
	}
	if err != nil {
		s.workInfoRunError(w, err, "The video or the work-info plugin was not found.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, toAPIWorkInfoFetch(fetch), s.logger)
}

// fetchWorkInfo fetches by link when link is set, else by id; the peer holds a remote video.
func (s *server) fetchWorkInfo(
	r *http.Request, videoID, pluginID int64, id, link *string,
) (domain.WorkInfoFetchResult, error) {
	ctx, remote := r.Context(), domain.IsRemoteID(videoID)
	switch {
	case remote && link != nil:
		return s.workInfoPlugins.FetchPeerWorkInfoByLink(ctx, s.peerWorkInfo(r), videoID, pluginID, *link)
	case remote:
		return s.workInfoPlugins.FetchPeerWorkInfo(ctx, s.peerWorkInfo(r), videoID, pluginID, *id)
	case link != nil:
		return s.workInfoPlugins.FetchWorkInfoByLink(ctx, videoID, pluginID, *link)
	default:
		return s.workInfoPlugins.FetchWorkInfo(ctx, videoID, pluginID, *id)
	}
}

// ListWorkInfoFillTargets lists the videos the fill dialog searches for, one per user-data key.
func (s *server) ListWorkInfoFillTargets(w http.ResponseWriter, r *http.Request) {
	var body gen.WorkInfoFillTargetsRequest
	if !s.readJSONBody(w, r, &body) || !s.workInfoPluginsConfigured(w) {
		return
	}
	videoIDs, ok := s.resolveVideoTargets(w, r, body.VideoIds, body.SmartFolderIds, maxVideoTagsIDs)
	if !ok {
		return
	}
	queries, err := s.workInfoPlugins.WorkInfoFillQueries(r.Context(), videoIDs)
	if err != nil {
		s.internalError(w, "Could not list the videos to fill.", err)
		return
	}
	targets := make([]gen.WorkInfoFillTarget, 0, len(queries))
	for _, q := range queries {
		targets = append(targets, gen.WorkInfoFillTarget{VideoId: q.VideoID, Title: q.Title, Query: q.Query})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.WorkInfoFillTargetList{Targets: targets}, s.logger)
}

// Use empty arrays for absent lists; omit absent code and description.
func toAPIWorkInfoFetch(fetch domain.WorkInfoFetchResult) gen.WorkInfoFetch {
	details := fetch.Details
	dates := make([]gen.WorkInfoFetchedDate, 0, len(details.Dates))
	for _, d := range details.Dates {
		dates = append(dates, gen.WorkInfoFetchedDate{Kind: gen.WorkDateKind(d.Kind), Date: d.Date})
	}
	people := make([]gen.WorkInfoFetchedPerson, 0, len(details.People))
	today := time.Now()
	for i, p := range details.People {
		person := gen.WorkInfoFetchedPerson{
			Role: gen.WorkPersonRole(p.Role), Name: p.Name, ExternalIds: toAPIExternalIDs(p.ExternalIDs),
			Match: toAPIWorkInfoMatch(fetch.PersonMatches, i),
		}
		if p.Image != nil {
			person.HasImage = new(true)
		}
		if profile := domain.ShownFetchedProfile(p.Profile, today); !profile.Equal(domain.PersonProfile{}) {
			person.Profile = new(toAPIProfile(profile))
		}
		people = append(people, person)
	}
	organizations := make([]gen.WorkInfoFetchedOrganization, 0, len(details.Organizations))
	for i, o := range details.Organizations {
		organizations = append(organizations, gen.WorkInfoFetchedOrganization{
			Role: gen.WorkOrganizationRole(o.Role), Name: o.Name, ExternalIds: toAPIExternalIDs(o.ExternalIDs),
			Match: toAPIWorkInfoMatch(fetch.OrganizationMatches, i),
		})
	}
	urls := details.URLs
	if urls == nil {
		urls = []string{}
	}
	covers := make([]gen.WorkInfoFetchedCover, 0, len(fetch.Covers))
	for _, cover := range fetch.Covers {
		covers = append(covers, gen.WorkInfoFetchedCover{Width: cover.Width, Height: cover.Height})
	}
	return gen.WorkInfoFetch{
		Covers:        covers,
		FetchId:       fetch.ID,
		ExpiresAt:     fetch.ExpiresAt.Unix(),
		Title:         optionalString(fetch.Title),
		Tags:          fetch.Tags,
		Code:          optionalString(details.Code),
		Description:   optionalString(details.Description),
		Dates:         dates,
		People:        people,
		Organizations: organizations,
		Urls:          urls,
		ExternalIds:   toAPIExternalIDs(details.ExternalIDs),
	}
}

// Use new when no match result exists.
func toAPIWorkInfoMatch(matches []domain.CreditMatch, i int) gen.WorkInfoMatch {
	if i >= len(matches) {
		return gen.WorkInfoMatch{State: gen.WorkInfoMatchStateNew}
	}
	match := gen.WorkInfoMatch{State: gen.WorkInfoMatchState(matches[i].State)}
	if credit := matches[i].Credit; match.State == gen.WorkInfoMatchStateExisting && credit != nil {
		match.Id, match.Name = &credit.ID, &credit.Name
	}
	return match
}

// Serve only cover bytes fetched and validated by vv, with the content type determined by vv.
func (s *server) GetWorkInfoFetchCover(
	w http.ResponseWriter, _ *http.Request, id gen.VideoId, fetchID gen.WorkInfoFetchId, index int,
) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	cover, err := s.workInfoPlugins.WorkInfoFetchCover(id, fetchID, index)
	if err != nil {
		s.notFound(w, "The fetched result has no cover at that index, or it belongs to another video.")
		return
	}
	writeFetchedImage(w, cover)
}

// GetWorkInfoFetchPersonImage serves the image vv kept for the fetched person at index, like the cover.
func (s *server) GetWorkInfoFetchPersonImage(
	w http.ResponseWriter, _ *http.Request, id gen.VideoId, fetchID gen.WorkInfoFetchId, index int,
) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	image, err := s.workInfoPlugins.WorkInfoFetchPersonImage(id, fetchID, index)
	switch {
	case errors.Is(err, domain.ErrPersonImageNotFound):
		s.notFoundReason(w, reasonPersonImageNotFound, "The fetched person has no image.")
	case err != nil:
		s.notFound(w, "The fetched result was not found, or it belongs to another video.")
	default:
		writeFetchedImage(w, image)
	}
}

// writeFetchedImage serves bytes fetched and header-checked by vv, with the content type vv determined.
func writeFetchedImage(w http.ResponseWriter, image domain.WorkInfoCover) {
	w.Header().Set("Content-Type", image.ContentType)
	w.Header().Set("Cache-Control", cacheNoStore)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image.Data)
}

// ApplyWorkInfoFetch applies a selection; tags restrict imported tags to the selected names, and the lists choose
// entries and values one by one.
func (s *server) ApplyWorkInfoFetch(w http.ResponseWriter, r *http.Request, id gen.VideoId, fetchID gen.WorkInfoFetchId) {
	var body gen.ApplyWorkInfoFetchRequest
	present, ok := s.readJSONBodyFields(w, r, &body)
	if !ok {
		return
	}
	fields := make([]domain.WorkInfoField, 0, len(body.Fields))
	for _, field := range body.Fields {
		if !field.Valid() {
			s.invalidRequest(w, "Each field must be a field of the fetched result, such as title, tags, code or dates.release.")
			return
		}
		fields = append(fields, domain.WorkInfoField(field))
	}
	tags, ok := applyTagSelection(body.Tags, present("tags"), fields)
	if !ok {
		s.invalidRequest(w, "tags must name at least one of the fetched tags, and only when fields includes tags.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	selection := domain.WorkInfoSelection{
		Fields: fields, Tags: tags, CoverIndex: -1, People: listOf(body.People), Organizations: listOf(body.Organizations),
		URLs: listOf(body.Urls), ExternalIDs: fromAPIExternalIDs(body.ExternalIds),
	}
	for _, same := range listOf(body.SamePeople) {
		selection.SamePeople = append(selection.SamePeople, domain.SamePerson{At: same.At, PersonID: same.PersonId})
	}
	if body.CoverIndex != nil {
		selection.CoverIndex = *body.CoverIndex
	}
	coverByPage := body.CoverByPage != nil && *body.CoverByPage
	var applied domain.WorkInfoApplied
	var err error
	if domain.IsRemoteID(id) {
		applied, err = s.workInfoPlugins.ApplyPeerWorkInfoFetch(r.Context(), s.peerWorkInfo(r), id, fetchID, selection, coverByPage)
	} else {
		applied, err = s.workInfoPlugins.ApplyWorkInfoFetch(r.Context(), id, fetchID, selection, coverByPage)
	}
	switch {
	case errors.Is(err, domain.ErrPeerUnreachable):
		s.peerUnreachable(w)
		return
	case errors.Is(err, domain.ErrWorkInfoApplyInvalid):
		s.invalidRequest(w, "Select at least one value the result carries. Whole fields need a value; people, "+
			"organizations, urls and externalIds go in their own lists; samePeople pairs a chosen performer with one person "+
			"each; cover needs the coverIndex of a cover; "+
			"tags must name at least one of the fetched tags, and only when fields includes tags.")
		return
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The video was not found, or the result belongs to another video.")
		return
	case errors.Is(err, domain.ErrWorkInfoFetchStale):
		s.writeError(w, http.StatusConflict, codeWorkInfoFetchStale, "The fetched result is out of date. Fetch it again.")
		return
	case err != nil:
		s.internalError(w, "Could not apply the fetched work information.", err)
		return
	}
	// After commit, return success even if detail enrichment fails: the result is single-use and cannot be retried. The emitted video event lets the UI reload missing fields.
	video, err := s.appliedVideoDetail(r, applied.Video)
	if err != nil {
		s.logger.Warn("cannot load part of the video after applying the fetched work information",
			slog.Int64("video", id), slog.Any("error", err))
	}
	skippedTags := make([]gen.SkippedTag, 0, len(applied.SkippedTags))
	for _, tag := range applied.SkippedTags {
		skippedTags = append(skippedTags, gen.SkippedTag{Name: tag.Name, Reason: gen.SkippedTagReason(tag.Reason)})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	linked := applied.LinkedPeople
	if linked == nil {
		linked = []int64{}
	}
	writeJSON(w, http.StatusOK, gen.WorkInfoFetchApplied{
		Video: video, SkippedTags: skippedTags, Skipped: toAPISkippedWorkInfoValues(applied.Skipped), LinkedPeople: linked,
	}, s.logger)
}

// appliedVideoDetail is the applied video as the screens read it; the peer gives its own video.
func (s *server) appliedVideoDetail(r *http.Request, video domain.Video) (gen.Video, error) {
	if domain.IsRemoteID(video.ID) {
		return s.peerVideoDetail(r, video.ID)
	}
	return s.videoDetail(r, video, domain.VideoContext{})
}

func toAPISkippedWorkInfoValues(values []domain.SkippedWorkInfoValue) []gen.SkippedWorkInfoValue {
	out := make([]gen.SkippedWorkInfoValue, 0, len(values))
	for _, value := range values {
		out = append(out, gen.SkippedWorkInfoValue{
			Field: gen.WorkInfoField(value.Field), Value: value.Value, Reason: gen.SkippedWorkInfoReason(value.Reason),
		})
	}
	return out
}

// Omitted tags means all tags (nil). Reject null, empty selections, or tags without the tags field selected; the registry validates names against the fetch result.
func applyTagSelection(tags *[]string, present bool, fields []domain.WorkInfoField) ([]string, bool) {
	if tags == nil {
		return nil, !present
	}
	if len(*tags) == 0 || !slices.Contains(fields, domain.WorkInfoFieldTags) {
		return nil, false
	}
	return *tags, true
}

func listOf[T any](values *[]T) []T {
	if values == nil {
		return nil
	}
	return *values
}

// notFound supplies the message for domain.ErrNotFound.
func (s *server) workInfoRunError(w http.ResponseWriter, err error, notFound string) {
	var (
		missing *domain.WorkInfoSettingsMissingError
		failed  *domain.WorkInfoRunError
	)
	switch {
	case errors.Is(err, domain.ErrWorkInfoRunInvalid):
		s.invalidRequest(w, "The query and the work's id must be 1 to 200 characters after trimming spaces.")
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, notFound)
	case errors.Is(err, domain.ErrWorkInfoPluginKindMismatch):
		s.writePluginKindMismatch(w)
	case errors.Is(err, domain.ErrWorkInfoPluginNotApproved):
		s.writeError(w, http.StatusConflict, codePluginNotApproved, "The plugin is not approved.")
	case errors.Is(err, domain.ErrWorkInfoOperationUnsupported):
		s.writeError(w, http.StatusConflict, codeOperationUnsupported, "The plugin does not support this operation.")
	case errors.As(err, &missing):
		keys := missing.Keys
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code: codePluginSettingsMissing, SettingKeys: &keys, Message: "The plugin needs settings first.",
		})
	case errors.As(err, &failed):
		s.writeErrorBody(w, workInfoRunFailureStatus(failed.Failure), workInfoRunFailureBody(failed))
	default:
		s.internalError(w, "Could not run the work-info plugin.", err)
	}
}

func workInfoRunFailureStatus(failure domain.WorkInfoRunFailure) int {
	switch failure {
	case domain.WorkInfoRunTimedOut:
		return http.StatusGatewayTimeout
	case domain.WorkInfoRunBusy:
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}

func workInfoRunFailureBody(failed *domain.WorkInfoRunError) gen.Error {
	body := gen.Error{
		Code:          codePluginFailed,
		Message:       "The plugin failed.",
		PluginMessage: optionalString(failed.PluginMessage),
		Host:          optionalString(failed.Host),
	}
	reason := func(r gen.ErrorReason, message string) {
		body.Reason, body.Message = &r, message
	}
	switch failed.Failure {
	case domain.WorkInfoRunTimedOut:
		body.Code, body.Message = codePluginTimedOut, "The plugin did not finish in 30 seconds and was stopped."
	case domain.WorkInfoRunBusy:
		body.Code, body.Message = codePluginBusy, "Another plugin is running. Try again in a moment."
	case domain.WorkInfoRunBlockedAddress:
		reason(gen.ErrorReasonBlockedAddress, "The plugin tried to reach an address that is not public.")
	case domain.WorkInfoRunNetworkError:
		reason(gen.ErrorReasonNetworkError, "The plugin could not reach a site.")
	case domain.WorkInfoRunMemoryExceeded:
		reason(gen.ErrorReasonMemoryExceeded, "The plugin used more than 128 MiB of memory and was stopped.")
	case domain.WorkInfoRunOutputTooLarge:
		reason(gen.ErrorReasonOutputTooLarge, "The plugin's output was over 1 MiB.")
	}
	return body
}
