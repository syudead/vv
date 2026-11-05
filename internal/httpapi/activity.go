package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	// ActivityDetail returns domain.ErrNotFound for a missing entry.
	ActivityDetail(ctx context.Context, id int64) (domain.ActivityDetail, error)
	RetryActivity(ctx context.Context, id int64) error
	// UndoActivity returns ErrNotFound for missing entries, unassigned tags and unmade links, and ErrActivityNotUndoable
	// when undo is unavailable.
	UndoActivity(ctx context.Context, id int64, target domain.ActivityUndoTarget) (domain.ActivityEntry, error)
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
		Items: make([]gen.ActivityListItem, 0, len(page.Items)),
		Counts: gen.ActivityCounts{
			Running: page.Counts.Running, Attention: page.Counts.Attention, Done: page.Counts.Done,
		},
		Features: toAPIActivityFeatureCounts(page.Features),
	}
	for _, entry := range page.Items {
		item, err := toAPIActivityListItem(entry)
		if err != nil {
			s.internalError(w, "Could not load the activity.", err)
			return
		}
		out.Items = append(out.Items, item)
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
		// The generator does not enforce OpenAPI maxItems, currently ten features.
		if len(*params.Feature) > len(domain.ActivityFeatures) {
			s.invalidRequest(w, "feature can be repeated at most 10 times.")
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

func (s *server) GetActivity(w http.ResponseWriter, r *http.Request, id gen.ActivityId) {
	if s.activity == nil {
		s.internalError(w, "Activity storage is not configured.", nil)
		return
	}
	detail, err := s.activity.ActivityDetail(r.Context(), id)
	if err != nil {
		s.writeActivityError(w, err, "Could not load the activity entry.")
		return
	}
	out, err := toAPIActivityDetail(detail)
	if err != nil {
		s.internalError(w, "Could not load the activity entry.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
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

// UndoActivity requires tagId for auto_tag entries and takes an optional appearanceId for face_analysis entries;
// other kinds accept an empty object.
func (s *server) UndoActivity(w http.ResponseWriter, r *http.Request, id gen.ActivityId) {
	var body gen.ActivityUndoRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.activity == nil {
		s.internalError(w, "Activity storage is not configured.", nil)
		return
	}
	var target domain.ActivityUndoTarget
	if body.TagId != nil {
		target.TagID = *body.TagId
	}
	if body.AppearanceId != nil {
		if *body.AppearanceId < 1 {
			s.invalidRequest(w, "appearanceId must be a positive integer.")
			return
		}
		target.AppearanceID = *body.AppearanceId
	}
	undo, err := s.activity.UndoActivity(r.Context(), id, target)
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
	case errors.Is(err, domain.ErrFaceRecognitionOff):
		s.conflictReason(w, gen.ErrorReasonFaceRecognitionOff, "Face recognition is turned off.")
	default:
		s.internalError(w, message, err)
	}
}

func toAPIActivityFeatureCounts(counts map[domain.ActivityFeature]int) gen.ActivityFeatureCounts {
	return gen.ActivityFeatureCounts{
		Import:          counts[domain.ActivityFeatureImport],
		Scan:            counts[domain.ActivityFeatureScan],
		DuplicateCheck:  counts[domain.ActivityFeatureDuplicateCheck],
		AutoTagging:     counts[domain.ActivityFeatureAutoTagging],
		WorkInfo:        counts[domain.ActivityFeatureWorkInfo],
		Tags:            counts[domain.ActivityFeatureTags],
		Versions:        counts[domain.ActivityFeatureVersions],
		MediaFolders:    counts[domain.ActivityFeatureMediaFolders],
		Organize:        counts[domain.ActivityFeatureOrganize],
		FaceRecognition: counts[domain.ActivityFeatureFaceRecognition],
		Assistant:       counts[domain.ActivityFeatureAssistant],
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
		Evidence:    toAPIJudgeEvidence(entry.Evidence),
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
	if entry.Kind == domain.ActivityFaceAnalysis {
		out.FaceLinks = toAPIActivityFaceLinks(entry.FaceLinks)
	}
	return out
}

func toAPIActivityFaceLinks(links []domain.ActivityFaceLink) *[]gen.ActivityFaceLink {
	out := make([]gen.ActivityFaceLink, 0, len(links))
	for _, link := range links {
		item := gen.ActivityFaceLink{
			AppearanceId: link.AppearanceID, CreatedPerson: link.CreatedPerson, Undone: link.UndoneBy != 0,
		}
		if link.Person != nil {
			item.Person = &gen.CreditRef{Id: link.Person.ID, Name: link.Person.Name, Provisional: link.Person.Provisional}
		}
		out = append(out, item)
	}
	return &out
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
	if entry.Subject.Kind == domain.ActivitySubjectPerson {
		if id, err := strconv.ParseInt(entry.Subject.Key, 10, 64); err == nil {
			subject.Items[0].PersonId = &id
		}
	}
	if entry.Subject.Kind == domain.ActivitySubjectPair {
		subject.Items = append(subject.Items, item(entry.Subject.LabelB, entry.VideoIDs[1]))
	}
	return subject
}

// fromActivityEntry fills a generated type that extends ActivityEntry or takes a subset of its properties, so the
// field mapping stays in toAPIActivityEntry.
func fromActivityEntry[T any](entry gen.ActivityEntry) (T, error) {
	var out T
	body, err := json.Marshal(entry)
	if err == nil {
		err = json.Unmarshal(body, &out)
	}
	if err != nil {
		return out, fmt.Errorf("cannot convert the activity entry %d: %w", entry.Id, err)
	}
	return out, nil
}

func toAPIActivityListItem(entry domain.ActivityEntry) (gen.ActivityListItem, error) {
	item, err := fromActivityEntry[gen.ActivityListItem](toAPIActivityEntry(entry))
	if err != nil || entry.Kind != domain.ActivityPipeline {
		return item, err
	}
	members := make([]gen.ActivityMemberSummary, 0, len(entry.Members))
	for _, member := range entry.Members {
		summary, err := toAPIActivityMemberSummary(member)
		if err != nil {
			return item, err
		}
		members = append(members, summary)
	}
	item.Members = &members
	return item, nil
}

// A member summary leaves out detail and tags; an auto_tag member counts its tags instead, and a face_analysis member
// keeps its links through the JSON copy.
func toAPIActivityMemberSummary(member domain.ActivityMember) (gen.ActivityMemberSummary, error) {
	summary, err := fromActivityEntry[gen.ActivityMemberSummary](toAPIActivityEntry(member.ActivityEntry))
	if err != nil {
		return summary, err
	}
	summary.Replaced, summary.Matched = member.Replaced, member.Matched
	if member.Kind == domain.ActivityAutoTag {
		count := len(member.Tags)
		summary.TagCount = &count
	}
	if member.StagesRecorded() {
		stages := make([]gen.ActivityStageSummary, 0, len(member.Stages))
		for _, stage := range member.Stages {
			stages = append(stages, gen.ActivityStageSummary{
				Stage: gen.ActivityStage(stage.Stage), State: gen.ActivityStageState(stage.State),
			})
		}
		summary.Stages = &stages
	}
	return summary, nil
}

func toAPIActivityDetail(detail domain.ActivityDetail) (gen.ActivityDetail, error) {
	out, err := fromActivityEntry[gen.ActivityDetail](toAPIActivityEntry(detail.ActivityEntry))
	if err != nil {
		return out, err
	}
	if detail.Kind == domain.ActivityPipeline {
		members := make([]gen.ActivityEntry, 0, len(detail.Members))
		for _, member := range detail.Members {
			entry := toAPIActivityEntry(member.ActivityEntry)
			replaced := member.Replaced
			entry.Replaced = &replaced
			members = append(members, entry)
		}
		out.Members = &members
	}
	if len(detail.Pipelines) > 0 {
		pipelines := make([]gen.ActivityPipelineRef, 0, len(detail.Pipelines))
		for _, pipeline := range detail.Pipelines {
			pipelines = append(pipelines, gen.ActivityPipelineRef{
				Id: pipeline.ID, Subject: toAPIActivitySubject(pipeline), Status: gen.ActivityStatus(pipeline.Status),
				CreatedAt: pipeline.CreatedAt,
			})
		}
		out.Pipelines = &pipelines
	}
	if detail.Kind == domain.ActivityImport {
		recorded := detail.StagesRecorded()
		out.StagesRecorded = &recorded
	}
	if detail.StagesRecorded() {
		stages := make([]gen.ActivityStageRow, 0, len(detail.Stages))
		for _, stage := range detail.Stages {
			row := gen.ActivityStageRow{
				Stage: gen.ActivityStage(stage.Stage), State: gen.ActivityStageState(stage.State), UpdatedAt: stage.UpdatedAt,
			}
			if stage.Detail != "" {
				message := stage.Detail
				row.Detail = &message
			}
			stages = append(stages, row)
		}
		out.Stages = &stages
	}
	return out, nil
}
