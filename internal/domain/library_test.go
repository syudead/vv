package domain

import (
	"cmp"
	"slices"
	"testing"
)

func TestVideoSortValid(t *testing.T) {
	for _, sort := range []VideoSort{
		SortAddedAsc, SortAddedDesc, SortModifiedAsc, SortModifiedDesc,
		SortTitleAsc, SortTitleDesc, SortDurationAsc, SortDurationDesc,
		SortSizeAsc, SortSizeDesc, SortPlayedAsc, SortPlayedDesc, SortRandom,
		SortCreatedAsc, SortCreatedDesc, SortFavoritedAsc, SortFavoritedDesc,
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

func TestVideoQueryListsSmartFolders(t *testing.T) {
	for _, q := range []VideoQuery{
		{},
		{Sort: SortTitleAsc, Seed: 3, Cursor: "c", Limit: 10},
		{Query: "   "},
		{FavoriteOnly: true},
		{Kind: VideoKindAll},
	} {
		if !q.ListsSmartFolders() {
			t.Errorf("%+v.ListsSmartFolders() = false, want true", q)
		}
	}
	for _, q := range []VideoQuery{
		{Query: "cat"},
		{TagIDs: []int64{1}},
		{Person: 1},
		{Organization: 1},
		{TagKindID: 1},
		{FavoriteOnly: true, TagIDs: []int64{1}},
		{Kind: VideoKindFile},
		{Kind: VideoKindVirtual},
	} {
		if q.ListsSmartFolders() {
			t.Errorf("%+v.ListsSmartFolders() = true, want false", q)
		}
	}
}
