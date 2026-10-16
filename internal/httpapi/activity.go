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

// アクティビティの記録の一覧・やり直し・取り消し（specs/055-activity/contracts/screen-api.md）。3 つの経路は
// /api/* の既定どおり所有者だけで、accessRoutes には入れない（受け入れ条件 8）。記録が書かれたか変わったことは
// /api/events の activity で知らせる（events.go）。

// Activity はアクティビティの記録の先である。internal/app の *Activity がこれを満たす。
type Activity interface {
	// ListActivity は query を満たす記録を id の降順に cursor の次から limit 件返す。読めない cursor は
	// domain.ErrInvalidCursor。
	ListActivity(
		ctx context.Context, audience domain.Audience, query domain.ActivityQuery, cursor string, limit int,
	) (domain.ActivityPage, error)
	// RetryActivity は記録の処理を始め直す。無ければ domain.ErrNotFound、やり直しを出さなければ
	// domain.ErrActivityNotRetryable。
	RetryActivity(ctx context.Context, id int64) error
	// UndoActivity は自動の判断を取り消し、所有者の undo の記録を返す。無い記録と付けていないタグは
	// domain.ErrNotFound、取り消しを出さなければ domain.ErrActivityNotUndoable。
	UndoActivity(ctx context.Context, id, tagID int64) (domain.ActivityEntry, error)
}

// 記録の 1 ページの件数（api/openapi.yaml の limit と同じ値）。
const (
	defaultActivityLimit = 50
	maxActivityLimit     = 100
)

// ListActivity は記録を新しい順に返す（GET /api/activity）。絞り込みはページを切る前に保存層が掛ける。
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

// parseActivityQuery は一覧の絞り込みを読む。知らない値、多すぎる feature と範囲の外の q は 400 invalid_request を書いて偽を返す。
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
		// openapi の maxItems は機能の数（9）で、生成器は数を確かめない。
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

// RetryActivity は記録の処理を始め直す（POST /api/activity/{id}/retry）。
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

// isActivityUndoPost は POST /api/activity/{id}/undo かを返す。本文は JSON で、mutationBoundary が
// Content-Type を確かめる。
func isActivityUndoPost(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	id, found := strings.CutPrefix(r.URL.Path, "/api/activity/")
	id, undo := strings.CutSuffix(id, "/undo")
	return found && undo && id != "" && !strings.Contains(id, "/")
}

// UndoActivity は自動の判断を取り消し、所有者の undo の記録を返す（POST /api/activity/{id}/undo）。本文は
// JSON のオブジェクトで、auto_tag の記録では取り消すタグを tagId で渡す。ほかの種類では {} でよい。
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

// writeActivityError はやり直しと取り消しの誤りを契約の応答にする。
func (s *server) writeActivityError(w http.ResponseWriter, err error, message string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The activity entry was not found.")
	case errors.Is(err, domain.ErrActivityNotRetryable):
		s.writeError(w, http.StatusConflict, codeActivityNotRetryable, "The activity entry cannot be retried.")
	case errors.Is(err, domain.ErrActivityNotUndoable):
		s.writeError(w, http.StatusConflict, codeActivityNotUndoable, "The activity entry cannot be undone.")
	default:
		s.internalError(w, message, err)
	}
}

// toAPIActivityFeatureCounts は機能ごとの数を契約の形にする。
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

// toAPIActivityEntry は記録を契約の形にする。やり直しと取り消しを出すかは domain の規則で決める。
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

// toAPIActivitySubject は対象を鍵ごとの項目にする。組は 2 項目、それ以外は 1 項目で、videoId は見る人が
// 開ける今の動画があるときだけ入る（research.md R-10）。
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
