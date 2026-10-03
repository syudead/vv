// Package foldcheck_test は、Go の domain.FoldForMatch と画面の foldForMatch
// （web/src/lib/foldForMatch.ts）が共有する入力の組
// internal/domain/testdata/fold_for_match.json を確かめ、-update で書き直す
// （specs/036-tag-admin-scale/research.md R-3）。domain.NaturalSortKey と画面の
// naturalSortKey（web/src/lib/naturalSortKey.ts）が共有する
// internal/domain/testdata/natural_sort_key.json も確かめる（R-12）。ファイルを読み書きするので、
// os を import できない internal/domain の外（depguard の
// domain-is-the-end-of-the-dependency-chain）に置く。
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

// updateFoldTable は testdata/fold_for_match.json の小文字化の表を
// unicode.ToLower から書き直す（`go test ./internal/domain/foldcheck -update`）。
var updateFoldTable = flag.Bool("update", false, "rewrite testdata/fold_for_match.json's lower table")

// foldForMatchFile は、Go の FoldForMatch と画面の foldForMatch
// （web/src/lib/foldForMatch.ts）が同じ照合形を返すことを確かめる共有の入力の組である
// （specs/036-tag-admin-scale/research.md R-3）。
type foldForMatchFile struct {
	// Cases は入力と期待の照合形の組である。
	Cases []foldForMatchCase `json:"cases"`
	// Lower は unicode.ToLower(r) != r となるすべての符号位置の組である。
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

// goLowerTable は unicode.ToLower が符号位置を変えるすべての組を、符号位置の順に返す。
func goLowerTable() []foldLowerPair {
	var pairs []foldLowerPair
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if lower := unicode.ToLower(r); lower != r {
			pairs = append(pairs, foldLowerPair{From: r, To: lower})
		}
	}
	return pairs
}

// writeFoldForMatchFile は 1 組を 1 行に書く。差分が読めるよう、組の並びと形を固定する。
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
