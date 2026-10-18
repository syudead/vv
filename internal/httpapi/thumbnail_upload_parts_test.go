package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestReadUploadPartBounds(t *testing.T) {
	const limit = 8
	for _, data := range []string{"", "1234567", "12345678"} {
		got, err := readUploadPart(strings.NewReader(data), limit)
		if err != nil || got == nil || string(got) != data {
			t.Errorf("read %q = %q, %v", data, got, err)
		}
	}
	reader := strings.NewReader("123456789extra")
	got, err := readUploadPart(reader, limit)
	var tooLarge *domain.ThumbnailUploadTooLargeError
	if got != nil || !errors.As(err, &tooLarge) || tooLarge.Limit != limit {
		t.Fatalf("over limit = %q, %v", got, err)
	}
	if reader.Len() != len("extra") {
		t.Errorf("reader has %d bytes left; should read only limit+1", reader.Len())
	}
}

func TestReadUploadPartErrors(t *testing.T) {
	readErr := errors.New("read failed")
	if _, err := readUploadPart(errReader{readErr}, 8); !errors.Is(err, readErr) {
		t.Errorf("read error = %v", err)
	}
	if _, err := readUploadPart(errReader{&http.MaxBytesError{Limit: 32}}, 8); !errors.Is(err, domain.ErrThumbnailUploadTooLarge) {
		t.Errorf("body limit error = %v", err)
	}
}

func TestReadUploadImagePartMapsBodyLimit(t *testing.T) {
	srv := &server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	if _, ok := srv.readUploadImagePart(rec, errReader{&http.MaxBytesError{Limit: thumbnailUploadMaxBody}}, srv.thumbnailUploadError); ok {
		t.Fatal("oversized body was accepted")
	}
	assertErrorBody(t, "body limit", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
		reason: reasonThumbnailUploadTooLarge, limit: domain.ThumbnailUploadMaxBytes,
	})
}

func TestMultipartBodyLimitOutsideParts(t *testing.T) {
	body, contentType := uploadBody(t, []byte("image"))
	// The preamble alone exceeds this small body limit, before a part is opened.
	reader := io.MultiReader(strings.NewReader(strings.Repeat("x\r\n", 12)), body)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", reader)
	req.Header.Set("Content-Type", contentType)
	srv := &server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	limits, parts := 0, 0
	ok := srv.readMultipart(rec, req, 32, func() { limits++ }, func(string, *multipart.Part) bool {
		parts++
		return true
	})
	if ok || limits != 1 || parts != 0 {
		t.Errorf("ok = %v, limit callbacks = %d, parts = %d", ok, limits, parts)
	}
}
