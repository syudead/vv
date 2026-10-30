package domain

import (
	"errors"
	"time"
)

// Limits of one turn of the tool loop; see docs/design-docs/assistant.md.
const (
	AssistantModelCallMax = 12
	AssistantToolCallMax  = 40
	AssistantTurnBudget   = 10 * time.Minute
	// AssistantTurnTTL is how long a finished turn is kept.
	AssistantTurnTTL = 24 * time.Hour
	// AssistantActiveTurnWindow is how long a finished turn stays in the active list the panel rebuilds from.
	AssistantActiveTurnWindow = time.Hour
)

// AssistantTurnStatus is where a turn is; running moves once to one of the others.
type AssistantTurnStatus string

const (
	AssistantTurnRunning AssistantTurnStatus = "running"
	AssistantTurnDone    AssistantTurnStatus = "done"
	AssistantTurnClarify AssistantTurnStatus = "clarify"
	AssistantTurnStopped AssistantTurnStatus = "stopped"
	// AssistantTurnInterrupted was running when the server stopped.
	AssistantTurnInterrupted AssistantTurnStatus = "interrupted"
)

// AssistantStopReason is why a stopped turn ended without its result.
type AssistantStopReason string

const (
	AssistantStopStepLimit  AssistantStopReason = "step_limit"
	AssistantStopTimeout    AssistantStopReason = "timeout"
	AssistantStopConnection AssistantStopReason = "connection"
	AssistantStopModel      AssistantStopReason = "model"
	AssistantStopUngrounded AssistantStopReason = "ungrounded"
	// AssistantStopNoAnswer: the model replied with neither text nor a tool call, twice; the model itself answered.
	AssistantStopNoAnswer AssistantStopReason = "no_answer"
)

// ErrAssistantBusy refuses a turn while another one runs; one local model serves one request at a time.
var ErrAssistantBusy = errors.New("another assistant turn is running")

// ErrAssistantTurnNotRunning refuses a change to a turn that already finished.
var ErrAssistantTurnNotRunning = errors.New("the assistant turn is not running")

// AssistantProgressKind says what kind of tool a progress item ran.
type AssistantProgressKind string

const (
	AssistantProgressBuiltin  AssistantProgressKind = "builtin"
	AssistantProgressRead     AssistantProgressKind = "read"
	AssistantProgressImage    AssistantProgressKind = "image"
	AssistantProgressWrite    AssistantProgressKind = "write"
	AssistantProgressExcluded AssistantProgressKind = "excluded"
)

// AssistantProgressItem is one tool call of a turn, composed by the server from the tool and its arguments, never
// from the model's text.
type AssistantProgressItem struct {
	Tool   string                `json:"tool"`
	Kind   AssistantProgressKind `json:"kind"`
	Labels []AssistantLabel      `json:"labels"`
	Images int                   `json:"images,omitempty"`
	N      int                   `json:"n,omitempty"`
	M      int                   `json:"m,omitempty"`
	// ReviewRunID is the review run a start_review call started or found running.
	ReviewRunID int64 `json:"reviewRunId,omitempty"`
	// Error is set when the call came back as an error result.
	Error string    `json:"error,omitempty"`
	At    time.Time `json:"at"`
}

// AssistantDirective is the last library filter or navigation of a turn, which the panel applies.
type AssistantDirective struct {
	Kind        string                `json:"kind"`
	Criteria    *AssistantCriteria    `json:"criteria,omitempty"`
	Destination *AssistantDestination `json:"destination,omitempty"`
}

// AssistantTurnClarification asks the owner which entity a name means; the turn then has no proposal.
type AssistantTurnClarification struct {
	Name       string               `json:"name"`
	Kind       AssistantLabelKind   `json:"kind"`
	Candidates []AssistantCandidate `json:"candidates"`
}

// AssistantExcludedRef names an excluded operation the model asked for, with why and where to do it instead.
type AssistantExcludedRef struct {
	Operation string `json:"operation"`
	Reason    string `json:"reason"`
	Page      string `json:"page"`
}

// AssistantTurnStop is why a stopped turn ended.
type AssistantTurnStop struct {
	Reason   AssistantStopReason   `json:"reason"`
	Excluded *AssistantExcludedRef `json:"excluded,omitempty"`
}

// AssistantTurn is one request run as a server-side job; the transcript stays in memory while it runs.
type AssistantTurn struct {
	ID             int64
	Status         AssistantTurnStatus
	Request        string
	Context        AssistantTurnContext
	PreviousTurnID int64
	Progress       []AssistantProgressItem
	// Answer is the model's text after the grounding check.
	Answer string
	// Judged marks an answer that used Clef's answers.
	Judged     bool
	Directive  *AssistantDirective
	Clarify    *AssistantTurnClarification
	ProposalID int64
	Stop       *AssistantTurnStop
	StartedAt  time.Time
	FinishedAt time.Time
}
