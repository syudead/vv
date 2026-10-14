package wasmplugin

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

// wazero の CompiledModule は関数とメモリの import しか見せず、テーブルも見せない。検査に要る
// import の種類とテーブルの要素数は、モジュールのバイト列からここで読む。読むのは import
// セクション（2）とテーブルセクション（4）だけで、他のセクションは飛ばす。最大値を宣言しない
// テーブルには、コンパイルの前に capTables が最大値を書き足す（research.md R-4）。

const (
	sectionImport = 2
	sectionTable  = 4

	importKindFunc = 0x00
)

var errMalformed = errors.New("malformed WebAssembly binary")

// moduleImport は import の 1 つである。
type moduleImport struct {
	module string
	name   string
	kind   byte
}

// tableLimits はテーブルの要素数の宣言である。hasMax が false なら最大値の宣言が無い。
// 位置はテーブルセクションの中身の先頭からのバイト数で、宣言を書き換えるときに使う。
type tableLimits struct {
	min    uint64
	hasMax bool
	max    uint64

	flags byte
	// start から end までがテーブルの宣言の全体、limitsStart から minEnd までが旗と最小値、
	// limitsEnd までが要素数の宣言である。
	start, limitsStart, minEnd, limitsEnd, end int
}

// moduleLayout は検査のためにバイト列から読んだ値である。
type moduleLayout struct {
	imports []moduleImport
	tables  []tableLimits
	// tableSection はテーブルセクションの位置で、無ければ header が -1 である。
	tableSection sectionSpan
}

// sectionSpan はモジュールの中のセクションの位置である。header は id のバイト、payload から
// end までが中身である。
type sectionSpan struct{ header, payload, end int }

// readLayout はモジュールの import とテーブルの宣言を読む。関数でない import に出会ったら、
// その import までで読むのをやめる（その時点で import_unsupported と決まり、後の形は要らない）。
func readLayout(b []byte) (moduleLayout, error) {
	r := &reader{b: b}
	if len(b) < 8 || string(b[:4]) != "\x00asm" {
		return moduleLayout{}, errMalformed
	}
	r.pos = 8
	layout := moduleLayout{tableSection: sectionSpan{header: -1}}
	for r.pos < len(r.b) {
		header := r.pos
		id, err := r.byte()
		if err != nil {
			return moduleLayout{}, err
		}
		size, err := r.u32()
		if err != nil {
			return moduleLayout{}, err
		}
		payload := r.pos
		section, err := r.bytes(int(size))
		if err != nil {
			return moduleLayout{}, err
		}
		if id == sectionTable {
			layout.tableSection = sectionSpan{header: header, payload: payload, end: r.pos}
		}
		if err := layout.read(id, section); err != nil {
			return moduleLayout{}, err
		}
	}
	return layout, nil
}

func (l *moduleLayout) read(id byte, section []byte) error {
	var err error
	switch id {
	case sectionImport:
		l.imports, err = readImports(&reader{b: section})
	case sectionTable:
		l.tables, err = readTables(&reader{b: section})
	}
	return err
}

func readImports(r *reader) ([]moduleImport, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	imports := make([]moduleImport, 0, min(n, 1024))
	for range n {
		module, err := r.name()
		if err != nil {
			return nil, err
		}
		name, err := r.name()
		if err != nil {
			return nil, err
		}
		kind, err := r.byte()
		if err != nil {
			return nil, err
		}
		imports = append(imports, moduleImport{module: module, name: name, kind: kind})
		if kind != importKindFunc {
			return imports, nil
		}
		if _, err := r.u32(); err != nil {
			return nil, err
		}
	}
	return imports, nil
}

func readTables(r *reader) ([]tableLimits, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	tables := make([]tableLimits, 0, min(n, 1024))
	for range n {
		limits, err := r.table()
		if err != nil {
			return nil, err
		}
		tables = append(tables, limits)
	}
	return tables, nil
}

// table はテーブルの宣言を 1 つ読む。初期値の式を持つ形（0x40 0x00 で始まる）は、式を
// 定数の命令だけと見て読み飛ばす。それ以外の命令を含む式は errMalformed にする。
func (r *reader) table() (tableLimits, error) {
	start := r.pos
	first, err := r.byte()
	if err != nil {
		return tableLimits{}, err
	}
	hasInit := first == 0x40
	if hasInit {
		if reserved, err := r.byte(); err != nil || reserved != 0x00 {
			return tableLimits{}, errMalformed
		}
		if first, err = r.byte(); err != nil {
			return tableLimits{}, err
		}
	}
	if err := r.refType(first); err != nil {
		return tableLimits{}, err
	}
	limits, err := r.limits()
	if err != nil {
		return tableLimits{}, err
	}
	if hasInit {
		if err := r.skipConstExpr(); err != nil {
			return tableLimits{}, err
		}
	}
	limits.start, limits.end = start, r.pos
	return limits, nil
}

// refType は要素の型の残りを読む。0x63 と 0x64 はヒープの型（s33）が続く。
func (r *reader) refType(first byte) error {
	if first == 0x63 || first == 0x64 {
		_, err := r.uleb(33)
		return err
	}
	return nil
}

// skipConstExpr は ref.null、ref.func、global.get だけから成る定数の式を end まで読み飛ばす。
func (r *reader) skipConstExpr() error {
	for {
		op, err := r.byte()
		if err != nil {
			return err
		}
		switch op {
		case 0x0b: // end
			return nil
		case 0xd0: // ref.null
			_, err = r.uleb(33)
		case 0xd2, 0x23: // ref.func, global.get
			_, err = r.u32()
		default:
			return errMalformed
		}
		if err != nil {
			return err
		}
	}
}

// reader はモジュールのバイト列を先頭から読む。
type reader struct {
	b   []byte
	pos int
}

func (r *reader) byte() (byte, error) {
	if r.pos >= len(r.b) {
		return 0, errMalformed
	}
	c := r.b[r.pos]
	r.pos++
	return c, nil
}

func (r *reader) bytes(n int) ([]byte, error) {
	if n < 0 || n > len(r.b)-r.pos {
		return nil, errMalformed
	}
	out := r.b[r.pos : r.pos+n]
	r.pos += n
	return out, nil
}

func (r *reader) name() (string, error) {
	n, err := r.u32()
	if err != nil {
		return "", err
	}
	b, err := r.bytes(int(n))
	return string(b), err
}

func (r *reader) u32() (uint32, error) {
	v, err := r.uleb(32)
	return uint32(v), err
}

// uleb は bits ビット以下の符号なし LEB128 を読む。
func (r *reader) uleb(bits uint) (uint64, error) {
	var v uint64
	for shift := uint(0); shift < bits+7; shift += 7 {
		c, err := r.byte()
		if err != nil {
			return 0, err
		}
		v |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			if bits < 64 && v>>bits != 0 {
				return 0, errMalformed
			}
			return v, nil
		}
	}
	return 0, errMalformed
}

// limits はテーブルかメモリの要素数の宣言を読む。旗の 0x04 は 64 ビットの数を表す。
func (r *reader) limits() (tableLimits, error) {
	limitsStart := r.pos
	flags, err := r.byte()
	if err != nil {
		return tableLimits{}, err
	}
	if flags > 0x07 {
		return tableLimits{}, fmt.Errorf("%w: limits flags %#x", errMalformed, flags)
	}
	bits := uint(32)
	if flags&0x04 != 0 {
		bits = 64
	}
	minimum, err := r.uleb(bits)
	if err != nil {
		return tableLimits{}, err
	}
	limits := tableLimits{min: minimum, flags: flags, limitsStart: limitsStart, minEnd: r.pos}
	if flags&0x01 != 0 {
		if limits.max, err = r.uleb(bits); err != nil {
			return tableLimits{}, err
		}
		limits.hasMax = true
	}
	limits.limitsEnd = r.pos
	return limits, nil
}

// needsCap はテーブルが最大値を宣言せず、最小値が limit 以下で、vv が最大値を書き足すものかを返す。
// 最小値が limit を超えるテーブルは書き換えず、検査が table_too_large にする。
func (t tableLimits) needsCap(limit uint64) bool {
	return !t.hasMax && t.min <= limit
}

// capTables は最大値を宣言しないテーブルに最大値 limit を書き足したモジュールのバイト列を返す。
// 書き足すテーブルが無ければ module をそのまま返す。module は layout を読んだバイト列である。
func capTables(module []byte, layout moduleLayout, limit uint64) []byte {
	if !slices.ContainsFunc(layout.tables, func(t tableLimits) bool { return t.needsCap(limit) }) {
		return module
	}
	span := layout.tableSection
	section := module[span.payload:span.end]
	payload := binary.AppendUvarint(nil, uint64(len(layout.tables)))
	for _, t := range layout.tables {
		if !t.needsCap(limit) {
			payload = append(payload, section[t.start:t.end]...)
			continue
		}
		payload = append(payload, section[t.start:t.limitsStart]...)
		payload = append(payload, t.flags|0x01)
		payload = append(payload, section[t.limitsStart+1:t.minEnd]...)
		payload = binary.AppendUvarint(payload, limit)
		payload = append(payload, section[t.limitsEnd:t.end]...)
	}
	out := make([]byte, 0, len(module)+len(payload)-len(section)+8)
	out = append(out, module[:span.header]...)
	out = append(out, sectionTable)
	out = binary.AppendUvarint(out, uint64(len(payload)))
	out = append(out, payload...)
	return append(out, module[span.end:]...)
}
