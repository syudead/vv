package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

// AssistantOutcomeKind is what the model decided a request asks for; each kind reads one member.
type AssistantOutcomeKind string

const (
	AssistantOutcomeFilter      AssistantOutcomeKind = "filter"
	AssistantOutcomeNavigate    AssistantOutcomeKind = "navigate"
	AssistantOutcomeAnswer      AssistantOutcomeKind = "answer"
	AssistantOutcomeAction      AssistantOutcomeKind = "action"
	AssistantOutcomeOrganize    AssistantOutcomeKind = "organize"
	AssistantOutcomeClarify     AssistantOutcomeKind = "clarify"
	AssistantOutcomeUnsupported AssistantOutcomeKind = "unsupported"
)

// AssistantFilter is a library filter by names; the server resolves every name.
type AssistantFilter struct {
	Q             string   `json:"q,omitempty"`
	Favorite      bool     `json:"favorite,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	TagKind       string   `json:"tagKind,omitempty"`
	Person        string   `json:"person,omitempty"`
	Organization  string   `json:"organization,omitempty"`
	MinDurationMs *int64   `json:"minDurationMs,omitempty"`
	MaxDurationMs *int64   `json:"maxDurationMs,omitempty"`
	AddedFrom     string   `json:"addedFrom,omitempty"`
	AddedTo       string   `json:"addedTo,omitempty"`
	WatchState    string   `json:"watchState,omitempty"`
	Sort          string   `json:"sort,omitempty"`
}

// AssistantNavigation names a screen; Name is the entity's name for the kinds that take one.
type AssistantNavigation struct {
	To           string `json:"to"`
	Name         string `json:"name,omitempty"`
	SettingsPage string `json:"settingsPage,omitempty"`
}

// AssistantEntityName is a name with the kind the model inferred, or "any".
type AssistantEntityName struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// AssistantQuestion is one of the typed questions the server answers from its own reads.
type AssistantQuestion struct {
	Question string               `json:"question"`
	Filter   *AssistantFilter     `json:"filter,omitempty"`
	Entity   *AssistantEntityName `json:"entity,omitempty"`
	Limit    int                  `json:"limit,omitempty"`
}

// AssistantTargetRequest is what an action runs on, in the model's words.
type AssistantTargetRequest struct {
	Kind   string           `json:"kind"`
	Names  []string         `json:"names,omitempty"`
	Filter *AssistantFilter `json:"filter,omitempty"`
}

// AssistantArguments holds every argument a catalog row may read; a row reads only those it lists.
type AssistantArguments struct {
	Name              string   `json:"name,omitempty"`
	NewName           *string  `json:"newName,omitempty"`
	Names             []string `json:"names,omitempty"`
	TagNames          []string `json:"tagNames,omitempty"`
	TagKindName       string   `json:"tagKindName,omitempty"`
	PersonNames       []string `json:"personNames,omitempty"`
	OrganizationNames []string `json:"organizationNames,omitempty"`
	Role              string   `json:"role,omitempty"`
	Synonym           string   `json:"synonym,omitempty"`
	Aliases           []string `json:"aliases,omitempty"`
	Code              string   `json:"code,omitempty"`
	URL               string   `json:"url,omitempty"`
	DateKind          string   `json:"dateKind,omitempty"`
	Date              string   `json:"date,omitempty"`
	Enabled           *bool    `json:"enabled,omitempty"`
	Number            *float64 `json:"number,omitempty"`
	Public            *bool    `json:"public,omitempty"`
	Model             string   `json:"model,omitempty"`
	Encoder           string   `json:"encoder,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	Sort              string   `json:"sort,omitempty"`
	ParentName        string   `json:"parentName,omitempty"`
	Scope             string   `json:"scope,omitempty"`
}

// AssistantActionRequest is one catalog operation the owner confirms before it runs.
type AssistantActionRequest struct {
	Operation string                  `json:"operation"`
	Target    *AssistantTargetRequest `json:"target,omitempty"`
	Arguments *AssistantArguments     `json:"arguments,omitempty"`
}

// AssistantOrganizeRequest starts a review run of one kind.
type AssistantOrganizeRequest struct {
	Kind string `json:"kind"`
}

// AssistantClarifyRequest says which part of the request is missing.
type AssistantClarifyRequest struct {
	About string `json:"about"`
}

// AssistantOutcome is the model's one answer to a turn. No member is text the owner reads.
type AssistantOutcome struct {
	Kind     AssistantOutcomeKind      `json:"kind"`
	Filter   *AssistantFilter          `json:"filter,omitempty"`
	Navigate *AssistantNavigation      `json:"navigate,omitempty"`
	Answer   *AssistantQuestion        `json:"answer,omitempty"`
	Action   *AssistantActionRequest   `json:"action,omitempty"`
	Organize *AssistantOrganizeRequest `json:"organize,omitempty"`
	Clarify  *AssistantClarifyRequest  `json:"clarify,omitempty"`
}

// Navigation destinations, questions, target kinds and the other enums of the outcome schema.
var (
	AssistantNavigationKinds = []string{
		"video", "person", "organization", "playlist", "group", "smartFolder", "tag", "library", "folders", "tags",
		"people", "organizations", "playlists", "history", "duplicates", "activity", "settings", "resume",
	}
	AssistantSettingsPages = []string{
		"scan", "library", "auto-tagging", "face-recognition", "connections", "plugins", "conversion", "network",
		"account", "api", "danger-zone",
	}
	AssistantQuestions   = []string{"count", "appearances", "recent", "list"}
	AssistantEntityKinds = []string{"person", "organization", "tag", "any"}
	AssistantTargetKinds = []string{"selection", "current", "filter", "named"}
	AssistantReviewKinds = []string{"tags", "people", "works"}
	AssistantAbouts      = []string{"operation", "target", "value"}
	AssistantScopes      = []string{"all", "untagged"}
)

// AssistantAnswerLimit bounds the titles a recent or list question answers.
const (
	AssistantAnswerLimit        = 20
	AssistantAnswerDefaultLimit = 5
)

var assistantVideoSorts = []string{
	string(SortAddedAsc), string(SortAddedDesc), string(SortModifiedAsc), string(SortModifiedDesc),
	string(SortCreatedAsc), string(SortCreatedDesc), string(SortTitleAsc), string(SortTitleDesc),
	string(SortDurationAsc), string(SortDurationDesc), string(SortSizeAsc), string(SortSizeDesc),
	string(SortPlayedAsc), string(SortPlayedDesc), string(SortFavoritedAsc), string(SortFavoritedDesc),
	string(SortRandom),
}

func stringsOf[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

type outcomeSchema struct {
	raw      json.RawMessage
	resolved *jsonschema.Resolved
}

var assistantOutcomeSchema = sync.OnceValues(buildAssistantOutcomeSchema)

// AssistantOutcomeSchema returns the JSON schema the model's answer must satisfy, built once from the outcome types
// and the catalog.
func AssistantOutcomeSchema() (json.RawMessage, error) {
	built, err := assistantOutcomeSchema()
	return built.raw, err
}

func buildAssistantOutcomeSchema() (outcomeSchema, error) {
	schema, err := jsonschema.For[AssistantOutcome](nil)
	if err != nil {
		return outcomeSchema{}, fmt.Errorf("cannot build the assistant outcome schema: %w", err)
	}
	dropNullTypes(schema)
	props := schema.Properties
	setEnum(props["kind"], stringsOf([]AssistantOutcomeKind{
		AssistantOutcomeFilter, AssistantOutcomeNavigate, AssistantOutcomeAnswer, AssistantOutcomeAction,
		AssistantOutcomeOrganize, AssistantOutcomeClarify, AssistantOutcomeUnsupported,
	}))
	filterEnums(props["filter"])
	nav := props["navigate"].Properties
	setEnum(nav["to"], AssistantNavigationKinds)
	setEnum(nav["settingsPage"], AssistantSettingsPages)
	answer := props["answer"].Properties
	setEnum(answer["question"], AssistantQuestions)
	filterEnums(answer["filter"])
	setEnum(answer["entity"].Properties["kind"], AssistantEntityKinds)
	answer["limit"].Minimum, answer["limit"].Maximum = jsonschema.Ptr(1.0), jsonschema.Ptr(float64(AssistantAnswerLimit))
	action := props["action"].Properties
	ids := make([]string, len(assistantCatalog))
	for i, row := range assistantCatalog {
		ids[i] = row.ID
	}
	setEnum(action["operation"], ids)
	setEnum(action["target"].Properties["kind"], AssistantTargetKinds)
	filterEnums(action["target"].Properties["filter"])
	args := action["arguments"].Properties
	setEnum(args["role"], stringsOf(PersonRoles))
	setEnum(args["dateKind"], stringsOf(DateKinds))
	setEnum(args["sort"], assistantVideoSorts)
	setEnum(args["scope"], AssistantScopes)
	setEnum(props["organize"].Properties["kind"], AssistantReviewKinds)
	setEnum(props["clarify"].Properties["about"], AssistantAbouts)
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return outcomeSchema{}, fmt.Errorf("cannot resolve the assistant outcome schema: %w", err)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return outcomeSchema{}, fmt.Errorf("cannot encode the assistant outcome schema: %w", err)
	}
	return outcomeSchema{raw: raw, resolved: resolved}, nil
}

func filterEnums(s *jsonschema.Schema) {
	setEnum(s.Properties["watchState"], stringsOf([]WatchState{WatchStateUnwatched, WatchStateInProgress, WatchStateWatched}))
	setEnum(s.Properties["sort"], assistantVideoSorts)
	for _, bound := range []string{"minDurationMs", "maxDurationMs"} {
		s.Properties[bound].Minimum = jsonschema.Ptr(0.0)
	}
}

func setEnum(s *jsonschema.Schema, values []string) {
	s.Enum = make([]any, len(values))
	for i, v := range values {
		s.Enum[i] = v
	}
}

// dropNullTypes makes optional members absent rather than null, which keeps the grammar Ollama builds simple.
func dropNullTypes(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if len(s.Types) == 2 && slices.Contains(s.Types, "null") {
		s.Type = s.Types[0]
		if s.Type == "null" {
			s.Type = s.Types[1]
		}
		s.Types = nil
	}
	for _, p := range s.Properties {
		dropNullTypes(p)
	}
	dropNullTypes(s.Items)
}

// ParseAssistantOutcome checks the model's answer against the schema and the member its kind reads; any mismatch,
// including a field the schema does not define, wraps ErrGenerationUnreadable.
func ParseAssistantOutcome(raw json.RawMessage) (AssistantOutcome, error) {
	built, err := assistantOutcomeSchema()
	if err != nil {
		return AssistantOutcome{}, err
	}
	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		return AssistantOutcome{}, fmt.Errorf("%w: %w", ErrGenerationUnreadable, err)
	}
	if err := built.resolved.Validate(instance); err != nil {
		return AssistantOutcome{}, fmt.Errorf("%w: %w", ErrGenerationUnreadable, err)
	}
	var outcome AssistantOutcome
	if err := json.Unmarshal(raw, &outcome); err != nil {
		return AssistantOutcome{}, fmt.Errorf("%w: %w", ErrGenerationUnreadable, err)
	}
	if !outcome.membersMatchKind() {
		return AssistantOutcome{}, fmt.Errorf("%w: the %s outcome does not carry its own member alone",
			ErrGenerationUnreadable, outcome.Kind)
	}
	return outcome, nil
}

func (o AssistantOutcome) membersMatchKind() bool {
	present := map[AssistantOutcomeKind]bool{
		AssistantOutcomeFilter:   o.Filter != nil,
		AssistantOutcomeNavigate: o.Navigate != nil,
		AssistantOutcomeAnswer:   o.Answer != nil,
		AssistantOutcomeAction:   o.Action != nil,
		AssistantOutcomeOrganize: o.Organize != nil,
		AssistantOutcomeClarify:  o.Clarify != nil,
	}
	for kind, has := range present {
		if has != (kind == o.Kind) {
			return false
		}
	}
	return true
}
