package domain

import (
	"cmp"
	"math/bits"
	"slices"
	"time"
)

const (
	// ImageSearchRefineMaxDistance bounds the near misses worth decoding between keyframes; unrelated hashes fall within 18 bits once in about 2,200 pairs.
	ImageSearchRefineMaxDistance = 18
	ImageSearchRefineLimit       = 10
	ImageSearchRefineMaxMs       = 10_000
	ImageSearchMaxResults        = 50

	imageSearchSeedIntervalMs = 10_000
	imageSearchMinSeeds       = 64
	imageSearchMaxSeeds       = 512
	imageSearchVoteBinMs      = 2_000
	// Every index entry has a query frame within this distance when the query is sampled once a second.
	imageSearchFrameToleranceMs = 500
	imageSearchHashBits         = 63
)

type ImageSearchMatchKind string

const (
	ImageSearchMatchFrame   ImageSearchMatchKind = "frame"
	ImageSearchMatchWhole   ImageSearchMatchKind = "whole"
	ImageSearchMatchPartial ImageSearchMatchKind = "partial"
)

type ImageSearchFrame struct {
	TimeMs int64
	Hash   uint64
}

// ImageSearchContent is one searchable video; Frames are its non-flat index entries in increasing time.
type ImageSearchContent struct {
	VideoID    int64
	AddedAt    time.Time
	DurationMs int64
	Frames     []ImageSearchFrame
}

// ImageSearchVideoQuery holds a query video's frames, Frames[i] taken at i*IntervalMs.
type ImageSearchVideoQuery struct {
	DurationMs int64
	IntervalMs int64
	Frames     []FrameHash
}

// ImageSearchMatch is one listed video; the section fields are set only for a video query.
type ImageSearchMatch struct {
	VideoID      int64
	AddedAt      time.Time
	Kind         ImageSearchMatchKind
	Rate         float64
	PositionMs   int64
	StartMs      int64
	EndMs        int64
	QueryStartMs int64
	QueryEndMs   int64
}

// ImageSearchRefinement asks for the frames of [StartMs, EndMs) to be decoded and compared with the query image.
type ImageSearchRefinement struct {
	VideoID  int64
	AddedAt  time.Time
	Distance int
	StartMs  int64
	EndMs    int64
}

// ImageMatchRate ranks image matches by closeness, since a single query frame cannot be counted as a share.
func ImageMatchRate(distance int) float64 {
	return float64(imageSearchHashBits-distance) / imageSearchHashBits
}

// NewImageFrameMatch builds the match of an image query at one frame of a content.
func NewImageFrameMatch(content ImageSearchContent, distance int, positionMs int64) ImageSearchMatch {
	return ImageSearchMatch{
		VideoID:    content.VideoID,
		AddedAt:    content.AddedAt,
		Kind:       ImageSearchMatchFrame,
		Rate:       ImageMatchRate(distance),
		PositionMs: positionMs,
	}
}

// MatchImageQuery lists each content whose nearest keyframe is within FingerprintMatchMaxDistance, and returns the closest near misses to refine from the file.
func MatchImageQuery(query uint64, contents []ImageSearchContent) ([]ImageSearchMatch, []ImageSearchRefinement) {
	var matches []ImageSearchMatch
	var refinements []ImageSearchRefinement
	for _, content := range contents {
		index, distance, ok := nearestImageSearchFrame(query, content.Frames)
		switch {
		case !ok || distance > ImageSearchRefineMaxDistance:
		case distance <= FingerprintMatchMaxDistance:
			matches = append(matches, NewImageFrameMatch(content, distance, content.Frames[index].TimeMs))
		default:
			start := content.Frames[index].TimeMs
			refinements = append(refinements, ImageSearchRefinement{
				VideoID:  content.VideoID,
				AddedAt:  content.AddedAt,
				Distance: distance,
				StartMs:  start,
				EndMs:    min(entrySpanEnd(content, index), start+ImageSearchRefineMaxMs),
			})
		}
	}
	slices.SortStableFunc(refinements, func(a, b ImageSearchRefinement) int {
		return cmp.Compare(a.Distance, b.Distance)
	})
	if len(refinements) > ImageSearchRefineLimit {
		refinements = refinements[:ImageSearchRefineLimit]
	}
	return matches, refinements
}

// nearestImageSearchFrame returns the earliest frame at the smallest distance.
func nearestImageSearchFrame(query uint64, frames []ImageSearchFrame) (index, distance int, ok bool) {
	distance = imageSearchHashBits + 1
	for i, frame := range frames {
		if d := bits.OnesCount64(query ^ frame.Hash); d < distance {
			index, distance = i, d
		}
	}
	return index, distance, len(frames) > 0
}

// entrySpanEnd is where entry i's span ends: the next entry, or the end of the video.
func entrySpanEnd(content ImageSearchContent, i int) int64 {
	if i+1 < len(content.Frames) {
		return content.Frames[i+1].TimeMs
	}
	return max(content.DurationMs, content.Frames[i].TimeMs)
}

func entrySpanStart(content ImageSearchContent, i int) int64 {
	if i == 0 {
		return 0
	}
	return content.Frames[i].TimeMs
}

// VideoQueryMatcher matches one video query against contents; Match is safe to call from several goroutines.
type VideoQueryMatcher struct {
	query ImageSearchVideoQuery
	seeds []ImageSearchFrame
	votes int
}

// NewVideoQueryMatcher picks one non-flat seed frame per 10 s of the query, between 64 (or all, when fewer) and 512.
func NewVideoQueryMatcher(query ImageSearchVideoQuery) VideoQueryMatcher {
	var informative []int
	for i, frame := range query.Frames {
		if !frame.Flat {
			informative = append(informative, i)
		}
	}
	want := min(max(query.DurationMs/imageSearchSeedIntervalMs, imageSearchMinSeeds), imageSearchMaxSeeds)
	count := int(min(want, int64(len(informative))))
	seeds := make([]ImageSearchFrame, count)
	for k := range count {
		i := informative[k*len(informative)/count]
		seeds[k] = ImageSearchFrame{TimeMs: int64(i) * query.IntervalMs, Hash: query.Frames[i].Hash}
	}
	votes := 2
	if count == 1 {
		votes = 1
	}
	return VideoQueryMatcher{query: query, seeds: seeds, votes: votes}
}

// MatchVideoQuery runs one matcher over every content.
func MatchVideoQuery(query ImageSearchVideoQuery, contents []ImageSearchContent) []ImageSearchMatch {
	matcher := NewVideoQueryMatcher(query)
	var matches []ImageSearchMatch
	for _, content := range contents {
		if match, ok := matcher.Match(content); ok {
			matches = append(matches, match)
		}
	}
	return matches
}

// Match verifies every voted offset over the whole overlap and lists the best one at a rate of at least 50% over at least FingerprintMinComparableFrames entries.
func (m VideoQueryMatcher) Match(content ImageSearchContent) (ImageSearchMatch, bool) {
	var best offsetVerification
	if m.query.IntervalMs <= 0 {
		return ImageSearchMatch{}, false
	}
	for _, offset := range m.votedOffsets(content) {
		if v := m.verify(content, offset); v.better(best) {
			best = v
		}
	}
	if !best.listable() {
		return ImageSearchMatch{}, false
	}
	return m.section(content, best), true
}

// votedOffsets returns, in increasing order, the offsets (content time minus query time) of the 2 s bins enough distinct seeds voted for, with their sampling centres.
func (m VideoQueryMatcher) votedOffsets(content ImageSearchContent) []int64 {
	seedsByBin := map[int64]map[int]struct{}{}
	offsetsByBin := map[int64][]int64{}
	for s, seed := range m.seeds {
		for _, frame := range content.Frames {
			if bits.OnesCount64(seed.Hash^frame.Hash) > FingerprintMatchMaxDistance {
				continue
			}
			offset := frame.TimeMs - seed.TimeMs
			bin := floorDiv(offset, imageSearchVoteBinMs)
			if seedsByBin[bin] == nil {
				seedsByBin[bin] = map[int]struct{}{}
			}
			seedsByBin[bin][s] = struct{}{}
			offsetsByBin[bin] = append(offsetsByBin[bin], offset)
		}
	}
	var offsets []int64
	for bin, seeds := range seedsByBin {
		if len(seeds) >= m.votes {
			offsets = append(offsets, offsetsByBin[bin]...)
		}
	}
	slices.Sort(offsets)
	return withSamplingCentres(slices.Compact(offsets))
}

// withSamplingCentres adds the centre of each run of offsets spanning at most twice the frame tolerance, since every vote is off the true offset by up to that tolerance.
func withSamplingCentres(offsets []int64) []int64 {
	out := slices.Clone(offsets)
	last := 0
	for i, low := range offsets {
		last = max(last, i)
		for last+1 < len(offsets) && offsets[last+1]-low <= 2*imageSearchFrameToleranceMs {
			last++
		}
		if last > i {
			out = append(out, low+(offsets[last]-low)/2)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

type offsetVerification struct {
	offset            int64
	matched, compared int
	first, last       int
}

// better prefers more matching entries, then a listable verification, then fewer compared ones; offsets arrive in increasing order, so ties keep the earlier one.
func (v offsetVerification) better(than offsetVerification) bool {
	if v.matched != than.matched {
		return v.matched > than.matched
	}
	if v.matched == 0 {
		return false
	}
	if v.listable() != than.listable() {
		return v.listable()
	}
	return v.compared < than.compared
}

// listable reports a rate of at least 50% over at least FingerprintMinComparableFrames entries.
func (v offsetVerification) listable() bool {
	return v.compared >= FingerprintMinComparableFrames && 2*v.matched >= v.compared
}

// verify compares each entry inside the overlap with the non-flat query frame within 0.5 s of it under offset.
func (m VideoQueryMatcher) verify(content ImageSearchContent, offset int64) offsetVerification {
	v := offsetVerification{offset: offset, first: -1}
	interval := m.query.IntervalMs
	for k, frame := range content.Frames {
		at := frame.TimeMs - offset
		if at < 0 || at >= m.query.DurationMs {
			continue
		}
		j := (at + interval/2) / interval
		if j >= int64(len(m.query.Frames)) || abs64(j*interval-at) > imageSearchFrameToleranceMs || m.query.Frames[j].Flat {
			continue
		}
		v.compared++
		if bits.OnesCount64(m.query.Frames[j].Hash^frame.Hash) > FingerprintMatchMaxDistance {
			continue
		}
		v.matched++
		if v.first < 0 {
			v.first = k
		}
		v.last = k
	}
	return v
}

// section spans the first to the last matching entry's span; it is whole when it covers 90% of both the content and the query.
func (m VideoQueryMatcher) section(content ImageSearchContent, v offsetVerification) ImageSearchMatch {
	start := entrySpanStart(content, v.first)
	end := entrySpanEnd(content, v.last)
	queryStart := min(max(start-v.offset, 0), m.query.DurationMs)
	queryEnd := min(max(end-v.offset, 0), m.query.DurationMs)
	kind := ImageSearchMatchPartial
	if 10*(end-start) >= 9*content.DurationMs && 10*(queryEnd-queryStart) >= 9*m.query.DurationMs {
		kind = ImageSearchMatchWhole
	}
	return ImageSearchMatch{
		VideoID:      content.VideoID,
		AddedAt:      content.AddedAt,
		Kind:         kind,
		Rate:         float64(v.matched) / float64(v.compared),
		PositionMs:   content.Frames[v.first].TimeMs,
		StartMs:      start,
		EndMs:        end,
		QueryStartMs: queryStart,
		QueryEndMs:   queryEnd,
	}
}

// CompareImageSearchMatches orders by match rate, then matched section length, then the newer video, all descending.
func CompareImageSearchMatches(a, b ImageSearchMatch) int {
	if c := cmp.Compare(b.Rate, a.Rate); c != 0 {
		return c
	}
	if c := cmp.Compare(b.EndMs-b.StartMs, a.EndMs-a.StartMs); c != 0 {
		return c
	}
	if c := b.AddedAt.Compare(a.AddedAt); c != 0 {
		return c
	}
	return cmp.Compare(b.VideoID, a.VideoID)
}

// RankImageSearchMatches sorts matches and keeps the first ImageSearchMaxResults; callers collapse version members before ranking.
func RankImageSearchMatches(matches []ImageSearchMatch) []ImageSearchMatch {
	slices.SortFunc(matches, CompareImageSearchMatches)
	if len(matches) > ImageSearchMaxResults {
		matches = matches[:ImageSearchMaxResults]
	}
	return matches
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}
