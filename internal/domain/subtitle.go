package domain

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const SubtitleFileLimit int64 = 4 << 20

// ErrSubtitleUnreadable includes empty/undecodable files, SRT without cues, and WebVTT without its header.
var ErrSubtitleUnreadable = errors.New("subtitle file is unreadable")

type SubtitleFormat string

const (
	SubtitleFormatSRT SubtitleFormat = "srt"
	SubtitleFormatVTT SubtitleFormat = "vtt"
)

type SidecarEntry struct {
	Name string
	Size int64
}

type SubtitleSidecar struct {
	// File retains the original directory entry spelling for opening.
	File   string
	Label  string
	Format SubtitleFormat
}

// SubtitleSidecars matches NFC/case-folded video stems and extensions, preserving label spelling; it excludes oversized files, prefers VTT per folded label, and orders unlabeled first then labels naturally.
func SubtitleSidecars(videoFileName string, entries []SidecarEntry) []SubtitleSidecar {
	stem := norm.NFC.String(videoFileName)
	if dot := strings.LastIndexByte(stem, '.'); dot > 0 {
		stem = stem[:dot]
	}
	chosen := map[string]SubtitleSidecar{}
	for _, entry := range entries {
		if entry.Size > SubtitleFileLimit {
			continue
		}
		candidate, ok := matchSidecar(stem, entry.Name)
		if !ok {
			continue
		}
		key := FoldKey(norm.NFC.String(candidate.Label))
		if current, exists := chosen[key]; !exists || preferSidecar(candidate, current) {
			chosen[key] = candidate
		}
	}
	sidecars := make([]SubtitleSidecar, 0, len(chosen))
	for _, sidecar := range chosen {
		sidecars = append(sidecars, sidecar)
	}
	slices.SortFunc(sidecars, func(a, b SubtitleSidecar) int {
		if (a.Label == "") != (b.Label == "") {
			if a.Label == "" {
				return -1
			}
			return 1
		}
		if order := CompareNatural(a.Label, b.Label); order != 0 {
			return order
		}
		if order := strings.Compare(a.Label, b.Label); order != 0 {
			return order
		}
		return strings.Compare(a.File, b.File)
	})
	return sidecars
}

// Normalize prefixes at original dot boundaries because NFC can change byte lengths while labels must retain their spelling.
func matchSidecar(stem, name string) (SubtitleSidecar, bool) {
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 {
		return SubtitleSidecar{}, false
	}
	var format SubtitleFormat
	switch strings.ToLower(name[dot+1:]) {
	case string(SubtitleFormatSRT):
		format = SubtitleFormatSRT
	case string(SubtitleFormatVTT):
		format = SubtitleFormatVTT
	default:
		return SubtitleSidecar{}, false
	}
	base := name[:dot]
	for end := 0; end <= len(base); end++ {
		if end < len(base) && base[end] != '.' {
			continue
		}
		rest, ok := trimPrefixFold(norm.NFC.String(base[:end]), stem)
		if !ok || rest != "" {
			continue
		}
		if end == len(base) {
			return SubtitleSidecar{File: name, Label: "", Format: format}, true
		}
		if end+1 == len(base) {
			return SubtitleSidecar{}, false
		}
		return SubtitleSidecar{File: name, Label: base[end+1:], Format: format}, true
	}
	return SubtitleSidecar{}, false
}

// Compare rune-by-rune because case-equivalent characters can have different byte lengths.
func trimPrefixFold(s, prefix string) (string, bool) {
	for prefix != "" {
		if s == "" {
			return "", false
		}
		rp, sizeP := utf8.DecodeRuneInString(prefix)
		rs, sizeS := utf8.DecodeRuneInString(s)
		if !equalFoldRune(rp, rs) {
			return "", false
		}
		prefix, s = prefix[sizeP:], s[sizeS:]
	}
	return s, true
}

func equalFoldRune(a, b rune) bool {
	if a == b {
		return true
	}
	for r := unicode.SimpleFold(a); r != a; r = unicode.SimpleFold(r) {
		if r == b {
			return true
		}
	}
	return false
}

// FoldKey selects the smallest rune in each Unicode simple-fold class so equivalent strings share a key (including Σ, σ, and ς).
func FoldKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		least := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			least = min(least, f)
		}
		b.WriteRune(least)
	}
	return b.String()
}

func preferSidecar(a, b SubtitleSidecar) bool {
	if a.Format != b.Format {
		return a.Format == SubtitleFormatVTT
	}
	if order := CompareNatural(a.File, b.File); order != 0 {
		return order < 0
	}
	return a.File < b.File
}
