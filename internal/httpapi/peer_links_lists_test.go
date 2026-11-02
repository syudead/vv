package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// linkedNodes is b reading a in slot 1 and c in slot 2, each node holding the guest fixture.
type linkedNodes struct {
	fa, fb, fc *guestFixture
	a, b, c    *peerNode
}

// threeNodesWith serves a, c and b in that order with extend applied to each node's options, and pairs b with a
// and then with c.
func threeNodesWith(t *testing.T, extend func(db *store.DB, opts *Options)) linkedNodes {
	t.Helper()
	var n linkedNodes
	n.fa, n.a, _ = mergedNode(t, "node-a", extend)
	n.fc, n.c, _ = mergedNode(t, "node-c", extend)
	n.fb, n.b, _ = mergedNode(t, "node-b", extend)
	pairNodes(t, n.a, n.b)
	pairNodes(t, n.c, n.b)
	return n
}

func (n linkedNodes) label(t *testing.T, id int64) string {
	t.Helper()
	return nodeLabel(t, []string{"b", "a", "c"}, []*guestFixture{n.fb, n.fa, n.fc}, id)
}

func (n linkedNodes) owner(target string) *httptest.ResponseRecorder {
	return n.b.owner(http.MethodGet, target, "")
}

func (n linkedNodes) pageVideos(t *testing.T, path string, query url.Values) ([]string, []int) {
	t.Helper()
	return pageVideosBy(t, func(id int64) string { return n.label(t, id) }, n.owner, path, query)
}

func TestMergedVideoListPagesEveryLinkOnce(t *testing.T) {
	n := threeNodesWith(t, nil)

	// Every node holds e as the newest and a as the oldest; on equal values the local video comes first, then
	// the links in slot order.
	got, totals := n.pageVideos(t, "/api/videos", url.Values{"sort": {"addedDesc"}})
	var want []string
	for _, video := range []string{"e", "d", "c", "b", "a"} {
		want = append(want, "b:"+video, "a:"+video, "c:"+video)
	}
	if !slices.Equal(got, want) {
		t.Errorf("addedDesc = %v, want %v", got, want)
	}
	if slices.ContainsFunc(totals, func(total int) bool { return total != 15 }) {
		t.Errorf("totals = %v, want 15 on every page", totals)
	}
}

// A link that is not connected when the listing starts is not asked, so the page lists this node and the other
// link without waiting for it.
func TestMergedListLeavesOutALinkThatIsNotConnected(t *testing.T) {
	n := threeNodesWith(t, nil)
	n.b.pairing.ReportFailure(context.Background(), 2, &domain.PeerUnreachableError{Message: "down"})

	got, totals := n.pageVideos(t, "/api/videos", url.Values{"sort": {"titleAsc"}})
	var want []string
	for _, video := range []string{"a", "b", "c", "d", "e"} {
		want = append(want, "b:"+video, "a:"+video)
	}
	if !slices.Equal(got, want) {
		t.Errorf("with c not connected = %v, want %v", got, want)
	}
	if slices.ContainsFunc(totals, func(total int) bool { return total != 10 }) {
		t.Errorf("totals = %v, want 10 on every page", totals)
	}
}

func TestWatchHistoryMergesEveryLinkByPlayedAt(t *testing.T) {
	// The nodes are served a, c, b, so the histories go to them in that order.
	ha, hc, hb := &fakeWatchHistory{}, &fakeWatchHistory{}, &fakeWatchHistory{}
	histories := []*fakeWatchHistory{ha, hc, hb}
	n := threeNodesWith(t, func(_ *store.DB, opts *Options) {
		opts.WatchHistory, histories = histories[0], histories[1:]
	})
	ha.entries = []domain.WatchHistoryEntry{historyEntry(t, n.fa, 1, 500, ""), historyEntry(t, n.fa, 2, 100, "")}
	hc.entries = []domain.WatchHistoryEntry{historyEntry(t, n.fc, 1, 400, ""), historyEntry(t, n.fc, 2, 300, "")}
	hb.entries = []domain.WatchHistoryEntry{historyEntry(t, n.fb, 1, 450, ""), historyEntry(t, n.fb, 2, 300, "")}
	ha.days, hc.days, hb.days = []string{"2026-10-29"}, []string{"2026-10-31"}, []string{"2026-10-30", "2026-10-29"}
	// On equal times this node's entry comes first, then the links in slot order.
	want := []int64{domain.RemoteID(1, 1), 1, domain.RemoteID(2, 1), 2, domain.RemoteID(2, 2), domain.RemoteID(1, 2)}

	var got []int64
	target := "/api/watch-history?limit=2"
	for range len(want) {
		page := decode[gen.WatchHistoryPage](t, n.owner(target))
		for _, item := range page.Items {
			got = append(got, item.Id)
		}
		if page.NextCursor == nil {
			break
		}
		target = fmt.Sprintf("/api/watch-history?limit=2&cursor=%s", *page.NextCursor)
	}
	if !slices.Equal(got, want) {
		t.Errorf("entries = %v, want %v", got, want)
	}

	dates := decode[gen.WatchHistoryDates](t, n.owner("/api/watch-history/dates?tz=UTC"))
	if want := []string{"2026-10-31", "2026-10-30", "2026-10-29"}; !slices.Equal(dates.Days, want) {
		t.Errorf("days = %v, want %v", dates.Days, want)
	}
}

func TestSmartFolderCardsSumEveryLinksCounts(t *testing.T) {
	n := threeNodesWith(t, func(db *store.DB, opts *Options) {
		opts.SmartFolders = db.SmartFolders()
	})
	cat := tagOn(t, n.fb, "猫", "c", "e")
	aCat := tagOn(t, n.fa, "猫", "b", "d", "e")
	tagOn(t, n.fc, "猫", "a")
	local := createSmartFolder(t, n.b, fmt.Sprintf(`{"name":"猫","rule":{"tags":[%d]}}`, cat.ID))
	remote := domain.RemoteID(1, createSmartFolder(t, n.a, fmt.Sprintf(`{"name":"a の猫","rule":{"tags":[%d]}}`, aCat.ID)))

	// Every video holds 4096 bytes: b has 2 matches, a 3 and c 1, whichever node holds the folder.
	cards := folderCards(t, n.owner, local, remote)
	for _, id := range []int64{local, remote} {
		if card := cards[id]; card.VideoCount != 6 || card.SizeBytes != 6*4096 {
			t.Errorf("card %d = %d videos, %d bytes, want 6 and %d", id, card.VideoCount, card.SizeBytes, 6*4096)
		}
	}
}
