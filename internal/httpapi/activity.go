package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Activity routes remain owner-only by omission from accessRoutes; changes are announced through /api/events.

type Activity interface {
	// ListActivity orders by descending ID; malformed cursors return domain.ErrInvalidCursor.
	ListActivity(
		ctx context.Context, audience domain.Audience, query domain.ActivityQuery, cursor string, limit int,
	) (domain.ActivityPage, error)
	RetryActivity(ctx context.Context, id int64) error
	// UndoActivity returns ErrNotFound for missing entries/unassigned tags and ErrActivityNotUndoable when undo is unavailable.
	UndoActivity(ctx context.Context, id, tagID int64) (domain.ActivityEntry, error)
}

// Keep page limits aligned with api/openapi.yaml.
const (
	defaultActivityLimit = 50
	maxActivityLimit     = 100
)

func (s *server) ListActivity(w http.ResponseWriter, r *http.Request, params gen.ListActivityParams) {
	if s.activity == nil {
		s.internalError(w, "Activity storage is not configured.", nil)
		return
	}
	limit := defaultActivityLimit
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > maxActivityLimit {
			s.invalidRequest(w, "limit must be between 1 and 100.")
			return
		}
		limit = *params.Limit
	}
	query, ok := s.parseActivityQuery(w, params)
	if !ok {
		return
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	page, err := s.activity.ListActivity(r.Context(), audienceFrom(r.Context()), query, cursor, limit)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load the activity.", err)
		return
	}

	out := gen.ActivityPage{
		Items: make([]gen.ActivityEntry, 0, len(page.Items)),
		Counts: gen.ActivityCounts{
			Running: page.Counts.Running, Attention: page.Counts.Attention, Done: page.Counts.Done,
		},
		Features: toAPIActivityFeatureCounts(page.Features),
	}
	for _, entry := range page.Items {
		out.Items = append(out.Items, toAPIActivityEntry(entry))
	}
	if page.NextCursor != "" {
		next := page.NextCursor
		out.NextCursor = &next
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

func (s *server) parseActivityQuery(w http.ResponseWriter, params gen.ListActivityParams) (domain.ActivityQuery, bool) {
	var query domain.ActivityQuery
	var err error
	if params.Status != nil {
		if query.Status, err = domain.ParseActivityStatus(string(*params.Status)); err != nil {
			s.invalidRequest(w, "status must be running, attention or done.")
			return domain.ActivityQuery{}, false
		}
	}
	if params.Feature != nil {
		// The generator does not enforce OpenAPI maxItems, currently nine features.
		if len(*params.Feature) > len(domain.ActivityFeatures) {
			s.invalidRequest(w, "feature can be repeated at most 9 times.")
			return domain.ActivityQuery{}, false
		}
		for _, value := range *params.Feature {
			feature, err := domain.ParseActivityFeature(string(value))
			if err != nil {
				s.invalidRequest(w, "feature must be one of the activity features.")
				return domain.ActivityQuery{}, false
			}
			query.Features = append(query.Features, feature)
		}
	}
	if params.Decider != nil {
		if query.Decider, err = domain.ParseActivityDecider(string(*params.Decider)); err != nil {
			s.invalidRequest(w, "decider must be rule, ai or owner.")
			return domain.ActivityQuery{}, false
		}
	}
	if params.Since != nil {
		query.Since = *params.Since
	}
	if params.Q != nil {
		if length := utf8.RuneCountInString(*params.Q); length < 1 || length > domain.MaxActivitySearchLength {
			s.invalidRequest(w, "q must be 1 to 200 characters.")
			return domain.ActivityQuery{}, false
		}
		query.Search = *params.Q
	}
	return query, true
}

func (s *server) RetryActivity(w http.ResponseWriter, r *http.Request, id gen.ActivityId) {
	if s.activity == nil {
		s.internalError(w, "Activity storage is not configured.", nil)
		return
	}
	if err := s.activity.RetryActivity(r.Context(), id); err != nil {
		s.writeActivityError(w, err, "Could not retry the activity entry.")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// mutationBoundary validates the JSON Content-Type.
func isActivityUndoPost(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	id, found := strings.CutPrefix(r.URL.Path, "/api/activity/")
	id, undo := strings.CutSuffix(id, "/undo")
	return found && undo && id != "" && !strings.Contains(id, "/")
}

// UndoActivity requires tagId for auto_tag entries; other kinds accept an empty object.
func (s *server) UndoActivity(w http.ResponseWriter, r *http.Request, id gen.ActivityId) {
	var body gen.ActivityUndoRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.activity == nil {
		s.internalError(w, "Activity storage is not configured.", nil)
		return
	}
	var tagID int64
	if body.TagId != nil {
		tagID = *body.TagId
	}
	undo, err := s.activity.UndoActivity(r.Context(), id, tagID)
	if err != nil {
		s.writeActivityError(w, err, "Could not undo the activity entry.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIActivityEntry(undo), s.logger)
}

func (s *server) writeActivityError(w http.ResponseWriter, err error, message string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The activity entry was not found.")
	case errors.Is(err, domain.ErrActivityNotRetryable):
		s.writeError(w, http.StatusConflict, codeActivityNotRetryable, "The activity entry cannot be retried.")
	case errors.Is(err, domain.ErrActivityNotUndoable):
		s.writeError(w, http.StatusConflict, codeActivityNotUndoable, "The activity entry cannot be undone.")
	case errors.Is(err, domain.ErrScanRunning):
		s.writeError(w, http.StatusConflict, codeScanInProgress, "A scan is in progress.")
	case errors.Is(err, domain.ErrAutoTaggingOff):
		s.conflictReason(w, gen.ErrorReasonAutoTaggingOff, "Auto-tagging is turned off.")
	default:
		s.internalError(w, message, err)
	}
}

func toAPIActivityFeatureCounts(counts map[domain.ActivityFeature]int) gen.ActivityFeatureCounts {
	return gen.ActivityFeatureCounts{
		Import:         counts[domain.ActivityFeatureImport],
		Scan:           counts[domain.ActivityFeatureScan],
		DuplicateCheck: counts[domain.ActivityFeatureDuplicateCheck],
		AutoTagging:    counts[domain.ActivityFeatureAutoTagging],
		WorkInfo:       counts[domain.ActivityFeatureWorkInfo],
		Tags:           counts[domain.ActivityFeatureTags],
		Versions:       counts[domain.ActivityFeatureVersions],
		MediaFolders:   counts[domain.ActivityFeatureMediaFolders],
		Organize:       counts[domain.ActivityFeatureOrganize],
	}
}

func toAPIActivityEntry(entry domain.ActivityEntry) gen.ActivityEntry {
	out := gen.ActivityEntry{
		Id:          entry.ID,
		Kind:        gen.ActivityKind(entry.Kind),
		Feature:     gen.ActivityFeature(entry.Feature),
		Status:      gen.ActivityStatus(entry.Status),
		Decider:     gen.ActivityDecider(entry.Decider),
		Probability: entry.Probability,
		Subject:     toAPIActivitySubject(entry),
		Candidate:   entry.Candidate,
		RetryRun:    entry.RetryOffered(),
		Undoable:    entry.UndoOffered(),
		Undone:      entry.UndoneBy != 0,
		CreatedAt:   entry.CreatedAt,
		UpdatedAt:   entry.UpdatedAt,
	}
	if entry.Reason != "" {
		reason := gen.ActivityReason(entry.Reason)
		out.Reason = &reason
	}
	if entry.Outcome != "" {
		outcome := gen.ActivityOutcome(entry.Outcome)
		out.Outcome = &outcome
	}
	if entry.Detail != "" {
		detail := entry.Detail
		out.Detail = &detail
	}
	if entry.Stage != "" {
		stage := gen.ActivityStage(entry.Stage)
		out.Stage = &stage
	}
	if !entry.RetryAt.IsZero() {
		retryAt := entry.RetryAt
		out.RetryAt = &retryAt
	}
	if progress := entry.Progress; progress != nil {
		out.Progress = &gen.ActivityProgress{Stage: gen.ActivityStage(progress.Stage), Done: progress.Done, Total: progress.Total}
	}
	if undoes := entry.Undoes; undoes != nil {
		out.Undoes = &gen.ActivityUndone{Kind: gen.ActivityKind(undoes.Kind)}
		if undoes.Outcome != "" {
			outcome := gen.ActivityOutcome(undoes.Outcome)
			out.Undoes.Outcome = &outcome
		}
		if undoes.TagName != "" {
			name := undoes.TagName
			out.Undoes.TagName = &name
		}
	}
	if entry.Kind == domain.ActivityAutoTag {
		tags := make([]gen.ActivityTag, 0, len(entry.Tags))
		for _, tag := range entry.Tags {
			tags = append(tags, gen.ActivityTag{
				Id: tag.TagID, Name: tag.Name, Suggested: tag.Suggested, Probability: tag.Probability,
				Undone: tag.UndoneBy != 0,
			})
		}
		out.Tags = &tags
	}
	return out
}

// Pairs produce two subject items; videoId appears only for currently accessible videos.
func toAPIActivitySubject(entry domain.ActivityEntry) gen.ActivitySubject {
	subject := gen.ActivitySubject{Kind: gen.ActivitySubjectKind(entry.Subject.Kind)}
	item := func(label string, videoID int64) gen.ActivitySubjectItem {
		out := gen.ActivitySubjectItem{Label: label}
		if videoID != 0 {
			id := videoID
			out.VideoId = &id
		}
		return out
	}
	subject.Items = []gen.ActivitySubjectItem{item(entry.Subject.Label, entry.VideoIDs[0])}
	if entry.Subject.Kind == domain.ActivitySubjectPair {
		subject.Items = append(subject.Items, item(entry.Subject.LabelB, entry.VideoIDs[1]))
	}
	return subject
}
