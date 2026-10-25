package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// tagNamed reports f's tag holding name as its canonical name or a synonym, with its video count.
func tagNamed(t *testing.T, f *guestFixture, name string) (domain.Tag, bool) {
	t.Helper()
	page, err := f.env.db.Tags().ListTags(context.Background(), domain.TagListQuery{Sort: domain.TagSortName})
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range page.Items {
		if tag.Name == name || slices.Contains(tag.Synonyms, name) {
			return tag, true
		}
	}
	return domain.Tag{}, false
}

func TestRenameATagRenamesItOnBothNodes(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	cat := tagOn(t, fb, "猫", "c")
	tagOn(t, fa, "猫", "d")

	rec := b.owner(http.MethodPatch, tagPath(cat.ID, ""), `{"name":"ねこ"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: status = %d: %s", rec.Code, rec.Body)
	}
	for node, f := range map[string]*guestFixture{"a": fa, "b": fb} {
		if _, found := tagNamed(t, f, "ねこ"); !found {
			t.Errorf("%s has no tag ねこ", node)
		}
		if _, found := tagNamed(t, f, "猫"); found {
			t.Errorf("%s still has 猫", node)
		}
	}
	owner := func(target string) *httptest.ResponseRecorder { return b.owner(http.MethodGet, target, "") }
	got, _ := pageVideos(t, fa, fb, owner, "/api/videos", url.Values{"tag": {strconv.FormatInt(cat.ID, 10)}, "sort": {"titleAsc"}})
	if want := []string{"a:d", "b:c"}; !slices.Equal(sortedCopy(got), want) {
		t.Errorf("videos of ねこ = %v, want %v", got, want)
	}
}

func sortedCopy(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return out
}

func TestRenameToANameOnlyThePeerHoldsChangesNothing(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	cat := tagOn(t, fb, "猫")
	tagOn(t, fa, "猫")
	tagOn(t, fa, "犬")

	rec := b.owner(http.MethodPatch, tagPath(cat.ID, ""), `{"name":"犬"}`)
	assertErrorBody(t, "相手だけにある名前", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeTagNameTaken, reason: gen.ErrorReasonNameIsTag, tagName: "犬",
	})
	for node, f := range map[string]*guestFixture{"a": fa, "b": fb} {
		if _, found := tagNamed(t, f, "猫"); !found {
			t.Errorf("%s lost 猫", node)
		}
	}
	if _, found := tagNamed(t, fb, "犬"); found {
		t.Error("b gained 犬")
	}
}

func TestRenameWithTheStoppedPeerChangesNothing(t *testing.T) {
	fa, fb, b, stopA := mergedNodes(t)
	cat := tagOn(t, fb, "猫")
	tagOn(t, fa, "猫")
	stopA()

	rec := b.owner(http.MethodPatch, tagPath(cat.ID, ""), `{"name":"ねこ"}`)
	assertErrorBody(t, "止まった相手", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonPeerUnreachable,
	})
	if _, found := tagNamed(t, fb, "猫"); !found {
		t.Error("b renamed 猫 although the peer was stopped")
	}
}

func TestMergeTagsMergesTheSameNamesOnThePeerAndCreatesTheDestination(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	cat := tagOn(t, fb, "猫", "c")
	kitten := tagOn(t, fb, "子猫", "d")
	tagOn(t, fa, "子猫", "c")
	// 仔猫 is a's alone, so b lists it with the offset.
	peerOnly := tagOn(t, fa, "仔猫", "d")

	body := `{"sourceIds":[` + strconv.FormatInt(kitten.ID, 10) + `,` + strconv.FormatInt(domain.ToRemoteID(peerOnly.ID), 10) + `]}`
	rec := b.owner(http.MethodPost, tagPath(cat.ID, "/merge"), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("merge: status = %d: %s", rec.Code, rec.Body)
	}
	if merged := decode[gen.TagMergeResponse](t, rec); merged.Tag.Id != cat.ID || len(merged.NotFoundIds) != 0 {
		t.Errorf("merge = %+v, want b's 猫 and no missing sources", merged)
	}
	destination, found := tagNamed(t, fa, "猫")
	if !found || destination.VideoCount != 2 || destination.Tentative {
		t.Errorf("a's 猫 = %+v (found %v), want a confirmed tag on both of a's videos", destination, found)
	}
	for node, f := range map[string]*guestFixture{"a": fa, "b": fb} {
		for _, name := range []string{"子猫", "仔猫"} {
			if tag, found := tagNamed(t, f, name); found && tag.Name == name {
				t.Errorf("%s still has the tag %s", node, name)
			}
		}
	}
	if tag, _ := tagNamed(t, fb, "猫"); tag.VideoCount != 2 {
		t.Errorf("b's 猫 counts %d videos, want 2", tag.VideoCount)
	}
}

func TestDeleteAPersonAndARemoteOnlyTagOnBothNodes(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	ctx := context.Background()
	create := func(f *guestFixture, name string) domain.Credit {
		credit, err := f.env.db.Credits().Create(ctx, domain.CreditPerson, domain.CreditInput{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return credit
	}
	sato := create(fb, "佐藤")
	create(fa, "佐藤")
	create(fa, "鈴木")

	if rec := b.owner(http.MethodDelete, "/api/people/"+strconv.FormatInt(sato.ID, 10), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d: %s", rec.Code, rec.Body)
	}
	for node, f := range map[string]*guestFixture{"a": fa, "b": fb} {
		page, err := f.env.db.Credits().List(ctx, domain.CreditPerson, domain.CreditQuery{})
		if err != nil {
			t.Fatal(err)
		}
		for _, person := range page.Items {
			if person.Name == "佐藤" {
				t.Errorf("%s still has 佐藤", node)
			}
		}
	}
	dog := tagOn(t, fa, "犬")
	if rec := b.owner(http.MethodDelete, tagPath(domain.ToRemoteID(dog.ID), ""), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete the remote tag: status = %d: %s", rec.Code, rec.Body)
	}
	if _, found := tagNamed(t, fa, "犬"); found {
		t.Error("a still has 犬")
	}
}

func TestMergeARemoteOnlyPersonCreatesTheDestinationOnThePeer(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	ctx := context.Background()
	sato, err := fb.env.db.Credits().Create(ctx, domain.CreditPerson, domain.CreditInput{Name: "佐藤"})
	if err != nil {
		t.Fatal(err)
	}
	suzuki, err := fa.env.db.Credits().Create(ctx, domain.CreditPerson, domain.CreditInput{Name: "鈴木"})
	if err != nil {
		t.Fatal(err)
	}

	body := `{"sourceIds":[` + strconv.FormatInt(domain.ToRemoteID(suzuki.ID), 10) + `]}`
	rec := b.owner(http.MethodPost, "/api/people/"+strconv.FormatInt(sato.ID, 10)+"/merge", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("merge: status = %d: %s", rec.Code, rec.Body)
	}
	if merged := decode[gen.PersonMergeResponse](t, rec); merged.Person.Id != sato.ID || len(merged.NotFoundIds) != 0 {
		t.Errorf("merge = %+v, want b's 佐藤 and no missing sources", merged)
	}
	page, err := fa.env.db.Credits().List(ctx, domain.CreditPerson, domain.CreditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, person := range page.Items {
		names = append(names, person.Name)
	}
	if want := []string{"佐藤"}; !slices.Equal(names, want) {
		t.Errorf("a's people = %v, want %v with 鈴木 merged into it", names, want)
	}
}

func TestASynonymStaysOnItsNode(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	cat := tagOn(t, fb, "猫")
	tagOn(t, fa, "猫")

	if rec := b.owner(http.MethodPost, tagPath(cat.ID, "/synonyms"), `{"name":"にゃんこ"}`); rec.Code != http.StatusOK {
		t.Fatalf("synonym: status = %d: %s", rec.Code, rec.Body)
	}
	if _, found := tagNamed(t, fb, "にゃんこ"); !found {
		t.Error("b has no synonym にゃんこ")
	}
	if _, found := tagNamed(t, fa, "にゃんこ"); found {
		t.Error("the synonym reached a")
	}
}
