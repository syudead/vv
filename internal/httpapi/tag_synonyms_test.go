package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func addSynonymOn(t *testing.T, f *guestFixture, id int64, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := f.env.db.Tags().AddSynonym(context.Background(), id, name, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGetTagAnswersTheTagListTagsGives(t *testing.T) {
	f := newGuestFixture(t, true)
	cat := tagOn(t, f, "猫", "a", "c")
	addSynonymOn(t, f, cat.ID, "ねこ")
	kindOn(t, f, "動物", cat.ID)

	list := f.ownerRequest(http.MethodGet, "/api/tags", "")
	listed, found := findTag(decode[gen.TagList](t, list).Items, cat.ID)
	if !found {
		t.Fatalf("listTags has no tag %d: %s", cat.ID, list.Body)
	}
	rec := f.ownerRequest(http.MethodGet, tagPath(cat.ID, ""), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("getTag: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Tag](t, rec); !reflect.DeepEqual(got, listed) {
		t.Errorf("getTag = %+v, want listTags' %+v", got, listed)
	}

	for _, id := range []int64{999_999, domain.RemoteID(1, cat.ID)} {
		rec := f.ownerRequest(http.MethodGet, tagPath(id, ""), "")
		assertErrorBody(t, "getTag "+strconv.FormatInt(id, 10), rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusNotFound, code: gen.ErrorCodeTagNotFound,
		})
	}
}

// Each operation maps its store error to the contract's answer, and a remote id answers 404 without calling the store.
func TestSynonymOperationsAnswerTheirErrors(t *testing.T) {
	_, emptyName := domain.NormalizeTagName(" ")
	cases := []struct {
		label, target, body string
		err                 error
		wantCall            string
		want                wantError
	}{
		{"move to its own tag", "/api/tags/3/synonyms/move", `{"name":"ねこ","targetId":3}`, domain.ErrMoveSameTag, "move",
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonMoveSameTag}},
		{"promote a name that is no synonym", "/api/tags/3/synonyms/promote", `{"name":"猫"}`, domain.ErrTagSynonymNotFound,
			"promote", wantError{status: http.StatusConflict, code: gen.ErrorCodeTagSynonymNotFound}},
		{"detach from a missing tag", "/api/tags/3/synonyms/detach", `{"name":"ねこ"}`, domain.ErrTagNotFound, "detach",
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeTagNotFound}},
		{"detach an empty name", "/api/tags/3/synonyms/detach", `{"name":" "}`,
			emptyName, "detach",
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonTagNameEmpty}},
		{"promote on a remote id", tagPath(domain.RemoteID(1, 3), "/synonyms/promote"), `{"name":"ねこ"}`, nil, "",
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeTagNotFound}},
	}
	for _, tc := range cases {
		fake := &fakeTags{err: tc.err}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, tc.target, tc.body)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), tc.want)
		if fake.operation != tc.wantCall {
			t.Errorf("%s: store call = %q, want %q", tc.label, fake.operation, tc.wantCall)
		}
	}
}

func TestPromoteChangesTheCanonicalNameOnBothNodesAndMoveStaysLocal(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	cat := tagOn(t, fb, "猫")
	addSynonymOn(t, fb, cat.ID, "ねこ", "にゃんこ")
	dog := tagOn(t, fb, "犬")
	peerCat := tagOn(t, fa, "猫")
	addSynonymOn(t, fa, peerCat.ID, "にゃんこ")

	rec := b.owner(http.MethodPost, tagPath(cat.ID, "/synonyms/promote"), `{"name":"ねこ"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("promote: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Tag](t, rec); got.Name != "ねこ" || !slices.Equal(got.Synonyms, []string{"にゃんこ", "猫"}) {
		t.Errorf("promote = %+v, want ねこ with にゃんこ and 猫", got)
	}
	// a's 猫 lacks the synonym ねこ, so it gains ねこ as its canonical name and keeps 猫 and its own synonym.
	if tag, _ := tagNamed(t, fa, "ねこ"); tag.Name != "ねこ" || !slices.Equal(tag.Synonyms, []string{"にゃんこ", "猫"}) {
		t.Errorf("a's tag = %+v, want ねこ with にゃんこ and 猫", tag)
	}

	body := `{"name":"にゃんこ","targetId":` + strconv.FormatInt(dog.ID, 10) + `}`
	rec = b.owner(http.MethodPost, tagPath(cat.ID, "/synonyms/move"), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("move: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.TagSynonymMoveResponse](t, rec); !slices.Equal(got.Target.Synonyms, []string{"にゃんこ"}) {
		t.Errorf("move target = %+v, want 犬 with にゃんこ", got.Target)
	}
	if tag, _ := tagNamed(t, fa, "にゃんこ"); tag.Name != "ねこ" {
		t.Errorf("a's にゃんこ is on %+v, want it to stay on ねこ", tag)
	}
}

// A promote that changed the link but stopped before the local write can be repeated, and joins the tag again.
func TestPromoteRepeatsAfterOnlyTheLinkChanged(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	cat := tagOn(t, fb, "猫")
	addSynonymOn(t, fb, cat.ID, "ねこ")
	tagOn(t, fa, "猫")
	if _, err := fa.env.db.Tags().ApplyTagByName(context.Background(), domain.VocabularyChange{
		Kind: domain.VocabularyTag, Action: domain.VocabularyPromote, Name: "猫", NewName: "ねこ",
	}); err != nil {
		t.Fatal(err)
	}

	rec := b.owner(http.MethodPost, tagPath(cat.ID, "/synonyms/promote"), `{"name":"ねこ"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("repeated promote: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Tag](t, rec); got.Name != "ねこ" {
		t.Errorf("b's tag = %+v, want ねこ", got)
	}
	if tag, _ := tagNamed(t, fa, "猫"); tag.Name != "ねこ" || !slices.Equal(tag.Synonyms, []string{"猫"}) {
		t.Errorf("a's tag = %+v, want ねこ with 猫", tag)
	}
}

func TestPromoteWithTheStoppedPeerChangesNothing(t *testing.T) {
	fa, fb, b, stopA := mergedNodes(t)
	cat := tagOn(t, fb, "猫")
	addSynonymOn(t, fb, cat.ID, "ねこ")
	tagOn(t, fa, "猫")
	stopA()

	rec := b.owner(http.MethodPost, tagPath(cat.ID, "/synonyms/promote"), `{"name":"ねこ"}`)
	assertErrorBody(t, "止まった相手", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonPeerUnreachable, peer: "node-a",
	})
	if tag, _ := tagNamed(t, fb, "猫"); tag.Name != "猫" {
		t.Errorf("b's tag = %+v, want 猫 unchanged", tag)
	}
}
