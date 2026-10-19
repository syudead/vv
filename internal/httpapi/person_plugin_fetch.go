package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Person plugin fetches from the person page are owner-only and excluded from accessRoutes.

// FetchPersonWithPlugin keeps a person plugin's result without changing the person.
func (s *server) FetchPersonWithPlugin(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	var body struct {
		PluginID *int64  `json:"pluginId"`
		Source   *string `json:"source"`
		ID       *string `json:"id"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.PluginID == nil {
		s.invalidRequest(w, "pluginId is required.")
		return
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	fetch, err := s.workInfoPlugins.FetchPersonWithPlugin(r.Context(), id, *body.PluginID, body.Source, body.ID)
	switch {
	case errors.Is(err, domain.ErrPersonNotFound):
		s.notFoundReason(w, gen.ErrorReasonPersonNotFound, "The person was not found.")
	case errors.Is(err, domain.ErrPersonExternalIDMissing):
		s.writeError(w, http.StatusConflict, codePersonExternalIDMissing,
			"The person has no external ID the plugin accepts. Search by name instead.")
	case errors.Is(err, domain.ErrWorkInfoRunInvalid):
		s.invalidRequest(w, "source and id must come together; source must be one the plugin accepts, "+
			"and id must be 1 to 200 characters after trimming spaces.")
	case err != nil:
		s.workInfoRunError(w, err, "The person plugin was not found.")
	default:
		w.Header().Set("Cache-Control", cacheNoStore)
		writeJSON(w, http.StatusCreated, gen.PersonPluginFetch{
			FetchId: fetch.ID, ExpiresAt: fetch.ExpiresAt.Unix(), Person: toAPIPluginPerson(fetch),
		}, s.logger)
	}
}

// Show only profile values that pass validation, as a work-info fetch shows a fetched person's.
func toAPIPluginPerson(fetch domain.PersonPluginFetchResult) gen.PersonPluginFetchedPerson {
	person := gen.PersonPluginFetchedPerson{
		Name: fetch.Person.Name, Aliases: toAPIAliases(fetch.Person.Aliases),
		ExternalIds: toAPIExternalIDs(fetch.Person.ExternalIDs),
	}
	if fetch.HasImage {
		person.HasImage = new(true)
	}
	if profile := domain.ShownFetchedProfile(fetch.Person.Profile, time.Now()); !profile.Equal(domain.PersonProfile{}) {
		person.Profile = new(toAPIProfile(profile))
	}
	return person
}

// GetPersonPluginFetchImage serves the image vv kept with a person fetch, like a work-info fetch's cover.
func (s *server) GetPersonPluginFetchImage(
	w http.ResponseWriter, _ *http.Request, id gen.PersonId, fetchID gen.WorkInfoFetchId,
) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	image, err := s.workInfoPlugins.PersonPluginFetchImage(id, fetchID)
	switch {
	case errors.Is(err, domain.ErrPersonImageNotFound):
		s.notFoundReason(w, reasonPersonImageNotFound, "The fetched person has no image.")
	case err != nil:
		s.notFound(w, "The fetched result was not found, or it belongs to another person.")
	default:
		writeFetchedImage(w, image)
	}
}

// ApplyPersonPluginFetch applies the selected items of a person fetch once.
func (s *server) ApplyPersonPluginFetch(
	w http.ResponseWriter, r *http.Request, id gen.PersonId, fetchID gen.WorkInfoFetchId,
) {
	var body gen.ApplyPersonPluginFetchRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	items := make([]domain.PersonPluginItem, 0, len(body.Items))
	for _, item := range body.Items {
		if !item.Valid() {
			s.invalidRequest(w, "Each item must be image, aliases, externalIds or profile.<property>.")
			return
		}
		items = append(items, domain.PersonPluginItem(item))
	}
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	applied, err := s.workInfoPlugins.ApplyPersonPluginFetch(r.Context(), id, fetchID, items)
	switch {
	case errors.Is(err, domain.ErrWorkInfoApplyInvalid):
		s.invalidRequest(w, "items must name at least one item, and each must have a value in the result.")
		return
	case errors.Is(err, domain.ErrPersonNotFound):
		s.notFoundReason(w, gen.ErrorReasonPersonNotFound, "The person was deleted or merged into another.")
		return
	case errors.Is(err, domain.ErrWorkInfoFetchStale):
		s.writeError(w, http.StatusConflict, codeWorkInfoFetchStale, "The fetched result is out of date. Fetch it again.")
		return
	case err != nil:
		s.internalError(w, "Could not apply the fetched person.", err)
		return
	}
	skipped := make([]gen.SkippedPersonValue, 0, len(applied.Skipped))
	for _, value := range applied.Skipped {
		skipped = append(skipped, gen.SkippedPersonValue{
			Item: gen.PersonPluginItem(value.Item), Value: value.Value, Reason: gen.SkippedPersonReason(value.Reason),
			HolderName: optionalString(value.HolderName),
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.PersonPluginFetchApplied{Person: toAPIPerson(applied.Person), Skipped: skipped}, s.logger)
}
