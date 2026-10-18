package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/mediafs"
)

// Verify main's stream-store wiring because isolated HTTP tests cannot detect a store-role mismatch
// that makes all streams return 404.
func TestStreamServesIndexedFileWithStore(t *testing.T) {
	f := newListAPIFixture(t)
	content := []byte("not really an mp4 but bytes all the same")
	path := filepath.Join(f.root.Path, "movie.mp4")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	video, err := f.db.ScanIndex().UpsertVideo(f.ctx, domain.VideoFile{
		Path: path, Title: "movie", ContentKey: "key-movie",
		SizeBytes: int64(len(content)), MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewRouter(httpapi.Options{
		Videos: f.db.Library(), MediaFolders: f.db.Settings(), Files: mediafs.New(), Assets: fstest.MapFS{}, Auth: ownerAuth{},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/"+strconv.FormatInt(video.ID, 10)+"/stream", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := rec.Body.String(); got != string(content) {
		t.Fatalf("body = %q, want the file's bytes", got)
	}
}
