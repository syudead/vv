package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Proxy settings are available in every build; unsaved URL is empty.
func (s *server) GetPluginProxySettings(w http.ResponseWriter, r *http.Request) {
	if s.pluginProxy == nil {
		s.internalError(w, "Plugin proxy settings are not configured.", nil)
		return
	}
	url, err := s.pluginProxy.URL(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the plugin proxy setting.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.PluginProxySettings{Url: url}, s.logger)
}

func (s *server) UpdatePluginProxySettings(w http.ResponseWriter, r *http.Request) {
	if s.pluginProxy == nil {
		s.internalError(w, "Plugin proxy settings are not configured.", nil)
		return
	}
	// Distinguish missing required url from an explicit empty reset.
	var body struct {
		URL *string `json:"url"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.URL == nil {
		s.invalidRequest(w, "url must be a string.")
		return
	}
	url, err := s.pluginProxy.SetURL(r.Context(), *body.URL)
	if err != nil {
		if errors.Is(err, domain.ErrWorkInfoPluginProxyURLInvalid) {
			s.invalidRequest(w, "url must start with http://, https:// or socks5:// and have a host and no path, query or fragment.")
			return
		}
		s.internalError(w, "Could not save the plugin proxy setting.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.PluginProxySettings{Url: url}, s.logger)
}
