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

// TagStore.ApplyVideoTags validates names/resolves all videos atomically; skipped rejected
// tentative names are reported in skippedTags.
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

// Read tentative as raw JSON because generated *bool conflates omitted and null; shadowing
// preserves unknown-field rejection.
type videoTagsRequestBody struct {
	extgen.VideoTagsRequest
	Tentative json.RawMessage `json:"tentative"`
}

// Omitted tentative defaults false; explicit null and nonbooleans are invalid.
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

// Require exactly one nonempty reference; nonpositive IDs are lookup misses (404), not shape
// errors.
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

func (e *externalServer) invalidRequestLimit(w http.ResponseWriter, reason extgen.ErrorReason, limit int, message string) {
	writeExternalError(w, e.s, http.StatusBadRequest, extgen.Error{
		Code: extgen.ErrorCodeInvalidRequest, Reason: &reason, Limit: &limit, Message: message,
	})
}

// readJSONBody accepts exactly one value, rejects unknown fields, and rechecks Content-Type at the
// handler boundary.
func (e *externalServer) readJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		e.invalidRequest(w, nil, "Content-Type must be application/json.")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, e.bodyLimit()))
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

// Oversized bodies report the limit rather than a misleading truncated-JSON error.
func (e *externalServer) invalidBody(w http.ResponseWriter, err error, message string) {
	if tooLarge := (*http.MaxBytesError)(nil); errors.As(err, &tooLarge) {
		message = fmt.Sprintf("The body must be at most %d bytes. Split the videos into smaller requests.", tooLarge.Limit)
	}
	e.invalidRequest(w, nil, message)
}

// The body cap accommodates 20,000 path references at roughly 1.6KB each, including multibyte
// names.
const externalBodyLimit = 32 << 20

func (e *externalServer) bodyLimit() int64 {
	if e.maxBody > 0 {
		return e.maxBody
	}
	return externalBodyLimit
}
