package httpapi

import (
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// countingPeerLink records the paths this node asks its links for.
type countingPeerLink struct {
	PeerLink
	mu    sync.Mutex
	paths []string
}

func (l *countingPeerLink) Forward(req *http.Request, slot int64, guest, media bool) (*http.Response, error) {
	l.mu.Lock()
	l.paths = append(l.paths, req.URL.Path)
	l.mu.Unlock()
	return l.PeerLink.Forward(req, slot, guest, media)
}

func (l *countingPeerLink) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paths = nil
}

func (l *countingPeerLink) asked() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.paths)
}

// assertTalliesMatchListTotals checks each tally against the total the list gives for its filter alone.
func assertTalliesMatchListTotals(t *testing.T, get func(target string) gen.TagList, tallies gen.TagTallies) {
	t.Helper()
	for _, tc := range []struct {
		query string
		got   int
	}{
		{"tentative=true", tallies.Tentative},
		{"kind=none", tallies.NoKind},
		{"unused=true", tallies.Unused},
	} {
		if total := get("/api/tags?limit=1&" + tc.query).Total; tc.got != total {
			t.Errorf("tally for %s = %d, want the list's total %d", tc.query, tc.got, total)
		}
	}
}

func TestTagTalliesEqualTheLocalListTotals(t *testing.T) {
	f := newGuestFixture(t, true)
	// The fixture's tag is used without a kind; 猫 is unused with a kind; 仮 is tentative on c without a kind.
	cat := tagOn(t, f, "猫")
	kindOn(t, f, "動物", cat.ID)
	f.addTentativeTag(t, "c", "仮")

	tallies := decode[gen.TagTallies](t, f.ownerRequest(http.MethodGet, "/api/tags/tallies", ""))
	if want := (gen.TagTallies{Tentative: 1, NoKind: 2, Unused: 1}); tallies != want {
		t.Errorf("tallies = %+v, want %+v", tallies, want)
	}
	assertTalliesMatchListTotals(t, func(target string) gen.TagList {
		return decode[gen.TagList](t, f.ownerRequest(http.MethodGet, target, ""))
	}, tallies)
}

// On a node that reads a link, the tallies count the tags joined by name with this node's kind and tentative
// state, as the merged list does, from one read of the link's tags.
func TestTagTalliesCountTheMergedTagsFromOneReadOfTheLink(t *testing.T) {
	var links []*countingPeerLink
	fa, fb, _, b, _ := mergedNodesWith(t, func(_ *store.DB, opts *Options) {
		link := &countingPeerLink{PeerLink: opts.PeerLink}
		links = append(links, link)
		opts.PeerLink = link
	})
	readerLink := links[1]
	// The fixture's tag is on both nodes and has a kind on b only, so it has b's kind.
	kindOn(t, fb, "動物", fb.tagID)
	// 猫 is unused and kindless on b, tentative, used and of a kind on a: joined, it is kindless only.
	tagOn(t, fb, "猫")
	kindOn(t, fa, "色", fa.addTentativeTag(t, "c", "猫"))
	// 犬 is a's alone and unused, 鳥 a's alone and tentative, 仮 b's alone and tentative; all kindless.
	tagOn(t, fa, "犬")
	fa.addTentativeTag(t, "c", "鳥")
	fb.addTentativeTag(t, "c", "仮")

	readerLink.reset()
	tallies := decode[gen.TagTallies](t, b.owner(http.MethodGet, "/api/tags/tallies", ""))
	if want := []string{"/api/tags"}; !slices.Equal(readerLink.asked(), want) {
		t.Errorf("the link was asked for %v, want %v", readerLink.asked(), want)
	}
	if want := (gen.TagTallies{Tentative: 2, NoKind: 4, Unused: 1}); tallies != want {
		t.Errorf("tallies = %+v, want %+v", tallies, want)
	}
	assertTalliesMatchListTotals(t, func(target string) gen.TagList {
		return decode[gen.TagList](t, b.owner(http.MethodGet, target, ""))
	}, tallies)
}
