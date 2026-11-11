package httpapi

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// imageSearchFunc reads the file and asks for the rest of the request as the use case does, then answers.
type imageSearchFunc func(file []byte) (domain.ImageSearchResult, error)

func (f imageSearchFunc) Search(
	_ context.Context, _ domain.Audience, file io.Reader, onlyFile func() error,
) (domain.ImageSearchResult, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return domain.ImageSearchResult{}, err
	}
	if err := onlyFile(); err != nil {
		return domain.ImageSearchResult{}, err
	}
	return f(data)
}

func postImageSearch(t *testing.T, search ImageSearch, parts ...string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, name := range parts {
		part, err := writer.CreateFormFile(name, "query.mp4")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("video bytes")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/image-search", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	newTestServer(t, Options{ImageSearch: search}).ServeHTTP(rec, req)
	return rec
}

func TestSearchByImageMapsRefusalsToTheirCodes(t *testing.T) {
	t.Parallel()
	fails := func(err error) ImageSearch {
		return imageSearchFunc(func([]byte) (domain.ImageSearchResult, error) { return domain.ImageSearchResult{}, err })
	}
	cases := []struct {
		name   string
		search ImageSearch
		parts  []string
		want   wantError
	}{
		{"two files", fails(nil), []string{"file", "file"},
			wantError{status: http.StatusBadRequest, code: codeInvalidRequest}},
		{"no file", fails(nil), nil, wantError{status: http.StatusBadRequest, code: codeInvalidRequest}},
		{"unreadable", fails(domain.ErrImageSearchUnreadable), []string{"file"}, wantError{
			status: http.StatusBadRequest, code: codeInvalidRequest, reason: gen.ErrorReasonImageSearchUnreadable}},
		{"oversized image", fails(&domain.ImageSearchTooLargeError{Limit: domain.ImageSearchImageMaxPixels}),
			[]string{"file"}, wantError{status: http.StatusRequestEntityTooLarge, code: codeInvalidRequest,
				reason: gen.ErrorReasonImageSearchTooLarge, limit: domain.ImageSearchImageMaxPixels}},
		{"too long", fails(&domain.ImageSearchTooLongError{LimitMs: domain.ImageSearchVideoMaxMs}),
			[]string{"file"}, wantError{status: http.StatusBadRequest, code: codeInvalidRequest,
				reason: gen.ErrorReasonImageSearchTooLong, limit: int(domain.ImageSearchVideoMaxMs)}},
	}
	for _, tc := range cases {
		rec := postImageSearch(t, tc.search, tc.parts...)
		assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), tc.want)
	}
}

// The listed video is the bundle's representative while the match names the member it was found in; section fields
// are present only for a video query.
func TestSearchByImageMapsTheResult(t *testing.T) {
	t.Parallel()
	var received []byte
	search := imageSearchFunc(func(file []byte) (domain.ImageSearchResult, error) {
		received = file
		return domain.ImageSearchResult{
			Kind: domain.ImageSearchQueryVideo, DurationMs: 30_000, UnindexedCount: 4,
			Items: []domain.ImageSearchItem{{
				Video: sampleVideo(12, "代表"),
				Match: domain.ImageSearchMatch{VideoID: 13, Kind: domain.ImageSearchMatchPartial, Rate: 0.75,
					PositionMs: 8000, StartMs: 8000, EndMs: 24_000, QueryStartMs: 0, QueryEndMs: 16_000},
			}},
		}, nil
	})
	rec := postImageSearch(t, search, "file")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if string(received) != "video bytes" {
		t.Fatalf("the use case received %q", received)
	}
	got := decode[gen.ImageSearchResult](t, rec)
	if got.Query.Kind != gen.ImageSearchQueryKindVideo || got.Query.DurationMs == nil || *got.Query.DurationMs != 30_000 ||
		got.UnindexedCount != 4 || len(got.Items) != 1 {
		t.Fatalf("result = %+v", got)
	}
	item := got.Items[0]
	match := item.Match
	if item.Video.Id != 12 || match.VideoId != 13 || match.Kind != gen.ImageSearchMatchKindPartial || match.Rate != 0.75 ||
		match.PositionMs != 8000 || match.StartMs == nil || *match.StartMs != 8000 || match.EndMs == nil ||
		*match.EndMs != 24_000 || match.QueryStartMs == nil || *match.QueryStartMs != 0 || match.QueryEndMs == nil ||
		*match.QueryEndMs != 16_000 {
		t.Fatalf("item = %+v, match = %+v", item, match)
	}
}

func TestSearchByImageCancelledWritesNoResponse(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	search := imageSearchFunc(func([]byte) (domain.ImageSearchResult, error) {
		cancel()
		return domain.ImageSearchResult{}, context.Canceled
	})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if _, err := writer.CreateFormFile("file", "query.png"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/image-search", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	newTestServer(t, Options{ImageSearch: search}).ServeHTTP(rec, req)
	if rec.Body.Len() != 0 || rec.Header().Get("Content-Type") != "" {
		t.Fatalf("a cancelled search wrote %d %q", rec.Code, rec.Body)
	}
}

// stalledSearch reads the start of the file, then the rest until the read fails, as receiving a video does.
type stalledSearch struct {
	reading chan struct{}
	ended   chan error
}

func (s stalledSearch) Search(
	_ context.Context, _ domain.Audience, file io.Reader, _ func() error,
) (domain.ImageSearchResult, error) {
	if _, err := io.ReadFull(file, make([]byte, 1)); err != nil {
		s.ended <- err
		return domain.ImageSearchResult{}, err
	}
	close(s.reading)
	_, err := io.Copy(io.Discard, file)
	s.ended <- err
	return domain.ImageSearchResult{}, err
}

// Logging out while the sender has stopped sending ends the blocked body read, so the use case can remove the upload.
func TestSearchByImageEndsAStalledUploadWhenTheSessionIsRevoked(t *testing.T) {
	search := stalledSearch{reading: make(chan struct{}), ended: make(chan error, 1)}
	env := newAuthEnv(t, t.TempDir(), Options{ImageSearch: search})
	cookie := env.setup()
	server := httptest.NewServer(env.handler)
	t.Cleanup(server.Close)

	body, sender := io.Pipe()
	t.Cleanup(func() { _ = sender.Close() })
	writer := multipart.NewWriter(sender)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/image-search", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	go func() {
		if resp, err := server.Client().Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	part, err := writer.CreateFormFile("file", "query.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("the start of a video")); err != nil {
		t.Fatal(err)
	}
	<-search.reading

	logout, _ := http.NewRequest(http.MethodPost, server.URL+"/api/auth/logout", nil)
	logout.AddCookie(cookie)
	resp, err := server.Client().Do(logout)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	select {
	case err := <-search.ended:
		if err == nil {
			t.Fatal("the body read ended without an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the body read did not end after logout")
	}
}
