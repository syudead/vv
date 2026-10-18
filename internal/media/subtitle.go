package media

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	xunicode "golang.org/x/text/encoding/unicode"

	"github.com/syudead/vv/internal/domain"
)

// Convert sidecar SRT/WebVTT to shifted UTF-8 WebVTT without external processes.

// SubtitleConverter is stateless and its zero value is usable.
type SubtitleConverter struct{}

func NewSubtitleConverter() SubtitleConverter { return SubtitleConverter{} }

// Convert subtracts offsetMs, drops cues ending at or before zero, and clamps negative starts to
// zero; unreadable input wraps domain.ErrSubtitleUnreadable.
func (SubtitleConverter) Convert(src []byte, format domain.SubtitleFormat, offsetMs int64) ([]byte, error) {
	text, err := decodeSubtitle(src)
	if err != nil {
		return nil, err
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	switch format {
	case domain.SubtitleFormatSRT:
		return convertSRT(text, offsetMs)
	case domain.SubtitleFormatVTT:
		return convertVTT(text, offsetMs)
	default:
		return nil, fmt.Errorf("%w: unknown format %q", domain.ErrSubtitleUnreadable, format)
	}
}

// Scripts accepted as unambiguous BOM-less UTF-8 subtitles.
var subtitleScripts = []*unicode.RangeTable{
	unicode.Common, unicode.Inherited, unicode.Latin, unicode.Greek, unicode.Cyrillic,
	unicode.Hebrew, unicode.Arabic, unicode.Thai, unicode.Hangul, unicode.Han,
	unicode.Hiragana, unicode.Katakana, unicode.Bopomofo,
}

// Detect encoding in order: BOM, valid UTF-8, then Shift_JIS.
func decodeSubtitle(src []byte) (string, error) {
	switch {
	case len(src) == 0:
		return "", fmt.Errorf("%w: empty file", domain.ErrSubtitleUnreadable)
	case bytes.HasPrefix(src, []byte{0xEF, 0xBB, 0xBF}):
		rest := src[3:]
		if !utf8.Valid(rest) {
			return "", fmt.Errorf("%w: invalid UTF-8", domain.ErrSubtitleUnreadable)
		}
		return string(rest), nil
	case bytes.HasPrefix(src, []byte{0xFF, 0xFE}):
		return decodeUTF16(src, xunicode.LittleEndian)
	case bytes.HasPrefix(src, []byte{0xFE, 0xFF}):
		return decodeUTF16(src, xunicode.BigEndian)
	}
	valid := utf8.Valid(src)
	if valid && inSubtitleScripts(src) {
		return string(src), nil
	}
	if decoded, err := japanese.ShiftJIS.NewDecoder().Bytes(src); err == nil && cleanShiftJIS(decoded) {
		return string(decoded), nil
	}
	if valid {
		return string(src), nil
	}
	return "", fmt.Errorf("%w: unknown character encoding", domain.ErrSubtitleUnreadable)
}

func decodeUTF16(src []byte, order xunicode.Endianness) (string, error) {
	decoded, err := xunicode.UTF16(order, xunicode.ExpectBOM).NewDecoder().Bytes(src)
	if err != nil {
		return "", fmt.Errorf("%w: invalid UTF-16: %w", domain.ErrSubtitleUnreadable, err)
	}
	return strings.TrimPrefix(string(decoded), "\uFEFF"), nil
}

func inSubtitleScripts(src []byte) bool {
	for _, r := range string(src) {
		if !unicode.In(r, subtitleScripts...) {
			return false
		}
	}
	return true
}

// The Shift_JIS decoder substitutes U+FFFD without error and passes 0x80 as U+0080; reject both
// cases explicitly.
func cleanShiftJIS(decoded []byte) bool {
	for _, r := range string(decoded) {
		if r == utf8.RuneError || (r >= 0x80 && r <= 0x9F) {
			return false
		}
	}
	return true
}

type cueTiming struct {
	startMs, endMs int64
	// Preserve leading whitespace before WebVTT cue settings.
	settings string
}

func (c cueTiming) shift(offsetMs int64) (cueTiming, bool) {
	c.startMs -= offsetMs
	c.endMs -= offsetMs
	if c.endMs <= 0 {
		return c, false
	}
	c.startMs = max(c.startMs, 0)
	return c, true
}

func (c cueTiming) String() string {
	return formatTimestamp(c.startMs) + " --> " + formatTimestamp(c.endMs) + c.settings
}

// Accept h:mm:ss or mm:ss with optional one-to-three decimal digits separated by comma or period.
func parseTiming(line string) (cueTiming, bool) {
	before, after, found := strings.Cut(line, "-->")
	if !found {
		return cueTiming{}, false
	}
	start, ok := parseTimestamp(strings.TrimSpace(before))
	if !ok {
		return cueTiming{}, false
	}
	after = strings.TrimLeft(after, " \t")
	endText, settings := after, ""
	if i := strings.IndexAny(after, " \t"); i >= 0 {
		endText, settings = after[:i], after[i:]
	}
	end, ok := parseTimestamp(endText)
	if !ok {
		return cueTiming{}, false
	}
	return cueTiming{startMs: start, endMs: end, settings: strings.TrimRight(settings, " \t")}, true
}

func parseTimestamp(text string) (int64, bool) {
	clock, fraction := text, ""
	if i := strings.LastIndexAny(text, ",."); i >= 0 {
		clock, fraction = text[:i], text[i+1:]
		if fraction == "" || len(fraction) > 3 || !allDigits(fraction) {
			return 0, false
		}
	}
	parts := strings.Split(clock, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var hours, minutes, seconds int64
	values := make([]int64, len(parts))
	for i, part := range parts {
		if part == "" || !allDigits(part) || (i > 0 && len(part) > 2) || len(part) > 9 {
			return 0, false
		}
		values[i], _ = strconv.ParseInt(part, 10, 64)
	}
	if len(values) == 3 {
		hours, minutes, seconds = values[0], values[1], values[2]
	} else {
		minutes, seconds = values[0], values[1]
	}
	if (len(values) == 3 && minutes > 59) || seconds > 59 {
		return 0, false
	}
	var millis int64
	if fraction != "" {
		millis, _ = strconv.ParseInt((fraction + "00")[:3], 10, 64)
	}
	return ((hours*60+minutes)*60+seconds)*1000 + millis, true
}

func formatTimestamp(ms int64) string {
	hours := ms / 3_600_000
	minutes := ms / 60_000 % 60
	seconds := ms / 1000 % 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, ms%1000)
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// Whitespace-only lines also separate blocks.
func blocks(text string) [][]string {
	var result [][]string
	var current []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				result = append(result, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		result = append(result, current)
	}
	return result
}

// Remove SRT numbering, preserve cue text, and drop unreadable timings.
func convertSRT(text string, offsetMs int64) ([]byte, error) {
	var out strings.Builder
	out.WriteString("WEBVTT\n")
	cues := 0
	for _, block := range blocks(text) {
		lines := block
		if len(lines) >= 2 && allDigits(strings.TrimSpace(lines[0])) {
			if _, ok := parseTiming(strings.TrimSpace(lines[1])); ok {
				lines = lines[1:]
			}
		}
		timing, ok := parseTiming(strings.TrimSpace(lines[0]))
		if !ok {
			continue
		}
		cues++
		// SRT trailing coordinates are not WebVTT cue settings.
		timing.settings = ""
		shifted, keep := timing.shift(offsetMs)
		if !keep {
			continue
		}
		out.WriteString("\n" + shifted.String() + "\n")
		for _, line := range lines[1:] {
			out.WriteString(line + "\n")
		}
	}
	if cues == 0 {
		return nil, fmt.Errorf("%w: no readable SRT cue", domain.ErrSubtitleUnreadable)
	}
	return []byte(out.String()), nil
}

// Preserve NOTE/STYLE/REGION and unreadable blocks; zero offset leaves decoded content unchanged.
func convertVTT(text string, offsetMs int64) ([]byte, error) {
	text = strings.TrimPrefix(text, "\uFEFF")
	header, _, _ := strings.Cut(text, "\n")
	if header != "WEBVTT" && !strings.HasPrefix(header, "WEBVTT ") && !strings.HasPrefix(header, "WEBVTT\t") {
		return nil, fmt.Errorf("%w: missing WEBVTT header", domain.ErrSubtitleUnreadable)
	}
	// Without shifting, only decoding and line-ending normalization apply.
	if offsetMs == 0 {
		return []byte(text), nil
	}
	all := blocks(text)
	var out strings.Builder
	for i, block := range all {
		if i > 0 && !isVTTMetadata(block[0]) {
			var keep bool
			if block, keep = shiftVTTCue(block, offsetMs); !keep {
				continue
			}
		}
		if i > 0 {
			out.WriteString("\n")
		}
		for _, line := range block {
			out.WriteString(line + "\n")
		}
	}
	return []byte(out.String()), nil
}

func isVTTMetadata(first string) bool {
	for _, keyword := range []string{"NOTE", "STYLE", "REGION"} {
		if first == keyword || strings.HasPrefix(first, keyword+" ") || strings.HasPrefix(first, keyword+"\t") {
			return true
		}
	}
	return false
}

// Blocks without timing lines pass through unchanged.
func shiftVTTCue(block []string, offsetMs int64) ([]string, bool) {
	for i := 0; i < len(block) && i < 2; i++ {
		timing, ok := parseTiming(strings.TrimSpace(block[i]))
		if !ok {
			continue
		}
		shifted, keep := timing.shift(offsetMs)
		if !keep {
			return nil, false
		}
		out := append([]string(nil), block...)
		out[i] = shifted.String()
		return out, true
	}
	return block, true
}
