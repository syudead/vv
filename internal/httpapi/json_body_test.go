package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestReadLargeJSONBodyLimit(t *testing.T) {
	const limit = 32
	srv := &server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, size := range []int{limit - 1, limit, limit + 1} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			// Trailing whitespace makes the second Decode verify the body boundary too.
			const value = `{"value":"ok"}`
			body := value + strings.Repeat(" ", size-len(value))
			req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			var got struct {
				Value string `json:"value"`
			}
			ok := srv.readLargeJSONBody(rec, req, limit, &got)
			if size <= limit {
				if !ok || got.Value != "ok" {
					t.Fatalf("accepted = %v, value = %q: %s", ok, got.Value, rec.Body)
				}
			} else if ok || rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "32 bytes") {
				t.Fatalf("accepted = %v, status = %d: %s", ok, rec.Code, rec.Body)
			}
		})
	}
}
