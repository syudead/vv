package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// relatedNodes pairs two nodes that each hold the guest fixture, with their screens. On b, video b no longer
// has the tag, so its only clue shared with a is the folder.
func relatedNodes(t *testing.T) (a, b *guestFixture, onA, onB *peerNode) {
	t.Helper()
	nodes := make([]*peerNode, 2)
	fixtures := make([]*guestFixture, 2)
	for i, name := range []string{"node-a", "node-b"} {
		var pairing *app.Pairing
		fixtures[i] = newGuestFixtureWith(t, true, func(db *store.DB, opts *Options) {
			pairing = withTestPairing(db, name, nil, opts)
		})
		server := httptest.NewServer(fixtures[i].env.handler)
		t.Cleanup(server.Close)
		nodes[i] = &peerNode{env: fixtures[i].env, url: server.URL, cookie: fixtures[i].owner, pairing: pairing}
	}
	pairNodes(t, nodes[0], nodes[1])
	a, b = fixtures[0], fixtures[1]
	if _, _, err := b.env.db.Tags().DetachTag(context.Background(), []int64{b.ids["b"]}, b.tagID); err != nil {
		t.Fatal(err)
	}
	return a, b, nodes[0], nodes[1]
}

func (f *guestFixture) remoteID(name string) int64 {
	return domain.ToRemoteID(f.ids[name])
}

func relatedIDs(t *testing.T, rec *httptest.ResponseRecorder) (gen.RelatedVideos, []int64) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("関連動画: status = %d: %s", rec.Code, rec.Body)
	}
	related := decode[gen.RelatedVideos](t, rec)
	ids := make([]int64, 0, len(related.Items))
	for _, item := range related.Items {
		ids = append(ids, item.Id)
	}
	return related, ids
}

func TestRelatedVideosOfALocalVideoRankBothNodes(t *testing.T) {
	a, b, _, screens := relatedNodes(t)

	related, ids := relatedIDs(t, screens.owner(http.MethodGet, b.videoPath("a", "/related"), ""))
	// a's a and b share the tag (30 points) and outrank b's b, which shares only the folder (20); a's b was
	// added later than a's a.
	if want := []int64{a.remoteID("b"), a.remoteID("a"), b.ids["b"]}; len(ids) < 3 || !slices.Equal(ids[:3], want) {
		t.Fatalf("ids = %v, want %v で始まる", ids, want)
	}
	if related.NextId == nil || *related.NextId != a.remoteID("b") {
		t.Errorf("nextId = %v, want %d", related.NextId, a.remoteID("b"))
	}
	if slices.Contains(ids, b.ids["a"]) {
		t.Errorf("開いている動画が一覧にある: %v", ids)
	}
	// The peer's tag of the same name is b's tag, so the remote card's chip opens it.
	if tags := related.Items[1].Tags; len(tags) != 1 || tags[0].Id != b.tagID {
		t.Errorf("a の a のタグ = %+v, want 手元の ID %d", tags, b.tagID)
	}

	var paged []int64
	target := b.videoPath("a", "/related?limit=4")
	for range 5 {
		page, pageIDs := relatedIDs(t, screens.owner(http.MethodGet, target, ""))
		paged = append(paged, pageIDs...)
		if page.NextCursor == nil {
			break
		}
		target = b.videoPath("a", "/related?limit=4&cursor="+*page.NextCursor)
	}
	if !slices.Equal(paged, ids) {
		t.Errorf("4 件ずつの順 = %v, want %v", paged, ids)
	}
	if unique := slices.Compact(slices.Sorted(slices.Values(paged))); len(unique) != len(paged) || len(paged) < 9 {
		t.Errorf("重複か欠落がある（両方の台の 9 本）: %v", paged)
	}
}

func TestRelatedVideosOfARemoteVideoScoreLocalVideos(t *testing.T) {
	a, b, _, screens := relatedNodes(t)

	related, ids := relatedIDs(t, screens.owner(http.MethodGet, a.remotePath("a", "/related"), ""))
	// b's a shares the tag by name, as a's b does on a; a's b was added later.
	if want := []int64{a.remoteID("b"), b.ids["a"]}; len(ids) < 2 || !slices.Equal(ids[:2], want) {
		t.Fatalf("ids = %v, want %v で始まる", ids, want)
	}
	if slices.Contains(ids, a.remoteID("a")) {
		t.Errorf("開いている動画が一覧にある: %v", ids)
	}
	if related.NextId == nil || *related.NextId != a.remoteID("b") {
		t.Errorf("nextId = %v, want %d", related.NextId, a.remoteID("b"))
	}
}

func TestRelatedVideosOfARemoteVersionLeaveOutItsBundle(t *testing.T) {
	a, _, _, screens := relatedNodes(t)
	if _, err := a.env.db.Versions().Bundle(context.Background(), []int64{a.ids["a"], a.ids["b"]}, a.ids["b"]); err != nil {
		t.Fatal(err)
	}

	// a's a is now a version of a's b, which the peer lists as the bundle's representative.
	related, ids := relatedIDs(t, screens.owner(http.MethodGet, a.remotePath("a", "/related"), ""))
	if slices.Contains(ids, a.remoteID("b")) {
		t.Errorf("開いている版の代表が一覧にある: %v", ids)
	}
	if related.NextId != nil && *related.NextId == a.remoteID("b") {
		t.Errorf("nextId = 開いている版の代表 %d", *related.NextId)
	}
}

func TestRelatedVideosOfARemoteVideoRankLikeOnItsNode(t *testing.T) {
	a, b, onA, onB := relatedNodes(t)

	// b's b shares only the folder with b's a, so it follows a's two tagged videos only with the folder points.
	_, direct := relatedIDs(t, onB.owner(http.MethodGet, b.videoPath("a", "/related"), ""))
	if want := []int64{a.remoteID("b"), a.remoteID("a"), b.ids["b"]}; len(direct) < 3 || !slices.Equal(direct[:3], want) {
		t.Fatalf("b で開いた ids = %v, want %v で始まる", direct, want)
	}
	_, remote := relatedIDs(t, onA.owner(http.MethodGet, b.remotePath("a", "/related"), ""))
	if want := []int64{a.ids["b"], a.ids["a"], b.remoteID("b")}; len(remote) < 3 || !slices.Equal(remote[:3], want) {
		t.Fatalf("a で開いた ids = %v, want %v で始まる（b で開いたときと同じ順）", remote, want)
	}
}

// acceptingPairing takes every peer credential; the other pairing operations are not used.
type acceptingPairing struct{ Pairing }

func (acceptingPairing) Authenticate(context.Context, string) (bool, error) { return true, nil }

func TestRankPeerRelatedPassesTheOpenVideoID(t *testing.T) {
	const clues = `"clues":{"tags":[],"words":[],"addedAt":"2000-01-01T00:00:00Z"}`
	for _, tc := range []struct {
		name, body string
		status     int
		want       []int64
	}{
		{"open video", `{` + clues + `,"openVideoId":7}`, http.StatusOK, []int64{7}},
		{"no open video", `{` + clues + `}`, http.StatusOK, []int64{0}},
		{"open video 0", `{` + clues + `,"openVideoId":0}`, http.StatusBadRequest, nil},
	} {
		catalog := &fakeCatalog{}
		handler := newTestServer(t, Options{Catalog: catalog, Pairing: acceptingPairing{}})
		req := httptest.NewRequest(http.MethodPost, peerRelatedPath, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "VV-Peer secret")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != tc.status {
			t.Errorf("%s: status = %d, want %d: %s", tc.name, rec.Code, tc.status, rec.Body)
		}
		if !slices.Equal(catalog.peerRelatedOpen, tc.want) {
			t.Errorf("%s: catalog に渡した openID = %v, want %v", tc.name, catalog.peerRelatedOpen, tc.want)
		}
	}
}

func TestCluesFromRefusesAFingerprintLongerThanASprite(t *testing.T) {
	frameBytes := domain.MaxFingerprintBytes / domain.SeekSpriteMaxFrames
	for _, tc := range []struct {
		frames int
		ok     bool
	}{{domain.SeekSpriteMaxFrames, true}, {domain.SeekSpriteMaxFrames + 1, false}} {
		body := gen.RelatedClues{Fingerprint: &gen.RelatedFingerprint{
			Version: domain.FingerprintVersion, IntervalMs: 1000, Frames: make([]byte, tc.frames*frameBytes),
		}}
		clues, err := cluesFrom(body)
		if tc.ok && (err != nil || clues.Fingerprint == nil || len(clues.Fingerprint.Frames) != tc.frames) {
			t.Errorf("%d フレーム: clues = %+v, err = %v, want 受理", tc.frames, clues.Fingerprint, err)
		}
		if !tc.ok && !errors.Is(err, domain.ErrInvalidFingerprint) {
			t.Errorf("%d フレーム: err = %v, want ErrInvalidFingerprint", tc.frames, err)
		}
	}
}

func TestGuestRelatedVideosHoldPublicVideosOfBothNodes(t *testing.T) {
	a, b, _, screens := relatedNodes(t)

	_, ids := relatedIDs(t, screens.env.get(b.videoPath("a", "/related")))
	slices.Sort(ids)
	want := []int64{b.ids["d"], a.remoteID("a"), a.remoteID("d")}
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		t.Fatalf("ゲストの ids = %v, want 両方の台の公開動画 %v", ids, want)
	}

	// The clues carry tag names a guest may not read, so only the peer reaches them.
	clues := screens.owner(http.MethodGet, "/api/peer/videos/"+strconv.FormatInt(b.ids["a"], 10)+"/clues", "")
	assertStatus(t, "画面からの手がかり", clues, http.StatusNotFound)
}
