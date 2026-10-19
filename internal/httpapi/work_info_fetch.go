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
	out := make([]gen.WorkInfoCandidate, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, gen.WorkInfoCandidate{Id: c.ID, Url: c.URL, Title: c.Title, Detail: optionalString(c.Detail)})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.WorkInfoSearchResult{Candidates: out}, s.logger)
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
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.PluginID == nil || body.ID == nil {
		s.invalidRequest(w, "pluginId and id are required.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	fetch, err := s.workInfoPlugins.FetchWorkInfo(r.Context(), id, *body.PluginID, *body.ID)
	if err != nil {
		s.workInfoRunError(w, err, "The video or the work-info plugin was not found.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, toAPIWorkInfoFetch(fetch), s.logger)
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
	var cover *gen.WorkInfoFetchedCover
	if fetch.Cover != nil {
		cover = &gen.WorkInfoFetchedCover{Width: fetch.Cover.Width, Height: fetch.Cover.Height}
	}
	return gen.WorkInfoFetch{
		Cover:         cover,
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
func (s *server) GetWorkInfoFetchCover(w http.ResponseWriter, _ *http.Request, id gen.VideoId, fetchID gen.WorkInfoFetchId) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	cover, err := s.workInfoPlugins.WorkInfoFetchCover(id, fetchID)
	if err != nil {
		s.notFound(w, "The fetched result has no cover, or it belongs to another video.")
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

// ApplyWorkInfoFetch applies selected fields; tags restrict imported tags to the selected names.
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
	if len(fields) == 0 {
		s.invalidRequest(w, "fields must name at least one field.")
		return
	}
	tags, ok := applyTagSelection(body.Tags, present("tags"), fields)
	if !ok {
		s.invalidRequest(w, "tags must name at least one of the fetched tags, and only when fields includes tags.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	applied, err := s.workInfoPlugins.ApplyWorkInfoFetch(
		r.Context(), id, fetchID, fields, tags, body.CoverByPage != nil && *body.CoverByPage,
	)
	switch {
	case errors.Is(err, domain.ErrWorkInfoApplyInvalid):
		s.invalidRequest(w, "fields must name at least one field, and each must have a value in the result. "+
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
	video, err := s.videoDetail(r, applied.Video, domain.VideoContext{})
	if err != nil {
		s.logger.Warn("cannot load part of the video after applying the fetched work information",
			slog.Int64("video", id), slog.Any("error", err))
	}
	skippedTags := make([]gen.SkippedTag, 0, len(applied.SkippedTags))
	for _, tag := range applied.SkippedTags {
		skippedTags = append(skippedTags, gen.SkippedTag{Name: tag.Name, Reason: gen.SkippedTagReason(tag.Reason)})
	}
	skipped := make([]gen.SkippedWorkInfoValue, 0, len(applied.Skipped))
	for _, value := range applied.Skipped {
		skipped = append(skipped, gen.SkippedWorkInfoValue{
			Field: gen.WorkInfoField(value.Field), Value: value.Value, Reason: gen.SkippedWorkInfoReason(value.Reason),
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.WorkInfoFetchApplied{Video: video, SkippedTags: skippedTags, Skipped: skipped}, s.logger)
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
