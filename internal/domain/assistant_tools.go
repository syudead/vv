package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// AssistantToolKind is how the assistant treats one screen API operation.
type AssistantToolKind string

const (
	// AssistantToolRead runs at once and returns its result to the model.
	AssistantToolRead AssistantToolKind = "read"
	// AssistantToolWrite becomes a proposal step that runs after the owner approves it.
	AssistantToolWrite AssistantToolKind = "write"
	// AssistantToolExcluded has no tool; the assistant answers its reason and page instead.
	AssistantToolExcluded AssistantToolKind = "excluded"
)

// AssistantPage is one page of the app, which help answers and exclusions point to.
type AssistantPage struct {
	Route   string
	Title   string
	Purpose string
}

const (
	pageLibrary          = "/"
	pageHistory          = "/history"
	pageVideo            = "/videos/:id"
	pagePerson           = "/people/:id"
	pageOnboarding       = "/welcome"
	pageSettingsLibrary  = "/settings/library"
	pageSettingsFaces    = "/settings/face-recognition"
	pageSettingsPlugins  = "/settings/plugins"
	pageSettingsNetwork  = "/settings/network"
	pageSettingsPairing  = "/settings/pairing"
	pageSettingsAccount  = "/settings/account"
	pageSettingsAPI      = "/settings/api"
	pageSettingsDangers  = "/settings/danger-zone"
	pageSettingsServices = "/settings/connections"
)

var assistantPages = []AssistantPage{
	{pageLibrary, "Library", "Browse and search every video. Filter narrows it by watch status, Favorites only, tags, " +
		"people, organizations, length and the date added, and Sort by orders it (Date favorited among others); the " +
		"filter can be saved as a smart folder. Select videos to add or remove tags, Add to favorites, add them to a " +
		"playlist or make a New group"},
	{"/folders", "Folders", "Browse videos by the folders of the media folders"},
	{pageVideo, "Video", "Play a video, Add to favorites, and edit its tags, people, organizations, work " +
		"information and thumbnail"},
	{"/videos/:id/edit", "Video details editor", "Edit a video's display name and work information at once"},
	{"/tags", "Tags", "Create, rename, merge, confirm or reject tags, and manage tag kinds and synonyms"},
	{"/people", "People", "List people, with their face recognition candidates"},
	{pagePerson, "Person", "A person's videos, image, aliases and profile"},
	{"/people/:id/faces", "Person faces", "Confirm, move or split the face appearances recognised as a person"},
	{"/people/:id/edit", "Person editor", "Edit a person's name, aliases, profile and image"},
	{"/organizations", "Organizations", "List, create, merge and edit organizations"},
	{pageHistory, "History", "The videos watched recently, with their positions"},
	{"/playlists", "Playlists", "List and create playlists"},
	{"/playlists/:id", "Playlist", "Play a playlist and reorder its videos"},
	{"/groups/:id", "Group", "The videos of a group, in their order"},
	{"/smart-folders/:id", "Smart folder", "The videos matching a saved filter"},
	{"/duplicates", "Duplicates", "Review version and duplicate candidates"},
	{"/activity", "Activity", "The log of background work and changes, with undo and retry"},
	{"/activity/:id", "Activity entry", "One entry of the activity log and what it changed"},
	{"/assistant/reviews/:id", "Assistant review", "Adopt or reject the candidates a review run judged"},
	{pageOnboarding, "Welcome", "First-run setup of the library"},
	{"/settings/scan", "Settings: Scan", "Import progress and starting a scan"},
	{pageSettingsLibrary, "Settings: Library", "Media folders, automatic import and version judging"},
	{"/settings/auto-tagging", "Settings: Auto tagging", "Automatic tagging with the classifier"},
	{pageSettingsFaces, "Settings: Face recognition", "Face recognition and its runtime"},
	{pageSettingsServices, "Settings: Connections", "The Ollama and ComfyUI connections"},
	{pageSettingsPlugins, "Settings: Plugins", "Upload, approve and configure work information plugins"},
	{"/settings/conversion", "Settings: Conversion", "Video transcoding"},
	{pageSettingsNetwork, "Settings: Network", "Network access and the plugin proxy"},
	{pageSettingsPairing, "Settings: Pairing", "Pairing with another VVMDM node"},
	{pageSettingsAccount, "Settings: Account", "The owner's account and sign-in"},
	{pageSettingsAPI, "Settings: API", "API tokens for external clients"},
	{pageSettingsDangers, "Settings: Danger zone", "Clearing every person"},
}

// AssistantPages returns the app's pages in navigation order.
func AssistantPages() []AssistantPage { return slices.Clone(assistantPages) }

// AssistantPageTitle answers the title of the page at the route, or the route itself for a route the table lacks.
func AssistantPageTitle(route string) string {
	if i := slices.IndexFunc(assistantPages, func(p AssistantPage) bool { return p.Route == route }); i >= 0 {
		return assistantPages[i].Title
	}
	return route
}

// AssistantExclusion is a screen operation the assistant never offers, with why and the page to use instead.
type AssistantExclusion struct {
	Operation string
	Group     string
	Reason    string
	// Page is the route of an AssistantPages entry; empty for the assistant's own operations.
	Page string
	// ByIssue separates the parent Issue's exclusions from those the plan added.
	ByIssue bool
}

const outOfScope = "The assistant leaves it to its page, as the feature's scope says"

func excluded(group, reason, page string, byIssue bool, operations ...string) []AssistantExclusion {
	out := make([]AssistantExclusion, 0, len(operations))
	for _, op := range operations {
		out = append(out, AssistantExclusion{Operation: op, Group: group, Reason: reason, Page: page, ByIssue: byIssue})
	}
	return out
}

var assistantExclusions = slices.Concat(
	excluded("Account and authentication", outOfScope, pageSettingsAccount, true,
		"setupAccount", "login", "logout", "updateAccount", "getAuthSession", "getAccount"),
	excluded("API tokens", outOfScope, pageSettingsAPI, true, "createApiToken", "deleteApiToken", "listApiTokens"),
	excluded("Plugin upload, approval, settings and order", outOfScope, pageSettingsPlugins, true,
		"uploadWorkInfoPlugin", "deleteWorkInfoPlugin", "approveWorkInfoPlugin", "revokeWorkInfoPluginApproval",
		"setWorkInfoPluginSettings", "setWorkInfoPluginLinkOrder"),
	excluded("Network and proxy settings", outOfScope, pageSettingsNetwork, true,
		"updateNetworkSettings", "updatePluginProxySettings"),
	excluded("Credentials in the response",
		"Its url may hold the proxy's user and password, which would reach the model", pageSettingsNetwork, false,
		"getPluginProxySettings"),
	excluded("Media folders", outOfScope, pageSettingsLibrary, true,
		"createMediaFolder", "updateMediaFolder", "deleteMediaFolder"),
	excluded("Watch history deletion", outOfScope, pageHistory, true, "clearWatchHistory", "deleteWatchHistoryEntry"),
	excluded("Face runtime download", outOfScope, pageSettingsFaces, true, "downloadFaceRuntime"),
	excluded("Onboarding", outOfScope, pageOnboarding, true, "updateOnboarding", "pickOnboardingInterests"),
	// Pairing is a trust decision made with the other node's code and certificate fingerprint on its own page,
	// and the fetch, match and vocabulary calls are the paired node's, with its credential, never the screens'.
	excluded("Pairing and the peer's own calls",
		"Pairing is a trust decision made with the other node's code, and the peer's calls carry its credential",
		pageSettingsPairing, true,
		"issuePairingCode", "pairPeer", "updatePeerAddress", "confirmPeerCertificate", "unpairPeer", "forgetPeer",
		"revokePeerGrant", "applyPeerVocabulary", "matchPeerCredits", "applyPeerFetchedWorkInfo",
		"applyPeerFetchedPerson", "peerHandshake", "getPeerRelatedClues", "rankPeerRelated", "countPeerRuleMatches"),
	excluded("Irreversible deletion", outOfScope, pageVideo, true, "deleteVideo"),
	excluded("Irreversible deletion", outOfScope, pageSettingsDangers, true, "clearPeople"),
	excluded("Files the browser sends",
		"The server holds no such file; the fetch-apply tools cover plugin images", pageVideo, false,
		"uploadThumbnailImage", "startThumbnailEditFromImage"),
	excluded("Files the browser sends",
		"The server holds no such file; the fetch-apply tools cover plugin images", pagePerson, false,
		"uploadPersonImage"),
	excluded("Drag operations",
		"They take drag tokens; the group, bundle and playlist tools make the same changes", pageLibrary, false,
		"organizeReorder", "organizeGroup", "organizeMove", "organizeMerge", "organizeBundle", "organizeUndo"),
	excluded("The server PC and streams",
		"They act on the server PC only, or answer a media stream rather than data", pageVideo, false,
		"openVideoFile", "streamVideo", "transcodeVideo", "getTranscodeStart", "getVideoSubtitle", "getVideoPreview",
		"streamEvents", "getHealth"),
)

// AssistantExcludedPathPrefix holds the assistant's own operations, which it never calls.
const AssistantExcludedPathPrefix = "/api/assistant/"

// assistantSelf answers the assistant's own operations.
var assistantSelf = AssistantExclusion{Group: "The assistant itself", Reason: "The assistant does not call itself"}

// assistantReadPosts write nothing although they are sent as POST.
var assistantReadPosts = []string{
	"tagImpact", "summarizeVideoTags", "summarizeVideoEdits", "summarizeVideoCodeEdits", "listWorkInfoFillTargets",
	"searchWorkInfo", "fetchWorkInfo", "searchPeopleWithPlugin", "fetchPersonWithPlugin", "checkOllama",
	"checkComfyUI", "checkAutoTaggingClassifier",
}

// AssistantCatalog returns the write rows in contract order; callers may not change the shared rows.
func AssistantCatalog() []AssistantAction { return slices.Clone(assistantCatalog) }

// AssistantModelToolName is the name a model receives for a tool. Providers take only function names matching
// ^[a-zA-Z0-9_-]{1,64}$, so a variant row's "." is written "__", which no tool name contains.
func AssistantModelToolName(name string) string { return strings.ReplaceAll(name, ".", "__") }

// AssistantToolNameFromModel turns a name a model called back into the tool's name.
func AssistantToolNameFromModel(name string) string { return strings.ReplaceAll(name, "__", ".") }

// AssistantExclusions returns the operations the assistant leaves out on purpose.
func AssistantExclusions() []AssistantExclusion { return slices.Clone(assistantExclusions) }

// Operations lists the operations a row calls: its own, or a composite row's stages and cleanup.
func (a AssistantAction) Operations() []string {
	if a.Operation != "" {
		return []string{a.Operation}
	}
	return slices.Concat(a.Stages, a.Cleanup)
}

var (
	// ErrAssistantUnclassified is an owner operation in no list; a GET is a read without being listed.
	ErrAssistantUnclassified = errors.New("assistant operation is not classified")
	// ErrAssistantClassifiedTwice is an operation in more than one list, or a GET listed as other than excluded.
	ErrAssistantClassifiedTwice = errors.New("assistant operation is classified more than once")
)

// AssistantClassification is the decision for one operation; Row and Exclusion are set for their kinds.
type AssistantClassification struct {
	Kind      AssistantToolKind
	Row       *AssistantAction
	Exclusion *AssistantExclusion
}

// ClassifyAssistantOperation decides the kind of one screen API operation: excluded when listed so or under the
// assistant's own path, a write when a catalog row calls it, and a read when it is a GET or a listed POST read.
func ClassifyAssistantOperation(operation, method, path string) (AssistantClassification, error) {
	var found []AssistantClassification
	if strings.HasPrefix(path, AssistantExcludedPathPrefix) {
		self := assistantSelf
		self.Operation = operation
		found = append(found, AssistantClassification{Kind: AssistantToolExcluded, Exclusion: &self})
	}
	for i := range assistantExclusions {
		if assistantExclusions[i].Operation == operation {
			found = append(found, AssistantClassification{Kind: AssistantToolExcluded, Exclusion: &assistantExclusions[i]})
		}
	}
	// Variants of one operation (updateVideoTags.add and .remove) list it once; the first row stands for them.
	if i := slices.IndexFunc(assistantCatalog, func(row AssistantAction) bool {
		return slices.Contains(row.Operations(), operation)
	}); i >= 0 {
		found = append(found, AssistantClassification{Kind: AssistantToolWrite, Row: &assistantCatalog[i]})
	}
	if slices.Contains(assistantReadPosts, operation) {
		found = append(found, AssistantClassification{Kind: AssistantToolRead})
	}
	get := strings.EqualFold(method, "GET")
	switch {
	case len(found) > 1 || (get && len(found) == 1 && found[0].Kind != AssistantToolExcluded):
		return AssistantClassification{}, fmt.Errorf("%w: %s", ErrAssistantClassifiedTwice, operation)
	case len(found) == 1:
		return found[0], nil
	case get:
		return AssistantClassification{Kind: AssistantToolRead}, nil
	}
	return AssistantClassification{}, fmt.Errorf("%w: %s", ErrAssistantUnclassified, operation)
}

// AssistantListedOperations returns every operation the lists name, so a check can find names the spec lacks.
func AssistantListedOperations() []string {
	var out []string
	for _, row := range assistantCatalog {
		out = append(out, row.Operations()...)
	}
	for _, exclusion := range assistantExclusions {
		out = append(out, exclusion.Operation)
	}
	return slices.Concat(out, assistantReadPosts)
}

// assistantEntityKinds are the kinds of id a confirmation card can label with a name.
var assistantEntityKinds = []string{
	"video", "tag", "tagKind", "person", "organization", "playlist", "group", "smartFolder", "activity",
	"faceAppearance",
}

// AssistantLabelsEntity reports whether a card can label an id of the kind.
func AssistantLabelsEntity(kind string) bool { return slices.Contains(assistantEntityKinds, kind) }

// AssistantIDField is an id the request of an operation carries, with the entity kind it names.
type AssistantIDField struct {
	// In is path or body.
	In string
	// Field is the path parameter, or a JSON pointer into the body where "*" stands for every array item.
	Field string
	Kind  string
}

// AssistantTool is one tool the assistant may be offered, built from an operation and its classification.
type AssistantTool struct {
	// Name is the operationId of a read or excluded operation, or a write row's id.
	Name      string
	Kind      AssistantToolKind
	Operation string
	Summary   string
	// Description is the operation's description, which the model reads.
	Description string
	// Parameters is the JSON Schema of a read's or schema row's arguments; a name-based row takes its own.
	Parameters json.RawMessage
	// Image marks a read whose result is an image reference.
	Image     bool
	IDs       []AssistantIDField
	Row       *AssistantAction
	Exclusion *AssistantExclusion
}

var (
	// ErrAssistantNotReadTool refuses a read of an operation that is not a read tool, before any call.
	ErrAssistantNotReadTool = errors.New("assistant operation is not a read tool")
	// ErrAssistantInvalidArguments refuses arguments that do not match the tool's schema, before any call.
	ErrAssistantInvalidArguments = errors.New("assistant tool arguments do not match the schema")
)

// AssistantExcludedError refuses an excluded operation and says why and where to go instead.
type AssistantExcludedError struct {
	Exclusion AssistantExclusion
}

func (e *AssistantExcludedError) Error() string {
	return fmt.Sprintf("%s is excluded from the assistant: %s", e.Exclusion.Operation, e.Exclusion.Reason)
}

func (e *AssistantExcludedError) Unwrap() error { return ErrAssistantNotReadTool }

// AssistantRead is one read tool call: the operation and the model's arguments.
type AssistantRead struct {
	Operation string
	Arguments json.RawMessage
}

// AssistantReadResult is what a read tool answers the model: the handler's JSON with URL credentials masked, or
// for an image operation, its reference.
type AssistantReadResult struct {
	Body json.RawMessage
}

// AssistantImage is the image an image reference names.
type AssistantImage struct {
	Bytes       []byte
	ContentType string
}
