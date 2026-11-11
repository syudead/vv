package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func (s *server) GetBackgroundWork(w http.ResponseWriter, r *http.Request) {
	if s.background == nil {
		s.internalError(w, "Background work is not configured.", nil)
		return
	}
	status, err := s.background.Status(r.Context(), audienceFrom(r.Context()))
	if err != nil {
		s.internalError(w, "Could not read the background work status.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIBackgroundWork(status), s.logger)
}

func (s *server) SetBackgroundPause(w http.ResponseWriter, r *http.Request) {
	if s.background == nil {
		s.internalError(w, "Background work is not configured.", nil)
		return
	}
	// Distinguish a missing paused from false.
	var body struct {
		Paused *bool `json:"paused"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Paused == nil {
		s.invalidRequest(w, "paused must be true or false.")
		return
	}
	status, err := s.background.SetPaused(r.Context(), audienceFrom(r.Context()), *body.Paused)
	if err != nil {
		s.internalError(w, "Could not change the background work pause.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIBackgroundWork(status), s.logger)
}

func toAPIBackgroundWork(status domain.BackgroundWork) gen.BackgroundWork {
	work := gen.BackgroundWork{
		Paused:  status.Paused,
		Running: make([]gen.BackgroundActivity, len(status.Running)),
		Groups:  make([]gen.BackgroundGroup, len(status.Groups)),
	}
	for i, activity := range status.Running {
		entry := gen.BackgroundActivity{Group: gen.BackgroundGroupName(activity.Group), Kind: activity.Kind}
		if activity.Video != nil {
			entry.Video = &gen.BackgroundVideo{Id: activity.Video.ID, Title: activity.Video.Title}
		}
		if activity.FileName != "" {
			entry.FileName = &activity.FileName
		}
		work.Running[i] = entry
	}
	for i, group := range status.Groups {
		entry := gen.BackgroundGroup{Group: gen.BackgroundGroupName(group.Group), Remaining: group.Remaining}
		// waiting belongs to the scan group and featureOff to the job groups.
		if group.Group == domain.BackgroundScan {
			entry.Waiting = &group.Waiting
		} else {
			entry.FeatureOff = &group.FeatureOff
		}
		work.Groups[i] = entry
	}
	return work
}
