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

// With a cap of four, a 7.2 s video keeps one keyframe per 1.8 s, which would be one entry over the cap. The frame_index
// handler's test shows the production cap through the spacing of a long video.
func TestSearchIndexCollectorCapsLongVideos(t *testing.T) {
	t.Parallel()
	collector := newSearchIndexCollector(7200, 4)
	for ms := int64(0); ms <= 7200; ms += 900 {
		collector.Add(KeyframeHash{TimeMs: ms, Frame: FrameHash{Hash: uint64(ms)}})
	}
	want := []SearchIndexEntry{{TimeMs: 0, Hash: 0}, {TimeMs: 1800, Hash: 1800}, {TimeMs: 3600, Hash: 3600},
		{TimeMs: 5400, Hash: 5400}}
	if got := collector.Index().Entries; !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %+v, want %+v", got, want)
	}
}
