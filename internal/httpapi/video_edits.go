package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// VideoEdits reads and applies bulk edits; each call is one store transaction.
type VideoEdits interface {
	// Summary counts the listed videos' values by user-data key.
	Summary(ctx context.Context, videoIDs []int64) (domain.VideoEditSummary, error)
	// Apply writes every part atomically; ContentKeys lists the targets' content keys when it made them private.
	Apply(ctx context.Context, videoIDs []int64, edit domain.VideoEdit) (domain.VideoEditResult, error)
}

func (s *server) SummarizeVideoEdits(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoTagsSummaryRequest
	if !s.readJSONBody(w, r, &body) || !s.videoEditsConfigured(w) {
		return
	}
	videoIDs, ok := s.resolveVideoTargets(w, r, body.VideoIds, body.SmartFolderIds, maxVideoTagsIDs)
	if !ok {
		return
	}
	summary, err := s.videoEdits.Summary(r.Context(), videoIDs)
	if err != nil {
		s.internalError(w, "Could not load the edit summary.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIVideoEditSummary(summary), s.logger)
}

// ApplyVideoEdits validates the whole edit before resolving smart folders, and ends guest streams of videos it makes private.
func (s *server) ApplyVideoEdits(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoEditRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if !validVideoTargetCounts(body.VideoIds, body.SmartFolderIds, maxVideoTagsIDs) {
		s.tooManyVideos(w, maxVideoTagsIDs)
		return
	}
	edit, err := domain.NormalizeVideoEdit(videoEditFromRequest(body))
	if err != nil {
		s.writeVideoEditError(w, err)
		return
	}
	if !s.videoEditsConfigured(w) {
		return
	}
	videoIDs, ok := s.resolveVideoTargets(w, r, body.VideoIds, body.SmartFolderIds, maxVideoTagsIDs)
	if !ok {
		return
	}
	result, err := s.applyVideoEdit(r.Context(), videoIDs, edit)
	if err != nil {
		s.writeVideoEditError(w, err)
		return
	}
	tags := make([]gen.TagRef, 0, len(result.Tags))
	for _, ref := range result.Tags {
		tags = append(tags, toAPITagRef(ref))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.VideoEditResponse{Applied: result.Applied, Tags: tags}, s.logger)
}

func (s *server) applyVideoEdit(ctx context.Context, videoIDs []int64, edit domain.VideoEdit) (domain.VideoEditResult, error) {
	if edit.Public == nil {
		return s.videoEdits.Apply(ctx, videoIDs, edit)
	}
	var result domain.VideoEditResult
	_, err := s.switchVisibility(ctx, *edit.Public, func(ctx context.Context) ([]string, error) {
		var err error
		result, err = s.videoEdits.Apply(ctx, videoIDs, edit)
		return result.ContentKeys, err
	})
	return result, err
}

func videoEditFromRequest(body gen.VideoEditRequest) domain.VideoEdit {
	edit := domain.VideoEdit{Public: body.Public}
	if body.Tags != nil {
		for _, input := range deref(body.Tags.Add) {
			edit.TagsAdd = append(edit.TagsAdd, domain.TagInput{ID: input.Id, Name: input.Name})
		}
		edit.TagsRemove = deref(body.Tags.Remove)
	}
	for _, credit := range deref(body.Credits) {
		edit.Credits = append(edit.Credits, domain.CreditEdit{
			Kind: domain.CreditKind(credit.Kind), Role: credit.Role, Add: deref(credit.Add), Remove: deref(credit.Remove),
		})
	}
	for _, date := range deref(body.Dates) {
		edit.Dates = append(edit.Dates, domain.DateEdit{Kind: domain.DateKind(date.Kind), Date: date.Date})
	}
	return edit
}

func deref[T any](list *[]T) []T {
	if list == nil {
		return nil
	}
	return *list
}

func (s *server) writeVideoEditError(w http.ResponseWriter, err error) {
	var nameAt *domain.TagNameAtError
	switch {
	case errors.Is(err, domain.ErrNoVideoEdit):
		s.invalidRequest(w, "The edit must change tags, credits, dates or visibility.")
	case errors.As(err, &nameAt):
		field := fmt.Sprintf("tags.add[%d].name", nameAt.Index)
		body := tagNameErrorBody(err, field+": ")
		body.Field = &field
		s.writeErrorBody(w, http.StatusBadRequest, body)
	case errors.Is(err, domain.ErrTagNotFound):
		s.writeVideoTagsError(w, err)
	default:
		s.writeVideoWorkInfoError(w, err)
	}
}

func (s *server) videoEditsConfigured(w http.ResponseWriter) bool {
	if s.videoEdits == nil {
		s.internalError(w, "Video edit storage is not configured.", nil)
		return false
	}
	return true
}

func toAPIVideoEditSummary(summary domain.VideoEditSummary) gen.VideoEditSummary {
	out := gen.VideoEditSummary{
		Total: summary.Total, Public: summary.Public,
		Tags:          make([]gen.VideoTagsSummaryItem, 0, len(summary.Tags)),
		People:        make([]gen.VideoEditPersonCount, 0, len(summary.People)),
		Organizations: make([]gen.VideoEditOrganizationCount, 0, len(summary.Organizations)),
		Dates:         make([]gen.VideoEditDateCount, 0, len(summary.Dates)),
	}
	for _, item := range summary.Tags {
		out.Tags = append(out.Tags, gen.VideoTagsSummaryItem{Tag: toAPITagRef(item.Tag), Count: item.Count, ManualCount: item.ManualCount})
	}
	for _, p := range summary.People {
		item := gen.VideoEditPersonCount{Role: gen.WorkPersonRole(p.Role), Person: toAPIEditCreditRef(p.Person), Count: p.Count}
		if p.ImageRevision != 0 {
			item.ImageRevision = &p.ImageRevision
		}
		out.People = append(out.People, item)
	}
	for _, o := range summary.Organizations {
		out.Organizations = append(out.Organizations, gen.VideoEditOrganizationCount{
			Role: gen.WorkOrganizationRole(o.Role), Organization: toAPIEditCreditRef(o.Organization), Count: o.Count,
		})
	}
	for _, d := range summary.Dates {
		item := gen.VideoEditDateCount{Kind: gen.WorkDateKind(d.Kind), Count: d.Count}
		if d.Date != "" {
			item.Date = &d.Date
		}
		out.Dates = append(out.Dates, item)
	}
	return out
}

func toAPIEditCreditRef(ref domain.CreditRef) gen.CreditRef {
	return gen.CreditRef{Id: ref.ID, Name: ref.Name, Provisional: ref.Provisional}
}
