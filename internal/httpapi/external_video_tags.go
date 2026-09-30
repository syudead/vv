package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// UpdateVideoTags は複数の動画に名前で指定したタグを付ける・外す・置き換える
// （POST /api/v1/video-tags、specs/026-external-api/contracts/external-api.md §4）。
// 本文の形と件数の上限をここで確かめ、名前の規則と動画の引き当ては 1 つのトランザクションで
// 行う保存先（TagStore.ApplyVideoTags、research.md R-7）に任せる。internal/app は通さない。
// tentative が真の add・replace で却下した名前に当たって飛ばした名前は skippedTags に返す
// （specs/031-tentative-tags/contracts/external-api.md §1）。
func (e *externalServer) UpdateVideoTags(w http.ResponseWriter, r *http.Request) {
	var request videoTagsRequestBody
	if !e.readJSONBody(w, r, &request) {
		return
	}
	body := request.VideoTagsRequest
	tentative, ok := request.tentative()
	if !ok {
		e.invalidRequest(w, nil, "tentative must be true or false.")
		return
	}
	action := domain.VideoTagsAction(body.Action)
	if !action.Valid() {
		e.invalidRequest(w, nil, "action must be add, remove or replace.")
		return
	}
	if len(body.Videos) < 1 || len(body.Videos) > maxVideoTagsIDs {
		e.invalidRequestLimit(w, extgen.TooManyVideos, maxVideoTagsIDs,
			fmt.Sprintf("videos must contain between 1 and %d items.", maxVideoTagsIDs))
		return
	}
	refs := make([]domain.VideoRef, 0, len(body.Videos))
	for i, video := range body.Videos {
		ref, ok := externalVideoRef(video)
		if !ok {
			index := i
			writeExternalError(w, e.s, http.StatusBadRequest, extgen.Error{
				Code: extgen.ErrorCodeInvalidRequest, Index: &index,
				Message: "Each video must have exactly one of id, contentKey and path.",
			})
			return
		}
		refs = append(refs, ref)
	}
	if body.Tags == nil {
		e.invalidRequest(w, nil, "tags is required.")
		return
	}
	minTags := 1
	if action == domain.VideoTagsReplace {
		minTags = 0
	}
	if len(body.Tags) < minTags || len(body.Tags) > domain.ExternalVideoTagsMaxNames {
		e.invalidRequestLimit(w, extgen.TooManyTags, domain.ExternalVideoTagsMaxNames,
			fmt.Sprintf("tags must contain between %d and %d items.", minTags, domain.ExternalVideoTagsMaxNames))
		return
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}

	outcome, err := e.s.tags.ApplyVideoTags(r.Context(), refs, action, body.Tags, tentative)
	if err != nil {
		e.writeVideoTagsError(w, err)
		return
	}
	skipped := outcome.SkippedNames
	if skipped == nil {
		skipped = []string{}
	}
	out := extgen.VideoTagsResponse{
		Items:       make([]extgen.VideoTagsItem, 0, len(outcome.Items)),
		SkippedTags: skipped,
	}
	for _, result := range outcome.Items {
		out.Items = append(out.Items, extgen.VideoTagsItem{
			Video: extgen.VideoTagsVideo{Id: result.VideoID, ContentKey: result.ContentKey},
			Tags:  toExternalVideoTags(result.Tags),
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, e.s.logger)
}

// videoTagsRequestBody は POST /api/v1/video-tags の本文である。生成した
// extgen.VideoTagsRequest の Tentative（*bool）は省略と明示の null を区別できないので、
// tentative だけ生のまま読み、tentative() で確かめる。外側の Tentative が埋め込んだ型の同名の
// 項目を隠すので、知らない項目を誤りにする readJSONBody の扱いはそのまま効く。
type videoTagsRequestBody struct {
	extgen.VideoTagsRequest
	Tentative json.RawMessage `json:"tentative"`
}

// tentative は本文の tentative を返す。省略は偽（specs/031-tentative-tags/contracts/external-api.md
// §1）。null を含め真偽値でない値は ok = false にし、呼び出し側が 400 invalid_request を返す。
func (b videoTagsRequestBody) tentative() (value, ok bool) {
	if b.Tentative == nil {
		return false, true
	}
	if string(b.Tentative) == "null" {
		return false, false
	}
	if err := json.Unmarshal(b.Tentative, &value); err != nil {
		return false, false
	}
	return value, true
}

// externalVideoRef は本文の動画の指定を domain.VideoRef にする。id・contentKey・path の
// ちょうど 1 つを持たない、または contentKey・path が空なら ok = false。1 未満の id は形の
// 誤りにせず、どの動画にも当たらない指定として保存先が 404 にする（lookup と同じ）。
func externalVideoRef(video extgen.VideoRef) (domain.VideoRef, bool) {
	var ref domain.VideoRef
	given := 0
	if video.Id != nil {
		given++
		ref.ID = *video.Id
	}
	if video.ContentKey != nil {
		given++
		ref.ContentKey = *video.ContentKey
		if ref.ContentKey == "" {
			return domain.VideoRef{}, false
		}
	}
	if video.Path != nil {
		given++
		ref.Path = *video.Path
		if ref.Path == "" {
			return domain.VideoRef{}, false
		}
	}
	return ref, given == 1
}

// writeVideoTagsError はタグの一括操作の保存先の誤りを応答へ写す。
func (e *externalServer) writeVideoTagsError(w http.ResponseWriter, err error) {
	var notFound *domain.VideoRefNotFoundError
	if errors.As(err, &notFound) {
		reason, index := extgen.VideoNotFound, notFound.Index
		writeExternalError(w, e.s, http.StatusNotFound, extgen.Error{
			Code: extgen.ErrorCodeNotFound, Reason: &reason, Index: &index,
			Message: "Video " + strconv.Itoa(index) + " is not in the library. Nothing was changed.",
		})
		return
	}
	var nameAt *domain.TagNameAtError
	var invalid *domain.InvalidTagNameError
	if errors.As(err, &nameAt) && errors.As(err, &invalid) {
		body := extgen.Error{Code: extgen.ErrorCodeInvalidRequest, Index: &nameAt.Index}
		prefix := "Tag " + strconv.Itoa(nameAt.Index) + ": "
		var reason extgen.ErrorReason
		switch invalid.Problem {
		case domain.TagNameEmpty:
			reason, body.Message = extgen.TagNameEmpty, prefix+"Enter a tag name."
		case domain.TagNameControlCharacters:
			reason, body.Message = extgen.TagNameControlCharacters, prefix+"Tag names cannot contain control characters."
		case domain.TagNameTooLong:
			limit := domain.TagNameMaxLength
			reason, body.Limit = extgen.TagNameTooLong, &limit
			body.Message = prefix + "Tag names must be at most " + strconv.Itoa(limit) + " characters."
		}
		if reason != "" {
			body.Reason = &reason
		} else {
			body.Message = prefix + "The tag name cannot be used."
		}
		writeExternalError(w, e.s, http.StatusBadRequest, body)
		return
	}
	e.internalError(w, "Could not update the video tags.", err)
}

// invalidRequestLimit は上限を添えた 400 invalid_request を返す。
func (e *externalServer) invalidRequestLimit(w http.ResponseWriter, reason extgen.ErrorReason, limit int, message string) {
	writeExternalError(w, e.s, http.StatusBadRequest, extgen.Error{
		Code: extgen.ErrorCodeInvalidRequest, Reason: &reason, Limit: &limit, Message: message,
	})
}

// readJSONBody は本文を 1 つの JSON の値として target に読む。読めなければ 400
// invalid_request を書いて false を返す。Content-Type は境界（mutationBoundary）も確かめるが、
// 画面の API の readJSONBody と同じくここでも確かめる。知らない項目は、画面の API と同じく
// 誤りにする（綴りの誤りを黙って無視しない）。
func (e *externalServer) readJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		e.invalidRequest(w, nil, "Content-Type must be application/json.")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, externalBodyLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		e.invalidBody(w, err, "Cannot parse the JSON body.")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		e.invalidBody(w, err, "The body must contain exactly one JSON value.")
		return false
	}
	return true
}

// invalidBody は本文を読めなかったときの 400 invalid_request を返す。上限を超えた本文は、
// 途中で切れた JSON の誤りとしてではなく、上限を超えたことを示す。
func (e *externalServer) invalidBody(w http.ResponseWriter, err error, message string) {
	if tooLarge := (*http.MaxBytesError)(nil); errors.As(err, &tooLarge) {
		message = fmt.Sprintf("The body must be at most %d bytes. Split the videos into smaller requests.", tooLarge.Limit)
	}
	e.invalidRequest(w, nil, message)
}

// externalBodyLimit は外部連携 API が読む本文の上限（バイト）である。videos の上限 20000 件を
// パスで指定しても収まる大きさにする（1 件あたり 1.6 KB 程度、日本語の名前で 500 文字ほどまで）。
// 超えた本文は invalidBody が上限を示して断る。
const externalBodyLimit = 32 << 20
