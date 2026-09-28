package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetTranscodingSettings はライブ変換の映像エンコード方式の設定と、各方式が使えるかを返す
// （specs/025-hardware-encoding/contracts/transcoding-settings-api.md §2）。
func (s *server) GetTranscodingSettings(w http.ResponseWriter, _ *http.Request) {
	if s.transcodeSettings == nil {
		s.internalError(w, "Transcoding settings are not configured.", nil)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITranscodingSettings(s.transcodeSettings.Current()), s.logger)
}

// UpdateTranscodingSettings は方式を保存し、保存後の状態を返す（契約 §3）。方式の決定と
// 使えない方式を拒む判断は TranscodeSettings が持つ。
func (s *server) UpdateTranscodingSettings(w http.ResponseWriter, r *http.Request) {
	var body gen.UpdateTranscodingSettingsRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	choice := domain.EncoderChoice(body.VideoEncoder)
	if !choice.Valid() {
		s.invalidRequest(w, "videoEncoder must be one of software, nvenc, qsv, vaapi, videotoolbox or auto.")
		return
	}
	if s.transcodeSettings == nil {
		s.internalError(w, "Transcoding settings are not configured.", nil)
		return
	}
	encoding, err := s.transcodeSettings.Select(r.Context(), choice)
	if err != nil {
		if errors.Is(err, domain.ErrEncoderUnavailable) {
			s.conflictReason(w, reasonEncoderUnavailable, "The selected video encoder is not available on this server.")
			return
		}
		s.internalError(w, "Could not save the transcoding settings.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITranscodingSettings(encoding), s.logger)
}

// toAPITranscodingSettings は今の状態を契約の形にする。確認の補足（Detail）はログ用で、
// 応答には載せない。
func toAPITranscodingSettings(encoding domain.TranscodeEncoding) gen.TranscodingSettings {
	encoders := make([]gen.EncoderAvailability, 0, len(encoding.Encoders))
	for _, a := range encoding.Encoders {
		item := gen.EncoderAvailability{
			Encoder: gen.VideoEncoder(a.Encoder),
			State:   gen.EncoderAvailabilityState(a.State),
		}
		if a.State == domain.EncoderUnavailable && a.Reason != "" {
			reason := gen.EncoderUnavailableReason(a.Reason)
			item.Reason = &reason
		}
		encoders = append(encoders, item)
	}
	out := gen.TranscodingSettings{
		VideoEncoder:     gen.VideoEncoderChoice(encoding.Choice),
		EffectiveEncoder: gen.VideoEncoder(encoding.Effective),
		Checking:         encoding.Checking,
		Encoders:         encoders,
	}
	if encoding.FallbackReason != "" {
		reason := gen.EncoderFallbackReason(encoding.FallbackReason)
		out.FallbackReason = &reason
	}
	return out
}
