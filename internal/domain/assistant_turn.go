package domain

import "errors"

// Limits of one turn: the request text, as a smart rule's query, and the selection, as the bulk operations.
const (
	AssistantTextMax      = 500
	AssistantSelectionMax = 20000
)

// ErrAssistantTooManyVideos refuses a target that expands to more videos than the bulk operations take.
var ErrAssistantTooManyVideos = errors.New("the assistant target holds too many videos")

// AssistantRouteKind is the kind of screen a turn was sent from.
type AssistantRouteKind string

const (
	AssistantRouteLibrary      AssistantRouteKind = "library"
	AssistantRouteFolder       AssistantRouteKind = "folder"
	AssistantRouteVideo        AssistantRouteKind = "video"
	AssistantRoutePerson       AssistantRouteKind = "person"
	AssistantRouteOrganization AssistantRouteKind = "organization"
	AssistantRoutePlaylist     AssistantRouteKind = "playlist"
	AssistantRouteGroup        AssistantRouteKind = "group"
	AssistantRouteSmartFolder  AssistantRouteKind = "smartFolder"
	AssistantRouteTag          AssistantRouteKind = "tag"
	AssistantRouteOther        AssistantRouteKind = "other"
)

// AssistantPin is an id the owner chose in a clarification; it replaces resolving the same name again.
type AssistantPin struct {
	Kind AssistantLabelKind
	Name string
	ID   int64
}

// AssistantTurnContext is the screen a turn was sent from, as the panel reports it.
type AssistantTurnContext struct {
	Route                  AssistantRouteKind
	RouteID                int64
	SelectedVideoIDs       []int64
	SelectedGroupIDs       []int64
	SelectedSmartFolderIDs []int64
	// Filter is the list screen's current filter, nil when the screen has none.
	Filter *SmartRule
	Sort   VideoSort
	Pinned []AssistantPin
}

// AssistantTurnInput is one request of the owner.
type AssistantTurnInput struct {
	Text    string
	Context AssistantTurnContext
	// PreviousTurnID is the turn a follow-up continues; 0 starts a new thread.
	PreviousTurnID int64
}

// AssistantResolutionKind is what resolving one filter, navigation, write or review request gave.
type AssistantResolutionKind string

const (
	AssistantResolvedFilter      AssistantResolutionKind = "filter"
	AssistantResolvedNavigate    AssistantResolutionKind = "navigate"
	AssistantResolvedProposal    AssistantResolutionKind = "proposal"
	AssistantResolvedReview      AssistantResolutionKind = "review"
	AssistantResolvedClarify     AssistantResolutionKind = "clarify"
	AssistantResolvedUnsupported AssistantResolutionKind = "unsupported"
)

// AssistantCriteria is a library filter with ids, and the labels the chips show for them.
type AssistantCriteria struct {
	Rule   SmartRule
	Sort   VideoSort
	Labels []AssistantLabel
}

// AssistantDestination is a screen the panel navigates to.
type AssistantDestination struct {
	Kind         string
	ID           int64
	SettingsPage string
}

// AssistantCandidate is one entity a name matched.
type AssistantCandidate struct {
	Kind  AssistantLabelKind
	ID    int64
	Name  string
	Count int
}

// AssistantClarification asks the owner which entity, target or value the request means.
type AssistantClarification struct {
	About      string
	Candidates []AssistantCandidate
	// Names are the names that matched nothing.
	Names []string
}

// AssistantResolution is what resolving one request against the library gave; the loop's tools turn it into a
// directive, a proposal step or a review run.
type AssistantResolution struct {
	Kind        AssistantResolutionKind
	Criteria    *AssistantCriteria
	Destination *AssistantDestination
	Proposal    *AssistantProposal
	Review      *AssistantReviewRun
	Clarify     *AssistantClarification
	// Unsupported is the operation the model named, when the catalog has it but the assistant cannot build it yet.
	Unsupported string
}
