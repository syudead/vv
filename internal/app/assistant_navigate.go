package app

import (
	"context"
	"fmt"
	"slices"

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

func (t *assistantTurn) navigate(ctx context.Context, n *domain.AssistantNavigation) (domain.AssistantResolution, error) {
	dest := domain.AssistantDestination{Kind: n.To}
	switch kind, entity := navigationEntities[n.To]; {
	case entity:
		label, ok, err := t.entity(ctx, kind, n.Name)
		if !ok || err != nil {
			return domain.AssistantResolution{}, err
		}
		dest.ID = label.ID
	case n.To == "video":
		label, ok, err := t.video(ctx, n.Name)
		if !ok || err != nil {
			return domain.AssistantResolution{}, err
		}
		dest.ID = label.ID
	case n.To == "resume":
		id, ok, err := t.resume(ctx)
		if !ok || err != nil {
			return domain.AssistantResolution{}, err
		}
		dest = domain.AssistantDestination{Kind: "video", ID: id}
	case n.To == "settings":
		dest.SettingsPage = n.SettingsPage
	}
	return domain.AssistantResolution{Kind: domain.AssistantResolvedNavigate, Destination: &dest}, nil
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

// video resolves "current" to the open video and any other name by a title search that must find one video, or
// that finds the video pinned in a clarification among the candidates it offers.
func (t *assistantTurn) video(ctx context.Context, name string) (domain.AssistantLabel, bool, error) {
	if name == "" || name == "current" {
		return t.entityVideo(ctx)
	}
	page, err := t.a.videos.ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Query: name, Limit: assistantClarifyLimit})
	if err != nil {
		return domain.AssistantLabel{}, false, fmt.Errorf("cannot find the video by title: %w", err)
	}
	v, ok := t.pinnedVideo(page.Items)
	if id, pinned := t.idPins[domain.FoldForMatch(name)]; !ok && pinned {
		i := slices.IndexFunc(page.Items, func(v domain.Video) bool { return v.ID == id })
		if ok = i >= 0; ok {
			v = page.Items[i]
		}
	}
	if !ok && page.Total == 1 && len(page.Items) == 1 {
		v, ok = page.Items[0], true
	}
	if ok {
		return domain.AssistantLabel{Kind: domain.AssistantLabelVideo, ID: v.ID, Key: v.UserKey, Name: v.Title}, true, nil
	}
	candidates := make([]domain.AssistantCandidate, len(page.Items))
	for i, v := range page.Items {
		candidates[i] = domain.AssistantCandidate{Kind: domain.AssistantLabelVideo, ID: v.ID, Name: v.Title}
	}
	t.noteClarify(domain.AssistantLabelVideo, name, candidates)
	t.clarify("target", candidates, name)
	return domain.AssistantLabel{}, false, nil
}

// pinnedVideo is the found video a clarification pinned.
func (t *assistantTurn) pinnedVideo(found []domain.Video) (domain.Video, bool) {
	for _, pin := range t.input.Context.Pinned {
		if pin.Kind != domain.AssistantLabelVideo {
			continue
		}
		if i := slices.IndexFunc(found, func(v domain.Video) bool { return v.ID == pin.ID }); i >= 0 {
			return found[i], true
		}
	}
	return domain.Video{}, false
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
