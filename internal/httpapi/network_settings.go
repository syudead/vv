package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetNetworkSettings は LAN からの接続の許可と、許可中に開けるアドレスを返す
// （specs/037-windows-app/contracts/network-settings-api.md §2）。デスクトップ版で
// なければ 404 を返し、画面はこれを「この節を出さない」と読む。
func (s *server) GetNetworkSettings(w http.ResponseWriter, _ *http.Request) {
	if s.networkSettings == nil {
		s.notFound(w, "Network settings are only available in the desktop app.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPINetworkSettings(s.networkSettings.Current()), s.logger)
}

// UpdateNetworkSettings は許可を切り替え、切り替えたあとの設定を返す（契約 §3）。
// 待ち受けの開き直しと保存の順、失敗時の戻し方は NetworkSettings が持つ。
func (s *server) UpdateNetworkSettings(w http.ResponseWriter, r *http.Request) {
	if s.networkSettings == nil {
		s.notFound(w, "Network settings are only available in the desktop app.")
		return
	}
	// lanAccess は必須なので、無いことを偽と区別して読む。
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
