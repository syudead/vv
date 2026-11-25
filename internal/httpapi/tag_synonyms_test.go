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

func TestSynonymOperationsAnswerTheirErrors(t *testing.T) {
	f := newGuestFixture(t, true)
	cat := tagOn(t, f, "猫")
	addSynonymOn(t, f, cat.ID, "ねこ")
	catID := strconv.FormatInt(cat.ID, 10)

	cases := []struct {
		label, target, body string
		want                wantError
	}{
		{"move to its own tag", tagPath(cat.ID, "/synonyms/move"), `{"name":"ねこ","targetId":` + catID + `}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonMoveSameTag}},
		{"promote the canonical name", tagPath(cat.ID, "/synonyms/promote"), `{"name":"猫"}`,
			wantError{status: http.StatusConflict, code: gen.ErrorCodeTagSynonymNotFound}},
		{"detach an empty name", tagPath(cat.ID, "/synonyms/detach"), `{"name":" "}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonTagNameEmpty}},
		{"promote on a remote id", tagPath(domain.RemoteID(1, cat.ID), "/synonyms/promote"), `{"name":"ねこ"}`,
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeTagNotFound}},
	}
	for _, tc := range cases {
		rec := f.ownerRequest(http.MethodPost, tc.target, tc.body)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), tc.want)
	}
	if tag, _ := tagNamed(t, f, "猫"); tag.Name != "猫" || !slices.Equal(tag.Synonyms, []string{"ねこ"}) {
		t.Errorf("猫 = %+v, want it unchanged", tag)
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
	// a's 猫 lacks the synonym ねこ, so it is renamed and keeps its own synonym.
	if tag, _ := tagNamed(t, fa, "ねこ"); tag.Name != "ねこ" || !slices.Equal(tag.Synonyms, []string{"にゃんこ"}) {
		t.Errorf("a's tag = %+v, want ねこ with にゃんこ", tag)
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
