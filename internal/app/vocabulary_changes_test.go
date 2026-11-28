package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// vocabularyLog records, in order, the reads and writes of a change by name, each write with whether the
// tag-name lock was held.
type vocabularyLog struct {
	calls []string
	held  func() bool
}

func (l *vocabularyLog) add(format string, args ...any) {
	call := fmt.Sprintf(format, args...)
	if l.held != nil && l.held() {
		call += " (held)"
	}
	l.calls = append(l.calls, call)
}

type fakeVocabularyLinks struct {
	log     *vocabularyLog
	states  []domain.PeerLinkState
	entries map[int64][]domain.LinkVocabularyEntry
	refuse  map[int64]error
}

func (f *fakeVocabularyLinks) Links(context.Context) ([]domain.PeerLinkState, error) {
	return f.states, nil
}

func (f *fakeVocabularyLinks) Entries(_ context.Context, slot int64, _ domain.VocabularyKind) ([]domain.LinkVocabularyEntry, error) {
	f.log.calls = append(f.log.calls, fmt.Sprintf("entries %d", slot))
	return f.entries[slot], nil
}

func (f *fakeVocabularyLinks) Apply(_ context.Context, link domain.PeerLink, change domain.VocabularyChange) (domain.VocabularyOutcome, error) {
	f.log.add("%s %d %s->%s", change.Action, link.Slot, change.Name, change.NewName)
	if err := f.refuse[link.Slot]; err != nil {
		return domain.VocabularyOutcome{}, err
	}
	return domain.VocabularyOutcome{Changed: true}, nil
}

// fakeVocabularyTags holds tag 3, 猫, with the names in holders held by the tags given.
type fakeVocabularyTags struct {
	log     *vocabularyLog
	holders map[string]domain.TagRef
}

func (f *fakeVocabularyTags) TagNames(context.Context, []int64) (map[int64]string, error) {
	return map[int64]string{3: "猫"}, nil
}

func (f *fakeVocabularyTags) LookupTagName(_ context.Context, name string) (domain.TagRef, bool, error) {
	holder, ok := f.holders[name]
	return holder, ok, nil
}

func (f *fakeVocabularyTags) RenameTag(_ context.Context, id int64, name string) (domain.Tag, error) {
	f.log.add("rename here %d->%s", id, name)
	return domain.Tag{ID: id, Name: name}, nil
}

func (f *fakeVocabularyTags) PromoteSynonym(_ context.Context, id int64, name string) (domain.Tag, error) {
	f.log.add("promote here %d->%s", id, name)
	return domain.Tag{ID: id, Name: name}, nil
}

// fakeVocabularyCredits holds person 5 with current and updates it.
type fakeVocabularyCredits struct {
	log     *vocabularyLog
	current domain.Credit
}

func (f *fakeVocabularyCredits) Get(context.Context, domain.CreditKind, int64) (domain.Credit, error) {
	return f.current, nil
}

func (f *fakeVocabularyCredits) Names(context.Context, domain.CreditKind, []int64) (map[int64]string, error) {
	return map[int64]string{5: f.current.Name}, nil
}

func (f *fakeVocabularyCredits) Update(_ context.Context, _ domain.CreditKind, id int64, _ domain.CreditPatch) (domain.Credit, error) {
	f.log.add("update here %d", id)
	return f.current, nil
}

func twoLinks(states ...domain.PeerConnection) []domain.PeerLinkState {
	links := []domain.PeerLinkState{
		{Link: domain.PeerLink{Slot: 1, Name: "a"}, State: domain.PeerConnected},
		{Link: domain.PeerLink{Slot: 2, Name: "b"}, State: domain.PeerConnected},
	}
	for i, state := range states {
		links[i].State = state
	}
	return links
}

// newVocabularyFixture is the use case over fakes sharing one log, which marks the writes made under its lock.
func newVocabularyFixture(links *fakeVocabularyLinks, tags *fakeVocabularyTags) (*VocabularyChanges, *vocabularyLog) {
	log := &vocabularyLog{}
	links.log, tags.log = log, log
	v := NewVocabularyChanges(VocabularyChangesOptions{Tags: tags})
	log.held = func() bool {
		if v.tagNames.TryLock() {
			v.tagNames.Unlock()
			return false
		}
		return true
	}
	return v, log
}

// A rename checks the new name on every link before writing any, then writes the links in slot order and this
// node last, all under the tag-name lock, so a write that waits for it cannot fall between them.
func TestRenameTagWritesEveryLinkInSlotOrderThenHere(t *testing.T) {
	links := &fakeVocabularyLinks{states: twoLinks()}
	v, log := newVocabularyFixture(links, &fakeVocabularyTags{})

	if _, err := v.RenameTag(context.Background(), links, 3, " ねこ "); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"entries 1", "entries 2", "rename 1 猫->ねこ (held)", "rename 2 猫->ねこ (held)", "rename here 3-> ねこ  (held)",
	}
	if !slices.Equal(log.calls, want) {
		t.Errorf("calls = %q, want %q", log.calls, want)
	}
}

// A link that is down or a name some node holds stops the rename before any node changes, naming that node.
func TestRenameTagChangesNoNodeWhenALinkIsDownOrTheNameTaken(t *testing.T) {
	tests := []struct {
		name    string
		states  []domain.PeerLinkState
		entries map[int64][]domain.LinkVocabularyEntry
		here    map[string]domain.TagRef
		want    error
	}{
		{
			name: "the second link is not connected", states: twoLinks(domain.PeerConnected, domain.PeerUnreachable),
			want: &domain.VocabularyLinkError{Link: "b", Err: domain.ErrPeerUnreachable},
		},
		{
			name: "a tag of the second link holds the name as a synonym", states: twoLinks(),
			entries: map[int64][]domain.LinkVocabularyEntry{2: {{ID: 4, Name: "猫"}, {ID: 9, Name: "cat", Synonyms: []string{"ねこ"}}}},
			want:    &domain.VocabularyNameTakenError{Link: "b", Kind: domain.VocabularyTag, NewName: "ねこ", Holder: "cat"},
		},
		{
			name: "a tag here holds the name", states: twoLinks(), here: map[string]domain.TagRef{"ねこ": {ID: 8, Name: "neko"}},
			want: &domain.VocabularyNameTakenError{Kind: domain.VocabularyTag, NewName: "ねこ", Holder: "neko"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			links := &fakeVocabularyLinks{states: tc.states, entries: tc.entries}
			v, log := newVocabularyFixture(links, &fakeVocabularyTags{holders: tc.here})

			_, err := v.RenameTag(context.Background(), links, 3, "ねこ")

			if fmt.Sprint(err) != fmt.Sprint(tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			for _, call := range log.calls {
				if !slices.Contains([]string{"entries 1", "entries 2"}, call) {
					t.Errorf("calls = %q, want reads only", log.calls)
					break
				}
			}
		})
	}
}

// A link that refuses the write stops the rename there: the links before it stay renamed and this node is not.
func TestRenameTagStopsAtTheLinkThatRefuses(t *testing.T) {
	refusal := errors.New("refused")
	links := &fakeVocabularyLinks{states: twoLinks(), refuse: map[int64]error{1: refusal}}
	v, log := newVocabularyFixture(links, &fakeVocabularyTags{})

	_, err := v.RenameTag(context.Background(), links, 3, "ねこ")

	var linkErr *domain.VocabularyLinkError
	if !errors.As(err, &linkErr) || linkErr.Link != "a" || linkErr.Change == nil || linkErr.Change.NewName != "ねこ" ||
		!errors.Is(err, refusal) {
		t.Errorf("err = %#v, want link a's refusal of the rename to ねこ", err)
	}
	if want := []string{"entries 1", "entries 2", "rename 1 猫->ねこ (held)"}; !slices.Equal(log.calls, want) {
		t.Errorf("calls = %q, want %q", log.calls, want)
	}
}

// A promote goes to the links only when the name is the tag's synonym here, and a link that already applied an
// earlier promote, whose tag of the new name holds the old one as a synonym, does not refuse it.
func TestPromoteTagSynonymSendsOnlyTheTagsOwnSynonym(t *testing.T) {
	tests := []struct {
		name    string
		holder  domain.TagRef
		entries map[int64][]domain.LinkVocabularyEntry
		want    []string
	}{
		{
			name: "a synonym of the tag, applied on link 1 already", holder: domain.TagRef{ID: 3, Name: "猫"},
			entries: map[int64][]domain.LinkVocabularyEntry{1: {{ID: 4, Name: "ねこ", Synonyms: []string{"猫"}}}},
			want: []string{
				"entries 1", "entries 2", "promote 1 猫->ねこ (held)", "promote 2 猫->ねこ (held)", "promote here 3->ねこ (held)",
			},
		},
		{
			name: "a synonym of another tag", holder: domain.TagRef{ID: 8, Name: "neko"},
			want: []string{"promote here 3->ねこ (held)"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			links := &fakeVocabularyLinks{states: twoLinks(), entries: tc.entries}
			v, log := newVocabularyFixture(links, &fakeVocabularyTags{holders: map[string]domain.TagRef{"ねこ": tc.holder}})

			if _, err := v.PromoteTagSynonym(context.Background(), links, 3, "ねこ"); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(log.calls, tc.want) {
				t.Errorf("calls = %q, want %q", log.calls, tc.want)
			}
		})
	}
}

// A patch emptying a named person's names unnames it on every link before the update here; a provisional person
// has no name a link holds, so naming it only checks the new name on the links.
func TestUpdateCreditUnnamesOrChecksTheLinksBeforeTheUpdate(t *testing.T) {
	none, named := []domain.CreditName{}, []domain.CreditName{{Name: "B"}}
	tests := []struct {
		name    string
		current domain.Credit
		names   *[]domain.CreditName
		want    []string
	}{
		{
			name: "emptying a named person", current: domain.Credit{Kind: domain.CreditPerson, ID: 5, Name: "A", Names: []domain.CreditName{{Name: "A"}}},
			names: &none, want: []string{"unname 1 A->", "unname 2 A->", "update here 5"},
		},
		{
			name: "naming a provisional person", current: domain.Credit{Kind: domain.CreditPerson, ID: 5, Provisional: true},
			names: &named, want: []string{"entries 1", "entries 2", "update here 5"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log := &vocabularyLog{}
			links := &fakeVocabularyLinks{log: log, states: twoLinks()}
			credits := &fakeVocabularyCredits{log: log, current: tc.current}
			v := NewVocabularyChanges(VocabularyChangesOptions{Credits: credits})

			if _, err := v.UpdateCredit(context.Background(), links, domain.CreditPerson, 5, domain.CreditPatch{Names: tc.names}); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(log.calls, tc.want) {
				t.Errorf("calls = %q, want %q", log.calls, tc.want)
			}
		})
	}
}

// A rename of a link's entry by its remote id renames the same name on the other links only, since the forward
// renames it on the id's own link.
func TestRenameRemoteCreditRenamesTheOtherLinksOnly(t *testing.T) {
	log := &vocabularyLog{}
	links := &fakeVocabularyLinks{log: log, states: twoLinks(), entries: map[int64][]domain.LinkVocabularyEntry{
		2: {{ID: 7, Name: "A"}},
	}}
	v := NewVocabularyChanges(VocabularyChangesOptions{Credits: &fakeVocabularyCredits{log: log}})

	if err := v.RenameRemoteCredit(context.Background(), links, domain.CreditPerson, domain.RemoteID(2, 7), "B"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"entries 2", "entries 2", "entries 1", "entries 2", "rename 1 A->B"}; !slices.Equal(log.calls, want) {
		t.Errorf("calls = %q, want %q", log.calls, want)
	}
}
