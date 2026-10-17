package wasmplugin

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// mustWithManifest は WithManifest の結果を返し、誤りならテストを止める。
func mustWithManifest(tb testing.TB, module []byte, manifest string) []byte {
	tb.Helper()
	packed, err := WithManifest(module, []byte(manifest))
	if err != nil {
		tb.Fatalf("WithManifest: %v", err)
	}
	return packed
}

// countManifestSections は module の vv.plugin のセクションの数である。
func countManifestSections(tb testing.TB, module []byte) int {
	tb.Helper()
	r := &reader{b: module, pos: 8}
	n := 0
	for r.pos < len(r.b) {
		id, err := r.byte()
		if err != nil {
			tb.Fatalf("read section id: %v", err)
		}
		size, err := r.u32()
		if err != nil {
			tb.Fatalf("read section size: %v", err)
		}
		payload, err := r.bytes(int(size))
		if err != nil {
			tb.Fatalf("read section: %v", err)
		}
		if id == sectionCustom && customSectionName(payload) == manifestSection {
			n++
		}
	}
	return n
}

func TestWithManifestAddsTheSectionToAModuleWithoutOne(t *testing.T) {
	spec := goodSpec()
	spec.manifest = nil
	module := spec.bytes()
	rt := newTestRuntime(t)
	if _, problem, err := rt.Inspect(context.Background(), module); err != nil || problem != "manifest_missing" {
		t.Fatalf("Inspect(no section) = %q, %v; want manifest_missing", problem, err)
	}

	packed := mustWithManifest(t, module, goodManifest)
	if n := countManifestSections(t, packed); n != 1 {
		t.Fatalf("vv.plugin sections = %d, want 1", n)
	}
	manifest, problem, err := rt.Inspect(context.Background(), packed)
	if err != nil || problem != "" || manifest.Name != "example.works" {
		t.Fatalf("Inspect(packed) = %+v, %q, %v; want example.works", manifest, problem, err)
	}
	if !bytes.Equal(packed[:len(module)], module) {
		t.Fatalf("the sections before vv.plugin changed")
	}
}

func TestWithManifestReplacesTheExistingSection(t *testing.T) {
	module := goodSpec().bytes()
	const next = `{"contract":1,"name":"next","version":"2.0.0","operations":["fetch"]}`
	packed := mustWithManifest(t, module, next)
	packed = mustWithManifest(t, packed, next)
	if n := countManifestSections(t, packed); n != 1 {
		t.Fatalf("vv.plugin sections = %d, want 1", n)
	}
	manifest, problem, err := newTestRuntime(t).Inspect(context.Background(), packed)
	if err != nil || problem != "" || manifest.Name != "next" || manifest.Version != "2.0.0" {
		t.Fatalf("Inspect(replaced) = %+v, %q, %v; want next 2.0.0", manifest, problem, err)
	}
}

func TestWithManifestRefusesBytesThatAreNotSections(t *testing.T) {
	for name, module := range map[string][]byte{
		"not wasm":  []byte("hello, world"),
		"cut short": append(goodSpec().bytes(), 0x01, 0x10, 0x00),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := WithManifest(module, []byte(goodManifest)); !errors.Is(err, errMalformed) {
				t.Fatalf("WithManifest err = %v, want errMalformed", err)
			}
		})
	}
}
