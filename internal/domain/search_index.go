package domain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// The search index keeps the hash of each kept keyframe of one content, for image search. It is versioned apart from
// the fingerprint, so changing one never rereads every file for the other; a row of another version counts as missing.

const (
	SearchIndexVersion = 1
	// SearchIndexMaxEntries keeps 5.5 hours at the one-second floor; a longer video keeps one per
	// duration / SearchIndexMaxEntries.
	SearchIndexMaxEntries = 20_000
	// SearchIndexMinSpacingMs stops a video with a keyframe every few frames from growing the index without new scenes.
	SearchIndexMinSpacingMs = 1000

	searchIndexEntryBytes = 12
)

// KeyframeHash is one decoded keyframe: its time from the start of the video and its frame hash.
type KeyframeHash struct {
	TimeMs int64
	Frame  FrameHash
}

// SearchIndexEntry is a kept keyframe; flat frames are never kept.
type SearchIndexEntry struct {
	TimeMs int64
	Hash   uint64
}

type SearchIndex struct {
	Version int
	// Entries are in strictly increasing time.
	Entries []SearchIndexEntry
}

var ErrInvalidSearchIndex = errors.New("the search index data is malformed")

// SearchIndexSpacingMs is the least time between kept entries of a video of durationMs; an unknown duration is zero.
func SearchIndexSpacingMs(durationMs int64) int64 {
	return max(SearchIndexMinSpacingMs, durationMs/SearchIndexMaxEntries)
}

// NewSearchIndex keeps, in order, each keyframe that is not flat and comes at least the spacing after the last kept
// one, up to SearchIndexMaxEntries. A keyframe out of order or outside the encodable time range is dropped.
func NewSearchIndex(durationMs int64, keyframes []KeyframeHash) SearchIndex {
	spacing := SearchIndexSpacingMs(durationMs)
	entries := make([]SearchIndexEntry, 0, min(len(keyframes), SearchIndexMaxEntries))
	for _, keyframe := range keyframes {
		if len(entries) == SearchIndexMaxEntries {
			break
		}
		if keyframe.Frame.Flat || keyframe.TimeMs < 0 || keyframe.TimeMs > math.MaxUint32 {
			continue
		}
		if len(entries) > 0 && keyframe.TimeMs < entries[len(entries)-1].TimeMs+spacing {
			continue
		}
		entries = append(entries, SearchIndexEntry{TimeMs: keyframe.TimeMs, Hash: keyframe.Frame.Hash})
	}
	return SearchIndex{Version: SearchIndexVersion, Entries: entries}
}

// Encode stores each entry as a four-byte big-endian time in milliseconds and the eight-byte big-endian hash; the
// version lives in its own column.
func (s SearchIndex) Encode() []byte {
	out := make([]byte, 0, len(s.Entries)*searchIndexEntryBytes)
	for _, entry := range s.Entries {
		out = binary.BigEndian.AppendUint32(out, uint32(entry.TimeMs))
		out = binary.BigEndian.AppendUint64(out, entry.Hash)
	}
	return out
}

// DecodeSearchIndex rejects data whose length is not a whole number of entries or whose times do not increase; the
// caller treats such a content as unindexed.
func DecodeSearchIndex(data []byte, version int) (SearchIndex, error) {
	if len(data)%searchIndexEntryBytes != 0 {
		return SearchIndex{}, fmt.Errorf("%w: %d bytes", ErrInvalidSearchIndex, len(data))
	}
	entries := make([]SearchIndexEntry, 0, len(data)/searchIndexEntryBytes)
	for offset := 0; offset < len(data); offset += searchIndexEntryBytes {
		entry := SearchIndexEntry{
			TimeMs: int64(binary.BigEndian.Uint32(data[offset:])),
			Hash:   binary.BigEndian.Uint64(data[offset+4:]),
		}
		if len(entries) > 0 && entry.TimeMs <= entries[len(entries)-1].TimeMs {
			return SearchIndex{}, fmt.Errorf("%w: time %d ms after %d ms", ErrInvalidSearchIndex,
				entry.TimeMs, entries[len(entries)-1].TimeMs)
		}
		entries = append(entries, entry)
	}
	return SearchIndex{Version: version, Entries: entries}, nil
}
