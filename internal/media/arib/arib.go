// Package arib decodes ARIB STD-B24 eight-bit strings from broadcast metadata to UTF-8, handling
// character-set shifts while ignoring presentation controls and unsupported glyphs. It performs no
// I/O and is imported only by media.
package arib

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
)

// Table dimensions for rows 85 through 94.
const (
	additionalFirstRow = 85
	additionalRows     = 10
	cellsPerRow        = 94
)

type charsetKind int

const (
	// Unsupported sets include mosaic, DRCS, macros, and JIS-compatible Kanji plane 2.
	kindIgnored charsetKind = iota
	kindKanji
	kindAlphanumeric
	kindHiragana
	kindKatakana
	kindJISKatakana
)

type charset struct {
	kind  charsetKind
	width int
}

var (
	kanjiSet     = charset{kindKanji, 2}
	ignoredSet1  = charset{kindIgnored, 1}
	ignoredSet2  = charset{kindIgnored, 2}
	hiraganaSet  = charset{kindHiragana, 1}
	katakanaSet  = charset{kindKatakana, 1}
	alphanumeric = charset{kindAlphanumeric, 1}
)

// Unknown final bytes designate ignored sets.
var oneByteSets = map[byte]charset{
	0x4A: alphanumeric,                      // Alphanumeric.
	0x36: alphanumeric,                      // Proportional alphanumeric.
	0x30: hiraganaSet,                       // Hiragana.
	0x37: hiraganaSet,                       // Proportional hiragana.
	0x31: katakanaSet,                       // Katakana.
	0x38: katakanaSet,                       // Proportional katakana.
	0x49: {kind: kindJISKatakana, width: 1}, // JIS X 0201 katakana.
}

// Unknown two-byte sets, including JIS-compatible Kanji plane 2, are ignored.
var twoByteSets = map[byte]charset{
	0x42: kanjiSet, // Kanji.
	0x39: kanjiSet, // JIS-compatible Kanji plane 1.
	0x3B: kanjiSet, // Additional symbols.
}

type lockingShift struct {
	right bool
	set   int
}

var lockingShifts = map[byte]lockingShift{
	0x6E: {right: false, set: 2},
	0x6F: {right: false, set: 3},
	0x7E: {right: true, set: 1},
	0x7D: {right: true, set: 2},
	0x7C: {right: true, set: 3},
}

// Control parameter byte counts.
var (
	c0Parameters = map[byte]int{0x16: 1, 0x1C: 2}
	c1Parameters = map[byte]int{
		0x8B: 1, 0x90: 1, 0x91: 1, 0x92: 1, 0x93: 1, 0x94: 1, 0x95: 1, 0x97: 1, 0x98: 1, 0x9D: 2,
	}
)

// Kana codes 0x77 through 0x7E contain iteration marks, long-vowel marks, and punctuation.
var (
	hiraganaTail = [8]rune{0x309D, 0x309E, 0x30FC, 0x3002, 0x300C, 0x300D, 0x3001, 0x30FB}
	katakanaTail = [8]rune{0x30FD, 0x30FE, 0x30FC, 0x3002, 0x300C, 0x300D, 0x3001, 0x30FB}
)

const (
	codeAPR = 0x0D // Carriage return/newline.
	codeLS1 = 0x0E
	codeLS0 = 0x0F
	codeSS2 = 0x19
	codeESC = 0x1B
	codeSS3 = 0x1D
	codeSP  = 0x20
	codeDEL = 0x7F
	codeCOL = 0x90
	codeCDC = 0x92
	codeCSI = 0x9B
)

type decoder struct {
	g      [4]charset
	gl, gr int
	// Single shift applies to the next GL character only; -1 means inactive.
	single int
	out    strings.Builder
	jis    *encoding.Decoder
}

// Decode starts with G0 Kanji, G1 alphanumeric, G2 hiragana, G3 katakana, GL=G0, and GR=G2;
// unsupported characters are dropped.
func Decode(data []byte) string {
	d := &decoder{
		g:      [4]charset{kanjiSet, alphanumeric, hiraganaSet, katakanaSet},
		gl:     0,
		gr:     2,
		single: -1,
		jis:    japanese.EUCJP.NewDecoder(),
	}
	for i := 0; i < len(data); {
		i = d.step(data, i)
	}
	return d.out.String()
}

func (d *decoder) step(data []byte, i int) int {
	b := data[i]
	switch {
	case b == codeESC:
		return d.escape(data, i+1)
	case b < codeSP:
		return d.control0(data, i)
	case b == codeSP:
		d.out.WriteByte(' ')
		return i + 1
	case b < codeDEL:
		set := d.g[d.gl]
		if d.single >= 0 {
			set = d.g[d.single]
			d.single = -1
		}
		return d.graphic(data, i, set)
	case b < 0xA0:
		return d.control1(data, i)
	case b == 0xA0 || b == 0xFF:
		return i + 1
	default:
		return d.graphic(data, i, d.g[d.gr])
	}
}

func (d *decoder) control0(data []byte, i int) int {
	switch data[i] {
	case codeLS0:
		d.gl = 0
	case codeLS1:
		d.gl = 1
	case codeSS2:
		d.single = 2
	case codeSS3:
		d.single = 3
	case codeAPR:
		d.out.WriteByte('\n')
	}
	return i + 1 + c0Parameters[data[i]]
}

func (d *decoder) control1(data []byte, i int) int {
	code := data[i]
	next := i + 1
	if code == codeCSI {
		// CSI ends at a final byte 0x40 through 0x6F after parameters and intermediate 0x20.
		for next < len(data) {
			b := data[next]
			next++
			if b >= 0x40 && b <= 0x6F {
				break
			}
		}
		return next
	}
	if (code == codeCOL || code == codeCDC) && next < len(data) && data[next] == codeSP {
		next++
	}
	return next + c1Parameters[code]
}

// i points immediately after ESC.
func (d *decoder) escape(data []byte, i int) int {
	if i >= len(data) {
		return i
	}
	if shift, ok := lockingShifts[data[i]]; ok {
		if shift.right {
			d.gr = shift.set
		} else {
			d.gl = shift.set
		}
		return i + 1
	}
	twoByte := data[i] == 0x24
	if twoByte {
		i++
	}
	target := 0
	if i < len(data) && data[i] >= 0x28 && data[i] <= 0x2B {
		target = int(data[i] - 0x28)
		i++
	} else if !twoByte {
		// Skip only one byte for an unknown escape sequence.
		return i + 1
	}
	drcs := i < len(data) && data[i] == codeSP
	if drcs {
		i++
	}
	if i >= len(data) {
		return i
	}
	d.g[target] = designate(data[i], twoByte, drcs)
	return i + 1
}

// DRCS and unlisted sets are ignored.
func designate(final byte, twoByte, drcs bool) charset {
	if drcs {
		if twoByte || final == 0x40 {
			return ignoredSet2
		}
		return ignoredSet1
	}
	if twoByte {
		if set, ok := twoByteSets[final]; ok {
			return set
		}
		return ignoredSet2
	}
	if set, ok := oneByteSets[final]; ok {
		return set
	}
	return ignoredSet1
}

func (d *decoder) graphic(data []byte, i int, set charset) int {
	if set.width == 2 {
		if i+1 >= len(data) {
			return len(data)
		}
		if set.kind == kindKanji {
			d.writeRune(d.kanji(data[i]&0x7F, data[i+1]&0x7F))
		}
		return i + 2
	}
	d.writeRune(singleByteRune(set.kind, data[i]&0x7F))
	return i + 1
}

func (d *decoder) writeRune(r rune) {
	if r > 0 && r != utf8.RuneError {
		d.out.WriteRune(r)
	}
}

// Rows 85 onward use the additional-symbol table; earlier rows use JIS X 0208, with zero for
// unsupported glyphs.
func (d *decoder) kanji(first, second byte) rune {
	if first < 0x21 || first > 0x7E || second < 0x21 || second > 0x7E {
		return 0
	}
	row := int(first) - 0x20
	if row >= additionalFirstRow {
		return additionalSymbols[(row-additionalFirstRow)*cellsPerRow+int(second)-0x21]
	}
	decoded, err := d.jis.Bytes([]byte{first | 0x80, second | 0x80})
	if err != nil {
		return 0
	}
	r, size := utf8.DecodeRune(decoded)
	if size != len(decoded) {
		return 0
	}
	return r
}

// Return zero for unsupported codes.
func singleByteRune(kind charsetKind, code byte) rune {
	switch kind {
	case kindAlphanumeric:
		return rune(code)
	case kindHiragana:
		return kanaRune(code, 0x3041, 0x73, &hiraganaTail)
	case kindKatakana:
		return kanaRune(code, 0x30A1, 0x76, &katakanaTail)
	case kindJISKatakana:
		if code <= 0x5F {
			return 0xFF61 + rune(code-0x21)
		}
	}
	return 0
}

// Kana runs from first at 0x21 through last; tail supplies 0x77 through 0x7E.
func kanaRune(code byte, first rune, last byte, tail *[8]rune) rune {
	switch {
	case code <= last:
		return first + rune(code-0x21)
	case code >= 0x77:
		return tail[code-0x77]
	}
	return 0
}
