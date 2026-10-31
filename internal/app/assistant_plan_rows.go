package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// tagInput is a screen API TagInput: an existing tag by id, or a missing one by the name that creates it, which
// also finds the tag an earlier step creates.
func tagInput(label domain.AssistantLabel) map[string]any {
	if label.ID <= 0 {
		return map[string]any{"name": label.Name}
	}
	return map[string]any{"id": label.ID}
}

// videoTag attaches or removes one tag; updateVideoTags takes a single tag per request.
func videoTag(use nameUse) planBuilder {
	return func(ctx context.Context, d *planDraft) (bool, error) {
		if !d.need(len(d.args.TagNames) == 1) {
			return false, nil
		}
		label, ok, err := d.t.one(ctx, domain.AssistantLabelTag, d.args.TagNames[0], use)
		if !ok || err != nil {
			return false, err
		}
		d.body("tag", tagInput(label))
		d.named(domain.AssistantArgTagNames, []domain.AssistantLabel{label})
		return true, nil
	}
}

// videoEdits builds applyVideoEdits: tags, people in one role, and on add the visibility and one date.
func videoEdits(add bool) planBuilder {
	key := "remove"
	use := valueName
	if add {
		key, use = "add", createdName
	}
	return func(ctx context.Context, d *planDraft) (bool, error) {
		a := d.args
		if !d.need(len(a.OrganizationNames) == 0 && (len(a.PersonNames) == 0 || a.Role != "")) {
			return false, nil
		}
		tags, ok, err := d.t.each(ctx, domain.AssistantLabelTag, a.TagNames, use)
		if !ok || err != nil {
			return false, err
		}
		people, ok, err := d.t.each(ctx, domain.AssistantLabelPerson, a.PersonNames, valueName)
		if !ok || err != nil {
			return false, err
		}
		changed := len(tags) > 0 || len(people) > 0
		if len(tags) > 0 {
			inputs := make([]any, len(tags))
			for i, tag := range tags {
				if !add {
					inputs[i] = tag.ID
					continue
				}
				inputs[i] = tagInput(tag)
			}
			d.body("tags", map[string]any{key: inputs})
			d.named(domain.AssistantArgTagNames, tags)
		}
		if len(people) > 0 {
			d.body("credits", []any{map[string]any{"kind": "person", "role": a.Role, key: labelIDs(people)}})
			d.named(domain.AssistantArgPersonNames, people)
			d.literal(domain.AssistantArgRole, a.Role)
		}
		if add {
			changed = videoEditValues(d) || changed
		}
		return d.need(changed), nil
	}
}

// videoEditValues adds the visibility and the date an applyVideoEdits.add sets.
func videoEditValues(d *planDraft) bool {
	changed := false
	if d.args.Public != nil {
		d.body("public", *d.args.Public)
		d.literal(domain.AssistantArgPublic, *d.args.Public)
		changed = true
	}
	if d.args.DateKind != "" && d.args.Date != "" {
		d.body("dates", []any{map[string]any{"kind": d.args.DateKind, "date": d.args.Date}})
		d.literal(domain.AssistantArgDateKind, d.args.DateKind)
		d.literal(domain.AssistantArgDate, d.args.Date)
		changed = true
	}
	return changed
}

func favorites(ctx context.Context, d *planDraft) (bool, error) {
	if !d.need(d.args.Enabled != nil) {
		return false, nil
	}
	if err := d.collectionTargets(ctx, domain.AssistantLabelGroup, "groupIds", d.groupIDs); err != nil {
		return false, err
	}
	if err := d.collectionTargets(ctx, domain.AssistantLabelSmartFolder, "smartFolderIds", d.folderIDs); err != nil {
		return false, err
	}
	d.body("favorite", *d.args.Enabled)
	d.literal(domain.AssistantArgEnabled, *d.args.Enabled)
	return true, nil
}

// collectionTargets labels the collections the card shows and sends their ids in field.
func (d *planDraft) collectionTargets(
	ctx context.Context, kind domain.AssistantLabelKind, field string, ids []int64,
) error {
	if len(ids) == 0 {
		return nil
	}
	names, err := d.t.a.vocabulary.EntityNames(ctx, kind, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if name, ok := names[id]; ok {
			d.plan.Targets.Labels = append(d.plan.Targets.Labels, domain.AssistantLabel{Kind: kind, ID: id, Name: name})
		}
	}
	d.body(field, ids)
	return nil
}

func displayName(_ context.Context, d *planDraft) (bool, error) {
	if !d.need(d.args.NewName != nil) {
		return false, nil
	}
	d.body("displayName", *d.args.NewName)
	d.literal(domain.AssistantArgNewName, *d.args.NewName)
	return true, nil
}

func thumbnailPosition(_ context.Context, d *planDraft) (bool, error) {
	var position any
	if d.args.Number != nil {
		position = int64(*d.args.Number)
	}
	d.body("positionMs", position)
	d.literal(domain.AssistantArgNumber, position)
	return true, nil
}

// videoTagPath names the one tag of the open video to confirm or reject.
func videoTagPath(ctx context.Context, d *planDraft) (bool, error) {
	if !d.need(len(d.args.TagNames) == 1) {
		return false, nil
	}
	label, ok, err := d.t.one(ctx, domain.AssistantLabelTag, d.args.TagNames[0], valueName)
	if !ok || err != nil {
		return false, err
	}
	d.path("tagId", label.ID)
	d.named(domain.AssistantArgTagNames, []domain.AssistantLabel{label})
	return true, nil
}

func virtualVideo(_ context.Context, d *planDraft) (bool, error) {
	if !d.need(d.args.Name != "" && d.args.URL != "") {
		return false, nil
	}
	d.body("title", d.args.Name)
	d.body("url", d.args.URL)
	d.literal(domain.AssistantArgName, d.args.Name)
	d.literal(domain.AssistantArgURL, d.args.URL)
	return true, nil
}

// createTag cannot set a kind: createTag takes only a name, so a kind needs batchTags.setKind afterwards.
func createTag(ctx context.Context, d *planDraft) (bool, error) {
	if !d.need(d.args.TagKindName == "") {
		return false, nil
	}
	return stringTo(domain.AssistantArgName, "name")(ctx, d)
}

func synonymQuery(_ context.Context, d *planDraft) (bool, error) {
	if !d.need(d.args.Synonym != "") {
		return false, nil
	}
	d.query("name", d.args.Synonym)
	d.literal(domain.AssistantArgSynonym, d.args.Synonym)
	return true, nil
}

func rejectedNameQuery(_ context.Context, d *planDraft) (bool, error) {
	if !d.need(d.args.Name != "") {
		return false, nil
	}
	d.query("name", d.args.Name)
	d.literal(domain.AssistantArgName, d.args.Name)
	return true, nil
}

func tagBatch(_ context.Context, d *planDraft) (bool, error) {
	d.body("ids", labelIDs(d.plan.Targets.Labels))
	return true, nil
}

func tagBatchKind(ctx context.Context, d *planDraft) (bool, error) {
	if !d.need(d.args.TagKindName != "") {
		return false, nil
	}
	kind, ok, err := d.t.one(ctx, domain.AssistantLabelTagKind, d.args.TagKindName, valueName)
	if !ok || err != nil {
		return false, err
	}
	d.body("kindId", kind.ID)
	d.named(domain.AssistantArgTagKindName, []domain.AssistantLabel{kind})
	return tagBatch(ctx, d)
}

func aliasInputs(aliases []string) []any {
	out := make([]any, len(aliases))
	for i, alias := range aliases {
		out[i] = map[string]any{"name": alias}
	}
	return out
}

// creditFields sets the aliases and, for an organization, the parent named by parentName.
func creditFields(ctx context.Context, d *planDraft, organization bool) (bool, error) {
	if len(d.args.Aliases) > 0 {
		d.body("aliases", aliasInputs(d.args.Aliases))
		d.literal(domain.AssistantArgAliases, d.args.Aliases)
	}
	if d.args.ParentName == "" {
		return true, nil
	}
	if !d.need(organization) {
		return false, nil
	}
	parent, ok, err := d.t.one(ctx, domain.AssistantLabelOrganization, d.args.ParentName, valueName)
	if !ok || err != nil {
		return false, err
	}
	d.body("parentId", parent.ID)
	d.named(domain.AssistantArgParentName, []domain.AssistantLabel{parent})
	return true, nil
}

func createCredit(organization bool) planBuilder {
	return func(ctx context.Context, d *planDraft) (bool, error) {
		if ok, err := stringTo(domain.AssistantArgName, "name")(ctx, d); !ok || err != nil {
			return false, err
		}
		return creditFields(ctx, d, organization)
	}
}

func updateCredit(organization bool) planBuilder {
	return func(ctx context.Context, d *planDraft) (bool, error) {
		renamed := d.args.NewName != nil && *d.args.NewName != ""
		if renamed {
			d.body("name", *d.args.NewName)
			d.literal(domain.AssistantArgNewName, *d.args.NewName)
		}
		if !d.need(renamed || len(d.args.Aliases) > 0 || d.args.ParentName != "") {
			return false, nil
		}
		return creditFields(ctx, d, organization)
	}
}

// faceCandidate names the other person of a face candidate pair.
func faceCandidate(ctx context.Context, d *planDraft) (bool, error) {
	if !d.need(len(d.args.Names) == 1) {
		return false, nil
	}
	other, ok, err := d.t.one(ctx, domain.AssistantLabelPerson, d.args.Names[0], valueName)
	if !ok || err != nil {
		return false, err
	}
	d.path("otherId", other.ID)
	d.named(domain.AssistantArgNames, []domain.AssistantLabel{other})
	return true, nil
}

func deleteCollections(_ context.Context, d *planDraft) (bool, error) {
	groups, folders := []int64{}, []int64{}
	for _, label := range d.plan.Targets.Labels {
		if label.Kind == domain.AssistantLabelGroup {
			groups = append(groups, label.ID)
		} else {
			folders = append(folders, label.ID)
		}
	}
	d.body("groupIds", groups)
	d.body("smartFolderIds", folders)
	return true, nil
}

// createSmartFolder saves the target filter, or the screen's filter when the request names none, as the rule.
func createSmartFolder(ctx context.Context, d *planDraft) (bool, error) {
	if ok, err := stringTo(domain.AssistantArgName, "name")(ctx, d); !ok || err != nil {
		return false, err
	}
	var rule domain.SmartRule
	switch {
	case d.req.Target != nil && d.req.Target.Filter != nil:
		criteria, ok, err := d.t.criteria(ctx, d.req.Target.Filter, true)
		if !ok || err != nil {
			return false, err
		}
		rule = criteria.Rule
		d.plan.Change = append(d.plan.Change, domain.AssistantChange{Argument: "rule", Labels: criteria.Labels})
	case d.t.input.Context.Filter != nil:
		rule = *d.t.input.Context.Filter
	default:
		d.t.clarify("target", nil)
		return false, nil
	}
	d.body("rule", rule)
	if d.args.Sort != "" {
		d.body("sort", d.args.Sort)
		d.literal(domain.AssistantArgSort, d.args.Sort)
	}
	if d.args.ParentName != "" {
		parent, ok, err := d.t.one(ctx, domain.AssistantLabelSmartFolder, d.args.ParentName, valueName)
		if !ok || err != nil {
			return false, err
		}
		d.body("parentId", parent.ID)
		d.named(domain.AssistantArgParentName, []domain.AssistantLabel{parent})
	}
	return true, nil
}

// ollamaSettings maps the name argument to the model field it changes: decision is model, generation is
// generationModel; the runner keeps every other saved field.
func ollamaSettings(_ context.Context, d *planDraft) (bool, error) {
	fields := map[string]string{"decision": "model", "generation": "generationModel"}
	changed := false
	if d.args.URL != "" {
		d.body("endpoint", d.args.URL)
		d.literal(domain.AssistantArgURL, d.args.URL)
		changed = true
	}
	if d.args.Model != "" {
		field, ok := fields[d.args.Name]
		if !d.need(ok) {
			return false, nil
		}
		d.body(field, d.args.Model)
		d.literal(domain.AssistantArgName, d.args.Name)
		d.literal(domain.AssistantArgModel, d.args.Model)
		changed = true
	}
	return d.need(changed), nil
}

// autoTaggingSettings maps the name argument to its field: use and automatic are switches, add and suggest are the
// two probabilities.
func autoTaggingSettings(_ context.Context, d *planDraft) (bool, error) {
	switches := map[string]string{"use": "enabled", "automatic": "afterImport"}
	thresholds := map[string]string{"add": "threshold", "suggest": "tentativeThreshold"}
	d.literal(domain.AssistantArgName, d.args.Name)
	if field, ok := switches[d.args.Name]; ok && d.args.Enabled != nil {
		d.body(field, *d.args.Enabled)
		d.literal(domain.AssistantArgEnabled, *d.args.Enabled)
		return true, nil
	}
	if field, ok := thresholds[d.args.Name]; ok && d.args.Number != nil {
		d.body(field, *d.args.Number)
		d.literal(domain.AssistantArgNumber, *d.args.Number)
		return true, nil
	}
	return d.need(false), nil
}

func comfyUISettings(_ context.Context, d *planDraft) (bool, error) {
	changed := false
	for _, field := range []struct {
		arg   domain.AssistantArgument
		name  string
		value string
	}{
		{domain.AssistantArgProvider, "provider", d.args.Provider},
		{domain.AssistantArgURL, "url", d.args.URL},
		{domain.AssistantArgModel, "model", d.args.Model},
	} {
		if field.value != "" {
			d.body(field.name, field.value)
			d.literal(field.arg, field.value)
			changed = true
		}
	}
	return d.need(changed), nil
}
