package wasmplugin

import (
	"encoding/binary"
	"fmt"
)

// sectionCustom はカスタムセクションの id である。
const sectionCustom = 0

// WithManifest は module の vv.plugin のカスタムセクションをすべて外し、manifest を中身とする
// vv.plugin のセクションを末尾に足した新しいバイト列を返す。何度かけてもセクションは 1 つである。マニフェストの中身は確かめない。セクションの並びとして
// 読めないバイト列には errMalformed を包んだ誤りを返す。
func WithManifest(module, manifest []byte) ([]byte, error) {
	if len(module) < 8 || string(module[:4]) != "\x00asm" {
		return nil, errMalformed
	}
	out := make([]byte, 0, len(module)+len(manifest)+len(manifestSection)+12)
	out = append(out, module[:8]...)
	r := &reader{b: module, pos: 8}
	for r.pos < len(r.b) {
		start := r.pos
		id, err := r.byte()
		if err != nil {
			return nil, err
		}
		size, err := r.u32()
		if err != nil {
			return nil, err
		}
		payload, err := r.bytes(int(size))
		if err != nil {
			return nil, fmt.Errorf("%w: section %d is cut short", errMalformed, id)
		}
		if id == sectionCustom && customSectionName(payload) == manifestSection {
			continue
		}
		out = append(out, module[start:r.pos]...)
	}
	return appendCustomSection(out, manifestSection, manifest), nil
}

// customSectionName はカスタムセクションの中身の先頭にある名前を返す。読めなければ空を返す。
func customSectionName(payload []byte) string {
	name, err := (&reader{b: payload}).name()
	if err != nil {
		return ""
	}
	return name
}

// appendCustomSection は名前 name、中身 data のカスタムセクション（id 0）を符号化して dst に足す。
func appendCustomSection(dst []byte, name string, data []byte) []byte {
	payload := binary.AppendUvarint(nil, uint64(len(name)))
	payload = append(payload, name...)
	payload = append(payload, data...)
	dst = append(dst, sectionCustom)
	dst = binary.AppendUvarint(dst, uint64(len(payload)))
	return append(dst, payload...)
}
