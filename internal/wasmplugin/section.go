package wasmplugin

import (
	"encoding/binary"
	"fmt"
)

const sectionCustom = 0

// WithManifest replaces all vv.plugin sections with one trailing section without validating
// manifest content; malformed section layout wraps errMalformed.
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

// Return empty when the custom-section name is unreadable.
func customSectionName(payload []byte) string {
	name, err := (&reader{b: payload}).name()
	if err != nil {
		return ""
	}
	return name
}

func appendCustomSection(dst []byte, name string, data []byte) []byte {
	payload := binary.AppendUvarint(nil, uint64(len(name)))
	payload = append(payload, name...)
	payload = append(payload, data...)
	dst = append(dst, sectionCustom)
	dst = binary.AppendUvarint(dst, uint64(len(payload)))
	return append(dst, payload...)
}
