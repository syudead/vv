// Package foldcheck_test validates shared Go/Web folding and sorting fixtures; it lives outside domain so fixture I/O does not violate depguard.
package foldcheck_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unicode"

	"github.com/syudead/vv/internal/domain"
)

// Regenerate Unicode lowercase mappings with go test ./internal/domain/foldcheck -update.
var updateFoldTable = flag.Bool("update", false, "rewrite testdata/fold_for_match.json's lower table")

type foldForMatchFile struct {
	Cases []foldForMatchCase `json:"cases"`
	// Lower contains every code point changed by unicode.ToLower.
	Lower []foldLowerPair `json:"lower"`
}

type foldForMatchCase struct {
	Note  string `json:"note"`
	Input string `json:"input"`
	Want  string `json:"want"`
}

type foldLowerPair struct {
	From rune `json:"from"`
	To   rune `json:"to"`
}

var foldForMatchPath = filepath.Join("..", "testdata", "fold_for_match.json")

func readFoldForMatchFile(t *testing.T) foldForMatchFile {
	t.Helper()
	data, err := os.ReadFile(foldForMatchPath)
	if err != nil {
		t.Fatal(err)
	}
	var file foldForMatchFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	return file
}

func goLowerTable() []foldLowerPair {
	var pairs []foldLowerPair
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if lower := unicode.ToLower(r); lower != r {
			pairs = append(pairs, foldLowerPair{From: r, To: lower})
		}
	}
	return pairs
}

// Use one case per line and stable formatting for readable diffs.
func writeFoldForMatchFile(t *testing.T, file foldForMatchFile) {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("{\n  \"cases\": [\n")
	for i, c := range file.Cases {
		line, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString("    ")
		b.Write(line)
		if i < len(file.Cases)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ],\n  \"lower\": [\n")
	for i, p := range file.Lower {
		fmt.Fprintf(&b, "    {\"from\": %d, \"to\": %d}", p.From, p.To)
		if i < len(file.Lower)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ]\n}\n")
	if err := os.WriteFile(foldForMatchPath, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFoldForMatchSharedCases(t *testing.T) {
	file := readFoldForMatchFile(t)
	if len(file.Cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range file.Cases {
		if got := domain.FoldForMatch(c.Input); got != c.Want {
			t.Errorf("%s: FoldForMatch(%q) = %q, want %q", c.Note, c.Input, got, c.Want)
		}
	}
}

func TestFoldForMatchSharedLowerTable(t *testing.T) {
	file := readFoldForMatchFile(t)
	want := goLowerTable()
	if *updateFoldTable {
		file.Lower = want
		writeFoldForMatchFile(t, file)
		return
	}
	if len(file.Lower) != len(want) {
		t.Fatalf("lower table has %d pairs, unicode.ToLower has %d; rerun with -update", len(file.Lower), len(want))
	}
	for i := range want {
		if file.Lower[i] != want[i] {
			t.Fatalf("lower[%d] = %+v, unicode.ToLower gives %+v; rerun with -update", i, file.Lower[i], want[i])
		}
	}
}
