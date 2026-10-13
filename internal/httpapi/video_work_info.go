package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 動画の作品情報の置き換えと読み出し（specs/047-video-work-info/contracts/screen-api.md、
// research.md R-5・R-7・R-8）。1 つの取引で済むので internal/app は通さず、internal/store の
// *WorkInfoStore を直接呼ぶ。

// WorkInfoStore は動画の作品情報の保存先である。internal/store の *WorkInfoStore がこれを満たす。
type WorkInfoStore interface {
	// Set は動画 videoID の作品情報を info でまるごと置き換え（整えて空の項目は解除）、作品情報を
	// 含む反映後の動画を返す。規則に合わなければ *domain.InvalidWorkInfoError、動画が無いか
	// 登録フォルダの下に所在が無ければ domain.ErrNotFound を返す。
	Set(ctx context.Context, videoID int64, info domain.WorkInfo) (domain.Video, error)
	// ByUserKeys は利用者データの鍵 userKeys の作品情報を返す。作品情報の無い鍵は含めない。
	ByUserKeys(ctx context.Context, userKeys []string) (map[string]domain.WorkInfo, error)
}

// SetVideoWorkInfo は動画の作品情報を置き換える（PUT /api/videos/{id}/work-info）。項目が無い・
// null・整えて空なら解除する（research.md R-5）。
func (s *server) SetVideoWorkInfo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	var body workInfoRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.workInfo == nil {
		s.internalError(w, "Work information storage is not configured.", nil)
		return
	}

	video, err := s.workInfo.Set(r.Context(), id, workInfoFromInput(gen.WorkInfoInput(body)))
	switch {
	case errors.Is(err, domain.ErrInvalidWorkInfo):
		s.invalidWorkInfo(w, err)
		return
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
		return
	case err != nil:
		s.internalError(w, "Could not save the work information.", err)
		return
	}
	s.writeVideoDetail(w, r, video)
}

// workInfoRequest は setVideoWorkInfo の本文である。gen.WorkInfoInput と同じ形だが、encoding/json が
// 黙って零値にする 2 つの null を断る。本文そのものの null（項目ごとの null と違い、解除ではなく
// 形の誤り）と、performers の要素の null（要素は string で、空の名前として落とすと置き換えで
// 出演者が消える）である。
type workInfoRequest gen.WorkInfoInput

var (
	errWorkInfoBodyNull      = errors.New("the work information body must be a JSON object")
	errWorkInfoPerformerNull = errors.New("performers must not contain null")
)

func (b *workInfoRequest) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errWorkInfoBodyNull
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var input gen.WorkInfoInput
	if err := decoder.Decode(&input); err != nil {
		return err
	}
	var performers struct {
		Performers []*string `json:"performers"`
	}
	if err := json.Unmarshal(data, &performers); err != nil {
		return err
	}
	if slices.Contains(performers.Performers, nil) {
		return errWorkInfoPerformerNull
	}
	*b = workInfoRequest(input)
	return nil
}

// workInfoFromInput は本文を domain.WorkInfo へ写す。無い項目と null は空（解除）にする。
func workInfoFromInput(body gen.WorkInfoInput) domain.WorkInfo {
	info := domain.WorkInfo{
		Code:        derefString(body.Code),
		ReleaseDate: derefString(body.ReleaseDate),
		Maker:       derefString(body.Maker),
		Description: derefString(body.Description),
		SourceURL:   derefString(body.SourceUrl),
	}
	if body.Performers != nil {
		info.Performers = *body.Performers
	}
	return info
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// workInfoErrorDetail は作品情報の規則違反を誤りの本文へ写したものである。画面の API と外部連携の
// API の両方が、それぞれの Error の形に載せる。Limit は上限を返す理由のときだけ入る。
type workInfoErrorDetail struct {
	Reason  gen.ErrorReason
	Field   domain.WorkInfoField
	Limit   *int
	Message string
}

// describeInvalidWorkInfo は *domain.InvalidWorkInfoError を reason・field・limit へ写す
// （contracts/screen-api.md「Added reason values and field」）。err が作品情報の規則違反でなければ
// false を返す。
func describeInvalidWorkInfo(err error) (workInfoErrorDetail, bool) {
	var invalid *domain.InvalidWorkInfoError
	if !errors.As(err, &invalid) {
		return workInfoErrorDetail{}, false
	}
	detail := workInfoErrorDetail{Field: invalid.Field, Message: invalid.Error()}
	switch invalid.Problem {
	case domain.WorkInfoControlCharacters:
		detail.Reason = reasonWorkInfoControlCharacters
	case domain.WorkInfoTooLong:
		detail.Reason = reasonWorkInfoTooLong
	case domain.WorkInfoInvalidDate:
		detail.Reason = reasonWorkInfoInvalidDate
	case domain.WorkInfoInvalidURL:
		detail.Reason = reasonWorkInfoInvalidURL
	case domain.WorkInfoTooMany:
		detail.Reason = reasonTooManyPerformers
	default:
		return workInfoErrorDetail{}, false
	}
	if invalid.Problem == domain.WorkInfoTooLong || invalid.Problem == domain.WorkInfoTooMany {
		limit := invalid.Limit
		detail.Limit = &limit
	}
	return detail, true
}

// invalidWorkInfo は作品情報の規則違反を reason・field 付きの 400 にする。
func (s *server) invalidWorkInfo(w http.ResponseWriter, err error) {
	detail, ok := describeInvalidWorkInfo(err)
	if !ok {
		s.invalidRequest(w, "The work information cannot be used.")
		return
	}
	reason := detail.Reason
	field := gen.WorkInfoField(detail.Field)
	s.writeErrorBody(w, http.StatusBadRequest, gen.Error{
		Code:    codeInvalidRequest,
		Reason:  &reason,
		Field:   &field,
		Limit:   detail.Limit,
		Message: detail.Message,
	})
}

// workInfoFor は動画 1 件の応答に載せる作品情報を返す。video が作品情報を持っていればそれを、
// 無ければ利用者データの鍵から読む。保存先が無いか、作品情報が無ければ nil を返す。
func (s *server) workInfoFor(ctx context.Context, video domain.Video) (*domain.WorkInfo, error) {
	if video.WorkInfo != nil {
		return video.WorkInfo, nil
	}
	if s.workInfo == nil || video.UserKey == "" {
		return nil, nil
	}
	infos, err := s.workInfo.ByUserKeys(ctx, []string{video.UserKey})
	if err != nil {
		return nil, err
	}
	info, ok := infos[video.UserKey]
	if !ok {
		return nil, nil
	}
	return &info, nil
}

// apiWorkInfo は作品情報を応答の形へ写す。未設定の項目は省き、どの項目も無ければ nil を返す
// （contracts/screen-api.md「WorkInfo schema」）。出演者は入力した順で、いなければ空配列にする。
func apiWorkInfo(info *domain.WorkInfo) *gen.WorkInfo {
	if info == nil || info.IsEmpty() {
		return nil
	}
	performers := info.Performers
	if performers == nil {
		performers = []string{}
	}
	return &gen.WorkInfo{
		Code:        optionalString(info.Code),
		ReleaseDate: optionalString(info.ReleaseDate),
		Maker:       optionalString(info.Maker),
		Performers:  performers,
		Description: optionalString(info.Description),
		SourceUrl:   optionalString(info.SourceURL),
	}
}
