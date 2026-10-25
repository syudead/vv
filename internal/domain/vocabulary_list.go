package domain

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// VocabularyEntry is one entry of a tag, tag kind, person or organization list merged from both nodes: the
// value the screen reads, with what joining, searching, sorting and paging read from it.
type VocabularyEntry[T any] struct {
	Value T
	ID    int64
	// Name joins the entries of both nodes by exact spelling, as a name is unique on one node; an empty name,
	// which a provisional person has, never joins.
	Name        string
	Provisional bool
	Count       int
	// CreatedAt is in Unix seconds.
	CreatedAt int64
	// BirthKey is a person's birth date as BirthKey gives it, zero when unknown.
	BirthKey int64
	// Names are the names, synonyms and aliases of both nodes that a search matches.
	Names   []string
	sortKey string
}

// MergeVocabulary joins each peer entry into the local entry of the same name with the lowest id, adding the
// counts and the searched names; add combines the values. Peer entries that join nothing follow the local ones
// in their own order.
func MergeVocabulary[T any](local, peer []VocabularyEntry[T], add func(local, peer T) T) []VocabularyEntry[T] {
	merged := slices.Clone(local)
	byName := make(map[string]int, len(merged))
	for i, entry := range merged {
		if j, ok := byName[entry.Name]; entry.Name != "" && (!ok || entry.ID < merged[j].ID) {
			byName[entry.Name] = i
		}
	}
	for _, entry := range peer {
		i, ok := byName[entry.Name]
		if !ok || entry.Name == "" {
			merged = append(merged, entry)
			continue
		}
		target := &merged[i]
		target.Value = add(target.Value, entry.Value)
		target.Count += entry.Count
		target.Names = append(slices.Clip(target.Names), entry.Names...)
	}
	return merged
}

// MatchesSearch folds search as the stores do and reports whether it is part of a folded name; an empty
// search matches every entry.
func (e VocabularyEntry[T]) MatchesSearch(search string) bool {
	folded := strings.TrimSpace(FoldForMatch(search))
	if folded == "" {
		return true
	}
	return slices.ContainsFunc(e.Names, func(name string) bool { return strings.Contains(FoldForMatch(name), folded) })
}

// SortVocabulary orders entries as the stores order a list: by the sort's value, then named before provisional,
// then natural name and id, so the local entry comes first on equal names.
func SortVocabulary[T any](entries []VocabularyEntry[T], sort TagSort) {
	for i := range entries {
		entries[i].sortKey = NaturalSortKey(entries[i].Name)
	}
	slices.SortStableFunc(entries, func(a, b VocabularyEntry[T]) int {
		if order := compareVocabularyValue(sort, a, b); order != 0 {
			return order
		}
		if a.Provisional != b.Provisional {
			if a.Provisional {
				return 1
			}
			return -1
		}
		if order := strings.Compare(a.sortKey, b.sortKey); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

// birthKeyUnknownLast stands for an unknown birth date in ageDesc, which lists it after every known one.
const birthKeyUnknownLast = 99999999

func compareVocabularyValue[T any](sort TagSort, a, b VocabularyEntry[T]) int {
	switch sort {
	case TagSortCountDesc:
		return cmp.Compare(b.Count, a.Count)
	case TagSortCountAsc:
		return cmp.Compare(a.Count, b.Count)
	case TagSortCreatedDesc:
		return cmp.Compare(b.CreatedAt, a.CreatedAt)
	case TagSortCreatedAsc:
		return cmp.Compare(a.CreatedAt, b.CreatedAt)
	case CreditSortAgeAsc:
		// The youngest first; an unknown date is zero and comes last.
		return cmp.Compare(b.BirthKey, a.BirthKey)
	case CreditSortAgeDesc:
		return cmp.Compare(ageDescKey(a.BirthKey), ageDescKey(b.BirthKey))
	default:
		return 0
	}
}

func ageDescKey(birthKey int64) int64 {
	if birthKey == 0 {
		return birthKeyUnknownLast
	}
	return birthKey
}

// BirthKey sorts a normalized partial date (YYYY, YYYY-MM or YYYY-MM-DD) as its first possible day, YYYYMMDD with
// missing parts as 00; ok is false for an empty or unreadable date.
func BirthKey(date string) (key int64, ok bool) {
	digits := strings.ReplaceAll(date, "-", "")
	if digits == "" {
		return 0, false
	}
	key, err := strconv.ParseInt((digits + "0000")[:8], 10, 64)
	if err != nil {
		return 0, false
	}
	return key, true
}

// vocabularyCursorPrefix tells a merged vocabulary cursor from a single node's, so a cursor of the other kind
// is refused.
const vocabularyCursorPrefix = "v1."

// PageVocabulary returns the sorted entries after cursor, at most limit of them, with the cursor that continues
// them, empty at the end. A zero limit returns every entry and ignores cursor. The cursor is the position in
// the merged list, which each page reads again whole.
func PageVocabulary[T any](entries []VocabularyEntry[T], sort TagSort, cursor string, limit int) ([]VocabularyEntry[T], string, error) {
	if limit <= 0 {
		return entries, "", nil
	}
	start := 0
	if cursor != "" {
		rest, ok := strings.CutPrefix(cursor, vocabularyCursorPrefix+string(sort)+".")
		if !ok {
			return nil, "", fmt.Errorf("%w: not a cursor of this merged list and sort", ErrInvalidCursor)
		}
		position, err := strconv.Atoi(rest)
		if err != nil || position < 0 {
			return nil, "", fmt.Errorf("%w: cannot read the position", ErrInvalidCursor)
		}
		start = min(position, len(entries))
	}
	end := min(start+limit, len(entries))
	next := ""
	if end < len(entries) {
		next = vocabularyCursorPrefix + string(sort) + "." + strconv.Itoa(end)
	}
	return entries[start:end], next, nil
}
