package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

func jsonValue(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("cannot encode an assistant argument: %w", err)
	}
	return encoded, nil
}

// action resolves the target and the arguments into the plan the card shows, and stores it as a proposal.
func (t *assistantTurn) action(ctx context.Context, req *domain.AssistantActionRequest) (domain.AssistantResolution, error) {
	if _, buildable := planBuilders[req.Operation]; !buildable {
		return domain.AssistantResolution{Kind: domain.AssistantResolvedUnsupported, Unsupported: req.Operation}, nil
	}
	plan, ok, err := t.buildPlan(ctx, req)
	if !ok || err != nil {
		return domain.AssistantResolution{}, err
	}
	proposal, err := t.a.Propose(ctx, 0, t.input.Text, []domain.AssistantPlan{plan})
	if err != nil {
		return domain.AssistantResolution{}, err
	}
	return domain.AssistantResolution{Kind: domain.AssistantResolvedProposal, Proposal: &proposal}, nil
}

// buildPlan resolves the target and the arguments of a row that has a builder into the plan the card shows; false
// means the turn asks first, or refuses the arguments with a failure.
func (t *assistantTurn) buildPlan(ctx context.Context, req *domain.AssistantActionRequest) (domain.AssistantPlan, bool, error) {
	row, found := domain.LookupAssistantAction(req.Operation)
	build, buildable := planBuilders[req.Operation]
	if !found || !buildable {
		return domain.AssistantPlan{}, false, fmt.Errorf("%w: %q", domain.ErrAssistantActionUnknown, req.Operation)
	}
	d := &planDraft{
		t: t, row: row, req: req,
		plan: domain.AssistantPlan{ActionID: row.ID, Targets: domain.AssistantPlanTargets{Kind: row.Target}},
	}
	if req.Arguments != nil {
		d.args = *req.Arguments
	}
	if ok, err := d.resolveTarget(ctx); !ok || err != nil {
		return d.plan, false, err
	}
	if ok, err := build(ctx, d); !ok || err != nil {
		return d.plan, false, err
	}
	d.plan.Created = t.created
	return d.plan, true, nil
}

// planBuilder fills the request of one catalog row from its resolved arguments; false means the turn asks first.
type planBuilder func(ctx context.Context, d *planDraft) (bool, error)

// planBuilders holds the rows the assistant can build a request for. A row without one answers unsupported: its
// request needs an id a plan cannot hold yet (a playlist or group item, a content version), a representative
// among the batch, or a field the arguments do not name.
var planBuilders = map[string]planBuilder{
	// Videos
	"updateVideoTags.add":       videoTag(createdName),
	"updateVideoTags.remove":    videoTag(valueName),
	"applyVideoEdits.add":       videoEdits(true),
	"applyVideoEdits.remove":    videoEdits(false),
	"updateVideoVisibility":     boolTo(domain.AssistantArgPublic, "public"),
	"updateFavorites":           favorites,
	"setVideoDisplayName":       displayName,
	"setVideoThumbnailPosition": thumbnailPosition,
	"reprobeVideo":              nothing,
	"autoTagVideo":              nothing,
	"confirmVideoTag":           videoTagPath,
	"rejectVideoTag":            videoTagPath,
	"makeRepresentativeVersion": nothing,
	"unbundleVideo":             nothing,
	"createVirtualVideo":        virtualVideo,
	"applyWorkInfoFetch":        applyWorkInfoFetch,
	"editVideoThumbnail":        editVideoThumbnail,

	// Tags and tag kinds
	"createTag":             createTag,
	"renameTag":             renameTo("name"),
	"deleteTag":             nothing,
	"mergeTag":              mergeSources(domain.AssistantLabelTag),
	"addTagSynonym":         stringTo(domain.AssistantArgSynonym, "name"),
	"removeTagSynonym":      synonymQuery,
	"confirmTag":            nothing,
	"rejectTag":             nothing,
	"forgetRejectedTagName": rejectedNameQuery,
	"batchTags.confirm":     tagBatch,
	"batchTags.reject":      tagBatch,
	"batchTags.delete":      tagBatch,
	"batchTags.setKind":     tagBatchKind,
	"batchTags.clearKind":   tagBatch,
	"createTagKind":         stringTo(domain.AssistantArgName, "name"),
	"renameTagKind":         renameTo("name"),
	"deleteTagKind":         nothing,
	"setTagKindCardFirst":   boolTo(domain.AssistantArgEnabled, "cardFirst"),

	// People and organizations
	"createPerson":           createCredit(false),
	"updatePerson":           updateCredit(false),
	"deletePerson":           nothing,
	"mergePeople":            mergeSources(domain.AssistantLabelPerson),
	"deletePersonImage":      nothing,
	"confirmFaceCandidate":   faceCandidate,
	"dismissFaceCandidate":   faceCandidate,
	"reopenFaceCandidate":    faceCandidate,
	"applyPersonPluginFetch": applyPersonPluginFetch,
	"createOrganization":     createCredit(true),
	"updateOrganization":     updateCredit(true),
	"deleteOrganization":     nothing,
	"mergeOrganizations":     mergeSources(domain.AssistantLabelOrganization),

	// Playlists, groups and smart folders
	"createPlaylist":    stringTo(domain.AssistantArgName, "name"),
	"renamePlaylist":    renameTo("name"),
	"deletePlaylist":    nothing,
	"addPlaylistItems":  intoCollection(domain.AssistantLabelPlaylist),
	"createGroup":       stringTo(domain.AssistantArgName, "name"),
	"renameGroup":       renameTo("name"),
	"dissolveGroup":     nothing,
	"addGroupItems":     intoCollection(domain.AssistantLabelGroup),
	"tagGroup":          nothing,
	"deleteCollections": deleteCollections,
	"createSmartFolder": createSmartFolder,
	"deleteSmartFolder": nothing,

	// Settings and scans
	"updateVersionJudgingSettings": boolTo(domain.AssistantArgEnabled, "enabled"),
	"updateAutoImportSettings":     boolTo(domain.AssistantArgEnabled, "enabled"),
	"updateTranscodingSettings":    stringTo(domain.AssistantArgEncoder, "videoEncoder"),
	"updateOllamaSettings":         ollamaSettings,
	"updateAutoTaggingSettings":    autoTaggingSettings,
	"updateComfyUISettings":        comfyUISettings,
	"startScan":                    emptyBody,
	"startAutoTagging":             runScope(map[string]string{"": "missing", "untagged": "missing", "all": "all"}),
	"startFaceRecognition":         runScope(map[string]string{"": "missing", "untagged": "missing"}),
}

func nothing(context.Context, *planDraft) (bool, error) { return true, nil }

func emptyBody(_ context.Context, d *planDraft) (bool, error) {
	d.plan.Call.Body = map[string]any{}
	return true, nil
}

// stringTo sends a required string argument as one body field.
func stringTo(arg domain.AssistantArgument, field string) planBuilder {
	return func(_ context.Context, d *planDraft) (bool, error) {
		value := stringArgument(d.args, arg)
		if !d.need(value != "") {
			return false, nil
		}
		d.body(field, value)
		d.literal(arg, value)
		return true, nil
	}
}

func stringArgument(args domain.AssistantArguments, arg domain.AssistantArgument) string {
	switch arg {
	case domain.AssistantArgName:
		return args.Name
	case domain.AssistantArgSynonym:
		return args.Synonym
	case domain.AssistantArgEncoder:
		return args.Encoder
	case domain.AssistantArgURL:
		return args.URL
	case domain.AssistantArgModel:
		return args.Model
	default:
		return ""
	}
}

// renameTo sends the new name as one body field.
func renameTo(field string) planBuilder {
	return func(_ context.Context, d *planDraft) (bool, error) {
		if !d.need(d.args.NewName != nil && *d.args.NewName != "") {
			return false, nil
		}
		d.body(field, *d.args.NewName)
		d.literal(domain.AssistantArgNewName, *d.args.NewName)
		return true, nil
	}
}

// boolTo sends a required switch as one body field.
func boolTo(arg domain.AssistantArgument, field string) planBuilder {
	return func(_ context.Context, d *planDraft) (bool, error) {
		value := d.args.Enabled
		if arg == domain.AssistantArgPublic {
			value = d.args.Public
		}
		if !d.need(value != nil) {
			return false, nil
		}
		d.body(field, *value)
		d.literal(arg, *value)
		return true, nil
	}
}

func mergeSources(kind domain.AssistantLabelKind) planBuilder {
	return func(ctx context.Context, d *planDraft) (bool, error) {
		if !d.need(len(d.args.Names) > 0) {
			return false, nil
		}
		sources, ok, err := d.t.each(ctx, kind, d.args.Names, valueName)
		if !ok || err != nil {
			return false, err
		}
		d.body("sourceIds", labelIDs(sources))
		d.named(domain.AssistantArgNames, sources)
		return true, nil
	}
}

// intoCollection adds the target videos to the playlist or group named by the name argument.
func intoCollection(kind domain.AssistantLabelKind) planBuilder {
	return func(ctx context.Context, d *planDraft) (bool, error) {
		if !d.need(d.args.Name != "") {
			return false, nil
		}
		label, ok, err := d.t.one(ctx, kind, d.args.Name, valueName)
		if !ok || err != nil {
			return false, err
		}
		d.path("id", label.ID)
		d.named(domain.AssistantArgName, []domain.AssistantLabel{label})
		return true, nil
	}
}

func runScope(scopes map[string]string) planBuilder {
	return func(_ context.Context, d *planDraft) (bool, error) {
		scope, ok := scopes[d.args.Scope]
		if !d.need(ok) {
			return false, nil
		}
		d.body("scope", scope)
		d.literal(domain.AssistantArgScope, scope)
		return true, nil
	}
}
