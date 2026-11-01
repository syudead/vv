package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// fakeWatchHistory holds a node's entries newest first with fixed times, which the store takes from the clock, and
// records clears and deletions.
type fakeWatchHistory struct {
	mu      sync.Mutex
	entries []domain.WatchHistoryEntry
	days    []string
	clears  int
	deleted []int64
}

func (f *fakeWatchHistory) ListWatchHistory(
	_ context.Context, _ domain.Audience, _ domain.WatchHistoryQuery, cursor string, limit int,
) (domain.WatchHistoryPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rest := f.entries
	if cursor != "" {
		after, err := domain.DecodeWatchHistoryCursor(cursor)
		if err != nil {
			return domain.WatchHistoryPage{}, err
		}
		rest = slices.DeleteFunc(slices.Clone(rest), func(e domain.WatchHistoryEntry) bool {
			ms := e.PlayedAt.UnixMilli()
			return ms > after.PlayedAtMs || (ms == after.PlayedAtMs && e.ID >= after.ID)
		})
	}
	page := domain.WatchHistoryPage{Items: rest[:min(limit, len(rest))]}
	if len(rest) > limit {
		page.NextCursor = domain.WatchHistoryItemCursor(rest[limit-1]).Cursor
	}
	return page, nil
}

func (f *fakeWatchHistory) ListWatchHistoryDays(context.Context, domain.Audience, domain.WatchHistoryQuery, *time.Location) ([]string, error) {
	return f.days, nil
}

func (f *fakeWatchHistory) DeleteWatchHistoryEntry(_ context.Context, id int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, id)
	return true, nil
}

func (f *fakeWatchHistory) ClearWatchHistory(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clears++
	return nil
}

// historyNodes are mergedNodes whose watch history is a fake on each node: a is read by b.
func historyNodes(t *testing.T) (fa, fb *guestFixture, ha, hb *fakeWatchHistory, b *peerNode, stopA func()) {
	t.Helper()
	ha, hb = &fakeWatchHistory{}, &fakeWatchHistory{}
	histories := []*fakeWatchHistory{ha, hb}
	fa, fb, _, b, stopA = mergedNodesWith(t, func(_ *store.DB, opts *Options) {
		opts.WatchHistory, histories = histories[0], histories[1:]
	})
	return fa, fb, ha, hb, b, stopA
}

func historyEntry(t *testing.T, f *guestFixture, id int64, playedAtMs int64, video string) domain.WatchHistoryEntry {
	t.Helper()
	entry := domain.WatchHistoryEntry{ID: id, PlayedAt: time.UnixMilli(playedAtMs), Title: fmt.Sprintf("entry %d", id)}
	if video != "" {
		found, err := f.env.db.Library().GetVideo(context.Background(), domain.AudienceOwner, f.ids[video])
		if err != nil {
			t.Fatal(err)
		}
		entry.Video = &found
	}
	return entry
}

func TestRemoteVideoProgressIsStoredOnThePeer(t *testing.T) {
	f, _, b, _ := remoteVideoNodes(t)

	rec := b.owner(http.MethodPut, f.remotePath("a", "/progress"), `{"positionMs":1000}`)
	if rec.Code != http.StatusOK || decode[gen.Progress](t, rec).PositionMs != 1000 {
		t.Fatalf("progress: status = %d: %s", rec.Code, rec.Body)
	}
	if local := decode[gen.Video](t, f.env.get(f.videoPath("a", ""), f.owner)); local.Progress == nil || local.Progress.PositionMs != 1000 {
		t.Errorf("the peer's progress = %+v, want 1000 ms", local.Progress)
	}
	if remote := decode[gen.Video](t, b.owner(http.MethodGet, f.remotePath("a", ""), "")); remote.Progress == nil ||
		remote.Progress.PositionMs != 1000 {
		t.Errorf("the remote video's progress = %+v, want 1000 ms", remote.Progress)
	}
}

// A selection of both nodes' videos is favorited on each node, the peer first; with the peer down neither changes.
func TestFavoritesOfBothNodesAreAppliedOnEach(t *testing.T) {
	fa, fb, b, stopA := mergedNodes(t)
	favorited := func(f *guestFixture, name string) bool {
		t.Helper()
		video := decode[gen.Video](t, f.env.get(f.videoPath(name, ""), f.owner))
		return video.Favorite != nil && *video.Favorite
	}

	rec := b.owner(http.MethodPut, "/api/favorites", favoritesBody(true, []int64{fb.ids["a"], domain.ToRemoteID(fa.ids["a"])}))
	if rec.Code != http.StatusOK || decode[gen.FavoritesResponse](t, rec).AppliedVideos != 2 {
		t.Fatalf("favorites: status = %d: %s, want 2 applied", rec.Code, rec.Body)
	}
	if !favorited(fa, "a") || !favorited(fb, "a") {
		t.Errorf("favorite on the peer = %v, here = %v, want both", favorited(fa, "a"), favorited(fb, "a"))
	}

	stopA()
	down := b.owner(http.MethodPut, "/api/favorites", favoritesBody(true, []int64{fb.ids["b"], domain.ToRemoteID(fa.ids["b"])}))
	assertErrorBody(t, "favorites with the peer down", down.Code, down.Body.Bytes(), wantError{
		status: http.StatusServiceUnavailable, code: gen.ErrorCodeServiceUnavailable, reason: gen.ErrorReasonPeerUnreachable,
	})
	if favorited(fa, "b") || favorited(fb, "b") {
		t.Errorf("with the peer down, favorite on the peer = %v, here = %v, want neither", favorited(fa, "b"), favorited(fb, "b"))
	}
}

// Both nodes' entries are listed by playedAt across pages of one composite cursor, the peer's in the offset form
// with its video's progress, and the dates are the union of both nodes'.
func TestWatchHistoryListsBothNodesNewestFirst(t *testing.T) {
	fa, fb, ha, hb, b, _ := historyNodes(t)
	ha.entries = []domain.WatchHistoryEntry{historyEntry(t, fa, 1, 300, "a"), historyEntry(t, fa, 2, 100, "")}
	hb.entries = []domain.WatchHistoryEntry{historyEntry(t, fb, 1, 400, ""), historyEntry(t, fb, 2, 200, "b")}
	ha.days, hb.days = []string{"2026-10-30"}, []string{"2026-10-31", "2026-10-30"}
	want := []int64{1, domain.ToRemoteID(1), 2, domain.ToRemoteID(2)}

	for _, limit := range []int{1, 3} {
		var got []int64
		target := fmt.Sprintf("/api/watch-history?limit=%d", limit)
		for range len(want) + 1 {
			page := decode[gen.WatchHistoryPage](t, b.owner(http.MethodGet, target, ""))
			for _, item := range page.Items {
				got = append(got, item.Id)
				if item.Id == domain.ToRemoteID(1) && (item.Video == nil || item.Video.Id != domain.ToRemoteID(fa.ids["a"]) ||
					item.Video.Progress == nil) {
					t.Errorf("the peer's entry video = %+v, want remote a with its progress", item.Video)
				}
			}
			if page.NextCursor == nil {
				break
			}
			target = fmt.Sprintf("/api/watch-history?limit=%d&cursor=%s", limit, *page.NextCursor)
		}
		if !slices.Equal(got, want) {
			t.Errorf("limit %d: entries = %v, want %v", limit, got, want)
		}
	}

	dates := decode[gen.WatchHistoryDates](t, b.owner(http.MethodGet, "/api/watch-history/dates?tz=UTC", ""))
	if !slices.Equal(dates.Days, []string{"2026-10-31", "2026-10-30"}) {
		t.Errorf("days = %v, want both nodes' days once", dates.Days)
	}
}

// A clear reaches both nodes and the peer's entry is deleted on the peer; with the peer down this node is unchanged.
func TestWatchHistoryClearAndDeleteReachThePeer(t *testing.T) {
	_, _, ha, hb, b, stopA := historyNodes(t)

	assertStatus(t, "clear", b.owner(http.MethodDelete, "/api/watch-history", ""), http.StatusNoContent)
	assertStatus(t, "delete the peer's entry", b.owner(http.MethodDelete, historyEntryPath(domain.ToRemoteID(2)), ""), http.StatusNoContent)
	if ha.clears != 1 || hb.clears != 1 || !slices.Equal(ha.deleted, []int64{2}) || len(hb.deleted) != 0 {
		t.Errorf("clears = %d on the peer, %d here; deleted = %v on the peer, %v here, want 1, 1, [2], []",
			ha.clears, hb.clears, ha.deleted, hb.deleted)
	}

	stopA()
	down := b.owner(http.MethodDelete, "/api/watch-history", "")
	assertErrorBody(t, "clear with the peer down", down.Code, down.Body.Bytes(), wantError{
		status: http.StatusServiceUnavailable, code: gen.ErrorCodeServiceUnavailable, reason: gen.ErrorReasonPeerUnreachable,
	})
	if hb.clears != 1 {
		t.Errorf("with the peer down, this node cleared %d times, want 1", hb.clears)
	}
}
