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

// 動画の隣の字幕ファイル（SRT・WebVTT）を、文字コードを判定して UTF-8 の WebVTT にし、
// 時刻をずらす（specs/028-sidecar-subtitles/research.md R-4〜R-8）。外部プロセスは
// 起動しない。

// SubtitleConverter は字幕ファイルを WebVTT に変換する。状態を持たないので、ゼロ値の
// まま使える。
type SubtitleConverter struct{}

// NewSubtitleConverter は字幕ファイルの変換を返す。
func NewSubtitleConverter() SubtitleConverter { return SubtitleConverter{} }

// Convert は src（format の形式の字幕ファイルの中身）を UTF-8 の WebVTT にし、すべての
// cue の時刻から offsetMs を引いて返す。終了時刻が 0 以下になる cue は返さず、開始
// 時刻が負になる cue は 0 から始める。読めない入力には domain.ErrSubtitleUnreadable を
// 包んだ誤りを返す。
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

// subtitleScripts は、BOM の無い UTF-8 の字幕をそのまま UTF-8 と決めてよい文字体系である
// （research.md R-5 の手順 1）。
var subtitleScripts = []*unicode.RangeTable{
	unicode.Common, unicode.Inherited, unicode.Latin, unicode.Greek, unicode.Cyrillic,
	unicode.Hebrew, unicode.Arabic, unicode.Thai, unicode.Hangul, unicode.Han,
	unicode.Hiragana, unicode.Katakana, unicode.Bopomofo,
}

// decodeSubtitle は字幕ファイルの文字コードを BOM → UTF-8 の妥当性 → Shift_JIS の順で
// 決め、UTF-8 の文字列にする（research.md R-5）。
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

// cleanShiftJIS は Shift_JIS の復号器の出力が「復号できた」ものかを確かめる。復号器は
// 読めない列でエラーを返さず U+FFFD を出し、0x80 を U+0080 に通す。
func cleanShiftJIS(decoded []byte) bool {
	for _, r := range string(decoded) {
		if r == utf8.RuneError || (r >= 0x80 && r <= 0x9F) {
			return false
		}
	}
	return true
}

// cueTiming は cue の時刻の行を読んだものである。
type cueTiming struct {
	startMs, endMs int64
	// settings は終了時刻のあとに続く WebVTT の cue の設定（先頭の空白を含む）である。
	settings string
}

// shift は時刻から offsetMs を引く。返さない cue なら false を返す。
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

// parseTiming は `start --> end [settings]` の行を読む。時刻は `h:mm:ss` か `mm:ss` に、
// `,` か `.` で区切った 1〜3 桁の小数が付いてよい。
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

// blocks は空行で区切った行のまとまりに分ける。空白だけの行も区切りとみなす。
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

// convertSRT は SRT を WebVTT にする（research.md R-8）。番号行を除き、時刻の行を
// WebVTT の形に揃え、cue の本文はそのまま通す。時刻の行が読めない cue は落とす。
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
		// SRT の終了時刻のあとの座標（X1: …）は WebVTT の設定ではないので捨てる。
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

// convertVTT は WebVTT のヘッダーを確かめ、cue の時刻だけを offsetMs ずらして通す。
// NOTE・STYLE・REGION のブロックと、時刻の行が読めないブロックはそのまま残す。
func convertVTT(text string, offsetMs int64) ([]byte, error) {
	text = strings.TrimPrefix(text, "\uFEFF")
	header, _, _ := strings.Cut(text, "\n")
	if header != "WEBVTT" && !strings.HasPrefix(header, "WEBVTT ") && !strings.HasPrefix(header, "WEBVTT\t") {
		return nil, fmt.Errorf("%w: missing WEBVTT header", domain.ErrSubtitleUnreadable)
	}
	all := blocks(text)
	var out strings.Builder
	for i, block := range all {
		if i > 0 && offsetMs != 0 && !isVTTMetadata(block[0]) {
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

// shiftVTTCue は cue のブロック（識別子の行があってもよい）の時刻の行をずらす。
// 時刻の行が無いブロックはそのまま返す。
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
