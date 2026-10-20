package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Person lookups of a work-info fetch are owner-only and excluded from accessRoutes.

// RunPersonLookup runs one planned lookup and keeps its result with the fetch.
func (s *server) RunPersonLookup(
	w http.ResponseWriter, r *http.Request, id gen.VideoId, fetchID gen.WorkInfoFetchId, index int,
) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	result, err := s.workInfoPlugins.RunPersonLookup(r.Context(), id, fetchID, index)
	switch {
	case errors.Is(err, domain.ErrWorkInfoFetchStale):
		s.writeError(w, http.StatusConflict, codeWorkInfoFetchStale, "The fetched result is out of date. Fetch it again.")
	case errors.Is(err, domain.ErrPersonLookupPluginChanged):
		s.writeError(w, http.StatusConflict, codePluginChanged,
			"The person plugin was replaced, revoked or deleted since the fetch. Fetch the work information again.")
	case err != nil:
		s.workInfoRunError(w, err, "The video, the fetched result or the lookup was not found.")
	default:
		out := gen.PersonPluginResult{Person: toAPIPluginPerson(result.Person, lookupImageCount(result.HasImage))}
		if result.Current != nil {
			out.Current = new(toAPIPerson(*result.Current))
		}
		w.Header().Set("Cache-Control", cacheNoStore)
		writeJSON(w, http.StatusOK, out, s.logger)
	}
}

// GetPersonLookupImage serves the image vv kept with a lookup, like a fetched person's.
func (s *server) GetPersonLookupImage(
	w http.ResponseWriter, _ *http.Request, id gen.VideoId, fetchID gen.WorkInfoFetchId, index int,
) {
	if !s.workInfoPluginsConfigured(w) {
		return
	}
	image, err := s.workInfoPlugins.PersonLookupImage(id, fetchID, index)
	switch {
	case errors.Is(err, domain.ErrPersonImageNotFound):
		s.notFoundReason(w, reasonPersonImageNotFound, "The lookup has no image.")
	case err != nil:
		s.notFound(w, "The fetched result or the lookup was not found, or it belongs to another video.")
	default:
		writeFetchedImage(w, image)
	}
}

// toAPIPersonLookups lists the plan and marks each fetched person's lookup or limit.
func toAPIPersonLookups(plan domain.PersonLookupPlan, people []gen.WorkInfoFetchedPerson) []gen.PersonLookup {
	out := make([]gen.PersonLookup, 0, len(plan.Lookups))
	for _, lookup := range plan.Lookups {
		out = append(out, gen.PersonLookup{
			PluginId: lookup.PluginID, PluginName: lookup.PluginName, Source: lookup.Source, Id: lookup.ID,
		})
	}
	for i := range people {
		if i < len(plan.EntryLookups) && plan.EntryLookups[i] >= 0 {
			people[i].Lookup = new(plan.EntryLookups[i])
		}
		if i < len(plan.Limited) && plan.Limited[i] {
			people[i].LookupLimited = new(true)
		}
	}
	return out
}

// personLookupSelections converts the apply's lookup items, reporting false for an explicit null or an unknown item.
func personLookupSelections(body *[]gen.PersonLookupSelection, present bool) ([]domain.PersonLookupSelection, bool) {
	if body == nil {
		return nil, !present
	}
	out := make([]domain.PersonLookupSelection, 0, len(*body))
	for _, selection := range *body {
		items := make([]domain.PersonPluginItem, 0, len(selection.Items))
		for _, item := range selection.Items {
			if !item.Valid() {
				return nil, false
			}
			items = append(items, domain.PersonPluginItem(item))
		}
		out = append(out, domain.PersonLookupSelection{Index: selection.Index, Items: items})
	}
	return out, true
}

func toAPISkippedWorkInfoValue(value domain.SkippedWorkInfoValue) gen.SkippedWorkInfoValue {
	out := gen.SkippedWorkInfoValue{
		Field: gen.WorkInfoField(value.Field), Value: value.Value, Reason: gen.SkippedWorkInfoReason(value.Reason),
		Lookup: value.Lookup, HolderName: optionalString(value.HolderName),
	}
	if value.Item != "" {
		out.Item = new(gen.PersonPluginItem(value.Item))
	}
	return out
}

func lookupImageCount(hasImage bool) int {
	if hasImage {
		return 1
	}
	return 0
}
