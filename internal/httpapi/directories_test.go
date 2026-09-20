package httpapi

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestListDirectoriesReturnsSortedRealChildren(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Zulu", "alpha"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "alpha"), filepath.Join(root, "link")); err != nil {
		t.Logf("symbolic link unavailable: %v", err)
	}
	handler := newTestServer(t, Options{})
	rec := do(t, handler, http.MethodGet, "/api/directories?path="+url.QueryEscape(root))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	listing := decode[gen.DirectoryListing](t, rec)
	if listing.CurrentPath == nil || *listing.CurrentPath != root || listing.ParentPath == nil {
		t.Fatalf("listing paths = %+v", listing)
	}
	if len(listing.Directories) != 2 || listing.Directories[0].Name != "alpha" || listing.Directories[1].Name != "Zulu" {
		t.Fatalf("directories = %+v", listing.Directories)
	}
	if rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatal("directory listing is cacheable")
	}
}

func TestListDirectoriesRootsAndErrors(t *testing.T) {
	handler := newTestServer(t, Options{})
	if rec := do(t, handler, http.MethodGet, "/api/directories"); rec.Code != http.StatusOK {
		t.Fatalf("roots response = %d %s", rec.Code, rec.Body)
	} else if roots := decode[gen.DirectoryListing](t, rec); len(roots.Directories) == 0 || roots.ParentPath != nil {
		t.Fatalf("roots listing = %+v", roots)
	}
	if rec := do(t, handler, http.MethodGet, "/api/directories?path=relative"); rec.Code != http.StatusBadRequest {
		t.Fatalf("relative status = %d", rec.Code)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if rec := do(t, handler, http.MethodGet, "/api/directories?path="+url.QueryEscape(missing)); rec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", rec.Code)
	}
}

func TestListDirectoriesRejectsSymbolicLinkComponent(t *testing.T) {
	root := t.TempDir()
	realChild := filepath.Join(root, "real", "child")
	if err := os.MkdirAll(realChild, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "real"), link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	handler := newTestServer(t, Options{})
	target := "/api/directories?path=" + url.QueryEscape(filepath.Join(link, "child"))
	if rec := do(t, handler, http.MethodGet, target); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
}
