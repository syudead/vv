// Package arib は ARIB STD-B24 の 8 単位符号の文字列を UTF-8 にする。TS の録画の SDT と EIT にある放送局名と番組名を読むためにある。
//
// 漢字・かな（JIS X 0208）は golang.org/x/text/encoding/japanese で、ひらがな・カタカナの
// 集合、JIS X 0201 のカタカナ、85〜94 区の追加記号は表で読む。符号の切り替え（locking
// shift・single shift・指示）を扱い、大きさ・色・位置などの制御は無視する。対応の無い
// 文字（モザイク・DRCS・表に無い符号）は落とす。外部の I/O を持たない純粋な package で、
// internal/media だけが import する。
package arib

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
)

// 85〜94 区の表の大きさ。
const (
	additionalFirstRow = 85
	additionalRows     = 10
	cellsPerRow        = 94
)

// charsetKind は符号の集合の種類である。
type charsetKind int

const (
	// kindIgnored は読まない集合（モザイク・DRCS・マクロ・JIS 互換漢字 2 面）。
	kindIgnored charsetKind = iota
	kindKanji
	kindAlphanumeric
	kindHiragana
	kindKatakana
	kindJISKatakana
)

// charset は G0〜G3 に指示される集合と、その 1 文字のバイト数である。
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

// oneByteSets は 1 バイトの集合の終端符号である。ここに無い符号（モザイクなど）は読まない。
var oneByteSets = map[byte]charset{
	0x4A: alphanumeric,                      // 英数
	0x36: alphanumeric,                      // プロポーショナル英数
	0x30: hiraganaSet,                       // 平仮名
	0x37: hiraganaSet,                       // プロポーショナル平仮名
	0x31: katakanaSet,                       // 片仮名
	0x38: katakanaSet,                       // プロポーショナル片仮名
	0x49: {kind: kindJISKatakana, width: 1}, // JIS X 0201 片仮名
}

// twoByteSets は 2 バイトの集合の終端符号である。ここに無い符号（JIS 互換漢字 2 面など）は
// 読まない。
var twoByteSets = map[byte]charset{
	0x42: kanjiSet, // 漢字
	0x39: kanjiSet, // JIS 互換漢字 1 面
	0x3B: kanjiSet, // 追加記号
}

// lockingShift は ESC の後の 1 バイトで GL か GR に呼び出す集合である。
type lockingShift struct {
	right bool
	set   int
}

// lockingShifts は ESC で始まる locking shift（LS2・LS3・LS1R・LS2R・LS3R）である。
var lockingShifts = map[byte]lockingShift{
	0x6E: {right: false, set: 2},
	0x6F: {right: false, set: 3},
	0x7E: {right: true, set: 1},
	0x7D: {right: true, set: 2},
	0x7C: {right: true, set: 3},
}

// c0Parameters と c1Parameters は引数を持つ制御符号と、その引数のバイト数である。
var (
	c0Parameters = map[byte]int{0x16: 1, 0x1C: 2}
	c1Parameters = map[byte]int{
		0x8B: 1, 0x90: 1, 0x91: 1, 0x92: 1, 0x93: 1, 0x94: 1, 0x95: 1, 0x97: 1, 0x98: 1, 0x9D: 2,
	}
)

// 平仮名・片仮名の集合の 0x77〜0x7E（踊り字・長音・句読点など）の文字である。
var (
	hiraganaTail = [8]rune{0x309D, 0x309E, 0x30FC, 0x3002, 0x300C, 0x300D, 0x3001, 0x30FB}
	katakanaTail = [8]rune{0x30FD, 0x30FE, 0x30FC, 0x3002, 0x300C, 0x300D, 0x3001, 0x30FB}
)

// 制御符号。
const (
	codeAPR = 0x0D // 復帰改行
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

// decoder は 1 つの文字列を読む間の符号の状態である。
type decoder struct {
	g      [4]charset
	gl, gr int
	// single は single shift で次の 1 文字だけ GL に呼び出す集合（無ければ -1）。
	single int
	out    strings.Builder
	jis    *encoding.Decoder
}

// Decode は ARIB STD-B24 の 8 単位符号の文字列を UTF-8 にする。初期の状態は G0 漢字、
// G1 英数、G2 平仮名、G3 片仮名で、GL に G0、GR に G2 を呼び出す。対応の無い文字は落とす。
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

// step は data[i] から 1 つの文字か制御を読み、次の位置を返す。
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

// control0 は C0 の制御符号を読む。
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

// control1 は C1 の制御符号（色・大きさ・位置など）を読み飛ばす。
func (d *decoder) control1(data []byte, i int) int {
	code := data[i]
	next := i + 1
	if code == codeCSI {
		// CSI は引数・中間符号 0x20 の後の終端符号（0x40〜0x6F）まで続く。
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

// escape は ESC の後の locking shift か集合の指示を読む。i は ESC の次の位置である。
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
		// 知らない ESC は 1 バイトだけ読み飛ばす。
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

// designate は終端符号 final の集合を返す。DRCS と表に無い集合は読まない集合である。
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

// graphic は集合 set の 1 文字を読んで書き出す。
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

// writeRune は対応のある文字だけを書き出す。
func (d *decoder) writeRune(r rune) {
	if r > 0 && r != utf8.RuneError {
		d.out.WriteRune(r)
	}
}

// kanji は漢字の集合の 1 文字（区点の各バイトは 0x21〜0x7E）を返す。85 区以降は追加記号の表、
// それより前は JIS X 0208 である。読めなければ 0。
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

// singleByteRune は 1 バイトの集合の 1 文字（0x21〜0x7E）を返す。読めなければ 0。
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

// kanaRune は平仮名か片仮名の集合の 1 文字を返す。0x21〜last は first から並ぶかなで、
// 0x77〜0x7E は tail の文字である。
func kanaRune(code byte, first rune, last byte, tail *[8]rune) rune {
	switch {
	case code <= last:
		return first + rune(code-0x21)
	case code >= 0x77:
		return tail[code-0x77]
	}
	return 0
}
