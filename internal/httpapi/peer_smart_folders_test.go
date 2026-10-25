package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// smartFolderNodes pairs b with a as mergedNodes does, with smart folders on both.
func smartFolderNodes(t *testing.T) (fa, fb *guestFixture, a, b *peerNode, stopA func()) {
	t.Helper()
	return mergedNodesWith(t, func(db *store.DB, opts *Options) {
		opts.SmartFolders = db.SmartFolders()
	})
}

func createSmartFolder(t *testing.T, n *peerNode, body string) int64 {
	t.Helper()
	rec := n.owner(http.MethodPost, "/api/smart-folders", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: status = %d: %s", body, rec.Code, rec.Body)
	}
	return decode[gen.SmartFolderSummary](t, rec).Id
}

func favoriteOn(t *testing.T, f *guestFixture, videos ...string) {
	t.Helper()
	ids := make([]int64, len(videos))
	for i, video := range videos {
		ids[i] = f.ids[video]
	}
	if _, err := f.env.db.Favorites().SetFavorites(context.Background(), domain.FavoriteChange{VideoIDs: ids, Favorite: true}); err != nil {
		t.Fatal(err)
	}
}

// pageSmartFolder follows the folder's pages with limit 2 from get and returns every item's label.
func pageSmartFolder(t *testing.T, fa, fb *guestFixture, get func(string) *httptest.ResponseRecorder, id int64) []string {
	t.Helper()
	path := "/api/smart-folders/" + strconv.FormatInt(id, 10)
	rec := get(path + "?limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", path, rec.Code, rec.Body)
	}
	detail := decode[gen.SmartFolderDetail](t, rec)
	items, next := detail.Items, detail.NextCursor
	var labels []string
	for range 20 {
		if len(items) > 2 {
			t.Fatalf("%s: %d items on one page", path, len(items))
		}
		for _, item := range items {
			if item.Video == nil {
				t.Fatalf("%s: item %+v is not a video", path, item)
			}
			labels = append(labels, listLabel(t, fa, fb, item.Video.Id))
		}
		if next == nil {
			return labels
		}
		rec := get(path + "/items?limit=2&cursor=" + *next)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s/items: status = %d: %s", path, rec.Code, rec.Body)
		}
		page := decode[gen.SmartFolderItemPage](t, rec)
		if page.PeerItems != nil {
			t.Fatalf("%s/items: the screen received peerItems", path)
		}
		items, next = page.Items, page.NextCursor
	}
	t.Fatalf("%s: the folder did not end", path)
	return nil
}

func folderCards(t *testing.T, get func(string) *httptest.ResponseRecorder, ids ...int64) map[int64]gen.LibrarySmartFolder {
	t.Helper()
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = strconv.FormatInt(id, 10)
	}
	rec := get("/api/smart-folders/cards?ids=" + strings.Join(values, ","))
	if rec.Code != http.StatusOK {
		t.Fatalf("cards: status = %d: %s", rec.Code, rec.Body)
	}
	cards := map[int64]gen.LibrarySmartFolder{}
	for _, card := range decode[gen.SmartFolderCards](t, rec).Cards {
		if card.PeerRule != nil {
			t.Errorf("card %d: the screen received peerRule", card.Id)
		}
		cards[card.Id] = card
	}
	return cards
}

func TestSmartFolderListsTheTaggedVideosOfBothNodes(t *testing.T) {
	fa, fb, a, b, _ := smartFolderNodes(t)
	cat := tagOn(t, fb, "猫", "c", "e")
	peerCat := tagOn(t, fa, "猫", "b", "d")
	local := createSmartFolder(t, b, fmt.Sprintf(`{"name":"猫","rule":{"tags":[%d]}}`, cat.ID))
	owner := func(target string) *httptest.ResponseRecorder { return b.owner(http.MethodGet, target, "") }

	// The folder's addedDesc order runs e, d, c, b; each node holds two of them.
	want := []string{"b:e", "a:d", "b:c", "a:b"}
	if got := pageSmartFolder(t, fa, fb, owner, local); !slices.Equal(got, want) {
		t.Errorf("local folder = %v, want %v", got, want)
	}

	// a's folder appears in b's sidebar list and lists b's matches too, with the rule in b's ids.
	remote := domain.ToRemoteID(createSmartFolder(t, a, fmt.Sprintf(`{"name":"猫と犬","rule":{"tags":[%d]}}`, peerCat.ID)))
	list := decode[gen.SmartFolderList](t, owner("/api/smart-folders"))
	var names []string
	for _, folder := range list.SmartFolders {
		names = append(names, folder.Name+"#"+strconv.FormatBool(domain.IsRemoteID(folder.Id)))
	}
	if !slices.Equal(names, []string{"猫#false", "猫と犬#true"}) || list.SmartFolders[1].Id != remote {
		t.Errorf("sidebar = %v (%+v), want b's 猫 then a's 猫と犬 at %d", names, list.SmartFolders, remote)
	}
	if got := pageSmartFolder(t, fa, fb, owner, remote); !slices.Equal(got, want) {
		t.Errorf("remote folder = %v, want %v", got, want)
	}
	detail := decode[gen.SmartFolderDetail](t, owner("/api/smart-folders/"+strconv.FormatInt(remote, 10)))
	if detail.Settings == nil || detail.Settings.Rule.Tags == nil || !slices.Equal(*detail.Settings.Rule.Tags, []int64{cat.ID}) {
		t.Errorf("remote settings = %+v, want the rule's tag as b's %d", detail.Settings, cat.ID)
	}
	if detail.SmartFolder.PeerRule != nil {
		t.Errorf("remote detail carries peerRule %+v", detail.SmartFolder.PeerRule)
	}
}

func TestRemoteSmartFolderReadsThePeerAfterALocalFirstPage(t *testing.T) {
	fa, fb, a, b, _ := smartFolderNodes(t)
	tagOn(t, fb, "猫", "e", "d")
	peerCat := tagOn(t, fa, "猫", "c", "b")
	remote := domain.ToRemoteID(createSmartFolder(t, a, fmt.Sprintf(`{"name":"猫","rule":{"tags":[%d]}}`, peerCat.ID)))
	owner := func(target string) *httptest.ResponseRecorder { return b.owner(http.MethodGet, target, "") }

	// b's matches are newer, so the first page of two takes none of a's.
	if got, want := pageSmartFolder(t, fa, fb, owner, remote), []string{"b:e", "b:d", "a:c", "a:b"}; !slices.Equal(got, want) {
		t.Errorf("remote folder = %v, want %v", got, want)
	}
}

func TestSmartFolderShowsAGuestThePublicMatchesOfBothNodes(t *testing.T) {
	fa, fb, _, b, _ := smartFolderNodes(t)
	// Public videos are a and d on each node.
	cat := tagOn(t, fb, "猫", "a", "c")
	tagOn(t, fa, "猫", "d", "e")
	id := createSmartFolder(t, b, fmt.Sprintf(`{"name":"猫","rule":{"tags":[%d]}}`, cat.ID))

	guest := func(target string) *httptest.ResponseRecorder { return b.env.get(target) }
	if got, want := pageSmartFolder(t, fa, fb, guest, id), []string{"a:d", "b:a"}; !slices.Equal(got, want) {
		t.Errorf("guest = %v, want %v", got, want)
	}
	owner := func(target string) *httptest.ResponseRecorder { return b.owner(http.MethodGet, target, "") }
	if got, want := pageSmartFolder(t, fa, fb, owner, id), []string{"a:e", "a:d", "b:c", "b:a"}; !slices.Equal(got, want) {
		t.Errorf("owner = %v, want %v", got, want)
	}
	if card := folderCards(t, guest, id)[id]; card.VideoCount != 2 {
		t.Errorf("guest card counts %d videos, want 2", card.VideoCount)
	}
}

func TestSmartFolderCardsCountBothNodes(t *testing.T) {
	fa, fb, a, b, stopA := smartFolderNodes(t)
	cat := tagOn(t, fb, "猫", "c", "e")
	peerCat := tagOn(t, fa, "猫", "b", "d", "e")
	local := createSmartFolder(t, b, fmt.Sprintf(`{"name":"猫","rule":{"tags":[%d]}}`, cat.ID))
	remote := domain.ToRemoteID(createSmartFolder(t, a, fmt.Sprintf(`{"name":"a の猫","rule":{"tags":[%d]}}`, peerCat.ID)))
	owner := func(target string) *httptest.ResponseRecorder { return b.owner(http.MethodGet, target, "") }

	// Every video holds 4096 bytes and lasts 60 s, except d, whose probe failed.
	cards := folderCards(t, owner, local, remote)
	for _, id := range []int64{local, remote} {
		card := cards[id]
		if card.VideoCount != 5 || card.SizeBytes != 5*4096 || card.DurationMs == nil || *card.DurationMs != 4*60_000 {
			t.Errorf("card %d = %d videos, %d bytes, %v ms, want 5, %d and %d", id, card.VideoCount, card.SizeBytes,
				card.DurationMs, 5*4096, 4*60_000)
		}
	}
	if card := cards[remote]; card.WatchState != nil || card.Favorite != nil {
		t.Errorf("remote card carries the peer's watch values: %+v", card)
	}

	stopA()
	if got := pageSmartFolder(t, fa, fb, owner, local); !slices.Equal(got, []string{"b:e", "b:c"}) {
		t.Errorf("with a stopped, the folder = %v, want b's matches", got)
	}
	cards = folderCards(t, owner, local, remote)
	if card, ok := cards[local]; !ok || card.VideoCount != 2 {
		t.Errorf("with a stopped, the card = %+v, want b's 2 videos", card)
	}
	if _, ok := cards[remote]; ok {
		t.Errorf("with a stopped, a's folder still has a card")
	}
}

func TestSmartFolderWithFavoriteListsTheLocalFavoritesOnly(t *testing.T) {
	fa, fb, a, b, _ := smartFolderNodes(t)
	cat := tagOn(t, fb, "猫", "c", "e")
	peerCat := tagOn(t, fa, "猫", "c", "e")
	favoriteOn(t, fb, "c", "b")
	favoriteOn(t, fa, "e")
	owner := func(target string) *httptest.ResponseRecorder { return b.owner(http.MethodGet, target, "") }

	local := createSmartFolder(t, b, fmt.Sprintf(`{"name":"好きな猫","rule":{"favorite":true,"tags":[%d]}}`, cat.ID))
	if got := pageSmartFolder(t, fa, fb, owner, local); !slices.Equal(got, []string{"b:c"}) {
		t.Errorf("b's favorite folder = %v, want b's favorite match only", got)
	}
	// a's favorite folder lists b's favorites matching its rule instead of a's favorites.
	remote := domain.ToRemoteID(createSmartFolder(t, a,
		fmt.Sprintf(`{"name":"a の好きな猫","rule":{"favorite":true,"tags":[%d]}}`, peerCat.ID)))
	if got := pageSmartFolder(t, fa, fb, owner, remote); !slices.Equal(got, []string{"b:c"}) {
		t.Errorf("a's favorite folder = %v, want b's favorite match only", got)
	}
	cards := folderCards(t, owner, local, remote)
	for _, id := range []int64{local, remote} {
		if cards[id].VideoCount != 1 {
			t.Errorf("card %d counts %d videos, want b's 1 favorite", id, cards[id].VideoCount)
		}
	}
}

func TestPeerSmartFolderEditsAreStoredOnThePeer(t *testing.T) {
	fa, fb, a, b, _ := smartFolderNodes(t)
	cat := tagOn(t, fb, "猫")
	peerCat := tagOn(t, fa, "猫")
	parent := domain.ToRemoteID(createSmartFolder(t, a, `{"name":"親","rule":{}}`))

	// A rule by b's tag reaches a as a's tag of the same name.
	child := createSmartFolder(t, b, fmt.Sprintf(`{"name":"子","rule":{"tags":[%d]},"parentId":%d}`, cat.ID, parent))
	if !domain.IsRemoteID(child) {
		t.Fatalf("the child %d was not created on a", child)
	}
	stored := decode[gen.SmartFolderDetail](t, a.owner(http.MethodGet, "/api/smart-folders/"+strconv.FormatInt(domain.FromRemoteID(child), 10), ""))
	if stored.Settings == nil || stored.Settings.Rule.Tags == nil || !slices.Equal(*stored.Settings.Rule.Tags, []int64{peerCat.ID}) ||
		stored.SmartFolder.ParentId == nil || *stored.SmartFolder.ParentId != domain.FromRemoteID(parent) {
		t.Errorf("a stored %+v under %v, want tag %d under %d", stored.Settings, stored.SmartFolder.ParentId, peerCat.ID, domain.FromRemoteID(parent))
	}

	childPath := "/api/smart-folders/" + strconv.FormatInt(child, 10)
	rec := b.owner(http.MethodPatch, childPath, `{"name":"子猫"}`)
	if rec.Code != http.StatusOK || decode[gen.SmartFolderSummary](t, rec).Id != child {
		t.Fatalf("rename: status = %d: %s", rec.Code, rec.Body)
	}
	local := createSmartFolder(t, b, `{"name":"手元","rule":{}}`)
	rec = b.owner(http.MethodPatch, childPath, fmt.Sprintf(`{"parentId":%d}`, local))
	assertErrorBody(t, "a parent of the other node", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonMixedNodes,
	})
	if rec := b.owner(http.MethodDelete, childPath, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d: %s", rec.Code, rec.Body)
	}
	if rec := a.owner(http.MethodGet, "/api/smart-folders/"+strconv.FormatInt(domain.FromRemoteID(child), 10), ""); rec.Code != http.StatusNotFound {
		t.Errorf("after the delete, a answers %d for the child", rec.Code)
	}
}

func TestPeerRuleCountsAnswerThePeerOnly(t *testing.T) {
	_, _, _, b, _ := smartFolderNodes(t)
	rec := b.owner(http.MethodPost, "/api/peer/rule-counts", `{"rules":[{}]}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("the screens' rule-counts: status = %d, want 404", rec.Code)
	}
}
