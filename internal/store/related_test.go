package store

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func siblingIDs(siblings []domain.RelatedSibling) []int64 {
	ids := make([]int64, 0, len(siblings))
	for _, sibling := range siblings {
		ids = append(ids, sibling.VideoID)
	}
	slices.Sort(ids)
	return ids
}

// 直下の動画だけを返し、孫のフォルダや別のフォルダの動画は混ざらない。同じ
// 動画が同じフォルダに2つの所在を持っても、パスの小さい方の1件になる。
func TestDirectVideoPathsReturnsDirectVideosOnce(t *testing.T) {
	db, ids := folderFixture(t,
		sampleFile("/media/show/ep 1.mp4", "ep 1", "key-1", 1, 0),
		sampleFile("/media/show/ep 10.mp4", "ep 10", "key-10", 1, 0),
		sampleFile("/media/show/ep 2.mp4", "ep 2", "key-2", 1, 0),
		sampleFile("/media/show/z copy.mp4", "z copy", "key-2", 1, 0),
		sampleFile("/media/show/sub/deep.mp4", "deep", "key-deep", 1, 0),
		sampleFile("/media/other/ep 3.mp4", "ep 3", "key-3", 1, 0),
		sampleFile("/media/showcase/ep 4.mp4", "ep 4", "key-4", 1, 0),
	)

	siblings, err := db.Library().DirectVideoPaths(context.Background(), domain.AudienceOwner, "/media/show")
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{ids["/media/show/ep 1.mp4"], ids["/media/show/ep 10.mp4"], ids["/media/show/ep 2.mp4"]}
	slices.Sort(want)
	if got := siblingIDs(siblings); !slices.Equal(got, want) {
		t.Fatalf("ids = %v, want %v (%+v)", got, want, siblings)
	}
	for _, sibling := range siblings {
		if sibling.VideoID == ids["/media/show/ep 2.mp4"] && sibling.Path != "/media/show/ep 2.mp4" {
			t.Fatalf("同じ動画の所在は小さいパスを使う: %q", sibling.Path)
		}
	}

	// 並べると nextId が同じフォルダの後続の先頭を指し、最後のファイルでは省かれる。
	self := domain.RelatedSelf{VideoID: ids["/media/show/ep 2.mp4"], Path: "/media/show/ep 2.mp4"}
	if order := domain.OrderRelated(self, siblings, nil); order.NextID != ids["/media/show/ep 10.mp4"] {
		t.Fatalf("nextId = %d, want ep 10", order.NextID)
	}
	self = domain.RelatedSelf{VideoID: ids["/media/show/ep 10.mp4"], Path: "/media/show/ep 10.mp4"}
	if order := domain.OrderRelated(self, siblings, nil); order.NextID != 0 {
		t.Fatalf("nextId = %d, want 省略", order.NextID)
	}
}

// 追加日時の前後それぞれ limit 件までを返す。自分自身と、登録フォルダの下に
// 所在の無い動画は含めない。
func TestVideosAddedNear(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	base := time.Unix(1_757_000_000, 0)
	add := func(path, key string, offset time.Duration) int64 {
		t.Helper()
		file := sampleFile(path, key, key, 1, 0)
		file.AddedAt = base.Add(offset)
		got, err := db.ScanIndex().UpsertVideo(ctx, file)
		if err != nil {
			t.Fatal(err)
		}
		return got.ID
	}
	self := add("/media/self.mp4", "self", 0)
	var before, after []int64
	for i := 1; i <= 4; i++ {
		before = append(before, add(fmt.Sprintf("/media/b%d.mp4", i), fmt.Sprintf("b%d", i), -time.Duration(i)*time.Hour))
		after = append(after, add(fmt.Sprintf("/media/a%d.mp4", i), fmt.Sprintf("a%d", i), time.Duration(i)*time.Hour))
	}
	// 同じ秒に追加された動画は、id に関わらず差 0 として両側から拾える。
	sameSecond := add("/media/same.mp4", "same", 0)
	add("/outside/unregistered.mp4", "unregistered", time.Minute)

	neighbors, err := db.Library().VideosAddedNear(ctx, domain.AudienceOwner, self, base, 3)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]int64, 0, len(neighbors))
	for _, neighbor := range neighbors {
		got = append(got, neighbor.VideoID)
	}
	want := []int64{before[0], before[1], before[2], sameSecond, after[0], after[1]}
	if !slices.Equal(got, want) {
		t.Fatalf("neighbors = %v, want %v", got, want)
	}
}

// 本体は指定の順で返し、登録フォルダの下に所在が無い動画は抜ける。
func TestVideosByIDsKeepsOrder(t *testing.T) {
	db, ids := folderFixture(t,
		sampleFile("/media/a.mp4", "a", "key-a", 1, 0),
		sampleFile("/media/b.mp4", "b", "key-b", 1, 0),
		sampleFile("/outside/c.mp4", "c", "key-c", 1, 0),
	)
	videos, err := db.Library().VideosByIDs(context.Background(), domain.AudienceOwner, []int64{ids["/media/b.mp4"], ids["/outside/c.mp4"], ids["/media/a.mp4"]})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, video := range videos {
		titles = append(titles, video.Title)
	}
	if want := []string{"b", "a"}; !slices.Equal(titles, want) {
		t.Fatalf("titles = %v, want %v", titles, want)
	}
	empty, err := db.Library().VideosByIDs(context.Background(), domain.AudienceOwner, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty = %v, %v", empty, err)
	}
}
