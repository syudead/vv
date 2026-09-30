package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 動画の表示名の設定・解除（specs/029-video-overrides/contracts/screen-api.md §1、
// research.md R-8・R-10）。1つの取引で済むので internal/app は通さず、
// internal/store の *OverrideStore を直接呼ぶ。

// OverrideStore は動画の上書きの保存先である。internal/store の *OverrideStore が
// これを満たす。
type OverrideStore interface {
	// SetDisplayName は動画 videoID の表示名を name にし（整えて空なら解除）、反映後の
	// 動画を返す。規則に合わなければ *domain.InvalidDisplayNameError、動画が無いか
	// 登録フォルダの下に所在が無ければ domain.ErrNotFound を返す。
	SetDisplayName(ctx context.Context, videoID int64, name string) (domain.Video, error)
}

// SetVideoDisplayName は動画の表示名を設定・解除する（PUT /api/videos/{id}/display-name）。
func (s *server) SetVideoDisplayName(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	// gen.DisplayNameUpdate の displayName は string なので、欠けても空（解除）に読めて
	// しまう。欠けた本文で表示名を消さないよう、有無を区別して読む。
	var body struct {
		DisplayName *string `json:"displayName"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.DisplayName == nil {
		s.invalidRequest(w, "displayName is required.")
		return
	}
	if s.overrides == nil {
		s.internalError(w, "Video override storage is not configured.", nil)
		return
	}

	video, err := s.overrides.SetDisplayName(r.Context(), id, *body.DisplayName)
	switch {
	case errors.Is(err, domain.ErrInvalidDisplayName):
		s.invalidDisplayName(w, err)
		return
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
		return
	case err != nil:
		s.internalError(w, "Could not save the display name.", err)
		return
	}
	s.writeVideoDetail(w, r, video)
}

// invalidDisplayName は表示名の規則違反を reason 付きの 400 にする
// （contracts/screen-api.md §1・§3）。
func (s *server) invalidDisplayName(w http.ResponseWriter, err error) {
	var invalid *domain.InvalidDisplayNameError
	if !errors.As(err, &invalid) {
		s.invalidRequest(w, "The display name cannot be used.")
		return
	}
	switch invalid.Problem {
	case domain.DisplayNameControlCharacters:
		s.invalidRequestReason(w, reasonDisplayNameControlCharacters, "Display names cannot contain control characters.")
	case domain.DisplayNameTooLong:
		s.invalidRequestLimit(w, reasonDisplayNameTooLong, domain.DisplayNameMaxLength,
			"Display names must be at most "+strconv.Itoa(domain.DisplayNameMaxLength)+" characters.")
	default:
		s.invalidRequest(w, "The display name cannot be used.")
	}
}
