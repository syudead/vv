package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestSearchIndexRoundTripsThroughItsEncoding(t *testing.T) {
	t.Parallel()
	index := NewSearchIndex(60_000, []KeyframeHash{
		{TimeMs: 0, Frame: FrameHash{Hash: 0x0123456789abcdef}},
		{TimeMs: 4_000_000_000, Frame: FrameHash{Hash: 1<<64 - 1}},
	})
	decoded, err := DecodeSearchIndex(index.Encode(), SearchIndexVersion)
	if err != nil {
		t.Fatal(err)
	}
	want := SearchIndex{Version: SearchIndexVersion, Entries: []SearchIndexEntry{
		{TimeMs: 0, Hash: 0x0123456789abcdef}, {TimeMs: 4_000_000_000, Hash: 1<<64 - 1},
	}}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("decoded = %+v, want %+v", decoded, want)
	}
}

func TestDecodeSearchIndexRejectsMalformedData(t *testing.T) {
	t.Parallel()
	increasing := SearchIndex{Entries: []SearchIndexEntry{{TimeMs: 2000, Hash: 1}, {TimeMs: 2000, Hash: 2}}}.Encode()
	for name, data := range map[string][]byte{
		"partial entry":      make([]byte, 13),
		"time does not rise": increasing,
	} {
		if _, err := DecodeSearchIndex(data, SearchIndexVersion); !errors.Is(err, ErrInvalidSearchIndex) {
			t.Errorf("%s: err = %v, want ErrInvalidSearchIndex", name, err)
		}
	}
}

func TestNewSearchIndexKeepsKeyframesOneSecondApartAndDropsFlatOnes(t *testing.T) {
	t.Parallel()
	index := NewSearchIndex(10_000, []KeyframeHash{
		{TimeMs: 0, Frame: FrameHash{Hash: 1}},
		{TimeMs: 999, Frame: FrameHash{Hash: 2}},
		{TimeMs: 1000, Frame: FrameHash{Hash: 3, Flat: true}},
		{TimeMs: 1000, Frame: FrameHash{Hash: 4}},
		{TimeMs: 500, Frame: FrameHash{Hash: 5}},
		{TimeMs: 2000, Frame: FrameHash{Hash: 6}},
	})
	want := []SearchIndexEntry{{TimeMs: 0, Hash: 1}, {TimeMs: 1000, Hash: 4}, {TimeMs: 2000, Hash: 6}}
	if !reflect.DeepEqual(index.Entries, want) {
		t.Fatalf("entries = %+v, want %+v", index.Entries, want)
	}
}

// A ten-hour video keeps one keyframe per 1.8 s, which would be one entry over the cap.
func TestNewSearchIndexCapsLongVideos(t *testing.T) {
	t.Parallel()
	const durationMs = 10 * 3600 * 1000
	keyframes := make([]KeyframeHash, 0, durationMs/900+1)
	for ms := int64(0); ms <= durationMs; ms += 900 {
		keyframes = append(keyframes, KeyframeHash{TimeMs: ms, Frame: FrameHash{Hash: uint64(ms)}})
	}
	entries := NewSearchIndex(durationMs, keyframes).Entries
	if len(entries) != SearchIndexMaxEntries {
		t.Fatalf("entries = %d, want %d", len(entries), SearchIndexMaxEntries)
	}
	if gap := entries[1].TimeMs - entries[0].TimeMs; gap != 1800 {
		t.Fatalf("first gap = %d ms, want 1800", gap)
	}
}
