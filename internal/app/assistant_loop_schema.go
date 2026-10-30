package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// cardLabelKind maps a registry id kind to the label the card shows; kinds outside it, such as a fetch or edit
// session id, are not labeled.
func cardLabelKind(kind string) (domain.AssistantLabelKind, bool) {
	if label, ok := progressLabelKinds[kind]; ok {
		return label, true
	}
	switch kind {
	case "activity":
		return domain.AssistantLabelActivity, true
	case "faceAppearance":
		return domain.AssistantLabelFaceAppearance, true
	}
	return "", false
}

// schemaStep builds a schema row's step: the operation's own path and body after validation, the entities its
// path and body ids name, and the other body fields as values.
func (l *turnLoop) schemaStep(ctx context.Context, tool domain.AssistantTool, raw json.RawMessage) (domain.AssistantPlan, string, error) {
	call, err := l.runner.WriteCall(tool.Row.ID, raw)
	if errors.Is(err, domain.ErrAssistantInvalidArguments) || errors.Is(err, domain.ErrAssistantActionUnknown) {
		return domain.AssistantPlan{}, err.Error(), nil
	}
	if err != nil {
		return domain.AssistantPlan{}, "", err
	}
	plan := domain.AssistantPlan{
		ActionID: tool.Row.ID, Targets: domain.AssistantPlanTargets{Kind: domain.AssistantTargetNone},
		Call: domain.AssistantPlanCall{Path: call.Path, Query: call.Query, Body: call.Body},
	}
	args := map[string]any{assistantBodyArgument: call.Body}
	for name, value := range call.Path {
		if id, err := strconv.ParseFloat(value, 64); err == nil {
			args[name] = id
		}
	}
	labeled := map[string]bool{}
	for _, field := range tool.IDs {
		kind, ok := cardLabelKind(field.Kind)
		ids := argumentIDs(args, field)
		if !ok || len(ids) == 0 {
			continue
		}
		labels, failure, err := l.cardLabels(ctx, kind, ids)
		if failure != "" || err != nil {
			return plan, failure, err
		}
		addIDLabels(&plan, field, labels, len(ids))
		labeled[strings.Split(strings.TrimPrefix(field.Field, "/"), "/")[0]] = field.In == "body"
	}
	for _, name := range slices.Sorted(maps.Keys(call.Body)) {
		if labeled[name] {
			continue
		}
		if value, err := json.Marshal(call.Body[name]); err == nil {
			plan.Change = append(plan.Change, domain.AssistantChange{Argument: domain.AssistantArgument(name), Value: value})
		}
	}
	return plan, "", nil
}

// assistantBodyArgument is the tool argument that holds a schema row's JSON body.
const assistantBodyArgument = "body"

// addIDLabels puts a path id's entity in the targets, and a body id's in the change under its field; a list of
// ids shows its first entity and its count.
func addIDLabels(plan *domain.AssistantPlan, field domain.AssistantIDField, labels []domain.AssistantLabel, count int) {
	if field.In == "path" {
		plan.Targets.Labels = append(plan.Targets.Labels, labels...)
		return
	}
	change := domain.AssistantChange{Argument: domain.AssistantArgument(strings.TrimPrefix(field.Field, "/")), Labels: labels}
	if count > 1 || strings.HasSuffix(field.Field, "Ids") || strings.Contains(field.Field, "*") {
		change.Labels = labels[:1]
		change.Value, _ = json.Marshal(count) // An int always encodes.
	}
	plan.Change = append(plan.Change, change)
}

// cardLabels names every id, so the card and the stale check have them; an id that names nothing is the model's
// mistake to fix.
func (l *turnLoop) cardLabels(ctx context.Context, kind domain.AssistantLabelKind, ids []int64) ([]domain.AssistantLabel, string, error) {
	labels := make([]domain.AssistantLabel, 0, len(ids))
	if kind == domain.AssistantLabelVideo {
		if l.a.videos == nil {
			return nil, "", errors.New("the assistant cannot read videos: it is not configured")
		}
		videos, err := l.a.videos.VideosByIDs(ctx, domain.AudienceOwner, ids)
		if err != nil {
			return nil, "", fmt.Errorf("cannot read the videos a step names: %w", err)
		}
		for _, id := range ids {
			i := slices.IndexFunc(videos, func(v domain.Video) bool { return v.ID == id })
			if i < 0 {
				return nil, fmt.Sprintf("there is no video with id %d", id), nil
			}
			labels = append(labels, domain.AssistantLabel{Kind: kind, ID: id, Key: videos[i].UserKey, Name: videos[i].Title})
		}
		return labels, "", nil
	}
	names, err := l.a.vocabulary.EntityNames(ctx, kind, ids)
	if err != nil {
		return nil, "", err
	}
	for _, id := range ids {
		name, ok := names[id]
		if !ok {
			return nil, fmt.Sprintf("there is no %s with id %d", kind, id), nil
		}
		labels = append(labels, domain.AssistantLabel{Kind: kind, ID: id, Name: name})
	}
	return labels, "", nil
}
