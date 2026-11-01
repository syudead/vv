package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// mergedNodes pairs two nodes that each hold the guest fixture; b lists a's videos beside its own. stopA stops
// a's server.
func mergedNodes(t *testing.T) (fa, fb *guestFixture, b *peerNode, stopA func()) {
	t.Helper()
	fa, fb, _, b, stopA = mergedNodesWith(t, nil)
	return fa, fb, b, stopA
}

// mergedNodesWith is mergedNodes with extend applied to both nodes' options.
func mergedNodesWith(t *testing.T, extend func(db *store.DB, opts *Options)) (fa, fb *guestFixture, a, b *peerNode, stopA func()) {
	t.Helper()
	node := func(name string) (*guestFixture, *peerNode, func()) {
		var pairing *app.Pairing
		f := newGuestFixtureWith(t, true, func(db *store.DB, opts *Options) {
			pairing = withTestPairing(db, name, nil, opts)
			opts.PersonImages = app.NewPersonImages(db.Credits(), app.NewIngest(app.IngestOptions{}))
			if extend != nil {
				extend(db, opts)
			}
		})
		server := httptest.NewServer(f.env.handler)
		t.Cleanup(server.Close)
		return f, &peerNode{env: f.env, url: server.URL, cookie: f.owner, pairing: pairing}, server.Close
	}
	fa, a, stopA = node("node-a")
	fb, b, _ = node("node-b")
	pairNodes(t, a, b)
	return fa, fb, a, b, stopA
}

// listLabel names a listed video by node and title, such as "a:e" for a's video e seen from b.
func listLabel(t *testing.T, fa, fb *guestFixture, id int64) string {
	t.Helper()
	for node, f := range map[string]*guestFixture{"a": fa, "b": fb} {
		for name, own := range f.ids {
			if (node == "b" && id == own) || (node == "a" && id == domain.ToRemoteID(own)) {
				return node + ":" + name
			}
		}
	}
	t.Fatalf("unknown id %d", id)
	return ""
}

// pageVideos follows nextCursor from path with limit 2 and returns every listed video's label with the totals.
func pageVideos(t *testing.T, fa, fb *guestFixture, get func(string) *httptest.ResponseRecorder, path string, query url.Values) ([]string, []int) {
	t.Helper()
	query.Set("limit", "2")
	var labels []string
	var totals []int
	for range 20 {
		rec := get(path + "?" + query.Encode())
		if rec.Code != http.StatusOK {
			t.Fatalf("%s?%s: status = %d: %s", path, query.Encode(), rec.Code, rec.Body)
		}
		page := decode[gen.VideoPage](t, rec)
		if len(page.Items) > 2 || page.PeerItems != nil {
			t.Fatalf("%s: %d items, peerItems %v", path, len(page.Items), page.PeerItems)
		}
		for _, video := range page.Items {
			labels = append(labels, listLabel(t, fa, fb, video.Id))
		}
		totals = append(totals, page.Total)
		if page.NextCursor == nil {
			return labels, totals
		}
		query.Set("cursor", *page.NextCursor)
	}
	t.Fatalf("%s: the list did not end", path)
	return nil, nil
}

func TestMergedVideoListPagesBothNodesOnce(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	owner := func(target string) *httptest.ResponseRecorder { return b.owner(http.MethodGet, target, "") }

	// Both nodes hold e as the newest and a as the oldest; on equal values the local video comes first.
	for _, tc := range []struct {
		sort string
		want []string
	}{
		{"addedDesc", []string{"b:e", "a:e", "b:d", "a:d", "b:c", "a:c", "b:b", "a:b", "b:a", "a:a"}},
		{"titleAsc", []string{"b:a", "a:a", "b:b", "a:b", "b:c", "a:c", "b:d", "a:d", "b:e", "a:e"}},
	} {
		got, totals := pageVideos(t, fa, fb, owner, "/api/videos", url.Values{"sort": {tc.sort}})
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s = %v, want %v", tc.sort, got, tc.want)
		}
		if slices.ContainsFunc(totals, func(total int) bool { return total != 10 }) {
			t.Errorf("%s: totals = %v, want 10 on every page", tc.sort, totals)
		}
	}

	got, _ := pageVideos(t, fa, fb, owner, "/api/videos", url.Values{"sort": {"random"}, "seed": {"7"}})
	want := []string{"a:a", "a:b", "a:c", "a:d", "a:e", "b:a", "b:b", "b:c", "b:d", "b:e"}
	if sorted := slices.Sorted(slices.Values(got)); !slices.Equal(sorted, want) {
		t.Errorf("random = %v, want each of %v once", got, want)
	}
	again, _ := pageVideos(t, fa, fb, owner, "/api/videos", url.Values{"sort": {"random"}, "seed": {"7"}})
	if !slices.Equal(again, got) {
		t.Errorf("random with the same seed = %v, then %v", got, again)
	}
}

func TestMergedLibrarySumsTotalsAndKeepsTheGuestsAudience(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)

	rec := b.owner(http.MethodGet, "/api/library?limit=3&sort=addedDesc", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("library: status = %d: %s", rec.Code, rec.Body)
	}
	library := decode[gen.LibraryPage](t, rec)
	if library.Total != 10 || len(library.Items) != 3 || library.NextCursor == nil {
		t.Errorf("library: total = %d, %d items, next %v, want 10 and 3 items with a next page",
			library.Total, len(library.Items), library.NextCursor)
	}
	var labels []string
	for _, item := range library.Items {
		if item.Video == nil {
			t.Fatalf("library item %+v is not a video", item)
		}
		labels = append(labels, listLabel(t, fa, fb, item.Video.Id))
	}
	if want := []string{"b:e", "a:e", "b:d"}; !slices.Equal(labels, want) {
		t.Errorf("library = %v, want %v", labels, want)
	}

	guest := func(target string) *httptest.ResponseRecorder { return b.env.get(target) }
	got, totals := pageVideos(t, fa, fb, guest, "/api/videos", url.Values{"sort": {"addedDesc"}})
	if want := []string{"b:d", "a:d", "b:a", "a:a"}; !slices.Equal(got, want) || totals[0] != 4 {
		t.Errorf("guest list = %v, totals %v, want %v and 4", got, totals, want)
	}
	rec = b.env.get("/api/library")
	assertAudience(t, "guest library", rec, "guest")
	for _, item := range rawItems(t, rec.Body.Bytes(), "items") {
		var video map[string]json.RawMessage
		if err := json.Unmarshal(item["video"], &video); err != nil {
			t.Fatal(err)
		}
		assertGuestVideo(t, "guest library", video)
	}
}

func TestMergedListSendsALocalTagToThePeerByName(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	ctx := context.Background()
	// The same name has another id on each node, and a's id of b's tag names a different tag.
	if _, err := fa.env.db.Tags().CreateTag(ctx, "別の名前"); err != nil {
		t.Fatal(err)
	}
	cat := func(f *guestFixture, video string) int64 {
		tag, err := f.env.db.Tags().CreateTag(ctx, "猫")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := f.env.db.Tags().AttachTagByID(ctx, []int64{f.ids[video]}, tag.ID); err != nil {
			t.Fatal(err)
		}
		return tag.ID
	}
	localCat := cat(fb, "c")
	if remoteCat := cat(fa, "e"); remoteCat == localCat {
		t.Fatalf("前提: 両方の台の猫が同じ id %d", localCat)
	}
	owner := func(target string) *httptest.ResponseRecorder { return b.owner(http.MethodGet, target, "") }

	got, totals := pageVideos(t, fa, fb, owner, "/api/videos", url.Values{"tag": {strconv.FormatInt(localCat, 10)}})
	if want := []string{"a:e", "b:c"}; !slices.Equal(got, want) || totals[0] != 2 {
		t.Errorf("tag filter = %v, totals %v, want %v and 2", got, totals, want)
	}
	// A filter by an entity only the peer has lists the peer's matches alone.
	remoteTag := strconv.FormatInt(domain.ToRemoteID(fa.tagID), 10)
	got, _ = pageVideos(t, fa, fb, owner, "/api/videos", url.Values{"tag": {remoteTag}})
	if want := []string{"a:b", "a:a"}; !slices.Equal(got, want) {
		t.Errorf("remote tag filter = %v, want %v", got, want)
	}
}

func TestMergedListContinuesWithTheLocalPartWhenThePeerStops(t *testing.T) {
	fa, fb, b, stopA := mergedNodes(t)

	first := decode[gen.VideoPage](t, b.owner(http.MethodGet, "/api/videos?limit=2&sort=addedDesc", ""))
	if first.Total != 10 || first.NextCursor == nil {
		t.Fatalf("first page: total = %d, next %v", first.Total, first.NextCursor)
	}
	stopA()

	owner := func(target string) *httptest.ResponseRecorder { return b.owner(http.MethodGet, target, "") }
	got, totals := pageVideos(t, fa, fb, owner, "/api/videos", url.Values{"sort": {"addedDesc"}, "cursor": {*first.NextCursor}})
	if want := []string{"b:d", "b:c", "b:b", "b:a"}; !slices.Equal(got, want) {
		t.Errorf("after the peer stopped = %v, want %v", got, want)
	}
	if !slices.Equal(totals, []int{5, 5}) {
		t.Errorf("totals = %v, want the local total on each page", totals)
	}
	if state, _ := decodePeerState(t, b.owner(http.MethodGet, "/api/peer", "")); state.State != gen.PeerStateStateUnreachable {
		t.Errorf("state = %s, want unreachable", state.State)
	}
}

func TestMergedFoldersListBothNodesRootsAndForwardARemoteRoot(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)

	roots := decode[gen.RootFolderListing](t, b.owner(http.MethodGet, "/api/folders", ""))
	var ids []int64
	for _, root := range roots.Folders {
		ids = append(ids, root.RootId)
		if domain.IsRemoteID(root.RootId) && root.Grouping != nil {
			t.Errorf("remote root %d carries its grouping", root.RootId)
		}
	}
	remoteRoot := domain.ToRemoteID(fa.rootID)
	for _, want := range []int64{fb.rootID, fb.otherID, remoteRoot, domain.ToRemoteID(fa.otherID)} {
		if !slices.Contains(ids, want) {
			t.Errorf("roots = %v, want %d among them", ids, want)
		}
	}

	path := fb.folderPath(remoteRoot, "/videos")
	got, _ := pageVideos(t, fa, fb, func(target string) *httptest.ResponseRecorder {
		return b.owner(http.MethodGet, target, "")
	}, path, url.Values{"scope": {"subtree"}, "sort": {"titleAsc"}})
	if want := []string{"a:a", "a:b", "a:c", "a:d"}; !slices.Equal(got, want) {
		t.Errorf("remote root videos = %v, want %v", got, want)
	}
	listing := decode[gen.FolderListing](t, b.owner(http.MethodGet, fb.folderPath(remoteRoot, ""), ""))
	if listing.Folder.RootId != remoteRoot || len(listing.Folders) == 0 || listing.Folders[0].RootId != remoteRoot {
		t.Errorf("remote root listing = %+v", listing)
	}
}

// A favorites-only read of the peer's folder lists the peer's own favorites.
func TestRemoteFolderFavoritesListThePeersFavorites(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	favoriteOn(t, fa, "a")

	got, totals := pageVideos(t, fa, fb, func(target string) *httptest.ResponseRecorder {
		return b.owner(http.MethodGet, target, "")
	}, fb.folderPath(domain.ToRemoteID(fa.rootID), "/videos"), url.Values{"scope": {"subtree"}, "favorite": {"true"}})
	if !slices.Equal(got, []string{"a:a"}) || !slices.Equal(totals, []int{1}) {
		t.Errorf("favorites of the remote root = %v, totals %v, want a:a", got, totals)
	}
}

// playedOn sets each video's last play on f to base plus its offset in seconds.
func playedOn(t *testing.T, f *guestFixture, base time.Time, offsets map[string]int) {
	t.Helper()
	for video, offset := range offsets {
		at := base.Add(time.Duration(offset) * time.Second)
		if err := store.SetPlayedAtForTest(context.Background(), f.env.db, "content-key-"+video, at); err != nil {
			t.Fatal(err)
		}
	}
}

// Played sorts and the favorite filter use each node's own history and favorites, so both nodes' videos
// interleave by play time and the peer's favorites are listed.
func TestMergedListSortsAndFiltersByEachNodesWatchValues(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	base := time.Now().Add(-time.Hour)
	playedOn(t, fa, base, map[string]int{"a": 5, "b": 3, "c": 1})
	playedOn(t, fb, base, map[string]int{"a": 4, "c": 2})
	favoriteOn(t, fa, "a", "b")
	favoriteOn(t, fb, "b", "c")
	owner := func(target string) *httptest.ResponseRecorder { return b.owner(http.MethodGet, target, "") }

	got, _ := pageVideos(t, fa, fb, owner, "/api/videos", url.Values{"sort": {"playedDesc"}})
	if played := []string{"a:a", "b:a", "a:b", "b:c", "a:c"}; len(got) != 10 || !slices.Equal(got[:5], played) {
		t.Errorf("playedDesc = %v, want %v first, then the five unplayed videos", got, played)
	}
	got, totals := pageVideos(t, fa, fb, owner, "/api/videos", url.Values{"sort": {"playedDesc"}, "favorite": {"true"}})
	if want := []string{"a:a", "a:b", "b:c", "b:b"}; !slices.Equal(got, want) || totals[0] != 4 {
		t.Errorf("favorites by playedDesc = %v, totals %v, want %v of 4", got, totals, want)
	}
}
