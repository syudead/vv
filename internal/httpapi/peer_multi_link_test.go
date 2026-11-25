package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// threeNodes makes b read a in slot 1 and c in slot 2, each node holding the guest fixture. stopC stops c's server.
func threeNodes(t *testing.T) (fa, fb, fc *guestFixture, b *peerNode, stopC func()) {
	t.Helper()
	fa, a, _ := mergedNode(t, "node-a", nil)
	fc, c, stopC := mergedNode(t, "node-c", nil)
	fb, b, _ = mergedNode(t, "node-b", nil)
	pairNodes(t, a, b)
	pairNodes(t, c, b)
	return fa, fb, fc, b, stopC
}

// downLink stops c's server and has b record the failure, as a failed request to c does.
func downLink(t *testing.T, b *peerNode, stopC func()) {
	t.Helper()
	stopC()
	b.pairing.ReportFailure(context.Background(), 2, errors.New("stopped"))
	if state := decode[gen.PeerState](t, b.owner(http.MethodGet, "/api/peer", "")); len(state.Links) != 2 ||
		state.Links[1].State != gen.PeerLinkStateStateUnreachable {
		t.Fatalf("b's links = %+v, want node-c unreachable", state.Links)
	}
}

func tagListOf(t *testing.T, n *peerNode) []gen.Tag {
	t.Helper()
	rec := n.owner(http.MethodGet, "/api/tags?"+url.Values{"limit": {"100"}}.Encode(), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/tags: status = %d: %s", rec.Code, rec.Body)
	}
	return decode[gen.TagList](t, rec).Items
}

// A name every node holds is one entry with the summed count, and a name only links hold takes the id of the
// first link in slot order.
func TestTagListJoinsEveryLinkByName(t *testing.T) {
	fa, fb, fc, b, _ := threeNodes(t)
	cat := tagOn(t, fb, "猫", "a")
	tagOn(t, fa, "猫", "a")
	tagOn(t, fc, "猫", "a", "b")
	dog := tagOn(t, fa, "犬", "a")
	tagOn(t, fc, "犬", "b")

	byName := map[string][]gen.Tag{}
	for _, tag := range tagListOf(t, b) {
		byName[tag.Name] = append(byName[tag.Name], tag)
	}
	if got := byName["猫"]; len(got) != 1 || got[0].Id != cat.ID || got[0].VideoCount != 4 {
		t.Errorf("猫 = %+v, want one entry with b's id %d and 4 videos", got, cat.ID)
	}
	if got := byName["犬"]; len(got) != 1 || got[0].Id != domain.RemoteID(1, dog.ID) || got[0].VideoCount != 2 {
		t.Errorf("犬 = %+v, want one entry with a's id in slot 1 and 2 videos", got)
	}
}

// While one link is down a rename changes no node and names that link.
func TestRenameWithALinkDownChangesNoNodeAndNamesIt(t *testing.T) {
	fa, fb, fc, b, stopC := threeNodes(t)
	cat := tagOn(t, fb, "猫")
	tagOn(t, fa, "猫")
	tagOn(t, fc, "猫")
	downLink(t, b, stopC)

	rec := b.owner(http.MethodPatch, tagPath(cat.ID, ""), `{"name":"ねこ"}`)
	assertErrorBody(t, "止まったリンク", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonPeerUnreachable, peer: "node-c",
	})
	for node, f := range map[string]*guestFixture{"a": fa, "b": fb} {
		if _, found := tagNamed(t, f, "猫"); !found {
			t.Errorf("%s renamed 猫 although node-c was down", node)
		}
	}
}

// A rename reaches every link in slot order before this node. A new name a later link holds is refused, naming that
// link, before any node changes, so the same rename succeeds once the name is free there.
func TestRenameReachesEveryLinkBeforeThisNode(t *testing.T) {
	fa, fb, fc, b, _ := threeNodes(t)
	cat := tagOn(t, fb, "猫")
	tagOn(t, fa, "猫")
	tagOn(t, fc, "猫")
	taken := tagOn(t, fc, "にゃん")

	if rec := b.owner(http.MethodPatch, tagPath(cat.ID, ""), `{"name":"ねこ"}`); rec.Code != http.StatusOK {
		t.Fatalf("rename: status = %d: %s", rec.Code, rec.Body)
	}
	for node, f := range map[string]*guestFixture{"a": fa, "b": fb, "c": fc} {
		if _, found := tagNamed(t, f, "ねこ"); !found {
			t.Errorf("%s has no tag ねこ", node)
		}
	}

	rec := b.owner(http.MethodPatch, tagPath(cat.ID, ""), `{"name":"にゃん"}`)
	assertErrorBody(t, "node-c が断る", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeTagNameTaken, reason: gen.ErrorReasonNameIsTag, tagName: "にゃん",
		peer: "node-c",
	})
	for node, f := range map[string]*guestFixture{"a": fa, "b": fb} {
		if _, found := tagNamed(t, f, "ねこ"); !found {
			t.Errorf("%s renamed ねこ although node-c refused", node)
		}
	}

	if err := fc.env.db.Tags().DeleteTag(context.Background(), taken.ID); err != nil {
		t.Fatal(err)
	}
	if rec := b.owner(http.MethodPatch, tagPath(cat.ID, ""), `{"name":"にゃん"}`); rec.Code != http.StatusOK {
		t.Fatalf("repeated rename: status = %d: %s", rec.Code, rec.Body)
	}
	for node, f := range map[string]*guestFixture{"a": fa, "b": fb, "c": fc} {
		if _, found := tagNamed(t, f, "にゃん"); !found {
			t.Errorf("%s has no tag にゃん after the repeated rename", node)
		}
	}
}

// A rename of a tag only links hold, addressed by the first link's id, renames it on the other link too.
func TestRenameARemoteOnlyTagRenamesItOnEveryLink(t *testing.T) {
	fa, _, fc, b, _ := threeNodes(t)
	dog := tagOn(t, fa, "犬")
	tagOn(t, fc, "犬")

	rec := b.owner(http.MethodPatch, tagPath(domain.RemoteID(1, dog.ID), ""), `{"name":"いぬ"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: status = %d: %s", rec.Code, rec.Body)
	}
	for node, f := range map[string]*guestFixture{"a": fa, "c": fc} {
		if _, found := tagNamed(t, f, "いぬ"); !found {
			t.Errorf("%s has no tag いぬ", node)
		}
	}
}

// A selection over both links applies each link's part and sums what they applied.
func TestSelectionTagEditAppliesOnEveryLink(t *testing.T) {
	fa, _, fc, b, _ := threeNodes(t)
	first, second := domain.RemoteID(1, fa.ids["a"]), domain.RemoteID(2, fc.ids["a"])

	got := updateVideoTags(t, b, fmt.Sprintf(`{"action":"add","tag":{"name":"星"},"videoIds":[%d,%d]}`, first, second))
	if got.Applied != 2 || got.Remote == nil || got.Remote.Applied != 2 || len(got.Remote.FailedVideoIds) != 0 {
		t.Errorf("response = %+v, remote %+v; want 2 applied, both on the links", got, got.Remote)
	}
	for node, f := range map[string]*guestFixture{"a": fa, "c": fc} {
		tag, found := tagNamed(t, f, "星")
		if !found || tag.VideoCount != 1 {
			t.Errorf("%s's 星 = %+v (found %v), want it on video a", node, tag, found)
		}
	}
}

// A selection's summaries give a name only links hold one entry with the count of every link's videos, as the tag
// list does.
func TestSelectionSummariesJoinLinksByName(t *testing.T) {
	fa, _, fc, b, _ := threeNodes(t)
	dog := tagOn(t, fa, "犬", "a")
	tagOn(t, fc, "犬", "a")
	body := fmt.Sprintf(`{"videoIds":[%d,%d]}`, domain.RemoteID(1, fa.ids["a"]), domain.RemoteID(2, fc.ids["a"]))
	want := domain.RemoteID(1, dog.ID)

	rec := b.owner(http.MethodPost, "/api/video-tags/summary", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("tag summary: status = %d: %s", rec.Code, rec.Body)
	}
	if got := summaryItemsNamed(decode[gen.VideoTagsSummary](t, rec).Items, "犬"); len(got) != 1 || got[0].Tag.Id != want ||
		got[0].Count != 2 {
		t.Errorf("tag summary's 犬 = %+v, want one entry with a's id in slot 1 and 2 videos", got)
	}
	rec = b.owner(http.MethodPost, "/api/video-edits/summary", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit summary: status = %d: %s", rec.Code, rec.Body)
	}
	if got := summaryItemsNamed(decode[gen.VideoEditSummary](t, rec).Tags, "犬"); len(got) != 1 || got[0].Tag.Id != want ||
		got[0].Count != 2 {
		t.Errorf("edit summary's 犬 = %+v, want one entry with a's id in slot 1 and 2 videos", got)
	}
}

func summaryItemsNamed(items []gen.VideoTagsSummaryItem, name string) []gen.VideoTagsSummaryItem {
	return slices.DeleteFunc(slices.Clone(items), func(item gen.VideoTagsSummaryItem) bool { return item.Tag.Name != name })
}

// A group belongs to one node, so a group of two links' videos is refused.
func TestGroupOfTwoLinksIsRefused(t *testing.T) {
	fa, _, fc, b, _ := threeNodes(t)

	rec := b.owner(http.MethodPost, "/api/groups", fmt.Sprintf(`{"name":"x","videoIds":[%d,%d]}`,
		domain.RemoteID(1, fa.ids["a"]), domain.RemoteID(2, fc.ids["a"])))
	assertErrorBody(t, "2 本のリンクにまたがるグループ", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonMixedNodes,
	})
}

// A clear reaches every link before this node; while one link is down no node changes.
func TestWatchHistoryClearReachesEveryLink(t *testing.T) {
	histories := []*fakeWatchHistory{{}, {}, {}}
	withHistory := func(h *fakeWatchHistory) func(*store.DB, *Options) {
		return func(_ *store.DB, opts *Options) { opts.WatchHistory = h }
	}
	_, a, _ := mergedNode(t, "node-a", withHistory(histories[0]))
	_, c, stopC := mergedNode(t, "node-c", withHistory(histories[2]))
	_, b, _ := mergedNode(t, "node-b", withHistory(histories[1]))
	pairNodes(t, a, b)
	pairNodes(t, c, b)

	assertStatus(t, "clear", b.owner(http.MethodDelete, "/api/watch-history", ""), http.StatusNoContent)
	for i, h := range histories {
		if h.clears != 1 {
			t.Errorf("node %d cleared %d times, want 1", i, h.clears)
		}
	}

	downLink(t, b, stopC)
	down := b.owner(http.MethodDelete, "/api/watch-history", "")
	assertErrorBody(t, "clear with node-c down", down.Code, down.Body.Bytes(), wantError{
		status: http.StatusServiceUnavailable, code: gen.ErrorCodeServiceUnavailable, reason: gen.ErrorReasonPeerUnreachable,
	})
	if histories[0].clears != 1 || histories[1].clears != 1 {
		t.Errorf("with node-c down, a cleared %d times and b %d, want 1 and 1", histories[0].clears, histories[1].clears)
	}
}

// Emptying the names of a person only links hold, addressed by the first link's id, unnames it on the other link too.
func TestEmptyingARemoteOnlyPersonsNamesUnnamesItOnEveryLink(t *testing.T) {
	fa, _, fc, b, _ := threeNodes(t)
	sato := fa.createWorkInfoPerson(t, "佐藤")
	fc.createWorkInfoPerson(t, "佐藤")

	rec := b.owner(http.MethodPatch, creditPath("/api/people", domain.RemoteID(1, sato), ""), `{"names":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("names []: status = %d: %s", rec.Code, rec.Body)
	}
	for node, f := range map[string]*guestFixture{"a": fa, "c": fc} {
		if got, want := personNamesOn(t, f), []string{""}; !slices.Equal(got, want) {
			t.Errorf("%s's people = %v, want one provisional person", node, got)
		}
	}
}

// A forwarded patch is read as its own link reads it: one the link refuses leaves the other link's person named, and
// null names empty the list on every link as [] does.
func TestForwardedNamesPatchReachesOtherLinksAsItsLinkReadsIt(t *testing.T) {
	fa, _, fc, b, _ := threeNodes(t)
	sato := fa.createWorkInfoPerson(t, "佐藤")
	fc.createWorkInfoPerson(t, "佐藤")
	path := creditPath("/api/people", domain.RemoteID(1, sato), "")

	refused := b.owner(http.MethodPatch, path, `{"name":"鈴木","names":[]}`)
	assertErrorBody(t, "name with names", refused.Code, refused.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
	})
	for node, f := range map[string]*guestFixture{"a": fa, "c": fc} {
		if got, want := personNamesOn(t, f), []string{"佐藤"}; !slices.Equal(got, want) {
			t.Errorf("after the refused patch, %s's people = %v, want %v", node, got, want)
		}
	}

	if rec := b.owner(http.MethodPatch, path, `{"names":null}`); rec.Code != http.StatusOK {
		t.Fatalf("names null: status = %d: %s", rec.Code, rec.Body)
	}
	for node, f := range map[string]*guestFixture{"a": fa, "c": fc} {
		if got, want := personNamesOn(t, f), []string{""}; !slices.Equal(got, want) {
			t.Errorf("after names null, %s's people = %v, want one provisional person", node, got)
		}
	}
}

// The joined list shows a name once with the first link's id, so a forwarded parent of another link goes to the
// edited link's organization of that name; one the edited link lacks is refused before any link is renamed.
func TestForwardedOrganizationParentJoinsLinksByNameBeforeAnyRename(t *testing.T) {
	fa, fb, fc, b, _ := threeNodes(t)
	ctx := context.Background()
	organization := func(f *guestFixture, name string) int64 {
		credit, err := f.env.db.Credits().Create(ctx, domain.CreditOrganization, domain.CreditInput{Names: namesOf(name)})
		if err != nil {
			t.Fatal(err)
		}
		return credit.ID
	}
	studioA, groupA, soloA := organization(fa, "Studio"), organization(fa, "Group"), organization(fa, "Solo")
	studioC, groupC := organization(fc, "Studio"), organization(fc, "Group")
	localOnly := organization(fb, "Local")
	onC := func() (string, int64) {
		credit, err := fc.env.db.Credits().Get(ctx, domain.CreditOrganization, studioC)
		if err != nil {
			t.Fatal(err)
		}
		if credit.Parent == nil {
			return credit.Name, 0
		}
		return credit.Name, credit.Parent.ID
	}
	pathC := creditPath("/api/organizations", domain.RemoteID(2, studioC), "")

	rec := b.owner(http.MethodPatch, pathC, fmt.Sprintf(`{"parentId":%d}`, domain.RemoteID(1, groupA)))
	if rec.Code != http.StatusOK {
		t.Fatalf("node-a's Group as node-c's parent: status = %d: %s", rec.Code, rec.Body)
	}
	if _, parent := onC(); parent != groupC {
		t.Fatalf("node-c's parent = %d, want node-c's Group %d", parent, groupC)
	}

	for _, tc := range []struct {
		name string
		path string
		body string
	}{
		{"a parent only node-a has", pathC, fmt.Sprintf(`{"parentId":%d}`, domain.RemoteID(1, soloA))},
		{"a rename with a parent node-a lacks", creditPath("/api/organizations", domain.RemoteID(1, studioA), ""),
			fmt.Sprintf(`{"names":[{"name":"Studio One"}],"parentId":%d}`, localOnly)},
	} {
		rec := b.owner(http.MethodPatch, tc.path, tc.body)
		assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonOrganizationNotFound})
		if got := decode[gen.Error](t, rec).Field; got == nil || *got != "parentId" {
			t.Errorf("%s: field = %v, want parentId", tc.name, got)
		}
		if name, parent := onC(); name != "Studio" || parent != groupC {
			t.Errorf("%s: node-c's Studio = %q under %d, want Studio under %d", tc.name, name, parent, groupC)
		}
	}
	if credit, err := fa.env.db.Credits().Get(ctx, domain.CreditOrganization, studioA); err != nil || credit.Name != "Studio" {
		t.Errorf("node-a's Studio = %+v, %v; want it unrenamed", credit, err)
	}
}
