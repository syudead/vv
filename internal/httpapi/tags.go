package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Tag operations use one TagStore transaction; this layer only parses and presents.

// Omitted limit returns all matching tags without nextCursor.
func (s *server) ListTags(w http.ResponseWriter, r *http.Request, params gen.ListTagsParams) {
	query, ok := s.parseTagListQuery(w, params)
	if !ok {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	page, err := s.tags.ListTags(r.Context(), query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load tags.", err)
		return
	}
	out := make([]gen.Tag, 0, len(page.Items))
	for _, tag := range page.Items {
		out = append(out, toAPITag(tag))
	}
	body := gen.TagList{Items: out, Total: page.Total, TotalAll: page.TotalAll}
	if page.NextCursor != "" {
		body.NextCursor = &page.NextCursor
	}
	if page.Exact != nil {
		exact := toAPITag(*page.Exact)
		body.Exact = &exact
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, s.logger)
}

// Storage decodes cursor contents; this boundary validates other query constraints.
func (s *server) parseTagListQuery(w http.ResponseWriter, params gen.ListTagsParams) (domain.TagListQuery, bool) {
	sort := sortParam(params.Sort)
	query, problem := buildTagListQuery(params.Q, params.Tentative, params.Unused, sort, params.Cursor, params.Limit)
	if !s.acceptTagListQuery(w, problem) {
		return domain.TagListQuery{}, false
	}
	if params.Kind != nil {
		kind, ok := parseTagKindFilter(*params.Kind)
		if !ok {
			s.invalidRequest(w, "kind must be a positive integer or none.")
			return domain.TagListQuery{}, false
		}
		query.Kind = kind
	}
	return query, true
}

func parseTagKindFilter(raw string) (domain.TagKindFilter, bool) {
	if raw == "none" {
		return domain.TagKindFilter{None: true}, true
	}
	if raw == "" || raw[0] < '1' || raw[0] > '9' {
		return domain.TagKindFilter{}, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return domain.TagKindFilter{}, false
	}
	return domain.TagKindFilter{ID: id}, true
}

func (s *server) acceptTagListQuery(w http.ResponseWriter, problem tagListProblem) bool {
	switch problem {
	case tagListSortUnknown:
		s.invalidRequest(w, "Unknown sort order.")
		return false
	case tagListLimitOutOfRange:
		s.invalidRequest(w, fmt.Sprintf("limit must be between 1 and %d.", domain.MaxTagPageLimit))
		return false
	case tagListSearchTooLong:
		s.invalidRequestLimit(w, reasonSearchTooLong, maxQueryLength,
			fmt.Sprintf("Search text must be at most %d characters.", maxQueryLength))
		return false
	}
	return true
}

func sortParam[T ~string](sort *T) *string {
	if sort == nil {
		return nil
	}
	value := string(*sort)
	return &value
}

type tagListProblem int

const (
	tagListOK tagListProblem = iota
	tagListSortUnknown
	tagListLimitOutOfRange
	tagListSearchTooLong
)

// UI and external tag lists share query rules: omitted sort means natural name order, and omitted limit returns all results while ignoring cursor.
func buildTagListQuery(q *string, tentative, unused *bool, sort, cursor *string, limit *int) (domain.TagListQuery, tagListProblem) {
	query := domain.TagListQuery{
		TentativeOnly: tentative != nil && *tentative,
		UnusedOnly:    unused != nil && *unused,
		Sort:          domain.TagSortName,
	}
	if sort != nil {
		query.Sort = domain.TagSort(*sort)
		if !query.Sort.Valid() {
			return domain.TagListQuery{}, tagListSortUnknown
		}
	}
	if limit != nil {
		if *limit < 1 || *limit > domain.MaxTagPageLimit {
			return domain.TagListQuery{}, tagListLimitOutOfRange
		}
		query.Limit = *limit
		if cursor != nil {
			query.Cursor = *cursor
		}
	}
	if q != nil {
		if len([]rune(*q)) > maxQueryLength {
			return domain.TagListQuery{}, tagListSearchTooLong
		}
		query.Search = *q
	}
	return query, tagListOK
}

func (s *server) CreateTag(w http.ResponseWriter, r *http.Request) {
	var body gen.CreateTagRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := s.tags.CreateTag(r.Context(), body.Name)
	if err != nil {
		s.writeTagError(w, err, normalizedTagNameOrRaw(body.Name))
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, toAPITag(tag), s.logger)
}

func (s *server) RenameTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	var body gen.RenameTagRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := s.tags.RenameTag(r.Context(), id, body.Name)
	if err != nil {
		s.writeTagError(w, err, normalizedTagNameOrRaw(body.Name))
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITag(tag), s.logger)
}

func (s *server) DeleteTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	if err := s.tags.DeleteTag(r.Context(), id); err != nil {
		s.writeTagError(w, err, "")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) MergeTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	var body gen.MergeTagRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if !s.validTagBatchIDs(w, "sourceIds", body.SourceIds) {
		return
	}
	// Reject self-merges at the API boundary even though storage silently skips the target among sources.
	if slices.Contains(body.SourceIds, id) {
		s.invalidRequestReason(w, reasonMergeSameTag, "The source and target of a merge must be different tags.")
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	outcome, err := s.tags.MergeTags(r.Context(), id, body.SourceIds)
	if err != nil {
		s.writeTagError(w, err, "")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.TagMergeResponse{
		Tag:         toAPITag(outcome.Tag),
		NotFoundIds: nonNilIDs(outcome.NotFoundIDs),
	}, s.logger)
}

func (s *server) AddTagSynonym(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	var body gen.AddTagSynonymRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := s.tags.AddSynonym(r.Context(), id, body.Name, body.MergeTagId)
	if err != nil {
		s.writeTagError(w, err, normalizedTagNameOrRaw(body.Name))
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITag(tag), s.logger)
}

func (s *server) RemoveTagSynonym(w http.ResponseWriter, r *http.Request, id gen.TagId, params gen.RemoveTagSynonymParams) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	// Normalize synonym removal exactly like creation; invalid raw names cannot match, while storage still reports a missing tag.
	name, err := domain.NormalizeTagName(params.Name)
	if err != nil {
		name = params.Name
	}
	if _, err := s.tags.RemoveSynonym(r.Context(), id, name); err != nil {
		s.writeTagError(w, err, "")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// Confirming an already-confirmed tag is a no-op returning its current state.
func (s *server) ConfirmTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	tag, err := s.tags.ConfirmTag(r.Context(), id)
	if err != nil {
		s.writeTagError(w, err, "")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPITag(tag), s.logger)
}

func (s *server) RejectTag(w http.ResponseWriter, r *http.Request, id gen.TagId) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	if _, err := s.tags.RejectTag(r.Context(), id); err != nil {
		s.writeTagError(w, err, "")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// BatchTags applies eligible IDs atomically and reports missing/inapplicable IDs unchanged (docs/design-docs/catalog-organization.md).
func (s *server) BatchTags(w http.ResponseWriter, r *http.Request) {
	var body gen.TagBatchRequest
	present, ok := s.readJSONBodyFields(w, r, &body)
	if !ok {
		return
	}
	req, ok := s.tagBatchRequest(w, body, present("kindId"))
	if !ok {
		return
	}
	if !s.validTagBatchIDs(w, "ids", body.Ids) {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	outcome, err := s.tags.BatchTags(r.Context(), req, body.Ids)
	if errors.Is(err, domain.ErrTagKindNotFound) {
		s.writeTagKindError(w, err)
		return
	}
	if err != nil {
		s.internalError(w, "Could not update the tags.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.TagBatchResponse{
		AppliedIds:       nonNilIDs(outcome.AppliedIDs),
		NotFoundIds:      nonNilIDs(outcome.NotFoundIDs),
		NotApplicableIds: nonNilIDs(outcome.NotApplicableIDs),
	}, s.logger)
}

func (s *server) tagBatchRequest(w http.ResponseWriter, body gen.TagBatchRequest, kindPresent bool) (domain.TagBatchRequest, bool) {
	req, problem := tagBatchRequestOf(string(body.Action), body.KindId, kindPresent)
	if problem != "" {
		s.invalidRequest(w, problem)
		return domain.TagBatchRequest{}, false
	}
	return req, true
}

// setKind requires a positive kindId; every other action rejects any present kindId, including null, so callers must preserve field presence.
func tagBatchRequestOf(action string, kindID *int64, kindPresent bool) (domain.TagBatchRequest, string) {
	req := domain.TagBatchRequest{Action: domain.TagBatchAction(action)}
	switch {
	case !req.Action.Valid():
		return domain.TagBatchRequest{}, "action must be one of confirm, reject, delete, setKind or clearKind."
	case req.Action == domain.TagBatchSetKind && (kindID == nil || *kindID < 1):
		return domain.TagBatchRequest{}, "setKind needs a positive kindId."
	case req.Action != domain.TagBatchSetKind && kindPresent:
		return domain.TagBatchRequest{}, "kindId is allowed only with setKind."
	}
	if kindID != nil {
		req.KindID = *kindID
	}
	return req, ""
}

func (s *server) TagImpact(w http.ResponseWriter, r *http.Request) {
	var body gen.TagImpactRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	action := domain.TagImpactAction(body.Action)
	if !action.Valid() {
		s.invalidRequest(w, "action must be one of reject, delete or merge.")
		return
	}
	if !s.validTagBatchIDs(w, "ids", body.Ids) {
		return
	}
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	impact, err := s.tags.TagImpact(r.Context(), action, body.Ids)
	if err != nil {
		s.internalError(w, "Could not count the affected videos.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.TagImpactResponse{TagCount: impact.TagCount, VideoCount: impact.VideoCount}, s.logger)
}

func (s *server) validTagBatchIDs(w http.ResponseWriter, field string, ids []int64) bool {
	if len(ids) == 0 || len(ids) > domain.MaxTagBatch {
		s.invalidRequestLimit(w, reasonTooManyTags, domain.MaxTagBatch,
			fmt.Sprintf("%s must contain between 1 and %d items.", field, domain.MaxTagBatch))
		return false
	}
	return true
}

func nonNilIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

const rejectedTagNamePageDefaultLimit = 100

func (s *server) ListRejectedTagNames(w http.ResponseWriter, r *http.Request, params gen.ListRejectedTagNamesParams) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	limit := rejectedTagNamePageDefaultLimit
	if params.Limit != nil {
		limit = *params.Limit
		if limit < 1 || limit > domain.MaxTagPageLimit {
			s.invalidRequest(w, fmt.Sprintf("limit must be between 1 and %d.", domain.MaxTagPageLimit))
			return
		}
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}

	page, err := s.tags.ListRejectedTagNames(r.Context(), cursor, limit)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load rejected tag names.", err)
		return
	}
	names := page.Items
	if names == nil {
		names = []string{}
	}
	body := gen.RejectedTagNameList{Items: names, Total: page.Total}
	if page.NextCursor != "" {
		body.NextCursor = &page.NextCursor
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, s.logger)
}

// Forgetting absent or invalid names is a no-op returning 204; storage normalizes valid input.
func (s *server) ForgetRejectedTagName(w http.ResponseWriter, r *http.Request, params gen.ForgetRejectedTagNameParams) {
	if s.tags == nil {
		s.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	if _, err := s.tags.ForgetRejectedTagName(r.Context(), params.Name); err != nil {
		s.internalError(w, "Could not update rejected tag names.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func toAPITag(tag domain.Tag) gen.Tag {
	synonyms := tag.Synonyms
	if synonyms == nil {
		synonyms = []string{}
	}
	return gen.Tag{
		Id: tag.ID, Name: tag.Name, Synonyms: synonyms, VideoCount: tag.VideoCount, Tentative: tag.Tentative,
		CreatedAt: tag.CreatedAt, Kind: toAPITagKindRef(tag.Kind),
	}
}

func toAPITagRef(ref domain.TagRef) gen.TagRef {
	return gen.TagRef{Id: ref.ID, Name: ref.Name, Tentative: ref.Tentative, Kind: toAPITagKindRef(ref.Kind)}
}

// Folder-only assignments still expose the tag's tentative state.
func toAPIVideoTag(tag domain.VideoTag) gen.VideoTag {
	out := gen.VideoTag{
		Id: tag.ID, Name: tag.Name, Manual: tag.Manual, FromFolder: tag.FromFolder, Tentative: tag.Tentative,
		Kind: toAPITagKindRef(tag.Kind),
	}
	if tag.Unconfirmed {
		unconfirmed := true
		out.Unconfirmed = &unconfirmed
	}
	return out
}

// Normalize submitted names before comparing with stored canonical spelling to avoid misclassifying a canonical-name conflict as a synonym conflict.
func normalizedTagNameOrRaw(name string) string {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return name
	}
	return normalized
}

func (s *server) writeTagError(w http.ResponseWriter, err error, submittedName string) {
	var nameConflict *domain.TagNameConflict
	var mergeRequired *domain.TagMergeRequired
	switch {
	case errors.Is(err, domain.ErrInvalidTagName):
		s.invalidTagName(w, err, "")
	case errors.Is(err, domain.ErrTagNotFound):
		s.writeError(w, http.StatusNotFound, codeTagNotFound, "Tag not found.")
	case errors.Is(err, domain.ErrTagNotTentative):
		s.writeError(w, http.StatusConflict, codeTagNotTentative, "The tag is already confirmed.")
	case errors.As(err, &nameConflict):
		s.writeTagNameTaken(w, submittedName, nameConflict.Tag)
	case errors.As(err, &mergeRequired):
		tagName := mergeRequired.Tag.Name
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code:    codeTagMergeRequired,
			TagName: &tagName,
			Message: fmt.Sprintf("%q is already the name of another tag. Confirm the tag to merge.", tagName),
		})
	default:
		s.internalError(w, "Could not update the tag.", err)
	}
}

// Canonical-name equality selects name_is_tag, otherwise name_is_synonym; tagName always preserves the owner's canonical spelling.
func (s *server) writeTagNameTaken(w http.ResponseWriter, submittedName string, owner domain.TagRef) {
	tagName := owner.Name
	body := gen.Error{Code: codeTagNameTaken, TagName: &tagName}
	var reason gen.ErrorReason
	if submittedName == owner.Name {
		reason = reasonNameIsTag
		body.Message = fmt.Sprintf("A tag named %q already exists.", owner.Name)
	} else {
		reason = reasonNameIsSynonym
		body.Message = fmt.Sprintf("%q is already a synonym of %q.", submittedName, owner.Name)
	}
	body.Reason = &reason
	s.writeErrorBody(w, http.StatusConflict, body)
}

func (s *server) invalidTagName(w http.ResponseWriter, err error, prefix string) {
	var invalid *domain.InvalidTagNameError
	if !errors.As(err, &invalid) {
		s.invalidRequest(w, prefix+"The tag name cannot be used.")
		return
	}
	switch invalid.Problem {
	case domain.TagNameEmpty:
		s.invalidRequestReason(w, reasonTagNameEmpty, prefix+"Enter a tag name.")
	case domain.TagNameControlCharacters:
		s.invalidRequestReason(w, reasonTagNameControlCharacters, prefix+"Tag names cannot contain control characters.")
	case domain.TagNameTooLong:
		s.invalidRequestLimit(w, reasonTagNameTooLong, domain.TagNameMaxLength,
			prefix+"Tag names must be at most "+strconv.Itoa(domain.TagNameMaxLength)+" characters.")
	default:
		s.invalidRequest(w, prefix+"The tag name cannot be used.")
	}
}
