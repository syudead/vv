package domain

import (
	"slices"
	"testing"
	"time"
)

var relatedBase = time.Unix(1_757_000_000, 0)

func relatedSelf(id int64, name string) RelatedSelf {
	return RelatedSelf{VideoID: id, Path: "/media/show/" + name, AddedAt: relatedBase}
}

func sibling(id int64, name string) RelatedSibling {
	return RelatedSibling{VideoID: id, Path: "/media/show/" + name}
}

func neighbor(id int64, offset time.Duration) RelatedNeighbor {
	return RelatedNeighbor{VideoID: id, AddedAt: relatedBase.Add(offset)}
}

// `#2` が `#10` より前に来る自然順で、後続・先行に分かれる。
func TestOrderRelatedUsesNaturalOrder(t *testing.T) {
	siblings := []RelatedSibling{sibling(1, "ep #2.mp4"), sibling(2, "ep #10.mp4"), sibling(3, "ep #9.mp4")}

	got := OrderRelated(relatedSelf(3, "ep #9.mp4"), siblings, nil)
	if want := []int64{2, 1}; !slices.Equal(got.IDs, want) {
		t.Fatalf("ids = %v, want %v", got.IDs, want)
	}
	if got.NextID != 2 {
		t.Fatalf("nextId = %d, want 2 (#10)", got.NextID)
	}

	got = OrderRelated(relatedSelf(1, "ep #2.mp4"), siblings, nil)
	if want := []int64{3, 2}; !slices.Equal(got.IDs, want) {
		t.Fatalf("ids = %v, want %v", got.IDs, want)
	}
	if got.NextID != 3 {
		t.Fatalf("nextId = %d, want 3 (#9)", got.NextID)
	}
}

// CompareNatural が同順位にする名前も、バイト順で全順序に並ぶ。id の大小は
// 名前が違う限り効かない。
func TestOrderRelatedBreaksNaturalTiesByBytes(t *testing.T) {
	cases := []struct {
		name         string
		first, later string
	}{
		{name: "leading zero", first: "01.mp4", later: "1.mp4"},
		{name: "case only", first: "A.mp4", later: "a.mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 先に来る名前に大きい id を与え、id ではなく名前で決まることを確かめる。
			const firstID, laterID = 9, 4
			siblings := []RelatedSibling{sibling(firstID, tc.first), sibling(laterID, tc.later)}

			fromFirst := OrderRelated(relatedSelf(firstID, tc.first), siblings, nil)
			if fromFirst.NextID != laterID || !slices.Equal(fromFirst.IDs, []int64{laterID}) {
				t.Fatalf("%s から: %+v, want next %d", tc.first, fromFirst, laterID)
			}

			fromLater := OrderRelated(relatedSelf(laterID, tc.later), siblings, nil)
			if fromLater.NextID != 0 {
				t.Fatalf("%s から: nextId = %d, want 省略", tc.later, fromLater.NextID)
			}
			if !slices.Equal(fromLater.IDs, []int64{firstID}) {
				t.Fatalf("%s から: ids = %v, want 先行に %d", tc.later, fromLater.IDs, firstID)
			}
		})
	}
}

// 後続が先行より前に来て、足りない分を追加日時の差の小さい順で補う。
// 差が同じなら id の大きい方が先。自分自身と、同じフォルダの動画は補う側に
// 重ねない。
func TestOrderRelatedFillsByAddedDistance(t *testing.T) {
	siblings := []RelatedSibling{sibling(10, "a.mp4"), sibling(11, "b.mp4"), sibling(12, "c.mp4")}
	neighbors := []RelatedNeighbor{
		neighbor(20, -3*time.Hour),
		neighbor(21, time.Hour),
		neighbor(22, -time.Hour),
		neighbor(23, 2*time.Hour),
		neighbor(10, time.Minute), // 同じフォルダの動画は補う側に重ねない
		neighbor(11, 0),           // 自分自身
	}

	got := OrderRelated(relatedSelf(11, "b.mp4"), siblings, neighbors)
	want := []int64{12, 10, 22, 21, 23, 20}
	if !slices.Equal(got.IDs, want) {
		t.Fatalf("ids = %v, want %v", got.IDs, want)
	}
	if got.NextID != 12 {
		t.Fatalf("nextId = %d, want 12", got.NextID)
	}
	if slices.Contains(got.IDs, 11) {
		t.Fatal("自分自身が含まれている")
	}
}

// 同じフォルダに他の動画が無いときは、追加日時の近い動画だけになり、次の動画は無い。
func TestOrderRelatedWithoutSiblings(t *testing.T) {
	got := OrderRelated(relatedSelf(1, "only.mp4"), []RelatedSibling{sibling(1, "only.mp4")},
		[]RelatedNeighbor{neighbor(2, time.Second), neighbor(3, -time.Second)})
	if want := []int64{3, 2}; !slices.Equal(got.IDs, want) {
		t.Fatalf("ids = %v, want %v", got.IDs, want)
	}
	if got.NextID != 0 {
		t.Fatalf("nextId = %d, want 省略", got.NextID)
	}
}

// 最大 20 件で、同じフォルダだけで埋まれば補わない。
func TestOrderRelatedCapsAtMax(t *testing.T) {
	var siblings []RelatedSibling
	for i := int64(1); i <= 30; i++ {
		siblings = append(siblings, RelatedSibling{VideoID: i, Path: "/media/show/" + time.Duration(i).String() + ".mp4"})
	}
	got := OrderRelated(RelatedSelf{VideoID: 25, Path: "/media/show/25ns.mp4"}, siblings,
		[]RelatedNeighbor{neighbor(99, time.Second)})
	if len(got.IDs) != MaxRelatedVideos {
		t.Fatalf("len = %d, want %d", len(got.IDs), MaxRelatedVideos)
	}
	// 後続 26..30 のあとに先行 1..15。
	want := []int64{26, 27, 28, 29, 30}
	for i := int64(1); i <= 15; i++ {
		want = append(want, i)
	}
	if !slices.Equal(got.IDs, want) {
		t.Fatalf("ids = %v, want %v", got.IDs, want)
	}

	var neighbors []RelatedNeighbor
	for i := int64(100); i < 140; i++ {
		neighbors = append(neighbors, neighbor(i, time.Duration(i)*time.Second))
	}
	got = OrderRelated(relatedSelf(1, "x.mp4"), nil, neighbors)
	if len(got.IDs) != MaxRelatedVideos || got.IDs[0] != 100 {
		t.Fatalf("ids = %v", got.IDs)
	}
}
