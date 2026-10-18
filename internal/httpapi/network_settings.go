package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Non-desktop builds return 404 so the UI hides LAN settings.
func (s *server) GetNetworkSettings(w http.ResponseWriter, _ *http.Request) {
	if s.networkSettings == nil {
		s.notFound(w, "Network settings are only available in the desktop app.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPINetworkSettings(s.networkSettings.Current()), s.logger)
}

// NetworkSettings owns listener reopening, persistence order, and rollback.
func (s *server) UpdateNetworkSettings(w http.ResponseWriter, r *http.Request) {
	if s.networkSettings == nil {
		s.notFound(w, "Network settings are only available in the desktop app.")
		return
	}
	// Distinguish missing required lanAccess from false.
	var body struct {
		LANAccess *bool `json:"lanAccess"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.LANAccess == nil {
		s.invalidRequest(w, "lanAccess must be true or false.")
		return
	}
	settings, err := s.networkSettings.SetLANAccess(r.Context(), *body.LANAccess)
	if err != nil {
		if errors.Is(err, domain.ErrListenFailed) {
			s.conflictReason(w, reasonListenFailed, "Could not listen on the new address; the previous setting is kept.")
			return
		}
		s.internalError(w, "Could not save the network settings.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPINetworkSettings(settings), s.logger)
}

func toAPINetworkSettings(settings domain.NetworkSettings) gen.NetworkSettings {
	addresses := settings.Addresses
	if addresses == nil {
		addresses = []string{}
	}
	return gen.NetworkSettings{LanAccess: settings.LANAccess, Port: settings.Port, Addresses: addresses}
}
