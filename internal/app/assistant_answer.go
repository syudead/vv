package app

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// navigationEntities maps the destinations that name one entity to its kind; video is resolved by title.
var navigationEntities = map[string]domain.AssistantLabelKind{
	"person": domain.AssistantLabelPerson, "organization": domain.AssistantLabelOrganization,
	"playlist": domain.AssistantLabelPlaylist, "group": domain.AssistantLabelGroup,
	"smartFolder": domain.AssistantLabelSmartFolder, "tag": domain.AssistantLabelTag,
}

// routeKinds maps a route to the entity kind it shows.
var routeKinds = map[domain.AssistantRouteKind]domain.AssistantLabelKind{
	domain.AssistantRoutePerson: domain.AssistantLabelPerson, domain.AssistantRouteOrganization: domain.AssistantLabelOrganization,
	domain.AssistantRoutePlaylist: domain.AssistantLabelPlaylist, domain.AssistantRouteGroup: domain.AssistantLabelGroup,
	domain.AssistantRouteSmartFolder: domain.AssistantLabelSmartFolder, domain.AssistantRouteTag: domain.AssistantLabelTag,
	domain.AssistantRouteVideo: domain.AssistantLabelVideo,
}

func (t *assistantTurn) navigate(ctx context.Context, n *domain.AssistantNavigation) (domain.AssistantTurnResult, error) {
	dest := domain.AssistantDestination{Kind: n.To}
	switch kind, entity := navigationEntities[n.To]; {
	case entity:
		label, ok, err := t.entity(ctx, kind, n.Name)
		if !ok || err != nil {
			return domain.AssistantTurnResult{}, err
		}
		dest.ID = label.ID
	case n.To == "video":
		label, ok, err := t.video(ctx, n.Name)
		if !ok || err != nil {
			return domain.AssistantTurnResult{}, err
		}
		dest.ID = label.ID
	case n.To == "resume":
		id, ok, err := t.resume(ctx)
		if !ok || err != nil {
			return domain.AssistantTurnResult{}, err
		}
		dest = domain.AssistantDestination{Kind: "video", ID: id}
	case n.To == "settings":
		dest.SettingsPage = n.SettingsPage
	}
	return domain.AssistantTurnResult{Kind: domain.AssistantTurnNavigate, Destination: &dest}, nil
}

// entity resolves a named entity, or the one the current screen shows when no name is given.
func (t *assistantTurn) entity(
	ctx context.Context, kind domain.AssistantLabelKind, name string,
) (domain.AssistantLabel, bool, error) {
	if name != "" {
		return t.one(ctx, kind, name, targetName)
	}
	route := t.input.Context
	if routeKinds[route.Route] == kind && route.RouteID > 0 {
		names, err := t.a.vocabulary.EntityNames(ctx, kind, []int64{route.RouteID})
		if err != nil {
			return domain.AssistantLabel{}, false, err
		}
		if current, ok := names[route.RouteID]; ok {
			return domain.AssistantLabel{Kind: kind, ID: route.RouteID, Name: current}, true, nil
		}
	}
	t.clarify("target", nil)
	return domain.AssistantLabel{}, false, nil
}

// video resolves "current" to the open video and any other name by a title search that must find one video.
func (t *assistantTurn) video(ctx context.Context, name string) (domain.AssistantLabel, bool, error) {
	if name == "" || name == "current" {
		return t.entityVideo(ctx)
	}
	page, err := t.a.videos.ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Query: name, Limit: assistantClarifyLimit})
	if err != nil {
		return domain.AssistantLabel{}, false, fmt.Errorf("cannot find the video by title: %w", err)
	}
	if page.Total == 1 {
		v := page.Items[0]
		return domain.AssistantLabel{Kind: domain.AssistantLabelVideo, ID: v.ID, Key: v.UserKey, Name: v.Title}, true, nil
	}
	candidates := make([]domain.AssistantCandidate, len(page.Items))
	for i, v := range page.Items {
		candidates[i] = domain.AssistantCandidate{Kind: domain.AssistantLabelVideo, ID: v.ID, Name: v.Title}
	}
	t.clarify("target", candidates, name)
	return domain.AssistantLabel{}, false, nil
}

func (t *assistantTurn) entityVideo(ctx context.Context) (domain.AssistantLabel, bool, error) {
	route := t.input.Context
	if route.Route == domain.AssistantRouteVideo && route.RouteID > 0 {
		videos, err := t.a.videos.VideosByIDs(ctx, domain.AudienceOwner, []int64{route.RouteID})
		if err != nil {
			return domain.AssistantLabel{}, false, fmt.Errorf("cannot read the open video: %w", err)
		}
		if len(videos) == 1 {
			v := videos[0]
			return domain.AssistantLabel{Kind: domain.AssistantLabelVideo, ID: v.ID, Key: v.UserKey, Name: v.Title}, true, nil
		}
	}
	t.clarify("target", nil)
	return domain.AssistantLabel{}, false, nil
}

// resume finds the video played most recently that is still in progress.
func (t *assistantTurn) resume(ctx context.Context) (int64, bool, error) {
	page, err := t.a.videos.ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{
		Sort: domain.SortPlayedDesc, Limit: 1,
		VideoConditions: domain.VideoConditions{WatchState: domain.WatchStateInProgress},
	})
	if err != nil {
		return 0, false, fmt.Errorf("cannot find the video to resume: %w", err)
	}
	if len(page.Items) == 0 {
		t.clarify("target", nil)
		return 0, false, nil
	}
	return page.Items[0].ID, true, nil
}

func answerLimit(limit int) int {
	if limit <= 0 {
		return domain.AssistantAnswerDefaultLimit
	}
	return min(limit, domain.AssistantAnswerLimit)
}

func (t *assistantTurn) answer(ctx context.Context, q *domain.AssistantQuestion) (domain.AssistantTurnResult, error) {
	result := domain.AssistantTurnResult{Kind: domain.AssistantTurnAnswer, Answer: &domain.AssistantAnswer{Question: q.Question}}
	var err error
	switch q.Question {
	case "count", "list":
		err = t.listAnswer(ctx, q, result.Answer)
	case "appearances":
		err = t.appearances(ctx, q.Entity, result.Answer)
	case "recent":
		err = t.recent(ctx, answerLimit(q.Limit), result.Answer)
	}
	return result, err
}

// listAnswer counts, and for list also names, the videos of the filter with the library's own read.
func (t *assistantTurn) listAnswer(ctx context.Context, q *domain.AssistantQuestion, answer *domain.AssistantAnswer) error {
	criteria, ok, err := t.criteria(ctx, q.Filter)
	if !ok || err != nil {
		return err
	}
	query := criteria.Rule.Query()
	query.Sort, query.Limit = criteria.Sort, 1
	if q.Question == "list" {
		query.Limit = answerLimit(q.Limit)
	}
	page, err := t.a.videos.ListVideos(ctx, domain.AudienceOwner, query)
	if err != nil {
		return fmt.Errorf("cannot read the videos of the question: %w", err)
	}
	answer.Total, answer.Criteria = &page.Total, &criteria
	if q.Question == "list" {
		for _, v := range page.Items {
			answer.Items = append(answer.Items, domain.AssistantAnswerItem{ID: v.ID, Title: v.Title})
		}
	}
	return nil
}

// appearances counts as the person, organization or tag page does: the library listing filtered by the entity.
func (t *assistantTurn) appearances(ctx context.Context, e *domain.AssistantEntityName, answer *domain.AssistantAnswer) error {
	if e == nil {
		t.clarify("target", nil)
		return nil
	}
	kind := domain.AssistantLabelKind(e.Kind)
	if e.Kind == "any" {
		kind = ""
	}
	label, ok, err := t.one(ctx, kind, e.Name, targetName)
	if !ok || err != nil {
		return err
	}
	var q domain.VideoQuery
	switch label.Kind {
	case domain.AssistantLabelPerson:
		q.Person = label.ID
	case domain.AssistantLabelOrganization:
		q.Organization = label.ID
	default:
		q.TagIDs = []int64{label.ID}
	}
	total, err := t.count(ctx, q)
	if err != nil {
		return err
	}
	answer.Total = &total
	answer.Page = &domain.AssistantDestination{Kind: string(label.Kind), ID: label.ID}
	return nil
}

func (t *assistantTurn) recent(ctx context.Context, limit int, answer *domain.AssistantAnswer) error {
	if t.a.history == nil {
		return fmt.Errorf("the assistant cannot read the watch history: it is not configured")
	}
	page, err := t.a.history.ListWatchHistory(ctx, domain.AudienceOwner, domain.WatchHistoryQuery{}, "", limit)
	if err != nil {
		return fmt.Errorf("cannot read the watch history: %w", err)
	}
	for _, entry := range page.Items {
		if entry.Video != nil {
			answer.Items = append(answer.Items, domain.AssistantAnswerItem{ID: entry.Video.ID, Title: entry.Title})
		}
	}
	return nil
}
