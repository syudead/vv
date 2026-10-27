package app

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeRelatedPeer pages a fixed ranking of the peer's videos and keeps the clues it was asked with.
type fakeRelatedPeer struct {
	ranked []domain.RankedRelated
	err    error
	clues  []domain.RelatedClues
}

func (p *fakeRelatedPeer) Related(_ context.Context, clues domain.RelatedClues, _ []int64, after *domain.RelatedCursor, limit int) (domain.RelatedSidePage, error) {
	p.clues = append(p.clues, clues)
	if p.err != nil {
		return domain.RelatedSidePage{}, p.err
	}
	items, next := domain.PageRelated(p.ranked, after, limit)
	return domain.RelatedSidePage{Items: items, More: next != nil}, nil
}

func relatedPoints(points int64) domain.RelatedScore {
	return domain.RelatedScore(points) * domain.RelatedScoreUnit
}

// mergedRelatedFixture holds, for video 1, local videos scoring 35 (3), 25 (2) and 5 (5) points; the
// peer's videos interleave with them, 11 ties 3 and 13 ties 5 but was added earlier.
func mergedRelatedFixture(peerErr error) (*Catalog, *fakeCatalogStore, *fakeRelatedPeer) {
	store := relatedFixture(
		relatedEntry(1, "/media/show/a.mp4", 7, 8),
		relatedEntry(2, "/media/show/b.mp4"),
		relatedEntry(3, "/media/other/c.mp4", 7, 8),
		relatedEntry(5, "/media/far/e.mp4"),
	)
	store.tagNames = map[int64]string{7: "旅行", 8: "海"}
	added := relatedBase.Unix()
	peer := &fakeRelatedPeer{err: peerErr, ranked: []domain.RankedRelated{
		{Score: relatedPoints(40), AddedAt: added, VideoID: 10},
		{Score: relatedPoints(35), AddedAt: added, VideoID: 11},
		{Score: relatedPoints(10), AddedAt: added, VideoID: 12},
		{Score: relatedPoints(5), AddedAt: added - int64(time.Hour/time.Second), VideoID: 13},
	}}
	return NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}}), store, peer
}

func TestMergedRelatedVideosPagesBothNodesInOrder(t *testing.T) {
	catalog, store, peer := mergedRelatedFixture(nil)
	ctx := context.Background()

	var got []int64
	page := domain.MergedRelatedPage{Limit: 3}
	for pages := 0; ; pages++ {
		if pages == 5 {
			t.Fatalf("終わらない: %v", got)
		}
		related, err := catalog.MergedRelatedVideos(ctx, domain.AudienceOwner, store.videos[1], page, peer)
		if err != nil {
			t.Fatal(err)
		}
		if local := videoIDs(related.Local); !slices.Equal(local, slices.DeleteFunc(slices.Clone(related.IDs), domain.IsRemoteID)) {
			t.Fatalf("手元の動画 = %v, ids = %v", local, related.IDs)
		}
		got = append(got, related.IDs...)
		if related.NextCursor == nil {
			break
		}
		// The screens send the cursor back as text.
		next, err := domain.ParseMergedRelatedCursor(related.NextCursor.String())
		if err != nil {
			t.Fatal(err)
		}
		page.Cursor = next
	}

	remote := domain.ToRemoteID
	want := []int64{remote(10), remote(11), 3, 2, remote(12), 5, remote(13)}
	if !slices.Equal(got, want) {
		t.Fatalf("順 = %v, want %v", got, want)
	}
	if len(peer.clues) == 0 || !slices.Equal(peer.clues[0].TagNames, []string{"旅行", "海"}) {
		t.Errorf("相手に送ったタグ = %+v, want 名前の 旅行 と 海", peer.clues)
	}
}

func TestMergedRelatedVideosSkipsAFailingPeer(t *testing.T) {
	catalog, store, peer := mergedRelatedFixture(&domain.PeerUnreachableError{Message: "down"})

	related, err := catalog.MergedRelatedVideos(context.Background(), domain.AudienceOwner, store.videos[1],
		domain.MergedRelatedPage{Limit: 10}, peer)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{3, 2, 5}; !slices.Equal(related.IDs, want) || related.NextID != 3 {
		t.Fatalf("ids = %v, nextId = %d, want %v と 3", related.IDs, related.NextID, want)
	}
	if related.NextCursor != nil {
		t.Errorf("続きのカーソル = %+v, want なし", related.NextCursor)
	}
}

func TestRemoteRelatedVideosScoresLocalVideosByClues(t *testing.T) {
	catalog, _, peer := mergedRelatedFixture(nil)
	peer.ranked = nil
	clues := domain.RelatedClues{TagNames: []string{"旅行", "海"}, Words: []string{}, AddedAt: relatedBase}

	related, err := catalog.RemoteRelatedVideos(context.Background(), domain.AudienceOwner, clues, []int64{20},
		domain.MergedRelatedPage{Limit: 10}, peer)
	if err != nil {
		t.Fatal(err)
	}
	// 1 and 3 share both tags by name; 2 and 5 only the date, so the larger id comes first.
	if want := []int64{3, 1, 5, 2}; !slices.Equal(related.IDs, want) {
		t.Fatalf("ids = %v, want %v", related.IDs, want)
	}
}

func TestPeerRelatedGivesFolderPointsByTheOpenVideo(t *testing.T) {
	owner, guest := domain.AudienceOwner, domain.AudienceGuest
	// The clues share nothing with the videos and are dated long before them, so only the folder can score.
	clues := domain.RelatedClues{TagNames: []string{}, Words: []string{}, AddedAt: relatedBase.AddDate(-20, 0, 0)}
	for _, tc := range []struct {
		name     string
		audience domain.Audience
		openID   int64
		want     map[int64]int64
		reads    []candidateRead
	}{
		{"open video", owner, 1, map[int64]int64{2: 20, 3: 0}, []candidateRead{{owner, 1}}},
		{"no open video", owner, 0, map[int64]int64{1: 0, 2: 0, 3: 0}, []candidateRead{{owner, 0}}},
		{"unknown id", owner, 9, map[int64]int64{1: 0, 2: 0, 3: 0}, []candidateRead{{owner, 9}, {owner, 0}}},
		// 2 shares 1's folder, but a guest's ranking must not take that folder from it.
		{"hidden from the audience", guest, 2, map[int64]int64{1: 0, 3: 0}, []candidateRead{{guest, 2}, {guest, 0}}},
	} {
		store := relatedFixture(
			relatedEntry(1, "/media/show/a.mp4"),
			relatedEntry(2, "/media/show/b.mp4"),
			relatedEntry(3, "/media/other/c.mp4"),
		)
		store.ownerOnly = map[int64]bool{2: true}
		catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})

		page, err := catalog.PeerRelated(context.Background(), tc.audience, clues, tc.openID, nil, nil, 10)
		if err != nil {
			t.Fatal(err)
		}
		got := map[int64]int64{}
		for _, item := range page.Items {
			got[item.Video.ID] = int64(item.Score / domain.RelatedScoreUnit)
		}
		if !maps.Equal(got, tc.want) {
			t.Errorf("%s: 点数 = %v, want %v", tc.name, got, tc.want)
		}
		if !slices.Equal(store.candidateReads, tc.reads) {
			t.Errorf("%s: 候補の読み出し = %+v, want %+v", tc.name, store.candidateReads, tc.reads)
		}
	}
}

func TestPeerRelatedFailsWhenTheOpenVideoCannotBeRead(t *testing.T) {
	store := relatedFixture(relatedEntry(1, "/media/show/a.mp4"))
	store.candidatesErr = errors.New("disk I/O error")
	catalog := NewCatalog(CatalogOptions{Index: store, Ingest: store, Files: fakeArtifactFiles{}})

	_, err := catalog.PeerRelated(context.Background(), domain.AudienceOwner, domain.RelatedClues{}, 1, nil, nil, 10)
	if !errors.Is(err, store.candidatesErr) {
		t.Fatalf("err = %v, want the read error", err)
	}
	// A failed read must not fall back to ranking without the open video.
	if want := []candidateRead{{domain.AudienceOwner, 1}}; !slices.Equal(store.candidateReads, want) {
		t.Errorf("候補の読み出し = %+v, want %+v", store.candidateReads, want)
	}
}
