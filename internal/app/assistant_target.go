package app

import (
	"context"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// planDraft is a plan while its target and arguments are resolved; the builders fill its call.
type planDraft struct {
	t    *assistantTurn
	row  domain.AssistantAction
	req  *domain.AssistantActionRequest
	args domain.AssistantArguments
	plan domain.AssistantPlan
	// groupIDs and folderIDs (smart folders) are collections an operation that takes them receives instead of
	// their videos.
	groupIDs  []int64
	folderIDs []int64
}

func (d *planDraft) body(field string, value any) {
	if d.plan.Call.Body == nil {
		d.plan.Call.Body = map[string]any{}
	}
	d.plan.Call.Body[field] = value
}

func (d *planDraft) path(name string, id int64) {
	if d.plan.Call.Path == nil {
		d.plan.Call.Path = map[string]string{}
	}
	d.plan.Call.Path[name] = fmt.Sprint(id)
}

func (d *planDraft) query(name, value string) {
	if d.plan.Call.Query == nil {
		d.plan.Call.Query = map[string]string{}
	}
	d.plan.Call.Query[name] = value
}

// literal records an argument the card shows as a value.
func (d *planDraft) literal(arg domain.AssistantArgument, value any) {
	encoded, err := jsonValue(value)
	if err != nil {
		return
	}
	d.plan.Change = append(d.plan.Change, domain.AssistantChange{Argument: arg, Value: encoded})
}

// named records an argument the card shows as the entities it names.
func (d *planDraft) named(arg domain.AssistantArgument, labels []domain.AssistantLabel) {
	d.plan.Change = append(d.plan.Change, domain.AssistantChange{Argument: arg, Labels: labels})
}

// need asks for the value when a required argument is missing.
func (d *planDraft) need(present bool) bool {
	if !present {
		d.t.clarify("value", nil)
	}
	return present
}

// assistantCardVideos is how many video labels a plan keeps for the card and the stale check; the count comes from
// the keys.
const assistantCardVideos = 10

// entityTargets maps a row's single-entity target to the label kind it resolves.
var entityTargets = map[domain.AssistantTarget]domain.AssistantLabelKind{
	domain.AssistantTargetTag: domain.AssistantLabelTag, domain.AssistantTargetTagKind: domain.AssistantLabelTagKind,
	domain.AssistantTargetPerson: domain.AssistantLabelPerson, domain.AssistantTargetOrganization: domain.AssistantLabelOrganization,
	domain.AssistantTargetPlaylist: domain.AssistantLabelPlaylist, domain.AssistantTargetGroup: domain.AssistantLabelGroup,
	domain.AssistantTargetSmartFolder: domain.AssistantLabelSmartFolder,
}

// resolveTarget fills the plan's targets for the row's target kind.
func (d *planDraft) resolveTarget(ctx context.Context) (bool, error) {
	target := d.req.Target
	if target == nil {
		target = &domain.AssistantTargetRequest{Kind: "selection"}
	}
	switch row := d.row.Target; {
	case row == domain.AssistantTargetNone:
		return true, nil
	case entityTargets[row] != "":
		return d.entityTarget(ctx, entityTargets[row], target)
	case row == domain.AssistantTargetTags:
		return d.namedTargets(ctx, domain.AssistantLabelTag, target)
	case row == domain.AssistantTargetCollections:
		return d.namedTargets(ctx, "", target)
	default:
		return d.videoTarget(ctx, target)
	}
}

// entityTarget resolves the one entity a row acts on, by name or from the current screen.
func (d *planDraft) entityTarget(
	ctx context.Context, kind domain.AssistantLabelKind, target *domain.AssistantTargetRequest,
) (bool, error) {
	name := ""
	switch {
	case target.Kind == "named" && len(target.Names) == 1:
		name = target.Names[0]
	case target.Kind != "current" && target.Kind != "selection":
		d.t.clarify("target", nil)
		return false, nil
	}
	label, ok, err := d.t.entity(ctx, kind, name)
	if !ok || err != nil {
		return false, err
	}
	d.plan.Targets.Labels = []domain.AssistantLabel{label}
	d.path("id", label.ID)
	return true, nil
}

// namedTargets resolves several entities by name; an empty kind takes groups and smart folders.
func (d *planDraft) namedTargets(
	ctx context.Context, kind domain.AssistantLabelKind, target *domain.AssistantTargetRequest,
) (bool, error) {
	if target.Kind != "named" || len(target.Names) == 0 {
		d.t.clarify("target", nil)
		return false, nil
	}
	for _, name := range target.Names {
		label, ok, err := d.collectionOrKind(ctx, kind, name)
		if !ok || err != nil {
			return false, err
		}
		d.plan.Targets.Labels = append(d.plan.Targets.Labels, label)
	}
	return true, nil
}

func (d *planDraft) collectionOrKind(
	ctx context.Context, kind domain.AssistantLabelKind, name string,
) (domain.AssistantLabel, bool, error) {
	if kind != "" {
		return d.t.one(ctx, kind, name, targetName)
	}
	found, err := d.t.kindsCandidates(
		ctx, []domain.AssistantLabelKind{domain.AssistantLabelGroup, domain.AssistantLabelSmartFolder}, name,
	)
	if err != nil {
		return domain.AssistantLabel{}, false, err
	}
	if len(found) != 1 {
		d.t.clarify("target", found, name)
		return domain.AssistantLabel{}, false, nil
	}
	return domain.AssistantLabel{Kind: found[0].Kind, ID: found[0].ID, Name: found[0].Name}, true, nil
}

// videoTarget expands the target to videos, checks the count the row takes and stores their keys in plan order.
func (d *planDraft) videoTarget(ctx context.Context, target *domain.AssistantTargetRequest) (bool, error) {
	ids, ok, err := d.videoIDs(ctx, target)
	if !ok || err != nil {
		return false, err
	}
	if len(ids) > domain.AssistantSelectionMax {
		return false, domain.ErrAssistantTooManyVideos
	}
	videos, err := d.t.a.videos.VideosByIDs(ctx, domain.AudienceOwner, ids)
	if err != nil {
		return false, fmt.Errorf("cannot read the target videos: %w", err)
	}
	order := make(map[int64]int, len(ids))
	for i, id := range ids {
		order[id] = i
	}
	slices.SortFunc(videos, func(a, b domain.Video) int { return order[a.ID] - order[b.ID] })
	d.setVideos(videos)
	if !d.videoCountFits() {
		d.t.clarify("target", nil)
		return false, nil
	}
	if d.row.Target == domain.AssistantTargetVideo || d.row.Target == domain.AssistantTargetVirtualVideo {
		d.plan.Call.VideoPathParam = "id"
	} else if len(d.plan.Targets.VideoKeys) > 0 {
		d.plan.Call.VideoField = "videoIds"
	}
	return true, nil
}

func (d *planDraft) videoCountFits() bool {
	videos := len(d.plan.Targets.VideoKeys)
	switch d.row.Target {
	case domain.AssistantTargetVideo, domain.AssistantTargetVirtualVideo:
		return videos == 1
	case domain.AssistantTargetVideoPair:
		return videos == 2
	case domain.AssistantTargetVideosOrGroups:
		return videos > 0 || len(d.groupIDs) > 0 || len(d.folderIDs) > 0
	default:
		return videos > 0
	}
}

// setVideos keeps one key per user key (or per version for a version operation) and the first labels.
func (d *planDraft) setVideos(videos []domain.Video) {
	byVersion := d.plan.ByVersion()
	seen := map[string]bool{}
	for _, v := range videos {
		key := v.UserKey
		if byVersion {
			key = v.ContentKey
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		d.plan.Targets.VideoKeys = append(d.plan.Targets.VideoKeys, key)
		if len(d.plan.Targets.Labels) < assistantCardVideos {
			d.plan.Targets.Labels = append(d.plan.Targets.Labels,
				domain.AssistantLabel{Kind: domain.AssistantLabelVideo, Key: key, Name: v.Title})
		}
	}
}

// videoIDs reads the video ids a target names: the selection or open video, the current screen, a filter, or titles.
func (d *planDraft) videoIDs(ctx context.Context, target *domain.AssistantTargetRequest) ([]int64, bool, error) {
	switch target.Kind {
	case "selection":
		return d.selectionIDs(ctx)
	case "current":
		return d.currentIDs(ctx)
	case "filter":
		criteria, ok, err := d.t.criteria(ctx, target.Filter, true)
		if !ok || err != nil {
			return nil, false, err
		}
		ids, err := d.t.queryIDs(ctx, criteria.Rule.Query())
		return ids, err == nil, err
	default:
		var ids []int64
		for _, name := range target.Names {
			label, ok, err := d.t.video(ctx, name)
			if !ok || err != nil {
				return nil, false, err
			}
			ids = append(ids, label.ID)
		}
		return ids, true, nil
	}
}

// selectionIDs are the list's selected videos and smart folders, or the open video; an operation that takes
// collections receives the selected groups and smart folders themselves instead.
func (d *planDraft) selectionIDs(ctx context.Context) ([]int64, bool, error) {
	c := d.t.input.Context
	ids := slices.Clone(c.SelectedVideoIDs)
	if len(ids) == 0 && len(c.SelectedSmartFolderIDs) == 0 && len(c.SelectedGroupIDs) == 0 &&
		c.Route == domain.AssistantRouteVideo && c.RouteID > 0 {
		ids = []int64{c.RouteID}
	}
	var err error
	if d.row.Target == domain.AssistantTargetVideosOrGroups {
		ids, err = d.collectionSelection(ctx, ids)
	} else if len(c.SelectedSmartFolderIDs) > 0 {
		var members []int64
		members, err = d.smartFolderIDs(ctx, c.SelectedSmartFolderIDs)
		ids = append(ids, members...)
	}
	if err != nil {
		return nil, false, err
	}
	if len(ids) == 0 && len(d.groupIDs) == 0 && len(d.folderIDs) == 0 {
		d.t.clarify("target", nil)
		return nil, false, nil
	}
	return ids, true, nil
}

// collectionSelection keeps the selected groups and smart folders as collections and drops the selected videos that
// only stand for a selected group's members, as the library's favorite action does.
func (d *planDraft) collectionSelection(ctx context.Context, ids []int64) ([]int64, error) {
	c := d.t.input.Context
	d.groupIDs = slices.Clone(c.SelectedGroupIDs)
	d.folderIDs = slices.Clone(c.SelectedSmartFolderIDs)
	if len(d.groupIDs) == 0 || len(ids) == 0 {
		return ids, nil
	}
	if d.t.a.groups == nil {
		return nil, fmt.Errorf("the assistant cannot read group members: it is not configured")
	}
	members, err := d.t.a.groups.GroupVideoIDs(ctx, d.groupIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot read the group videos: %w", err)
	}
	return slices.DeleteFunc(ids, func(id int64) bool { return slices.Contains(members, id) }), nil
}

func (d *planDraft) smartFolderIDs(ctx context.Context, folders []int64) ([]int64, error) {
	if d.t.a.smartFolders == nil {
		return nil, fmt.Errorf("the assistant cannot expand smart folders: it is not configured")
	}
	ids, err := d.t.a.smartFolders.SmartFolderVideoIDs(ctx, folders)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folder videos: %w", err)
	}
	return ids, nil
}

// currentIDs are the videos of the screen: its filter, its smart folder, the entity it shows, or the open video.
func (d *planDraft) currentIDs(ctx context.Context) ([]int64, bool, error) {
	c := d.t.input.Context
	switch c.Route {
	case domain.AssistantRouteVideo:
		return []int64{c.RouteID}, c.RouteID > 0, nil
	case domain.AssistantRouteSmartFolder:
		if d.row.Target == domain.AssistantTargetVideosOrGroups {
			d.folderIDs = []int64{c.RouteID}
			return nil, true, nil
		}
		ids, err := d.smartFolderIDs(ctx, []int64{c.RouteID})
		return ids, err == nil, err
	case domain.AssistantRouteGroup:
		if d.row.Target == domain.AssistantTargetVideosOrGroups {
			d.groupIDs = []int64{c.RouteID}
			return nil, true, nil
		}
	case domain.AssistantRoutePerson, domain.AssistantRouteOrganization, domain.AssistantRouteTag:
		ids, err := d.t.queryIDs(ctx, routeQuery(c))
		return ids, err == nil, err
	case domain.AssistantRouteLibrary:
		q := domain.VideoQuery{}
		if c.Filter != nil {
			q = c.Filter.Query()
		}
		ids, err := d.t.queryIDs(ctx, q)
		return ids, err == nil, err
	}
	d.t.clarify("target", nil)
	return nil, false, nil
}

func routeQuery(c domain.AssistantTurnContext) domain.VideoQuery {
	switch c.Route {
	case domain.AssistantRoutePerson:
		return domain.VideoQuery{Person: c.RouteID}
	case domain.AssistantRouteOrganization:
		return domain.VideoQuery{Organization: c.RouteID}
	default:
		return domain.VideoQuery{TagIDs: []int64{c.RouteID}}
	}
}

// queryIDs pages through the library listing; it stops one past the selection limit, which the caller refuses.
func (t *assistantTurn) queryIDs(ctx context.Context, q domain.VideoQuery) ([]int64, error) {
	q.Limit, q.Cursor = domain.MaxLimit, ""
	var ids []int64
	for {
		page, err := t.a.videos.ListVideos(ctx, domain.AudienceOwner, q)
		if err != nil {
			return nil, fmt.Errorf("cannot read the target videos: %w", err)
		}
		if page.Total > domain.AssistantSelectionMax {
			return nil, domain.ErrAssistantTooManyVideos
		}
		for _, v := range page.Items {
			ids = append(ids, v.ID)
		}
		if page.NextCursor == "" || len(page.Items) == 0 {
			return ids, nil
		}
		q.Cursor = page.NextCursor
	}
}
