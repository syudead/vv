package domain

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strconv"
	"time"
)

// AssistantProposalStatus is a write proposal's state; a proposal moves forward only, as CanMove allows.
type AssistantProposalStatus string

const (
	AssistantProposed  AssistantProposalStatus = "proposed"
	AssistantApproved  AssistantProposalStatus = "approved"
	AssistantRunning   AssistantProposalStatus = "running"
	AssistantDone      AssistantProposalStatus = "done"
	AssistantPartial   AssistantProposalStatus = "partial"
	AssistantStale     AssistantProposalStatus = "stale"
	AssistantFailed    AssistantProposalStatus = "failed"
	AssistantDiscarded AssistantProposalStatus = "discarded"
)

var assistantProposalMoves = map[AssistantProposalStatus][]AssistantProposalStatus{
	AssistantProposed: {AssistantApproved, AssistantDiscarded},
	AssistantApproved: {AssistantRunning},
	AssistantRunning:  {AssistantDone, AssistantPartial, AssistantStale, AssistantFailed},
}

// CanMove reports whether a proposal in s may move to next; the store guards each move by the previous status.
func (s AssistantProposalStatus) CanMove(next AssistantProposalStatus) bool {
	return slices.Contains(assistantProposalMoves[s], next)
}

// Finished is true for the statuses a run ends in.
func (s AssistantProposalStatus) Finished() bool {
	return s == AssistantDone || s == AssistantPartial || s == AssistantStale || s == AssistantFailed
}

// AssistantProposalTTL is how long a proposal waits for approval before the worker deletes it.
const AssistantProposalTTL = time.Hour

// ErrAssistantProposalNotPending refuses approving a proposal that is no longer proposed.
var ErrAssistantProposalNotPending = errors.New("the assistant proposal is not pending")

// AssistantLabelKind is the kind of entity a confirmation card names.
type AssistantLabelKind string

const (
	AssistantLabelVideo        AssistantLabelKind = "video"
	AssistantLabelTag          AssistantLabelKind = "tag"
	AssistantLabelTagKind      AssistantLabelKind = "tagKind"
	AssistantLabelPerson       AssistantLabelKind = "person"
	AssistantLabelOrganization AssistantLabelKind = "organization"
	AssistantLabelPlaylist     AssistantLabelKind = "playlist"
	AssistantLabelGroup        AssistantLabelKind = "group"
	AssistantLabelSmartFolder  AssistantLabelKind = "smartFolder"
	AssistantLabelFolderRoot   AssistantLabelKind = "folderRoot"
)

// AssistantLabel is one entity as the card showed it. A video carries its user key, which survives a rebuild,
// and is checked for presence only; every other kind is checked by its current name.
type AssistantLabel struct {
	Kind AssistantLabelKind `json:"kind"`
	ID   int64              `json:"id,omitempty"`
	Key  string             `json:"key,omitempty"`
	Name string             `json:"name"`
}

// AssistantLabelRef identifies a named entity for the stale check.
type AssistantLabelRef struct {
	Kind AssistantLabelKind
	ID   int64
}

// AssistantPlanTargets is what the operation acts on: named entities, video user keys, or both.
type AssistantPlanTargets struct {
	Kind   AssistantTarget  `json:"kind"`
	Labels []AssistantLabel `json:"labels,omitempty"`
	// VideoKeys are the videos' user keys in plan order; batches follow this order. A plan whose operation acts on
	// one version (see ByVersion) holds each version's content key instead, here and in its video labels.
	VideoKeys []string `json:"videoKeys,omitempty"`
}

// AssistantChange is one argument after resolution: the entities it names, or a literal value.
type AssistantChange struct {
	Argument AssistantArgument `json:"argument"`
	Labels   []AssistantLabel  `json:"labels,omitempty"`
	Value    json.RawMessage   `json:"value,omitempty"`
}

// AssistantPlanCall is the request the plan's operation receives, built when the plan is resolved. VideoField
// names the body field, and VideoPathParam the path parameter, that take the current video ids of each batch.
type AssistantPlanCall struct {
	Path           map[string]string `json:"path,omitempty"`
	Query          map[string]string `json:"query,omitempty"`
	Body           map[string]any    `json:"body,omitempty"`
	VideoField     string            `json:"videoField,omitempty"`
	VideoPathParam string            `json:"videoPathParam,omitempty"`
}

// AssistantImpact counts the videos a merge moves, as the card showed it.
type AssistantImpact struct {
	Videos int `json:"videos"`
}

// AssistantPlan is what an approved proposal executes, exactly as the card showed it.
type AssistantPlan struct {
	ActionID string               `json:"actionId"`
	Targets  AssistantPlanTargets `json:"targets"`
	Change   []AssistantChange    `json:"change,omitempty"`
	// Created are names the operation creates, such as a tag to attach that does not exist yet.
	Created []string          `json:"created,omitempty"`
	Call    AssistantPlanCall `json:"call"`
	Impact  *AssistantImpact  `json:"impact,omitempty"`
}

// Action returns the plan's catalog row.
func (p AssistantPlan) Action() (AssistantAction, bool) {
	return LookupAssistantAction(p.ActionID)
}

// Batched is true when the plan's videos run in batches of AssistantBatchSize user keys. A batching row whose plan
// names no videos, such as updateFavorites on groups only, is one call like any other operation.
func (p AssistantPlan) Batched() bool {
	row, ok := p.Action()
	return ok && row.Batches && len(p.Targets.VideoKeys) > 0
}

// Total is the proposal's count: user keys for a batched operation, otherwise one call.
func (p AssistantPlan) Total() int {
	if p.Batched() {
		return len(p.Targets.VideoKeys)
	}
	return 1
}

// Batches splits the video keys into the calls the worker makes, in plan order; an unbatched plan is one call
// holding every key, which may be none.
func (p AssistantPlan) Batches() [][]string {
	if !p.Batched() {
		return [][]string{slices.Clone(p.Targets.VideoKeys)}
	}
	var batches [][]string
	for chunk := range slices.Chunk(p.Targets.VideoKeys, AssistantBatchSize) {
		batches = append(batches, slices.Clone(chunk))
	}
	return batches
}

// Labels returns every named entity of the targets and the change.
func (p AssistantPlan) Labels() []AssistantLabel {
	labels := slices.Clone(p.Targets.Labels)
	for _, change := range p.Change {
		labels = append(labels, change.Labels...)
	}
	return labels
}

// AssistantCurrent is what the store reads back before the first batch.
type AssistantCurrent struct {
	// Names holds the current name of each named entity that still exists.
	Names map[AssistantLabelRef]string
	// Videos maps each user key that is still in the library to a current video id.
	Videos map[string]int64
}

// Stale is true when a video is gone or a named entity was renamed or deleted since the card showed it.
func (p AssistantPlan) Stale(current AssistantCurrent) bool {
	for _, key := range p.Targets.VideoKeys {
		if _, ok := current.Videos[key]; !ok {
			return true
		}
	}
	for _, label := range p.Labels() {
		if label.Kind == AssistantLabelVideo {
			if _, ok := current.Videos[label.Key]; !ok {
				return true
			}
			continue
		}
		name, ok := current.Names[AssistantLabelRef{Kind: label.Kind, ID: label.ID}]
		if !ok || name != label.Name {
			return true
		}
	}
	return false
}

// CallFor builds one batch's call from the plan's request and the batch's current video ids.
func (p AssistantPlan) CallFor(videoIDs []int64) AssistantCall {
	call := AssistantCall{
		ActionID: p.ActionID, Path: maps.Clone(p.Call.Path), Query: maps.Clone(p.Call.Query),
		Body: maps.Clone(p.Call.Body),
	}
	if p.Call.VideoField != "" {
		if call.Body == nil {
			call.Body = map[string]any{}
		}
		call.Body[p.Call.VideoField] = slices.Clone(videoIDs)
	}
	if p.Call.VideoPathParam != "" && len(videoIDs) > 0 {
		if call.Path == nil {
			call.Path = map[string]string{}
		}
		call.Path[p.Call.VideoPathParam] = strconv.FormatInt(videoIDs[0], 10)
	}
	return call
}

// MergeSources returns the target kind and source ids of a merge row, whose names argument holds the
// sources; ok is false for any other row.
func (p AssistantPlan) MergeSources() (AssistantTarget, []int64, bool) {
	row, found := p.Action()
	if !found || !assistantMergeOperations[row.Operation] {
		return "", nil, false
	}
	var ids []int64
	for _, change := range p.Change {
		if change.Argument != AssistantArgNames {
			continue
		}
		for _, label := range change.Labels {
			ids = append(ids, label.ID)
		}
	}
	return row.Target, ids, true
}

var assistantMergeOperations = map[string]bool{"mergeTag": true, "mergePeople": true, "mergeOrganizations": true}

// ByVersion is true when the operation acts on the given version rather than on the user data its bundle shares,
// so the worker resolves the plan's content keys to that version and not to any video of the bundle.
func (p AssistantPlan) ByVersion() bool {
	row, found := p.Action()
	return found && assistantVersionOperations[row.Operation]
}

var assistantVersionOperations = map[string]bool{
	"makeRepresentativeVersion": true, "unbundleVideo": true,
	"dismissVersionCandidate": true, "rejudgeVersionCandidate": true,
}

// AssistantProposalError is the first failed call's screen API code and reason, or "interrupted" after a restart.
type AssistantProposalError struct {
	Code   string `json:"code"`
	Reason string `json:"reason,omitempty"`
}

// AssistantInterrupted is the error of a run a restart stopped.
const AssistantInterrupted = "interrupted"

// AssistantCounts are the proposal's counts in user keys, or 0 and 1 for an operation without batches.
type AssistantCounts struct {
	Total   int
	Applied int
	Failed  int
}

// AssistantProposal is one stored write proposal.
type AssistantProposal struct {
	ID         int64
	Status     AssistantProposalStatus
	Request    string
	Plan       AssistantPlan
	Counts     AssistantCounts
	Error      *AssistantProposalError
	ActivityID int64
	CreatedAt  time.Time
	ApprovedAt time.Time
	StartedAt  time.Time
	FinishedAt time.Time
}

// Irreversible comes from the catalog row; the model never decides it.
func (p AssistantProposal) Irreversible() bool {
	row, ok := p.Plan.Action()
	return ok && row.Irreversible
}

// AssistantRunResult is how a run ended.
type AssistantRunResult struct {
	Status AssistantProposalStatus
	Counts AssistantCounts
	Error  *AssistantProposalError
}

// ActivityOutcome maps a finished run to its activity entry's outcome.
func (r AssistantRunResult) ActivityOutcome() ActivityOutcome {
	switch r.Status {
	case AssistantPartial:
		return ActivityPartial
	case AssistantStale:
		return ActivityStale
	case AssistantFailed:
		return ActivityOutcomeFailed
	default:
		return ActivityCompleted
	}
}

// AssistantRunDetail is the activity entry's detail of a run, as JSON the Activity screen reads.
type AssistantRunDetail struct {
	Operation string `json:"operation"`
	Total     int    `json:"total"`
	Applied   int    `json:"applied"`
	Failed    int    `json:"failed"`
}

// Encode never fails for this struct.
func (d AssistantRunDetail) Encode() string {
	body, _ := json.Marshal(d)
	return string(body)
}

// AssistantChanged follows every committed change of a proposal; it carries no payload.
type AssistantChanged struct{}

func (AssistantChanged) event() {}
