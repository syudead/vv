package domain

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
)

// AssistantProposalStepMax bounds the steps of one proposal, and so the write calls of one turn.
const AssistantProposalStepMax = 30

// AssistantStepStatus is one step's state within a proposal; a step moves forward only, as CanMove allows.
type AssistantStepStatus string

const (
	AssistantStepPending AssistantStepStatus = "pending"
	AssistantStepSkipped AssistantStepStatus = "skipped"
	AssistantStepRunning AssistantStepStatus = "running"
	AssistantStepDone    AssistantStepStatus = "done"
	AssistantStepPartial AssistantStepStatus = "partial"
	AssistantStepStale   AssistantStepStatus = "stale"
	AssistantStepFailed  AssistantStepStatus = "failed"
	// AssistantStepNotRun is a kept step after the one the run stopped at.
	AssistantStepNotRun AssistantStepStatus = "notRun"
)

var assistantStepMoves = map[AssistantStepStatus][]AssistantStepStatus{
	AssistantStepPending: {AssistantStepSkipped, AssistantStepRunning, AssistantStepNotRun},
	AssistantStepRunning: {AssistantStepDone, AssistantStepPartial, AssistantStepStale, AssistantStepFailed},
}

// CanMove reports whether a step in s may move to next.
func (s AssistantStepStatus) CanMove(next AssistantStepStatus) bool {
	return slices.Contains(assistantStepMoves[s], next)
}

// ProposalStatus is the proposal status a step that ran ends the proposal in; ran is false for a step that did
// not run to an end.
func (s AssistantStepStatus) ProposalStatus() (status AssistantProposalStatus, ran bool) {
	switch s {
	case AssistantStepDone:
		return AssistantDone, true
	case AssistantStepPartial:
		return AssistantPartial, true
	case AssistantStepStale:
		return AssistantStale, true
	case AssistantStepFailed:
		return AssistantFailed, true
	default:
		return "", false
	}
}

// AssistantCreatedBy names an entity that an earlier step of the same proposal creates.
type AssistantCreatedBy struct {
	Name string `json:"name"`
	Step int    `json:"step"`
}

// AssistantStepID puts the id an earlier step's response carries into this step's request: into a path parameter,
// or into a body field, where it is appended to a list of ids or replaces a single value.
type AssistantStepID struct {
	Step  int    `json:"step"`
	Path  string `json:"path,omitempty"`
	Field string `json:"field,omitempty"`
}

// AssistantStepState is a step's progress, stored apart from the plan the card showed.
type AssistantStepState struct {
	Status  AssistantStepStatus     `json:"status"`
	Total   int                     `json:"total"`
	Applied int                     `json:"applied"`
	Failed  int                     `json:"failed"`
	Error   *AssistantProposalError `json:"error,omitempty"`
}

// AssistantStep is one write of a proposal: the plan the card showed and its progress. Its index in the proposal
// is its position on the card.
type AssistantStep struct {
	Plan AssistantPlan
	AssistantStepState
}

// Irreversible comes from the catalog row; the model never decides it.
func (s AssistantStep) Irreversible() bool {
	row, ok := s.Plan.Action()
	return ok && row.Irreversible
}

// Stops reports whether the run ends at this step: it is stale, or a batch failed. Videos gone between batches
// leave the step partial without an error, and the run goes on.
func (s AssistantStepState) Stops() bool {
	return s.Status == AssistantStepStale || s.Status == AssistantStepFailed || s.Error != nil
}

// NewAssistantSteps makes pending steps of the plans, in card order.
func NewAssistantSteps(plans []AssistantPlan) []AssistantStep {
	steps := make([]AssistantStep, len(plans))
	for i, plan := range plans {
		steps[i] = AssistantStep{
			Plan: plan, AssistantStepState: AssistantStepState{Status: AssistantStepPending, Total: plan.Total()},
		}
	}
	return steps
}

// AssistantStepCounts sums the counts of the steps the owner kept.
func AssistantStepCounts(steps []AssistantStep) AssistantCounts {
	var counts AssistantCounts
	for _, step := range steps {
		if step.Status == AssistantStepSkipped {
			continue
		}
		counts.Total += step.Total
		counts.Applied += step.Applied
		counts.Failed += step.Failed
	}
	return counts
}

// AssistantStepNeedsSkippedError refuses an approval in which the kept step Step names an entity that an unticked
// step creates.
type AssistantStepNeedsSkippedError struct {
	Step int
}

func (e *AssistantStepNeedsSkippedError) Error() string {
	return fmt.Sprintf("assistant step %d needs a step that is not kept", e.Step)
}

// Select returns the steps after an approval that keeps the given indexes: the others become skipped. It returns
// ErrAssistantStepsInvalid for no index or an index out of range, and *AssistantStepNeedsSkippedError for a kept
// step whose createdBy names a step that is not kept.
func (p AssistantProposal) Select(kept []int) ([]AssistantStep, error) {
	if len(kept) == 0 {
		return nil, ErrAssistantStepsInvalid
	}
	keep := make(map[int]bool, len(kept))
	for _, i := range kept {
		if i < 0 || i >= len(p.Steps) {
			return nil, ErrAssistantStepsInvalid
		}
		keep[i] = true
	}
	steps := slices.Clone(p.Steps)
	for i := range steps {
		if !keep[i] {
			if err := steps[i].Move(AssistantStepSkipped); err != nil {
				return nil, err
			}
			continue
		}
		for _, created := range steps[i].Plan.CreatedBy {
			if !keep[created.Step] {
				return nil, &AssistantStepNeedsSkippedError{Step: i}
			}
		}
	}
	return steps, nil
}

// Move changes the step's status as CanMove allows.
func (s *AssistantStep) Move(next AssistantStepStatus) error {
	if !s.Status.CanMove(next) {
		return fmt.Errorf("an assistant step cannot move from %s to %s", s.Status, next)
	}
	s.Status = next
	return nil
}

// SkipAfter marks every pending step after i notRun, once the run stopped at step i.
func SkipAfter(steps []AssistantStep, i int) {
	for j := i + 1; j < len(steps); j++ {
		if steps[j].Status == AssistantStepPending {
			steps[j].Status = AssistantStepNotRun
		}
	}
}

// InterruptAssistantSteps ends a run a restart stopped: the running step fails with the interrupted error, and
// the pending steps after it do not run.
func InterruptAssistantSteps(steps []AssistantStep) []AssistantStep {
	steps = slices.Clone(steps)
	for i := range steps {
		if steps[i].Status == AssistantStepRunning {
			steps[i].Status = AssistantStepFailed
			steps[i].Error = &AssistantProposalError{Code: AssistantInterrupted}
			SkipAfter(steps, i)
		}
	}
	return steps
}

// TouchedBefore returns the named entities that the kept steps before step i act on or name. Step i does not
// re-read them: an earlier step that renames or merges what a later step names is expected.
func TouchedBefore(steps []AssistantStep, i int) map[AssistantLabelRef]bool {
	touched := map[AssistantLabelRef]bool{}
	for _, step := range steps[:i] {
		if step.Status == AssistantStepSkipped {
			continue
		}
		for _, label := range step.Plan.Labels() {
			if label.Kind != AssistantLabelVideo && label.ID != 0 {
				touched[AssistantLabelRef{Kind: label.Kind, ID: label.ID}] = true
			}
		}
	}
	return touched
}

// WithStepIDs fills the call with the ids that earlier steps created, by step index; ok is false when a step the
// plan needs left no id.
func (p AssistantPlan) WithStepIDs(call AssistantCall, created map[int]int64) (AssistantCall, bool) {
	if len(p.Call.StepIDs) == 0 {
		return call, true
	}
	call.Path, call.Body = maps.Clone(call.Path), maps.Clone(call.Body)
	for _, ref := range p.Call.StepIDs {
		id, ok := created[ref.Step]
		if !ok {
			return call, false
		}
		if ref.Path != "" {
			if call.Path == nil {
				call.Path = map[string]string{}
			}
			call.Path[ref.Path] = strconv.FormatInt(id, 10)
		}
		if ref.Field != "" {
			if call.Body == nil {
				call.Body = map[string]any{}
			}
			call.Body[ref.Field] = withStepID(call.Body[ref.Field], id)
		}
	}
	return call, true
}

// withStepID appends the id to a list of ids and replaces any other value.
func withStepID(value any, id int64) any {
	switch ids := value.(type) {
	case []int64:
		return append(slices.Clone(ids), id)
	case []any:
		return append(slices.Clone(ids), id)
	default:
		return id
	}
}
