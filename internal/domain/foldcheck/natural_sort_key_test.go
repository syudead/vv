package foldcheck_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// naturalSortKeyFile は、Go の NaturalSortKey と画面の naturalSortKey
// （web/src/lib/naturalSortKey.ts）が同じ鍵を返すことを確かめる共有の入力の組である
// （specs/036-tag-admin-scale/research.md R-12「移植の検査」）。組は鍵の
// strings.Compare の順に並べ、画面の compareNaturalSortKeys が同じ順を返すことを
// Vitest が同じファイルで確かめる。
type naturalSortKeyFile struct {
	Cases []naturalSortKeyCase `json:"cases"`
}

type naturalSortKeyCase struct {
	Note  string `json:"note"`
	Input string `json:"input"`
	Want  string `json:"want"`
}

var naturalSortKeyPath = filepath.Join("..", "testdata", "natural_sort_key.json")

func readNaturalSortKeyFile(t *testing.T) naturalSortKeyFile {
	t.Helper()
	data, err := os.ReadFile(naturalSortKeyPath)
	if err != nil {
		t.Fatal(err)
	}
	var file naturalSortKeyFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestNaturalSortKeySharedCases(t *testing.T) {
	file := readNaturalSortKeyFile(t)
	if len(file.Cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range file.Cases {
		if got := domain.NaturalSortKey(c.Input); got != c.Want {
			t.Errorf("%s: NaturalSortKey(%q) = %q, want %q", c.Note, c.Input, got, c.Want)
		}
	}
}

func TestNaturalSortKeySharedCasesAreInKeyOrder(t *testing.T) {
	file := readNaturalSortKeyFile(t)
	for i := 1; i < len(file.Cases); i++ {
		prev, cur := file.Cases[i-1], file.Cases[i]
		if strings.Compare(prev.Want, cur.Want) > 0 {
			t.Errorf("cases[%d] %q (%s) sorts after cases[%d] %q (%s)", i-1, prev.Want, prev.Note, i, cur.Want, cur.Note)
		}
	}
}
