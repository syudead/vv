package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// errPeerPart marks a failure of the peer's part of an operation over a selection, which leaves the local part
// to apply.
var errPeerPart = errors.New("the other VVMDM's part of the selection failed")

// provisionalPersonError is a provisional person, who has no name to join by, named for the other node's videos.
type provisionalPersonError struct{ field string }

func (e *provisionalPersonError) Error() string {
	return "a provisional person named by " + e.field + " cannot go to the other VVMDM"
}

// editVocabulary turns the tags, people and organizations of an edit into one node's ids, joining the other
// node's entities by name.
type editVocabulary struct {
	// own gives an id of this node in its own id space, or false for an id of the other node.
	own func(id int64) (int64, bool)
	// name is the canonical name of the other node's id; found is false when it is missing, and a provisional
	// person's name is empty.
	name func(kind domain.VocabularyKind, id int64) (name string, found bool, err error)
	// id is this node's id of name, or zero.
	id func(kind domain.VocabularyKind, name string) (int64, error)
	// create makes a person or organization named name on this node.
	create func(kind domain.CreditKind, name string) (int64, error)
}

// translateEdit is edit in v's node's ids: a tag to add goes by name when the node lacks its id, a person or
// organization to add is created there when missing, and an entity to remove the node lacks is dropped.
func translateEdit(edit domain.VideoEdit, v editVocabulary) (domain.VideoEdit, error) {
	v = rememberCreated(v)
	out := domain.VideoEdit{Dates: edit.Dates, Public: edit.Public}
	for _, input := range edit.TagsAdd {
		if input.ID == nil {
			out.TagsAdd = append(out.TagsAdd, input)
			continue
		}
		if id, ok := v.own(*input.ID); ok {
			out.TagsAdd = append(out.TagsAdd, domain.TagInput{ID: &id})
			continue
		}
		name, err := otherName(v, domain.VocabularyTag, *input.ID, "")
		if err != nil {
			return domain.VideoEdit{}, err
		}
		out.TagsAdd = append(out.TagsAdd, domain.TagInput{Name: &name})
	}
	for _, id := range edit.TagsRemove {
		mapped, err := translateID(v, domain.VocabularyTag, id, "", false)
		if err != nil {
			return domain.VideoEdit{}, err
		}
		if mapped != 0 {
			out.TagsRemove = append(out.TagsRemove, mapped)
		}
	}
	for i, credit := range edit.Credits {
		mapped, err := translateCreditEdit(v, i, credit)
		if err != nil {
			return domain.VideoEdit{}, err
		}
		if len(mapped.Add)+len(mapped.Remove) > 0 {
			out.Credits = append(out.Credits, mapped)
		}
	}
	return out, nil
}

// rememberCreated makes v's id return what v's create made earlier, so a person or organization named in
// several credits of one edit is created once: the name lookups hold the node's entities before the edit.
func rememberCreated(v editVocabulary) editVocabulary {
	created := map[domain.VocabularyKind]map[string]int64{}
	id, create := v.id, v.create
	v.id = func(kind domain.VocabularyKind, name string) (int64, error) {
		if found, ok := created[kind][name]; ok {
			return found, nil
		}
		return id(kind, name)
	}
	v.create = func(kind domain.CreditKind, name string) (int64, error) {
		made, err := create(kind, name)
		if err == nil {
			vocabulary := vocabularyKindOf(kind)
			if created[vocabulary] == nil {
				created[vocabulary] = map[string]int64{}
			}
			created[vocabulary][name] = made
		}
		return made, err
	}
	return v
}

func translateCreditEdit(v editVocabulary, index int, credit domain.CreditEdit) (domain.CreditEdit, error) {
	out := domain.CreditEdit{Kind: credit.Kind, Role: credit.Role}
	kind := vocabularyKindOf(credit.Kind)
	for _, list := range []struct {
		name string
		ids  []int64
		out  *[]int64
	}{{"add", credit.Add, &out.Add}, {"remove", credit.Remove, &out.Remove}} {
		for j, id := range list.ids {
			mapped, err := translateID(v, kind, id, fmt.Sprintf("credits[%d].%s[%d]", index, list.name, j), list.name == "add")
			if err != nil {
				return domain.CreditEdit{}, err
			}
			if mapped != 0 {
				*list.out = append(*list.out, mapped)
			}
		}
	}
	return out, nil
}

// translateID is id in v's node's ids, or zero for an entity to remove that the node lacks; a person or
// organization to add is created there.
func translateID(v editVocabulary, kind domain.VocabularyKind, id int64, field string, add bool) (int64, error) {
	if own, ok := v.own(id); ok {
		return own, nil
	}
	name, err := otherName(v, kind, id, field)
	if err != nil {
		return 0, err
	}
	found, err := v.id(kind, name)
	if err != nil || found != 0 || !add {
		return found, err
	}
	creditKind, _ := kind.CreditKind()
	return v.create(creditKind, name)
}

// otherName is the name of the other node's id, with the error the edit answers when it has none.
func otherName(v editVocabulary, kind domain.VocabularyKind, id int64, field string) (string, error) {
	name, found, err := v.name(kind, id)
	creditKind, credit := kind.CreditKind()
	switch {
	case err != nil:
		return "", err
	case !found && !credit:
		return "", domain.ErrTagNotFound
	case !found:
		return "", &domain.CreditNotFoundError{Kind: creditKind, ID: id, Field: field}
	case name == "":
		return "", &provisionalPersonError{field: field}
	}
	return name, nil
}

// peerEditVocabulary turns this node's entities into the peer's by name. Failures of the peer are errPeerPart.
func (s *server) peerEditVocabulary(r *http.Request) editVocabulary {
	ctx := r.Context()
	entries := s.peerEntryCache(r)
	return editVocabulary{
		own: func(id int64) (int64, bool) { return domain.FromRemoteID(id), domain.IsRemoteID(id) },
		name: func(kind domain.VocabularyKind, id int64) (string, bool, error) {
			creditKind, credit := kind.CreditKind()
			if !credit {
				names, err := s.tags.TagNames(ctx, []int64{id})
				name, found := names[id]
				return name, found, err
			}
			found, err := s.credits.Get(ctx, creditKind, id)
			if errors.Is(err, domain.ErrNotFound) {
				return "", false, nil
			}
			return found.Name, err == nil, err
		},
		id: func(kind domain.VocabularyKind, name string) (int64, error) {
			byName, _, err := entries(kind)
			return byName[name], err
		},
		create: func(kind domain.CreditKind, name string) (int64, error) {
			var created struct {
				ID int64 `json:"id"`
			}
			err := s.peerCall(r, http.MethodPost, vocabularyListPaths[vocabularyKindOf(kind)], nil, gen.CreditInput{Name: name}, &created)
			if err != nil {
				return 0, fmt.Errorf("%w: %w", errPeerPart, err)
			}
			return created.ID, nil
		},
	}
}

// localEditVocabulary turns the peer's entities into this node's by name, creating a missing person or
// organization here.
func (s *server) localEditVocabulary(r *http.Request) editVocabulary {
	ctx := r.Context()
	entries := s.peerEntryCache(r)
	var join *domain.VocabularyJoin
	return editVocabulary{
		own: func(id int64) (int64, bool) { return id, !domain.IsRemoteID(id) },
		name: func(kind domain.VocabularyKind, id int64) (string, bool, error) {
			_, byID, err := entries(kind)
			name, found := byID[domain.FromRemoteID(id)]
			return name, found, err
		},
		id: func(kind domain.VocabularyKind, name string) (int64, error) {
			if join == nil {
				loaded, err := s.vocabularyJoin(ctx)
				if err != nil {
					return 0, err
				}
				join = &loaded
			}
			byName := map[domain.VocabularyKind]map[string]int64{
				domain.VocabularyTag: join.Tags, domain.VocabularyPerson: join.People,
				domain.VocabularyOrganization: join.Organizations,
			}[kind]
			return byName[name], nil
		},
		create: func(kind domain.CreditKind, name string) (int64, error) {
			created, err := s.credits.Create(ctx, kind, domain.CreditInput{Name: name})
			return created.ID, err
		},
	}
}

// peerEntryCache reads each kind's whole list of the peer once, by name (the lowest id) and by id.
func (s *server) peerEntryCache(r *http.Request) func(domain.VocabularyKind) (map[string]int64, map[int64]string, error) {
	byName := map[domain.VocabularyKind]map[string]int64{}
	byID := map[domain.VocabularyKind]map[int64]string{}
	return func(kind domain.VocabularyKind) (map[string]int64, map[int64]string, error) {
		if names, ok := byName[kind]; ok {
			return names, byID[kind], nil
		}
		items, err := s.peerEntries(r, kind)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", errPeerPart, err)
		}
		names, ids := map[string]int64{}, map[int64]string{}
		for _, item := range items {
			ids[item.ID] = item.Name
			if found, ok := names[item.Name]; item.Name != "" && (!ok || item.ID < found) {
				names[item.Name] = item.ID
			}
		}
		byName[kind], byID[kind] = names, ids
		return names, ids, nil
	}
}
