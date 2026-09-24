package domain

import (
	"cmp"
	"slices"
	"testing"
)

// 並び順は contracts/list-api.md §3 の 13 通りだけを受け付ける。
func TestVideoSortValid(t *testing.T) {
	for _, sort := range []VideoSort{
		SortAddedAsc, SortAddedDesc, SortModifiedAsc, SortModifiedDesc,
		SortTitleAsc, SortTitleDesc, SortDurationAsc, SortDurationDesc,
		SortSizeAsc, SortSizeDesc, SortPlayedAsc, SortPlayedDesc, SortRandom,
	} {
		if !sort.Valid() {
			t.Errorf("%q が無効になった", sort)
		}
	}
	for _, sort := range []VideoSort{"", "randomDesc", "titleasc", "added"} {
		if sort.Valid() {
			t.Errorf("%q が有効になった", sort)
		}
	}
}

// ShuffleKey は seed と id だけで決まり、負にならない。seed が違えば並びが変わる。
func TestShuffleKey(t *testing.T) {
	for _, seed := range []int64{0, 1, 42, MaxShuffleSeed} {
		for _, id := range []int64{0, 1, 2, 1000, 1 << 40} {
			got := ShuffleKey(seed, id)
			if got < 0 {
				t.Errorf("ShuffleKey(%d, %d) = %d, 負になった", seed, id, got)
			}
			if again := ShuffleKey(seed, id); again != got {
				t.Errorf("ShuffleKey(%d, %d) が呼ぶたびに変わった: %d, %d", seed, id, got, again)
			}
		}
	}

	order := func(seed int64) []int64 {
		ids := make([]int64, 50)
		for i := range ids {
			ids[i] = int64(i + 1)
		}
		slices.SortFunc(ids, func(a, b int64) int {
			if c := cmp.Compare(ShuffleKey(seed, a), ShuffleKey(seed, b)); c != 0 {
				return c
			}
			return cmp.Compare(a, b)
		})
		return ids
	}
	first := order(1)
	if slices.IsSorted(first) {
		t.Errorf("seed=1 で id の順のままになった")
	}
	if slices.Equal(first, order(2)) {
		t.Errorf("seed=1 と seed=2 で同じ並びになった")
	}
}
