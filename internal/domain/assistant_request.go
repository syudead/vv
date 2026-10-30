package domain

// The requests below are the filter, navigation, write and review arguments the assistant's tools resolve
// against the library.

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
