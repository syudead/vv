package domain

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// MaxRelatedVideos は関連動画として返す上限である。
const MaxRelatedVideos = 20

// RelatedSelf は関連動画を並べる基準の動画である。Path は代表の所在の
// 絶対パスで、同じディレクトリの中での位置はこのファイル名で決まる。
type RelatedSelf struct {
	VideoID int64
	Path    string
	AddedAt time.Time
}

// RelatedSibling は代表の所在と同じディレクトリの直下にある動画1件である。
// 同じ動画の所在がそのディレクトリに2つ以上あるときは、パスの小さい方を渡す。
type RelatedSibling struct {
	VideoID int64
	Path    string
}

// RelatedNeighbor は追加日時の近い動画1件である。
type RelatedNeighbor struct {
	VideoID int64
	AddedAt time.Time
}

// RelatedOrder は関連動画の並びである。
type RelatedOrder struct {
	// IDs は返す順の動画 id。最大 MaxRelatedVideos 件で、基準の動画は含まない。
	IDs []int64
	// NextID は同じディレクトリで自然順の次の動画。無ければ 0。
	NextID int64
}

// RelatedVideos は関連動画の本体を返す順に並べたものである。
type RelatedVideos struct {
	Items []Video
	// NextID は同じディレクトリで自然順の次の動画。無ければ 0。
	NextID int64
}

// OrderRelated は関連動画を並べる。
//
// 並びは、同じディレクトリで基準より後のもの、前のもの、それ以外を追加日時の
// 差の小さい順、の順である。同じディレクトリの中は全順序で比べる
// （compareSiblings）。CompareNatural は `01.mp4` と `1.mp4` や大文字小文字だけが
// 違う名前を同順位にするので、それを後続にも先行にも入れない抜けを作らないため
// である。追加日時の差が同じときは id の大きい方を先にする。
func OrderRelated(self RelatedSelf, siblings []RelatedSibling, neighbors []RelatedNeighbor) RelatedOrder {
	selfEntry := siblingEntry{id: self.VideoID, name: filepath.Base(self.Path)}

	seen := map[int64]struct{}{self.VideoID: {}}
	var successors, predecessors []siblingEntry
	for _, sibling := range siblings {
		if _, dup := seen[sibling.VideoID]; dup {
			continue
		}
		seen[sibling.VideoID] = struct{}{}
		entry := siblingEntry{id: sibling.VideoID, name: filepath.Base(sibling.Path)}
		if compareSiblings(entry, selfEntry) > 0 {
			successors = append(successors, entry)
		} else {
			predecessors = append(predecessors, entry)
		}
	}
	slices.SortFunc(successors, compareSiblings)
	slices.SortFunc(predecessors, compareSiblings)

	order := RelatedOrder{IDs: make([]int64, 0, MaxRelatedVideos)}
	if len(successors) > 0 {
		order.NextID = successors[0].id
	}
	for _, entry := range slices.Concat(successors, predecessors) {
		if len(order.IDs) == MaxRelatedVideos {
			return order
		}
		order.IDs = append(order.IDs, entry.id)
	}

	fill := make([]RelatedNeighbor, 0, len(neighbors))
	for _, neighbor := range neighbors {
		if _, dup := seen[neighbor.VideoID]; dup {
			continue
		}
		seen[neighbor.VideoID] = struct{}{}
		fill = append(fill, neighbor)
	}
	slices.SortFunc(fill, func(a, b RelatedNeighbor) int {
		if byDistance := cmp.Compare(addedDistance(a.AddedAt, self.AddedAt), addedDistance(b.AddedAt, self.AddedAt)); byDistance != 0 {
			return byDistance
		}
		return cmp.Compare(b.VideoID, a.VideoID)
	})
	for _, neighbor := range fill {
		if len(order.IDs) == MaxRelatedVideos {
			break
		}
		order.IDs = append(order.IDs, neighbor.VideoID)
	}
	return order
}

type siblingEntry struct {
	id   int64
	name string
}

// compareSiblings は同じディレクトリの動画を全順序で比べる。ファイル名の
// 自然順、次にバイト順、最後に id の小さい方を先にする。
func compareSiblings(a, b siblingEntry) int {
	if order := CompareNatural(a.name, b.name); order != 0 {
		return order
	}
	if order := strings.Compare(a.name, b.name); order != 0 {
		return order
	}
	return cmp.Compare(a.id, b.id)
}

func addedDistance(a, b time.Time) time.Duration {
	if a.Before(b) {
		return b.Sub(a)
	}
	return a.Sub(b)
}
