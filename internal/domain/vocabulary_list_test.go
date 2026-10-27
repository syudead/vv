package domain

import (
	"errors"
	"slices"
	"testing"
)

func vocabularyIDs(entries []VocabularyEntry[string]) []int64 {
	ids := make([]int64, len(entries))
	for i, entry := range entries {
		ids[i] = entry.ID
	}
	return ids
}

func vocabularyEntry(id int64, name string, count int) VocabularyEntry[string] {
	return VocabularyEntry[string]{Value: name, ID: id, Name: name, Count: count, Names: []string{name}}
}

func TestMergeVocabularyJoinsExactNamesIntoTheLowestLocalID(t *testing.T) {
	local := []VocabularyEntry[string]{
		vocabularyEntry(7, "佐藤", 1), vocabularyEntry(3, "佐藤", 2), vocabularyEntry(5, "ABC", 1),
		{ID: 9, Provisional: true, Count: 1},
	}
	peer := []VocabularyEntry[string]{
		vocabularyEntry(ToRemoteID(1), "佐藤", 4),
		// Today a name differing only by width is another name, so it does not join.
		vocabularyEntry(ToRemoteID(2), "ＡＢＣ", 1),
		{ID: ToRemoteID(3), Provisional: true, Count: 1},
	}
	merged := MergeVocabulary(local, peer, func(local, peer string) string { return local + "+" + peer })

	if want := []int64{7, 3, 5, 9, ToRemoteID(2), ToRemoteID(3)}; !slices.Equal(vocabularyIDs(merged), want) {
		t.Fatalf("ids = %v, want %v", vocabularyIDs(merged), want)
	}
	joined := merged[1]
	if joined.Count != 6 || joined.Value != "佐藤+佐藤" || !slices.Equal(joined.Names, []string{"佐藤", "佐藤"}) {
		t.Errorf("joined = %+v, want counts summed into id 3", joined)
	}
	if merged[0].Count != 1 || len(local[1].Names) != 1 {
		t.Errorf("the other local 佐藤 or the input changed: %+v, %+v", merged[0], local[1])
	}
}

func TestSortVocabularyBreaksTiesByNameThenLocalFirst(t *testing.T) {
	entries := []VocabularyEntry[string]{
		vocabularyEntry(ToRemoteID(1), "b", 2), vocabularyEntry(4, "a10", 2), vocabularyEntry(2, "b", 2),
		vocabularyEntry(3, "a9", 2), {ID: 1, Provisional: true, Count: 2}, vocabularyEntry(5, "z", 3),
	}
	SortVocabulary(entries, TagSortCountDesc)
	if want := []int64{5, 3, 4, 2, ToRemoteID(1), 1}; !slices.Equal(vocabularyIDs(entries), want) {
		t.Errorf("countDesc = %v, want %v", vocabularyIDs(entries), want)
	}
}

func TestSortVocabularyByAgeListsUnknownBirthDatesLast(t *testing.T) {
	entries := []VocabularyEntry[string]{
		{ID: 1, Name: "a"}, {ID: 2, Name: "b", BirthKey: 19900000}, {ID: 3, Name: "c", BirthKey: 20000101},
	}
	SortVocabulary(entries, CreditSortAgeAsc)
	if want := []int64{3, 2, 1}; !slices.Equal(vocabularyIDs(entries), want) {
		t.Errorf("ageAsc = %v, want %v", vocabularyIDs(entries), want)
	}
	SortVocabulary(entries, CreditSortAgeDesc)
	if want := []int64{2, 3, 1}; !slices.Equal(vocabularyIDs(entries), want) {
		t.Errorf("ageDesc = %v, want %v", vocabularyIDs(entries), want)
	}
}

func TestVocabularyEntryMatchesFoldedSearch(t *testing.T) {
	entry := VocabularyEntry[string]{Names: []string{"スタジオ", "Studio Ｘ"}}
	for search, want := range map[string]bool{"": true, " すたじ ": true, "studio x": true, "犬": false} {
		if got := entry.MatchesSearch(search); got != want {
			t.Errorf("MatchesSearch(%q) = %v, want %v", search, got, want)
		}
	}
}

func TestPageVocabularyContinuesAtTheCursor(t *testing.T) {
	entries := []VocabularyEntry[string]{vocabularyEntry(1, "a", 0), vocabularyEntry(2, "b", 0), vocabularyEntry(3, "c", 0)}
	first, next, err := PageVocabulary(entries, TagSortName, "", 2)
	if err != nil || !slices.Equal(vocabularyIDs(first), []int64{1, 2}) || next == "" {
		t.Fatalf("first page = %v, next %q, err %v", vocabularyIDs(first), next, err)
	}
	second, end, err := PageVocabulary(entries, TagSortName, next, 2)
	if err != nil || !slices.Equal(vocabularyIDs(second), []int64{3}) || end != "" {
		t.Errorf("second page = %v, next %q, err %v", vocabularyIDs(second), end, err)
	}
	if _, _, err := PageVocabulary(entries, TagSortCountDesc, next, 2); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("a cursor of another sort: err = %v, want ErrInvalidCursor", err)
	}
}
