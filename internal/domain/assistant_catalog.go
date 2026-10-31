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
	AssistantArgFetchID           AssistantArgument = "fetchId"
	AssistantArgFields            AssistantArgument = "fields"
	AssistantArgAtMs              AssistantArgument = "atMs"
	AssistantArgInstruction       AssistantArgument = "instruction"
)

// AssistantAction is one catalog row: the only writes the assistant may propose and run.
type AssistantAction struct {
	// ID is the operationId, or operationId.action when the request picks one of several actions.
	ID string
	// Operation is empty for a composite row, which runs Stages instead.
	Operation string
	// Schema rows take the operation's own path parameters and body, ids included, instead of a target and
	// named arguments.
	Schema bool
	// Stages are the operations a composite row runs in order; Cleanup are those it calls when a stage fails.
	Stages  []string
	Cleanup []string
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

func schemaRow(id string, irreversible bool) AssistantAction {
	row := assistantRow(id, AssistantTargetNone, false, irreversible)
	row.Schema = true
	return row
}

// editVideoThumbnail runs a ComfyUI edit session from start to apply; revertThumbnailImage undoes the result.
var editVideoThumbnail = AssistantAction{
	ID: "editVideoThumbnail", Target: AssistantTargetVideo,
	Arguments: []AssistantArgument{AssistantArgAtMs, AssistantArgInstruction},
	Stages:    []string{"startThumbnailEdit", "runThumbnailEdit", "applyThumbnailEdit"},
	Cleanup:   []string{"cancelThumbnailEdit", "discardThumbnailEdit"},
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
	assistantRow("applyWorkInfoFetch", AssistantTargetVideo, oneCall, canUndo, AssistantArgFetchID, AssistantArgFields),
	editVideoThumbnail,
	schemaRow("revertThumbnailImage", canUndo),
	schemaRow("setVideoDetails", canUndo),
	schemaRow("putVideoProgress", canUndo),

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
	assistantRow("applyPersonPluginFetch", AssistantTargetPerson, oneCall, canUndo,
		AssistantArgFetchID, AssistantArgFields),
	schemaRow("moveFaceAppearances", cannotUndo),
	schemaRow("splitFaceAppearances", cannotUndo),
	schemaRow("splitFaceAppearance", cannotUndo),
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

	// Activity
	schemaRow("undoActivity", canUndo),
	schemaRow("retryActivity", canUndo),
}

// assistantRemoteVideoRows also act on the peer's videos, which carry PeerIDOffset, wherever the screens edit them:
// the bulk edits split their selection between the nodes, and the single-video rows call the operations
// remoteBoundary forwards or answers for a remote video. Every other video row refuses a remote video.
var assistantRemoteVideoRows = map[string]bool{
	"updateVideoTags.add": true, "updateVideoTags.remove": true, "applyVideoEdits.add": true,
	"applyVideoEdits.remove": true, "applyVideoCodeEdits": true, "updateVideoVisibility": true,
	"setVideoDisplayName": true, "setVideoWorkInfo": true, "setVideoDetails": true, "setVideoThumbnailPosition": true,
	"confirmVideoTag": true, "rejectVideoTag": true, "applyWorkInfoFetch": true, "makeRepresentativeVersion": true,
	"unbundleVideo": true, "revertThumbnailImage": true, "editVideoThumbnail": true, "putVideoProgress": true,
}

// RemoteVideos reports whether the row also acts on the peer's videos.
func (a AssistantAction) RemoteVideos() bool { return assistantRemoteVideoRows[a.ID] }

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
	// Operation names the stage or cleanup a composite row calls; other rows leave it empty.
	Operation string
	// Path fills the operation's path parameters by name, such as "id" or "tagId".
	Path  map[string]string
	Query map[string]string
	// Body holds the request fields; the runner adds the row's action and, for settings, the current values.
	// A schema row's body is checked against the operation's schema before the call.
	Body map[string]any
}

// AssistantCallResult is the successful response of an operation call.
type AssistantCallResult struct {
	Status      int
	ContentType string
	Body        []byte
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
