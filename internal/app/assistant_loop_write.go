package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// createdName is a name a create step of the turn makes, by its kind and folded name.
type createdKey struct {
	kind domain.AssistantLabelKind
	name string
}

// createRows are the rows whose response is the entity they create, so a later step can take its id.
var createRows = map[string]domain.AssistantLabelKind{
	"createTag": domain.AssistantLabelTag, "createTagKind": domain.AssistantLabelTagKind,
	"createPerson": domain.AssistantLabelPerson, "createOrganization": domain.AssistantLabelOrganization,
	"createPlaylist": domain.AssistantLabelPlaylist, "createGroup": domain.AssistantLabelGroup,
	"createSmartFolder": domain.AssistantLabelSmartFolder,
}

// fetchedResult is a fetch tool's result read in this turn, which a fetch-apply step names by its fetchId.
type fetchedResult struct {
	operation string
	body      json.RawMessage
}

// turnWrites are the steps a turn's write calls built, in call order, the names its create steps make, and the
// fetch results its reads answered.
type turnWrites struct {
	plans   []domain.AssistantPlan
	creates map[createdKey]int
	fetches map[string]fetchedResult
}

// createdEarlier resolves a name that an earlier create step of the turn makes to a placeholder id, -(step+1),
// which bindCreated turns into that step's created id.
func (w *turnWrites) createdEarlier(kind domain.AssistantLabelKind, name string) (domain.AssistantLabel, bool) {
	if w == nil {
		return domain.AssistantLabel{}, false
	}
	step, ok := w.creates[createdKey{kind, domain.FoldForMatch(name)}]
	if !ok {
		return domain.AssistantLabel{}, false
	}
	return domain.AssistantLabel{Kind: kind, ID: -int64(step + 1), Name: name}, true
}

func (w *turnWrites) add(plan domain.AssistantPlan) int {
	w.plans = append(w.plans, plan)
	step := len(w.plans) - 1
	if kind, ok := createRows[plan.ActionID]; ok {
		if name, _ := plan.Call.Body["name"].(string); name != "" {
			if w.creates == nil {
				w.creates = map[createdKey]int{}
			}
			w.creates[createdKey{kind, domain.FoldForMatch(name)}] = step
		}
	}
	return step
}

// fetchOperations are the reads whose results a fetch-apply step applies.
var fetchOperations = []string{"fetchWorkInfo", "fetchPersonWithPlugin"}

// remember keeps a fetch tool's result by its fetchId.
func (w *turnWrites) remember(operation string, body json.RawMessage) {
	if !slices.Contains(fetchOperations, operation) {
		return
	}
	var result struct {
		FetchID string `json:"fetchId"`
	}
	if json.Unmarshal(body, &result) != nil || result.FetchID == "" {
		return
	}
	if w.fetches == nil {
		w.fetches = map[string]fetchedResult{}
	}
	w.fetches[result.FetchID] = fetchedResult{operation: operation, body: body}
}

// proposable reports whether a write tool can become a step.
func proposable(tool domain.AssistantTool) bool {
	if tool.Row == nil {
		return false
	}
	_, built := planBuilders[tool.Row.ID]
	return tool.Row.Schema || built
}

// write turns one write call into a step of the turn's proposal and answers what was proposed. A name that needs
// the owner's choice ends the turn as clarify; it returns an error only for a storage failure.
func (l *turnLoop) write(ctx context.Context, call domain.ToolCall, tool domain.AssistantTool) (string, string, error) {
	if !proposable(tool) {
		return "", fmt.Sprintf("%s cannot be proposed by the assistant; tell the owner to do it on its page", tool.Name), nil
	}
	if len(l.writes.plans) >= domain.AssistantProposalStepMax {
		return "", fmt.Sprintf("a proposal holds at most %d steps", domain.AssistantProposalStepMax), nil
	}
	build := l.nameStep
	if tool.Row.Schema {
		build = l.schemaStep
	}
	plan, failure, err := build(ctx, tool, call.Arguments)
	if err != nil || failure != "" || l.turn.Status == domain.AssistantTurnClarify {
		return "", failure, err
	}
	if i := slices.IndexFunc(l.writes.plans, func(p domain.AssistantPlan) bool {
		return p.ActionID == plan.ActionID && reflect.DeepEqual(p.Call, plan.Call) &&
			slices.Equal(p.Targets.VideoKeys, plan.Targets.VideoKeys)
	}); i >= 0 {
		return "", fmt.Sprintf("this is step %d again; it is proposed once", i+1), nil
	}
	step := l.writes.add(plan)
	encoded, _ := json.Marshal(map[string]any{ // Plain values always encode.
		"step": step + 1, "proposed": describePlan(plan),
		"note": "Nothing has changed yet; the owner approves the proposal after your answer.",
	})
	return string(encoded), "", nil
}

// nameStep builds a name-based row's step through its plan builder.
func (l *turnLoop) nameStep(ctx context.Context, tool domain.AssistantTool, raw json.RawMessage) (domain.AssistantPlan, string, error) {
	req, pins, failure := rowRequest(*tool.Row, raw)
	if failure != "" {
		return domain.AssistantPlan{}, failure, nil
	}
	t := &assistantTurn{
		a: l.a, idPins: pins, writes: &l.writes,
		input: domain.AssistantTurnInput{Text: l.turn.Request, Context: l.turn.Context, PreviousTurnID: l.turn.PreviousTurnID},
	}
	plan, ok, err := t.buildPlan(ctx, req)
	switch {
	case errors.Is(err, domain.ErrAssistantTooManyVideos):
		return plan, fmt.Sprintf("the target holds more than %d videos", domain.AssistantSelectionMax), nil
	case err != nil:
		return plan, "", err
	case !ok:
		return plan, l.unresolved(t), nil
	}
	return bindCreated(plan)
}

// unresolved ends the turn as clarify when a name needs the owner's choice; any other reason is the failure the
// model receives to fix its call.
func (l *turnLoop) unresolved(t *assistantTurn) string {
	c := t.clarification
	switch {
	case t.failure != "":
		return t.failure
	case c != nil && t.clarifyName != "" && (len(c.Candidates) > 0 || len(c.Names) > 0):
		l.turn.Status = domain.AssistantTurnClarify
		l.turn.Clarify = &domain.AssistantTurnClarification{
			Name: t.clarifyName, Kind: t.clarifyKind, Candidates: append([]domain.AssistantCandidate{}, c.Candidates...),
		}
		l.writes.plans = nil
		return ""
	case c != nil && c.About == "target":
		return "the target names nothing this operation can act on: nothing is selected, or it holds another " +
			"number of items than the operation takes"
	}
	return "an argument the operation needs is missing or has a value it does not take"
}

// noteClarify records the name and kind the first clarification asks about.
func (t *assistantTurn) noteClarify(kind domain.AssistantLabelKind, name string, candidates []domain.AssistantCandidate) {
	if t.clarification != nil {
		return
	}
	if kind == "" && len(candidates) > 0 {
		kind = candidates[0].Kind
	}
	t.clarifyName, t.clarifyKind = name, kind
}

// refuse records why the call cannot become a step.
func (t *assistantTurn) refuse(format string, args ...any) bool {
	if t.failure == "" {
		t.failure = fmt.Sprintf(format, args...)
	}
	return false
}

// idPinned is the match whose id the call gave with the name; an id among no match is ignored.
func (t *assistantTurn) idPinned(name string, found []domain.AssistantCandidate) (domain.AssistantLabel, bool) {
	id, ok := t.idPins[domain.FoldForMatch(name)]
	if !ok {
		return domain.AssistantLabel{}, false
	}
	for _, c := range found {
		if c.ID == id {
			return domain.AssistantLabel{Kind: c.Kind, ID: c.ID, Name: c.Name}, true
		}
	}
	return domain.AssistantLabel{}, false
}

// nameArguments are the arguments, besides target.names, whose items may be { name, id }.
var nameArguments = []string{"name", "names", "tagNames", "personNames", "organizationNames", "tagKindName", "parentName"}

// listArguments take an array; a model often writes one string for one item.
var listArguments = []string{"names", "tagNames", "personNames", "organizationNames", "aliases", "fields"}

// rowRequest reads a name-based row's tool arguments: the target and the row's own arguments, with each
// { name, id } item replaced by its name and its id kept as a pin.
func rowRequest(row domain.AssistantAction, raw json.RawMessage) (*domain.AssistantActionRequest, map[string]int64, string) {
	args := map[string]any{}
	if len(raw) > 0 && json.Unmarshal(raw, &args) != nil {
		return nil, nil, "the arguments are not a JSON object"
	}
	allowed := []string{}
	if row.Target != domain.AssistantTargetNone {
		allowed = append(allowed, "target")
	}
	for _, arg := range row.Arguments {
		allowed = append(allowed, string(arg))
	}
	pins := map[string]int64{}
	for key, value := range args {
		if value == nil || value == "" {
			delete(args, key)
			continue
		}
		if !slices.Contains(allowed, key) {
			return nil, nil, fmt.Sprintf("%s takes no argument %s; it takes %s", row.ID, key, strings.Join(allowed, ", "))
		}
		value = unquoted(key, value)
		if slices.Contains(nameArguments, key) {
			value = takeNamePins(value, pins)
		}
		args[key] = value
	}
	if target, ok := args["target"].(map[string]any); ok {
		target["names"] = takeNamePins(target["names"], pins)
	}
	encoded, _ := json.Marshal(args) // Decoded JSON always encodes.
	var parsed struct {
		Target *domain.AssistantTargetRequest `json:"target"`
		domain.AssistantArguments
	}
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		return nil, nil, "the arguments do not match the tool's schema: " + err.Error()
	}
	return &domain.AssistantActionRequest{Operation: row.ID, Target: parsed.Target, Arguments: &parsed.AssistantArguments}, pins, ""
}

// unquoted reads a target or list argument that a model wrote as text: JSON encoded in a string, a target kind
// alone, or one item of a list.
func unquoted(key string, value any) any {
	text, isText := value.(string)
	if !isText || (key != "target" && !slices.Contains(listArguments, key)) {
		return value
	}
	var decoded any
	if json.Unmarshal([]byte(text), &decoded) == nil {
		if _, isText := decoded.(string); !isText {
			return decoded
		}
	}
	if key == "target" {
		return map[string]any{"kind": text}
	}
	return []any{text}
}

// takeNamePins replaces { name, id } items by their names and keeps each id by the folded name.
func takeNamePins(value any, pins map[string]int64) any {
	switch v := value.(type) {
	case []any:
		for i := range v {
			v[i] = takeNamePins(v[i], pins)
		}
		return v
	case map[string]any:
		name, _ := v["name"].(string)
		if id, ok := v["id"].(float64); ok && id > 0 && name != "" {
			pins[domain.FoldForMatch(name)] = int64(id)
		}
		return name
	}
	return value
}

// bindCreated turns the placeholder ids of names that earlier steps create into createdBy marks and into the
// call's step ids, which the worker fills from those steps' responses. A placeholder inside a nested body value
// cannot be filled, so that call is refused.
func bindCreated(plan domain.AssistantPlan) (domain.AssistantPlan, string, error) {
	mark := func(labels []domain.AssistantLabel) {
		for i, label := range labels {
			if label.ID >= 0 {
				continue
			}
			by := domain.AssistantCreatedBy{Name: label.Name, Step: int(-label.ID) - 1}
			if !slices.Contains(plan.CreatedBy, by) {
				plan.CreatedBy = append(plan.CreatedBy, by)
			}
			labels[i].ID = 0
		}
	}
	mark(plan.Targets.Labels)
	for i := range plan.Change {
		mark(plan.Change[i].Labels)
	}
	for name, value := range plan.Call.Path {
		if id, err := strconv.ParseInt(value, 10, 64); err == nil && id < 0 {
			plan.Call.StepIDs = append(plan.Call.StepIDs, domain.AssistantStepID{Step: int(-id) - 1, Path: name})
			delete(plan.Call.Path, name)
		}
	}
	for field, value := range plan.Call.Body {
		rest, steps := withoutPlaceholders(value)
		for _, step := range steps {
			plan.Call.StepIDs = append(plan.Call.StepIDs, domain.AssistantStepID{Step: step, Field: field})
		}
		switch {
		case rest == nil && len(steps) > 0:
			delete(plan.Call.Body, field)
		case hasPlaceholder(rest):
			return plan, fmt.Sprintf("%s cannot use what an earlier step creates in %s; propose it in a later "+
				"request, after the owner approves this one", plan.ActionID, field), nil
		default:
			plan.Call.Body[field] = rest
		}
	}
	return plan, "", nil
}

// withoutPlaceholders takes the placeholder ids out of a body field's top level: a placeholder value becomes nil,
// and placeholders are removed from a list of ids.
func withoutPlaceholders(value any) (any, []int) {
	switch v := value.(type) {
	case int64:
		if v < 0 {
			return nil, []int{int(-v) - 1}
		}
	case []int64:
		var steps []int
		kept := make([]int64, 0, len(v))
		for _, id := range v {
			if id < 0 {
				steps = append(steps, int(-id)-1)
				continue
			}
			kept = append(kept, id)
		}
		return kept, steps
	}
	return value, nil
}

func hasPlaceholder(value any) bool {
	switch v := value.(type) {
	case int64:
		return v < 0
	case []int64:
		return slices.ContainsFunc(v, func(id int64) bool { return id < 0 })
	case []any:
		return slices.ContainsFunc(v, hasPlaceholder)
	case map[string]any:
		for _, item := range v {
			if hasPlaceholder(item) {
				return true
			}
		}
	}
	return false
}

// propose stores the turn's steps as one proposal when the turn answered, or stopped only because its answer was
// ungrounded, and discards the pending proposal of the request it follows up.
func (l *turnLoop) propose(ctx context.Context) error {
	ungrounded := l.turn.Status == domain.AssistantTurnStopped && l.turn.Stop != nil &&
		l.turn.Stop.Reason == domain.AssistantStopUngrounded
	if len(l.writes.plans) == 0 || (l.turn.Status != domain.AssistantTurnDone && !ungrounded) {
		return nil
	}
	proposal, err := l.a.Propose(ctx, l.turn.ID, l.turn.Request, l.writes.plans)
	if err != nil {
		return err
	}
	l.turn.ProposalID = proposal.ID
	if l.turn.PreviousTurnID == 0 {
		return nil
	}
	prev, err := l.a.turns.Turn(ctx, l.turn.PreviousTurnID)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && prev.ProposalID == 0) {
		return nil
	}
	if err != nil {
		return err
	}
	err = l.a.store.DiscardProposal(ctx, prev.ProposalID, l.a.now())
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrAssistantProposalNotPending) {
		return nil
	}
	return err
}
