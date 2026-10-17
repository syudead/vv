package domain

import (
	"errors"
	"fmt"
)

// グループの編集。保存層は書き込みの
// 取引の中で入力を読み、ここの規則で決める。

// ErrGroupNotFound はグループが無いことを表す。errors.Is(err, ErrNotFound) も真になる。
var ErrGroupNotFound = fmt.Errorf("group %w", ErrNotFound)

// ErrGroupTargetNotFound は、編集の要求が名指した動画・グループ・項目のどれかが無いことを表す。
// その編集は何も書かない。API では 404 group_target_not_found で、グループ
// そのものが無い ErrGroupNotFound（404 not_found）と区別するため ErrNotFound を包まない。
var ErrGroupTargetNotFound = errors.New("group target not found")

// ErrGroupCycle は、グループを自分自身か自分の子孫に入れようとしたことを表す。
var ErrGroupCycle = errors.New("group cycle")

// GroupAdd はグループの作成と追加の入力である。項目は動画（VideoIDs）、次にグループ（GroupIDs）の
// 順に、それぞれ要求の順で末尾に足す。
type GroupAdd struct {
	VideoIDs []int64
	GroupIDs []int64
}

// GroupTag はグループをタグにした結果である。Created はタグを新しく作ったかどうかである。
type GroupTag struct {
	Tag     TagRef
	Created bool
}

// GroupEdge は親グループ Parent が子グループ Child を項目に持つことを表す。
type GroupEdge struct {
	Parent int64
	Child  int64
}

// ValidateNesting はグループ child を親 parent の項目にしてよいかを、今の入れ子 edges から
// 決める。child が parent そのものか parent の祖先（child から子をたどって parent に着く）なら
// ErrGroupCycle である。兄弟やほかの枝のグループは入れてよい。
func ValidateNesting(parent, child int64, edges []GroupEdge) error {
	if parent == child {
		return ErrGroupCycle
	}
	children := make(map[int64][]int64, len(edges))
	for _, edge := range edges {
		children[edge.Parent] = append(children[edge.Parent], edge.Child)
	}
	seen := map[int64]bool{child: true}
	stack := []int64{child}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, next := range children[id] {
			if next == parent {
				return ErrGroupCycle
			}
			if !seen[next] {
				seen[next] = true
				stack = append(stack, next)
			}
		}
	}
	return nil
}

// GroupPlayItem は再生順を決める入力にする項目1つである。動画の項目は VideoID に見る人に見せる
// 動画の id を持ち、引けない（見せない）なら 0 である。子グループの項目は ChildID を持つ。
type GroupPlayItem struct {
	VideoID int64
	ChildID int64
}

// GroupPlayOrder はグループの再生順の動画 id を返す。items はそのグループの項目を位置の順に、children は子孫のグループごとの項目を位置の順に
// 持つ。項目を順にたどり、子グループはその再生順で置き換え（深さ優先）、各動画は最初に出た所に
// だけ残す。引けない動画の項目と children に無いグループは飛ばす。同じグループに 2 度着いても
// 2 度目はたどらない（加えるものが無く、循環があっても止まる）。
func GroupPlayOrder(items []GroupPlayItem, children map[int64][]GroupPlayItem) []int64 {
	order := []int64{}
	seenVideos := map[int64]bool{}
	seenGroups := map[int64]bool{}
	var walk func([]GroupPlayItem)
	walk = func(items []GroupPlayItem) {
		for _, item := range items {
			switch {
			case item.ChildID != 0:
				if seenGroups[item.ChildID] {
					continue
				}
				seenGroups[item.ChildID] = true
				walk(children[item.ChildID])
			case item.VideoID != 0 && !seenVideos[item.VideoID]:
				seenVideos[item.VideoID] = true
				order = append(order, item.VideoID)
			}
		}
	}
	walk(items)
	return order
}
