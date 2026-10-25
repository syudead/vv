package app

import (
	"context"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// AssistantVocabulary finds the library's entities by name for the assistant, so the model never sees the
// vocabulary and never chooses an id.
type AssistantVocabulary interface {
	// FindNames returns the entities of kind whose name, tag synonym or alias folds (width, case and kana) to the
	// same key as name, in id order, with their video counts.
	FindNames(ctx context.Context, kind domain.AssistantLabelKind, name string) ([]domain.AssistantCandidate, error)
	// EntityNames reads the current names of the ids that still exist.
	EntityNames(ctx context.Context, kind domain.AssistantLabelKind, ids []int64) (map[int64]string, error)
}

// assistantClarifyLimit bounds the candidates a clarification offers.
const assistantClarifyLimit = 10

// clarify records the first clarification a turn needs; later ones are dropped so the owner answers one question.
func (t *assistantTurn) clarify(about string, candidates []domain.AssistantCandidate, names ...string) {
	if t.clarification != nil {
		return
	}
	t.clarification = &domain.AssistantClarification{
		About: about, Candidates: candidates[:min(len(candidates), assistantClarifyLimit)], Names: names,
	}
}

// candidates resolves one name of one kind: a pinned id first, then the vocabulary.
func (t *assistantTurn) candidates(
	ctx context.Context, kind domain.AssistantLabelKind, name string,
) ([]domain.AssistantCandidate, error) {
	key := domain.FoldForMatch(name)
	for _, pin := range t.input.Context.Pinned {
		if pin.Kind != kind || domain.FoldForMatch(pin.Name) != key {
			continue
		}
		names, err := t.a.vocabulary.EntityNames(ctx, kind, []int64{pin.ID})
		if err != nil {
			return nil, err
		}
		if current, ok := names[pin.ID]; ok {
			return []domain.AssistantCandidate{{Kind: kind, ID: pin.ID, Name: current}}, nil
		}
	}
	return t.a.vocabulary.FindNames(ctx, kind, name)
}

// anyKinds are the kinds a name of kind "any" may be, in the order they are tried.
var anyKinds = []domain.AssistantLabelKind{
	domain.AssistantLabelPerson, domain.AssistantLabelOrganization, domain.AssistantLabelTag,
}

// anyCandidates tries a person, an organization and a tag; a name that two kinds hold is ambiguous.
func (t *assistantTurn) anyCandidates(ctx context.Context, name string) ([]domain.AssistantCandidate, error) {
	var found []domain.AssistantCandidate
	for _, kind := range anyKinds {
		matches, err := t.candidates(ctx, kind, name)
		if err != nil {
			return nil, err
		}
		found = append(found, matches...)
	}
	return found, nil
}

// nameUse says what a name is for: the clarification it asks and whether a missing name may be created.
type nameUse struct {
	about     string
	creatable bool
}

var (
	targetName  = nameUse{about: "target"}
	valueName   = nameUse{about: "value"}
	createdName = nameUse{about: "value", creatable: true}
)

// one resolves a name to exactly one entity. Several matches, or none for a name that cannot be created, record a
// clarification and return ok false; a creatable name that matches nothing returns a label without an id.
func (t *assistantTurn) one(
	ctx context.Context, kind domain.AssistantLabelKind, name string, use nameUse,
) (domain.AssistantLabel, bool, error) {
	var found []domain.AssistantCandidate
	var err error
	if kind == "" {
		found, err = t.anyCandidates(ctx, name)
	} else {
		found, err = t.candidates(ctx, kind, name)
	}
	if err != nil {
		return domain.AssistantLabel{}, false, err
	}
	switch {
	case len(found) == 1:
		return domain.AssistantLabel{Kind: found[0].Kind, ID: found[0].ID, Name: found[0].Name}, true, nil
	case len(found) > 1:
		t.clarify(use.about, found)
		return domain.AssistantLabel{}, false, nil
	case use.creatable:
		t.created = append(t.created, name)
		return domain.AssistantLabel{Kind: kind, Name: name}, true, nil
	default:
		t.clarify(use.about, nil, name)
		return domain.AssistantLabel{}, false, nil
	}
}

// each resolves every name and stops at the first that needs a clarification.
func (t *assistantTurn) each(
	ctx context.Context, kind domain.AssistantLabelKind, names []string, use nameUse,
) ([]domain.AssistantLabel, bool, error) {
	labels := make([]domain.AssistantLabel, 0, len(names))
	for _, name := range slices.Compact(slices.Clone(names)) {
		label, ok, err := t.one(ctx, kind, name, use)
		if !ok || err != nil {
			return nil, false, err
		}
		labels = append(labels, label)
	}
	return labels, true, nil
}

func labelIDs(labels []domain.AssistantLabel) []int64 {
	ids := make([]int64, 0, len(labels))
	for _, label := range labels {
		ids = append(ids, label.ID)
	}
	return ids
}
