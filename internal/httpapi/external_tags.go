package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// External tag mutations reuse UI storage operations and take IDs from request bodies; this layer only validates and maps response errors.

// MergeTags is atomic; missing sources are returned in notFoundIds.
func (e *externalServer) MergeTags(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagMergeRequest
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if len(body.SourceIds) == 0 || len(body.SourceIds) > domain.MaxTagBatch {
		e.invalidRequestLimit(w, extgen.TooManyTags, domain.MaxTagBatch,
			fmt.Sprintf("sourceIds must contain between 1 and %d items.", domain.MaxTagBatch))
		return
	}
	// Reject self-merges like the UI even though storage would silently skip them.
	if slices.Contains(body.SourceIds, body.TargetId) {
		reason := extgen.MergeSameTag
		e.invalidRequest(w, &reason, "sourceIds must not contain targetId. Nothing was changed.")
		return
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	outcome, err := e.s.tags.MergeTags(r.Context(), body.TargetId, body.SourceIds)
	if err != nil {
		e.writeTagError(w, err, "Could not merge the tags.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, extgen.TagMergeResponse{
		Tag:         toExternalTag(outcome.Tag),
		NotFoundIds: nonNilIDs(outcome.NotFoundIDs),
	}, e.s.logger)
}

// Renaming a tentative tag confirms it when the name changes.
func (e *externalServer) RenameTag(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagRenameRequest
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := e.s.tags.RenameTag(r.Context(), body.Id, body.Name)
	if err != nil {
		e.writeTagError(w, err, "Could not rename the tag.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toExternalTag(tag), e.s.logger)
}

func (e *externalServer) UpdateTagSynonyms(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagSynonymsRequest
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if !body.Action.Valid() {
		e.invalidRequest(w, nil, "action must be add or remove.")
		return
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	var tag domain.Tag
	var err error
	if body.Action == extgen.TagSynonymsRequestActionAdd {
		tag, err = e.s.tags.AddSynonym(r.Context(), body.Id, body.Name, body.MergeTagId)
	} else {
		// Normalize synonym removal like creation; invalid names cannot match any stored synonym.
		name, normalizeErr := domain.NormalizeTagName(body.Name)
		if normalizeErr != nil {
			name = body.Name
		}
		tag, err = e.s.tags.RemoveSynonym(r.Context(), body.Id, name)
	}
	if err != nil {
		e.writeTagError(w, err, "Could not update the synonyms.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toExternalTag(tag), e.s.logger)
}

// BatchTags validates through the shared UI helper and applies eligible IDs atomically, reporting absent/inapplicable IDs unchanged.
func (e *externalServer) BatchTags(w http.ResponseWriter, r *http.Request) {
	var body extgen.TagBatchRequest
	fields, ok := e.readJSONBodyFields(w, r, &body)
	if !ok {
		return
	}
	req, problem := tagBatchRequestOf(string(body.Action), body.KindId, fields.has("kindId"))
	if problem != "" {
		e.invalidRequest(w, nil, problem)
		return
	}
	if len(body.Ids) == 0 || len(body.Ids) > domain.MaxTagBatch {
		e.invalidRequestLimit(w, extgen.TooManyTags, domain.MaxTagBatch,
			fmt.Sprintf("ids must contain between 1 and %d items.", domain.MaxTagBatch))
		return
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	outcome, err := e.s.tags.BatchTags(r.Context(), req, body.Ids)
	if errors.Is(err, domain.ErrTagKindNotFound) {
		e.writeTagKindError(w, err)
		return
	}
	if err != nil {
		e.internalError(w, "Could not update the tags. Nothing was changed.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, extgen.TagBatchResponse{
		AppliedIds:       nonNilIDs(outcome.AppliedIDs),
		NotFoundIds:      nonNilIDs(outcome.NotFoundIDs),
		NotApplicableIds: nonNilIDs(outcome.NotApplicableIDs),
	}, e.s.logger)
}

func (e *externalServer) ListRejectedTagNames(w http.ResponseWriter, r *http.Request, params extgen.ListRejectedTagNamesParams) {
	limit := rejectedTagNamePageDefaultLimit
	if params.Limit != nil {
		limit = *params.Limit
		if limit < 1 || limit > domain.MaxTagPageLimit {
			e.invalidRequest(w, nil, fmt.Sprintf("limit must be between 1 and %d.", domain.MaxTagPageLimit))
			return
		}
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	page, err := e.s.tags.ListRejectedTagNames(r.Context(), cursor, limit)
	if errors.Is(err, domain.ErrInvalidCursor) {
		reason := extgen.InvalidCursor
		e.invalidRequest(w, &reason, "Cannot read the cursor. Read the rejected names again from the start.")
		return
	}
	if err != nil {
		e.internalError(w, "Could not load rejected tag names.", err)
		return
	}
	names := page.Items
	if names == nil {
		names = []string{}
	}
	body := extgen.RejectedTagNameList{Items: names, Total: page.Total}
	if page.NextCursor != "" {
		body.NextCursor = &page.NextCursor
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, e.s.logger)
}

// ForgetRejectedTagName returns normalized spelling when valid and original spelling otherwise; absent names return removed=false.
func (e *externalServer) ForgetRejectedTagName(w http.ResponseWriter, r *http.Request, params extgen.ForgetRejectedTagNameParams) {
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	name := params.Name
	if normalized, err := domain.NormalizeTagName(name); err == nil {
		name = normalized
	}
	removed, err := e.s.tags.ForgetRejectedTagName(r.Context(), name)
	if err != nil {
		e.internalError(w, "Could not update rejected tag names.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, extgen.RejectedTagNameForgetResult{Name: name, Removed: removed}, e.s.logger)
}

// Conflicts include canonical tag ID/name so callers can request a merge without rereading the list.
func (e *externalServer) writeTagError(w http.ResponseWriter, err error, failure string) {
	var invalid *domain.InvalidTagNameError
	var nameConflict *domain.TagNameConflict
	var mergeRequired *domain.TagMergeRequired
	switch {
	case errors.As(err, &invalid):
		e.writeInvalidTagName(w, invalid)
	case errors.Is(err, domain.ErrInvalidTagName):
		e.invalidRequest(w, nil, "The tag name cannot be used.")
	case errors.Is(err, domain.ErrTagNotFound):
		reason := extgen.TagNotFound
		writeExternalError(w, e.s, http.StatusNotFound, extgen.Error{
			Code: extgen.ErrorCodeNotFound, Reason: &reason, Message: "Tag not found.",
		})
	case errors.As(err, &nameConflict):
		e.writeTagConflict(w, extgen.TagNameTaken, nameConflict.Tag,
			fmt.Sprintf("The name is already used by the tag %q. Nothing was changed.", nameConflict.Tag.Name))
	case errors.As(err, &mergeRequired):
		e.writeTagConflict(w, extgen.TagMergeRequired, mergeRequired.Tag,
			fmt.Sprintf("%q is the name of another tag. Send mergeTagId %d to merge that tag into this one. Nothing was changed.",
				mergeRequired.Tag.Name, mergeRequired.Tag.ID))
	default:
		e.internalError(w, failure, err)
	}
}

// Conflict names retain their original spelling.
func (e *externalServer) writeTagConflict(w http.ResponseWriter, reason extgen.ErrorReason, owner domain.TagRef, message string) {
	tagID, tagName := owner.ID, owner.Name
	writeExternalError(w, e.s, http.StatusConflict, extgen.Error{
		Code: extgen.ErrorCodeConflict, Reason: &reason, TagId: &tagID, TagName: &tagName, Message: message,
	})
}

func (e *externalServer) writeInvalidTagName(w http.ResponseWriter, invalid *domain.InvalidTagNameError) {
	switch invalid.Problem {
	case domain.TagNameEmpty:
		reason := extgen.TagNameEmpty
		e.invalidRequest(w, &reason, "Enter a tag name.")
	case domain.TagNameControlCharacters:
		reason := extgen.TagNameControlCharacters
		e.invalidRequest(w, &reason, "Tag names cannot contain control characters.")
	case domain.TagNameTooLong:
		e.invalidRequestLimit(w, extgen.TagNameTooLong, domain.TagNameMaxLength,
			"Tag names must be at most "+strconv.Itoa(domain.TagNameMaxLength)+" characters.")
	default:
		e.invalidRequest(w, nil, "The tag name cannot be used.")
	}
}
