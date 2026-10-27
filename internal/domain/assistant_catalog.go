package domain

import (
	"errors"
	"slices"
	"strings"
)

// AssistantTarget is what a catalog row acts on; the zero value acts on the arguments alone.
type AssistantTarget string

const (
	AssistantTargetNone           AssistantTarget = ""
	AssistantTargetVideos         AssistantTarget = "videos"
	AssistantTargetVideosOrGroups AssistantTarget = "videosOrGroups"
	AssistantTargetVideo          AssistantTarget = "video"
	AssistantTargetVideoPair      AssistantTarget = "videoPair"
	AssistantTargetVirtualVideo   AssistantTarget = "virtualVideo"
	AssistantTargetTag            AssistantTarget = "tag"
	// AssistantTargetTags names tags or selects them by a tag filter (tentative, unused, a kind).
	AssistantTargetTags         AssistantTarget = "tags"
	AssistantTargetTagKind      AssistantTarget = "tagKind"
	AssistantTargetPerson       AssistantTarget = "person"
	AssistantTargetOrganization AssistantTarget = "organization"
	AssistantTargetPlaylist     AssistantTarget = "playlist"
	AssistantTargetGroup        AssistantTarget = "group"
	AssistantTargetSmartFolder  AssistantTarget = "smartFolder"
	// AssistantTargetCollections names groups and smart folders together.
	AssistantTargetCollections AssistantTarget = "collections"
	AssistantTargetFolderRoot  AssistantTarget = "folderRoot"
)

// AssistantArgument is a field of the action outcome's arguments object.
type AssistantArgument string

const (
	AssistantArgName              AssistantArgument = "name"
	AssistantArgNewName           AssistantArgument = "newName"
	AssistantArgNames             AssistantArgument = "names"
	AssistantArgTagNames          AssistantArgument = "tagNames"
	AssistantArgTagKindName       AssistantArgument = "tagKindName"
	AssistantArgPersonNames       AssistantArgument = "personNames"
	AssistantArgOrganizationNames AssistantArgument = "organizationNames"
	AssistantArgRole              AssistantArgument = "role"
	AssistantArgSynonym           AssistantArgument = "synonym"
	AssistantArgAliases           AssistantArgument = "aliases"
	AssistantArgCode              AssistantArgument = "code"
	AssistantArgURL               AssistantArgument = "url"
	AssistantArgDateKind          AssistantArgument = "dateKind"
	AssistantArgDate              AssistantArgument = "date"
	AssistantArgEnabled           AssistantArgument = "enabled"
	AssistantArgNumber            AssistantArgument = "number"
	AssistantArgPublic            AssistantArgument = "public"
	AssistantArgModel             AssistantArgument = "model"
	AssistantArgEncoder           AssistantArgument = "encoder"
	AssistantArgProvider          AssistantArgument = "provider"
	AssistantArgSort              AssistantArgument = "sort"
	AssistantArgParentName        AssistantArgument = "parentName"
	AssistantArgScope             AssistantArgument = "scope"
)

// AssistantAction is one catalog row: the only writes the assistant may propose and run.
type AssistantAction struct {
	// ID is the operationId, or operationId.action when the request picks one of several actions.
	ID        string
	Operation string
	// Action is the variant the runner sends; empty for an operation with one action.
	Action    string
	Target    AssistantTarget
	Arguments []AssistantArgument
	// Batches splits video targets into batches of AssistantBatchSize user keys.
	Batches bool
	// Irreversible marks the confirmation card; the model never decides it.
	Irreversible bool
	// ReadsCurrent sends the current settings back with only the argument's field changed.
	ReadsCurrent bool
	// RuleFromFilter takes a smart folder's rule from the outcome's target filter.
	RuleFromFilter bool
}

// AssistantBatchSize is the number of video user keys one operation call receives.
const AssistantBatchSize = 500

// ErrAssistantActionUnknown refuses an operation that is not a catalog row.
var ErrAssistantActionUnknown = errors.New("assistant action is not in the catalog")

func assistantRow(
	id string, target AssistantTarget, batches, irreversible bool, args ...AssistantArgument,
) AssistantAction {
	operation, action, _ := strings.Cut(id, ".")
	return AssistantAction{
		ID: id, Operation: operation, Action: action, Target: target,
		Arguments: args, Batches: batches, Irreversible: irreversible,
	}
}

func settingsRow(id string, args ...AssistantArgument) AssistantAction {
	row := assistantRow(id, AssistantTargetNone, false, false, args...)
	row.ReadsCurrent = true
	return row
}

func smartFolderRow(id string, target AssistantTarget, args ...AssistantArgument) AssistantAction {
	row := assistantRow(id, target, false, false, args...)
	row.RuleFromFilter = true
	return row
}

const (
	viaBatches, oneCall = true, false
	cannotUndo, canUndo = true, false
)

var assistantCatalog = []AssistantAction{
	// Videos
	assistantRow("updateVideoTags.add", AssistantTargetVideos, viaBatches, canUndo, AssistantArgTagNames),
	assistantRow("updateVideoTags.remove", AssistantTargetVideos, viaBatches, canUndo, AssistantArgTagNames),
	assistantRow("applyVideoEdits.add", AssistantTargetVideos, viaBatches, canUndo,
		AssistantArgTagNames, AssistantArgPersonNames, AssistantArgRole, AssistantArgOrganizationNames,
		AssistantArgDateKind, AssistantArgDate, AssistantArgPublic),
	assistantRow("applyVideoEdits.remove", AssistantTargetVideos, viaBatches, canUndo,
		AssistantArgTagNames, AssistantArgPersonNames, AssistantArgRole, AssistantArgOrganizationNames),
	assistantRow("applyVideoCodeEdits", AssistantTargetVideos, viaBatches, canUndo, AssistantArgCode),
	assistantRow("updateVideoVisibility", AssistantTargetVideos, viaBatches, canUndo, AssistantArgPublic),
	assistantRow("updateFavorites", AssistantTargetVideosOrGroups, viaBatches, canUndo, AssistantArgEnabled),
	assistantRow("setVideoDisplayName", AssistantTargetVideo, oneCall, canUndo, AssistantArgNewName),
	assistantRow("setVideoWorkInfo", AssistantTargetVideo, oneCall, canUndo,
		AssistantArgCode, AssistantArgPersonNames, AssistantArgRole, AssistantArgOrganizationNames,
		AssistantArgDateKind, AssistantArgDate, AssistantArgURL),
	assistantRow("setVideoThumbnailPosition", AssistantTargetVideo, oneCall, canUndo, AssistantArgNumber),
	assistantRow("reprobeVideo", AssistantTargetVideo, oneCall, canUndo),
	assistantRow("autoTagVideo", AssistantTargetVideo, oneCall, canUndo),
	assistantRow("confirmVideoTag", AssistantTargetVideo, oneCall, canUndo, AssistantArgTagNames),
	assistantRow("rejectVideoTag", AssistantTargetVideo, oneCall, cannotUndo, AssistantArgTagNames),
	assistantRow("bundleVideos", AssistantTargetVideos, oneCall, canUndo),
	assistantRow("makeRepresentativeVersion", AssistantTargetVideo, oneCall, canUndo),
	assistantRow("unbundleVideo", AssistantTargetVideo, oneCall, canUndo),
	assistantRow("dismissVersionCandidate", AssistantTargetVideoPair, oneCall, canUndo),
	assistantRow("rejudgeVersionCandidate", AssistantTargetVideoPair, oneCall, canUndo),
	assistantRow("createVirtualVideo", AssistantTargetNone, oneCall, canUndo, AssistantArgName, AssistantArgURL),
	assistantRow("mergeVirtualVideo", AssistantTargetVirtualVideo, oneCall, cannotUndo, AssistantArgNames),

	// Tags and tag kinds
	assistantRow("createTag", AssistantTargetNone, oneCall, canUndo, AssistantArgName, AssistantArgTagKindName),
	assistantRow("renameTag", AssistantTargetTag, oneCall, canUndo, AssistantArgNewName),
	assistantRow("deleteTag", AssistantTargetTag, oneCall, cannotUndo),
	assistantRow("mergeTag", AssistantTargetTag, oneCall, cannotUndo, AssistantArgNames),
	assistantRow("addTagSynonym", AssistantTargetTag, oneCall, canUndo, AssistantArgSynonym),
	assistantRow("removeTagSynonym", AssistantTargetTag, oneCall, canUndo, AssistantArgSynonym),
	assistantRow("confirmTag", AssistantTargetTag, oneCall, canUndo),
	assistantRow("rejectTag", AssistantTargetTag, oneCall, cannotUndo),
	assistantRow("forgetRejectedTagName", AssistantTargetNone, oneCall, canUndo, AssistantArgName),
	assistantRow("batchTags.confirm", AssistantTargetTags, oneCall, canUndo),
	assistantRow("batchTags.reject", AssistantTargetTags, oneCall, cannotUndo),
	assistantRow("batchTags.delete", AssistantTargetTags, oneCall, cannotUndo),
	assistantRow("batchTags.setKind", AssistantTargetTags, oneCall, canUndo, AssistantArgTagKindName),
	assistantRow("batchTags.clearKind", AssistantTargetTags, oneCall, cannotUndo),
	assistantRow("createTagKind", AssistantTargetNone, oneCall, canUndo, AssistantArgName),
	assistantRow("renameTagKind", AssistantTargetTagKind, oneCall, canUndo, AssistantArgNewName),
	assistantRow("deleteTagKind", AssistantTargetTagKind, oneCall, cannotUndo),
	assistantRow("setTagKindCardFirst", AssistantTargetTagKind, oneCall, canUndo, AssistantArgEnabled),

	// People and organizations
	assistantRow("createPerson", AssistantTargetNone, oneCall, canUndo, AssistantArgName, AssistantArgAliases),
	assistantRow("updatePerson", AssistantTargetPerson, oneCall, canUndo, AssistantArgNewName, AssistantArgAliases),
	assistantRow("deletePerson", AssistantTargetPerson, oneCall, cannotUndo),
	assistantRow("mergePeople", AssistantTargetPerson, oneCall, cannotUndo, AssistantArgNames),
	assistantRow("deletePersonImage", AssistantTargetPerson, oneCall, cannotUndo),
	assistantRow("confirmFaceCandidate", AssistantTargetPerson, oneCall, cannotUndo, AssistantArgNames),
	assistantRow("dismissFaceCandidate", AssistantTargetPerson, oneCall, canUndo, AssistantArgNames),
	assistantRow("reopenFaceCandidate", AssistantTargetPerson, oneCall, canUndo, AssistantArgNames),
	assistantRow("createOrganization", AssistantTargetNone, oneCall, canUndo,
		AssistantArgName, AssistantArgAliases, AssistantArgParentName),
	assistantRow("updateOrganization", AssistantTargetOrganization, oneCall, canUndo,
		AssistantArgNewName, AssistantArgAliases, AssistantArgParentName),
	assistantRow("deleteOrganization", AssistantTargetOrganization, oneCall, cannotUndo),
	assistantRow("mergeOrganizations", AssistantTargetOrganization, oneCall, cannotUndo, AssistantArgNames),

	// Playlists, groups and smart folders
	assistantRow("createPlaylist", AssistantTargetNone, oneCall, canUndo, AssistantArgName),
	assistantRow("renamePlaylist", AssistantTargetPlaylist, oneCall, canUndo, AssistantArgNewName),
	assistantRow("deletePlaylist", AssistantTargetPlaylist, oneCall, cannotUndo),
	assistantRow("addPlaylistItems", AssistantTargetVideos, viaBatches, canUndo, AssistantArgName),
	assistantRow("removePlaylistItem", AssistantTargetVideos, viaBatches, canUndo, AssistantArgName),
	assistantRow("movePlaylistItem", AssistantTargetVideo, oneCall, canUndo, AssistantArgName, AssistantArgNumber),
	assistantRow("createGroup", AssistantTargetVideos, oneCall, canUndo, AssistantArgName),
	assistantRow("renameGroup", AssistantTargetGroup, oneCall, canUndo, AssistantArgNewName),
	assistantRow("dissolveGroup", AssistantTargetGroup, oneCall, cannotUndo),
	assistantRow("addGroupItems", AssistantTargetVideos, viaBatches, canUndo, AssistantArgName),
	assistantRow("removeGroupItems", AssistantTargetVideos, viaBatches, canUndo, AssistantArgName),
	assistantRow("moveGroupItem", AssistantTargetVideo, oneCall, canUndo, AssistantArgName, AssistantArgNumber),
	assistantRow("tagGroup", AssistantTargetGroup, oneCall, cannotUndo),
	assistantRow("deleteCollections", AssistantTargetCollections, oneCall, cannotUndo),
	assistantRow("setFolderGrouping", AssistantTargetFolderRoot, oneCall, canUndo, AssistantArgEnabled),
	smartFolderRow("createSmartFolder", AssistantTargetNone, AssistantArgName, AssistantArgSort, AssistantArgParentName),
	smartFolderRow("changeSmartFolder", AssistantTargetSmartFolder,
		AssistantArgNewName, AssistantArgSort, AssistantArgParentName),
	assistantRow("deleteSmartFolder", AssistantTargetSmartFolder, oneCall, cannotUndo),

	// Settings and scans
	settingsRow("updateAutoTaggingSettings", AssistantArgName, AssistantArgEnabled, AssistantArgNumber),
	assistantRow("startAutoTagging", AssistantTargetNone, oneCall, canUndo, AssistantArgScope),
	settingsRow("updateVersionJudgingSettings", AssistantArgEnabled),
	settingsRow("updateFaceRecognitionSettings", AssistantArgName, AssistantArgEnabled, AssistantArgNumber),
	assistantRow("startFaceRecognition", AssistantTargetNone, oneCall, canUndo, AssistantArgScope),
	settingsRow("updateAutoImportSettings", AssistantArgEnabled),
	settingsRow("updateTranscodingSettings", AssistantArgEncoder),
	settingsRow("updateOllamaSettings", AssistantArgName, AssistantArgURL, AssistantArgModel),
	settingsRow("updateComfyUISettings", AssistantArgProvider, AssistantArgURL, AssistantArgModel),
	assistantRow("startScan", AssistantTargetNone, oneCall, canUndo),
	assistantRow("startRescan", AssistantTargetNone, oneCall, canUndo, AssistantArgScope),
}

// AssistantExclusion is a non-GET screen operation the assistant never offers, with the reason group.
type AssistantExclusion struct {
	Operation string
	Group     string
	// ByIssue separates the parent Issue's exclusions from those the plan added.
	ByIssue bool
}

func excluded(group string, byIssue bool, operations ...string) []AssistantExclusion {
	out := make([]AssistantExclusion, 0, len(operations))
	for _, op := range operations {
		out = append(out, AssistantExclusion{Operation: op, Group: group, ByIssue: byIssue})
	}
	return out
}

var assistantExclusions = slices.Concat(
	excluded("Account, authentication and API tokens", true,
		"setupAccount", "login", "logout", "updateAccount", "createApiToken", "deleteApiToken"),
	excluded("Plugin upload, approval, settings and order", true,
		"uploadWorkInfoPlugin", "deleteWorkInfoPlugin", "approveWorkInfoPlugin", "revokeWorkInfoPluginApproval",
		"setWorkInfoPluginSettings", "setWorkInfoPluginLinkOrder"),
	excluded("Network and proxy settings", true, "updateNetworkSettings", "updatePluginProxySettings"),
	excluded("Media folders", true, "createMediaFolder", "updateMediaFolder", "deleteMediaFolder"),
	excluded("Watch history deletion", true, "clearWatchHistory", "deleteWatchHistoryEntry"),
	excluded("Face runtime download", true, "downloadFaceRuntime"),
	excluded("Onboarding", true, "updateOnboarding", "pickOnboardingInterests"),

	excluded("Deleting every person", false, "clearPeople"),
	excluded("Deleting a video", false, "deleteVideo"),
	excluded("Images the page sends", false, "uploadThumbnailImage", "revertThumbnailImage", "uploadPersonImage"),
	excluded("Thumbnail edit sessions", false,
		"startThumbnailEdit", "startThumbnailEditFromImage", "discardThumbnailEdit", "runThumbnailEdit",
		"cancelThumbnailEdit", "applyThumbnailEdit"),
	excluded("Plugin fetches", false,
		"searchWorkInfo", "fetchWorkInfo", "applyWorkInfoFetch", "searchPeopleWithPlugin", "fetchPersonWithPlugin",
		"applyPersonPluginFetch"),
	excluded("Face appearances", false, "moveFaceAppearances", "splitFaceAppearances", "splitFaceAppearance"),
	excluded("Drag operations", false,
		"organizeReorder", "organizeGroup", "organizeMove", "organizeMerge", "organizeBundle", "organizeUndo"),
	excluded("Activity log actions", false, "retryActivity", "undoActivity"),
	excluded("Playback and the server PC", false, "putVideoProgress", "openVideoFile"),
	excluded("Two operations in one", false, "setVideoDetails"),
	excluded("Reads and tests sent as POST", false,
		"summarizeVideoTags", "summarizeVideoEdits", "summarizeVideoCodeEdits", "tagImpact", "checkOllama",
		"checkComfyUI", "checkAutoTaggingClassifier", "listWorkInfoFillTargets"),
	// Pairing is a trust decision made with the other node's code and certificate fingerprint on its own page,
	// and applyPeerVocabulary is the paired node's call with its credential, never the screens'.
	excluded("Pairing with another node and the peer's own calls", false,
		"issuePairingCode", "pairPeer", "updatePeerAddress", "confirmPeerCertificate", "unpairPeer", "forgetPeer",
		"applyPeerVocabulary"),
)

// AssistantExcludedPathPrefix holds the assistant's own operations, which it never calls.
const AssistantExcludedPathPrefix = "/api/assistant/"

// AssistantCatalog returns the rows in contract order; callers may not change the shared rows.
func AssistantCatalog() []AssistantAction { return slices.Clone(assistantCatalog) }

// AssistantExclusions returns the operations the catalog leaves out on purpose.
func AssistantExclusions() []AssistantExclusion { return slices.Clone(assistantExclusions) }

// LookupAssistantAction finds a row by its id, such as "batchTags.reject".
func LookupAssistantAction(id string) (AssistantAction, bool) {
	i := slices.IndexFunc(assistantCatalog, func(row AssistantAction) bool { return row.ID == id })
	if i < 0 {
		return AssistantAction{}, false
	}
	return assistantCatalog[i], true
}

// AssistantCall is one operation call of a catalog row, with the request the runner sends.
type AssistantCall struct {
	// ActionID is the catalog row id; the runner refuses any other value before calling.
	ActionID string
	// Path fills the operation's path parameters by name, such as "id" or "tagId".
	Path  map[string]string
	Query map[string]string
	// Body holds the request fields; the runner adds the row's action and, for settings, the current values.
	Body map[string]any
}

// AssistantCallResult is the successful response of an operation call.
type AssistantCallResult struct {
	Status int
	Body   []byte
}

// AssistantCallError is a failed operation call, carrying the screen API's error code and reason.
type AssistantCallError struct {
	Status  int
	Code    string
	Reason  string
	Message string
}

func (e *AssistantCallError) Error() string {
	if e.Reason != "" {
		return "assistant action failed: " + e.Code + " (" + e.Reason + "): " + e.Message
	}
	return "assistant action failed: " + e.Code + ": " + e.Message
}
