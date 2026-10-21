package domain

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Related ranking, pagination, and reads depend only on RelatedScorer scores so scoring changes remain isolated.

type RelatedCandidate struct {
	VideoID int64
	Path    string
	Title   string
	AddedAt time.Time
	// TagIDs includes confirmed, tentative, and folder-derived tags without distinction.
	TagIDs      []int64
	Fingerprint *Fingerprint
}

// RelatedScore uses fixed-point units to keep ties and cursors independent of floating-point drift.
type RelatedScore int64

const RelatedScoreUnit RelatedScore = 1_000_000

type RelatedScorer interface {
	// For prepares the reference once and returns a scorer reused for every candidate.
	For(self RelatedCandidate) func(other RelatedCandidate) RelatedScore
}

// ContentRelatedness totals scene overlap (40), tag Jaccard (30), title/file words (25), shared directory (20), and date proximity (5).
type ContentRelatedness struct{}

var DefaultRelatedScorer RelatedScorer = ContentRelatedness{}

const (
	relatedFingerprintPoints = 40
	relatedTagPoints         = 30
	relatedTitlePoints       = 25
	relatedFolderPoints      = 20
	relatedDatePoints        = 5

	relatedDateFullRange = 24 * time.Hour
	// Date scores halve every 30 days after the full-score interval.
	relatedDateHalfLife = 30 * 24 * time.Hour
	// Scene similarity uses the same 63-bit distance threshold as duplicate detection.
	relatedSceneMaxDistance = FingerprintMatchMaxDistance
	// Scene overlap compares every scene with every other, so a fingerprint sampled each second is thinned to as
	// many scenes as a seek sprite holds.
	relatedSceneMaxFrames = SeekSpriteMaxFrames
)

func (ContentRelatedness) For(self RelatedCandidate) func(RelatedCandidate) RelatedScore {
	selfDir := filepath.Dir(self.Path)
	selfTags := uniqueIDs(self.TagIDs)
	selfWords := titleWords(self.Title, self.Path)
	selfScenes := sceneHashes(self.Fingerprint)
	return func(other RelatedCandidate) RelatedScore {
		points := relatedFingerprintPoints*sceneOverlap(selfScenes, sceneHashes(other.Fingerprint)) +
			relatedTagPoints*jaccard(selfTags, uniqueIDs(other.TagIDs)) +
			relatedTitlePoints*jaccard(selfWords, titleWords(other.Title, other.Path)) +
			relatedDatePoints*dateCloseness(self.AddedAt, other.AddedAt)
		if filepath.Dir(other.Path) == selfDir {
			points += relatedFolderPoints
		}
		return RelatedScore(math.Round(points * float64(RelatedScoreUnit)))
	}
}

// sceneHashes rejects missing/obsolete fingerprints and insufficient non-flat frames.
func sceneHashes(fingerprint *Fingerprint) []uint64 {
	if fingerprint == nil || fingerprint.Version != FingerprintVersion {
		return nil
	}
	hashes := make([]uint64, 0, len(fingerprint.Frames))
	for _, frame := range fingerprint.Frames {
		if !frame.Flat {
			hashes = append(hashes, frame.Hash)
		}
	}
	if len(hashes) < FingerprintMinComparableFrames {
		return nil
	}
	if len(hashes) <= relatedSceneMaxFrames {
		return hashes
	}
	spread := make([]uint64, 0, relatedSceneMaxFrames)
	for _, index := range SpreadFrameIndexes(len(hashes), relatedSceneMaxFrames) {
		spread = append(spread, hashes[index])
	}
	return spread
}

// sceneOverlap ignores timestamps so excerpts and reordered scenes can still be related.
func sceneOverlap(a, b []uint64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	matchedB := make([]bool, len(b))
	matched := 0
	for _, ha := range a {
		hit := false
		for j, hb := range b {
			if bits.OnesCount64(ha^hb) <= relatedSceneMaxDistance {
				hit = true
				matchedB[j] = true
			}
		}
		if hit {
			matched++
		}
	}
	for _, hit := range matchedB {
		if hit {
			matched++
		}
	}
	return float64(matched) / float64(len(a)+len(b))
}

func jaccard[T comparable](a, b map[T]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for item := range b {
		if _, ok := a[item]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}

func uniqueIDs(ids []int64) map[int64]struct{} {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// titleWords folds text, discards numeric runs, uses CJK bigrams (single characters when alone), and drops other one-letter words such as p in 1080p.
func titleWords(title, path string) map[string]struct{} {
	words := map[string]struct{}{}
	base := filepath.Base(path)
	for _, text := range []string{title, strings.TrimSuffix(base, filepath.Ext(base))} {
		addWords(words, FoldForMatch(text))
	}
	return words
}

func addWords(words map[string]struct{}, text string) {
	var run []rune
	runCJK := false
	flush := func() {
		switch {
		case runCJK && len(run) == 1:
			words[string(run)] = struct{}{}
		case runCJK:
			for i := 0; i+1 < len(run); i++ {
				words[string(run[i:i+2])] = struct{}{}
			}
		case len(run) > 1:
			words[string(run)] = struct{}{}
		}
		run = run[:0]
	}
	for _, r := range text {
		if !unicode.IsLetter(r) && !unicode.IsMark(r) {
			flush()
			continue
		}
		cjk := isCJK(r)
		if len(run) > 0 && cjk != runCJK && !unicode.IsMark(r) {
			flush()
		}
		if len(run) == 0 {
			runCJK = cjk
		}
		run = append(run, r)
	}
	flush()
}

// isCJK includes the kana prolonged-sound mark.
func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) || r == 'ー'
}

func dateCloseness(a, b time.Time) float64 {
	gap := a.Sub(b)
	if gap < 0 {
		gap = -gap
	}
	if gap <= relatedDateFullRange {
		return 1
	}
	return math.Exp2(-float64(gap-relatedDateFullRange) / float64(relatedDateHalfLife))
}

type RankedRelated struct {
	Score RelatedScore
	// AddedAt uses Unix seconds for tie-breaking.
	AddedAt int64
	VideoID int64
}

func (r RankedRelated) Cursor() RelatedCursor {
	return RelatedCursor(r)
}

// RankRelated orders by score, then newest AddedAt, then largest ID; it excludes self and duplicate IDs.
func RankRelated(scorer RelatedScorer, self RelatedCandidate, candidates []RelatedCandidate) []RankedRelated {
	score := scorer.For(self)
	seen := map[int64]struct{}{self.VideoID: {}}
	ranked := make([]RankedRelated, 0, len(candidates))
	for _, candidate := range candidates {
		if _, dup := seen[candidate.VideoID]; dup {
			continue
		}
		seen[candidate.VideoID] = struct{}{}
		ranked = append(ranked, RankedRelated{
			VideoID: candidate.VideoID,
			Score:   score(candidate),
			AddedAt: candidate.AddedAt.Unix(),
		})
	}
	slices.SortFunc(ranked, compareRanked)
	return ranked
}

func compareRanked(a, b RankedRelated) int {
	if order := cmp.Compare(b.Score, a.Score); order != 0 {
		return order
	}
	if order := cmp.Compare(b.AddedAt, a.AddedAt); order != 0 {
		return order
	}
	return cmp.Compare(b.VideoID, a.VideoID)
}

// RelatedCursor is stable across additions, removals, and visibility changes because scores depend only on the two videos' cues.
type RelatedCursor struct {
	Score   RelatedScore
	AddedAt int64
	VideoID int64
}

var ErrInvalidRelatedCursor = errors.New("the related videos cursor is malformed")

func (c RelatedCursor) String() string {
	return fmt.Sprintf("%d.%d.%d", c.Score, c.AddedAt, c.VideoID)
}

func ParseRelatedCursor(text string) (RelatedCursor, error) {
	parts := strings.Split(text, ".")
	if len(parts) != 3 {
		return RelatedCursor{}, ErrInvalidRelatedCursor
	}
	var values [3]int64
	for i, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return RelatedCursor{}, ErrInvalidRelatedCursor
		}
		values[i] = value
	}
	if values[2] < 1 {
		return RelatedCursor{}, ErrInvalidRelatedCursor
	}
	return RelatedCursor{Score: RelatedScore(values[0]), AddedAt: values[1], VideoID: values[2]}, nil
}

const (
	DefaultRelatedPageSize = 30
	MaxRelatedPageSize     = 100
)

// PageRelated returns a next cursor only when more results remain; nil after starts at the beginning.
func PageRelated(ranked []RankedRelated, after *RelatedCursor, limit int) ([]RankedRelated, *RelatedCursor) {
	start := 0
	if after != nil {
		start, _ = slices.BinarySearchFunc(ranked, RankedRelated(*after), compareRanked)
		if start < len(ranked) && ranked[start].Cursor() == *after {
			start++
		}
	}
	end := min(start+max(limit, 0), len(ranked))
	page := ranked[start:end]
	if end >= len(ranked) || len(page) == 0 {
		return page, nil
	}
	next := page[len(page)-1].Cursor()
	return page, &next
}
