package domain

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// SubtitleFileLimit は字幕ファイルの大きさの上限（4 MiB）である。これを超える
// ファイルは字幕の候補にしない（specs/028-sidecar-subtitles/research.md R-3）。
const SubtitleFileLimit int64 = 4 << 20

// ErrSubtitleUnreadable は、字幕ファイルの中身が読めないこと（空、復号できない、
// SRT の cue が 1 つも読めない、WebVTT のヘッダーが無い）を表す。
var ErrSubtitleUnreadable = errors.New("subtitle file is unreadable")

// SubtitleFormat は字幕ファイルの元の形式である。
type SubtitleFormat string

// 対応する字幕の形式。値は拡張子（小文字）と同じである。
const (
	SubtitleFormatSRT SubtitleFormat = "srt"
	SubtitleFormatVTT SubtitleFormat = "vtt"
)

// SidecarEntry は動画と同じフォルダにある通常ファイル 1 つの名前と大きさである。
type SidecarEntry struct {
	Name string
	Size int64
}

// SubtitleSidecar は動画の隣に置かれた字幕ファイル 1 つである。
type SubtitleSidecar struct {
	// File はフォルダの項目の名前そのもので、開くときの照合に使う。
	File string
	// Label はファイル名のラベル（`ja`、`en.forced`）。ラベルの無い字幕は "" である。
	Label  string
	Format SubtitleFormat
}

// SubtitleSidecars は、動画のファイル名 videoFileName と同じフォルダの項目 entries から、
// その動画の字幕ファイルの一覧を作る（research.md R-3）。
//
//   - `<名前>` は videoFileName から最後の拡張子だけを除いたものである。
//   - 候補は `<名前>.srt`・`<名前>.vtt`・`<名前>.<ラベル>.srt`・`<名前>.<ラベル>.vtt` で、
//     `<名前>` と拡張子は NFC 正規化のうえ大文字・小文字を区別せずに照合する。
//   - SubtitleFileLimit を超える項目は候補にしない。
//   - 同じラベル（大文字・小文字を区別しない）の `.srt` と `.vtt` は `.vtt` だけを残し、
//     同じ拡張子どうしは名前の自然順で先の 1 つを残す。
//   - 並びはラベルの無いものが先頭で、続いてラベルの自然順である。
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
		key := strings.ToLower(candidate.Label)
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

// matchSidecar は name が stem の字幕ファイルの名前のときだけ、その字幕を返す。
func matchSidecar(stem, name string) (SubtitleSidecar, bool) {
	normalized := norm.NFC.String(name)
	dot := strings.LastIndexByte(normalized, '.')
	if dot < 0 {
		return SubtitleSidecar{}, false
	}
	var format SubtitleFormat
	switch strings.ToLower(normalized[dot+1:]) {
	case string(SubtitleFormatSRT):
		format = SubtitleFormatSRT
	case string(SubtitleFormatVTT):
		format = SubtitleFormatVTT
	default:
		return SubtitleSidecar{}, false
	}
	rest, ok := trimPrefixFold(normalized[:dot], stem)
	if !ok {
		return SubtitleSidecar{}, false
	}
	var label string
	if rest != "" {
		if rest[0] != '.' || len(rest) == 1 {
			return SubtitleSidecar{}, false
		}
		label = rest[1:]
	}
	return SubtitleSidecar{File: name, Label: label, Format: format}, true
}

// trimPrefixFold は s が prefix で始まる（大文字・小文字を区別しない）とき、残りを返す。
// 大文字と小文字でバイト長が違う文字があるので、文字ごとに比べる。
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

// preferSidecar は同じラベルの 2 つのうち a を残すべきときに真を返す。
func preferSidecar(a, b SubtitleSidecar) bool {
	if a.Format != b.Format {
		return a.Format == SubtitleFormatVTT
	}
	if order := CompareNatural(a.File, b.File); order != 0 {
		return order < 0
	}
	return a.File < b.File
}
