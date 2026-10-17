package media

import (
	"os"
	"path/filepath"
	"testing"
)

// mediaFixture is a read-only input shared by tests; every generated output
// still belongs in t.TempDir(). See testdata/README.md for regeneration.
func mediaFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", "videos", name)
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	return path
}
