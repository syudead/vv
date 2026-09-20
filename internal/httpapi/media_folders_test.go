package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type fakeMediaFolders struct {
	folders   []domain.MediaFolder
	lastPath  string
	lastID    int64
	lastVer   int64
	operation string
	err       error
}

func (f *fakeMediaFolders) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	return f.folders, f.err
}
func (f *fakeMediaFolders) AddMediaFolder(_ context.Context, path string) (domain.MediaFolder, error) {
	f.operation, f.lastPath = "add", path
	if f.err != nil {
		return domain.MediaFolder{}, f.err
	}
	return domain.MediaFolder{ID: 3, Path: path, Version: 1, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0)}, nil
}
func (f *fakeMediaFolders) ReplaceMediaFolder(_ context.Context, id, version int64, path string) (domain.MediaFolder, error) {
	f.operation, f.lastID, f.lastVer, f.lastPath = "replace", id, version, path
	if f.err != nil {
		return domain.MediaFolder{}, f.err
	}
	return domain.MediaFolder{ID: id, Path: path, Version: version + 1, CreatedAt: time.Unix(1, 0), UpdatedAt: time.Unix(2, 0)}, nil
}
func (f *fakeMediaFolders) DeleteMediaFolder(_ context.Context, id, version int64) error {
	f.operation, f.lastID, f.lastVer = "delete", id, version
	return f.err
}

func TestListMediaFoldersReturnsEmptyArray(t *testing.T) {
	rec := do(t, newTestServer(t, Options{MediaFolders: &fakeMediaFolders{}}), http.MethodGet, "/api/media-folders")
	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatal("media folder list is cacheable")
	}
}

func TestMediaFolderMutationsAreIndividual(t *testing.T) {
	folders := &fakeMediaFolders{}
	handler := newTestServer(t, Options{MediaFolders: folders})

	created := jsonRequest(t, handler, http.MethodPost, "/api/media-folders", `{"path":"/media/a"}`)
	if created.Code != http.StatusCreated || decode[gen.MediaFolder](t, created).Path != "/media/a" {
		t.Fatalf("create = %d %s", created.Code, created.Body)
	}
	updated := jsonRequest(t, handler, http.MethodPut, "/api/media-folders/3", `{"path":"/media/b","version":1}`)
	if updated.Code != http.StatusOK || folders.operation != "replace" || folders.lastID != 3 || folders.lastVer != 1 {
		t.Fatalf("update = %d operation=%s id=%d version=%d", updated.Code, folders.operation, folders.lastID, folders.lastVer)
	}
	deleted := request(t, handler, http.MethodDelete, "/api/media-folders/3?version=2", "", nil)
	if deleted.Code != http.StatusNoContent || folders.operation != "delete" || folders.lastVer != 2 {
		t.Fatalf("delete = %d operation=%s version=%d", deleted.Code, folders.operation, folders.lastVer)
	}
}

func TestMediaFolderMutationErrors(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalidMediaFolder, 400, codeInvalidMediaDirectory},
		{domain.ErrUnsupportedMediaFolder, 400, codeUnsupportedMediaDirectory},
		{domain.ErrNotFound, 404, codeMediaFolderNotFound},
		{domain.ErrFolderConflict, 409, codeOverlappingMediaDirectories},
		{domain.ErrScanRunning, 409, codeScanInProgress},
		{domain.ErrVersionConflict, 409, codeConflict},
		{errors.New("database unavailable"), 500, codeInternal},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			handler := newTestServer(t, Options{MediaFolders: &fakeMediaFolders{err: tc.err}})
			rec := jsonRequest(t, handler, http.MethodPost, "/api/media-folders", `{"path":"/media"}`)
			if rec.Code != tc.status || decode[gen.Error](t, rec).Code != tc.code {
				t.Fatalf("response = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestMediaFolderMutationRequiresJSONAndSameOrigin(t *testing.T) {
	folders := &fakeMediaFolders{}
	handler := newTestServer(t, Options{MediaFolders: folders})

	rec := request(t, handler, http.MethodPost, "/api/media-folders", `{"path":"/media"}`, map[string]string{"Content-Type": "text/plain"})
	if rec.Code != http.StatusBadRequest || folders.operation != "" {
		t.Fatalf("content type response = %d operation=%s", rec.Code, folders.operation)
	}
	rec = request(t, handler, http.MethodPost, "/api/media-folders", `{"path":"/media"}`, map[string]string{
		"Content-Type": "application/json", "Origin": "https://attacker.example",
	})
	if rec.Code != http.StatusForbidden || folders.operation != "" {
		t.Fatalf("origin response = %d operation=%s", rec.Code, folders.operation)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS header was added")
	}
}

func jsonRequest(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	return request(t, handler, method, target, body, map[string]string{"Content-Type": "application/json"})
}

func request(t *testing.T, handler http.Handler, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
