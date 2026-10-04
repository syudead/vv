package httpapi

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func serveCompressed(t *testing.T, acceptEncoding string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/videos", nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	compressResponses(handler).ServeHTTP(rec, req)
	return rec
}

func gunzip(t *testing.T, body io.Reader) string {
	t.Helper()
	reader, err := gzip.NewReader(body)
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

var largeJSON = `{"items":[` + strings.Repeat(`{"id":1,"title":"video"},`, 200) + `{}]}`

func TestCompressResponsesGzipsLargeJSON(t *testing.T) {
	rec := serveCompressed(t, "br, gzip;q=0.8", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"body": largeJSON}, nil)
	})
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Errorf("Vary = %q", got)
	}
	if rec.Body.Len() >= len(largeJSON) {
		t.Errorf("compressed body is %d bytes, not smaller than %d", rec.Body.Len(), len(largeJSON))
	}
	if body := gunzip(t, rec.Body); !strings.Contains(body, `{\"id\":1`) {
		t.Errorf("decompressed body = %.80s", body)
	}
}

func TestCompressResponsesGzipsStaticFilesAndDropsTheLength(t *testing.T) {
	script := strings.Repeat("console.log('vv');\n", 200)
	rec := serveCompressed(t, "gzip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(script)))
		_, _ = io.WriteString(w, script)
	})
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length = %q, want none", got)
	}
	if body := gunzip(t, rec.Body); body != script {
		t.Errorf("decompressed body differs (%d bytes)", len(body))
	}
}

func TestCompressResponsesLeavesOthersUntouched(t *testing.T) {
	cases := []struct {
		name           string
		acceptEncoding string
		handler        http.HandlerFunc
	}{
		{"no Accept-Encoding", "", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, largeJSON, nil)
		}},
		{"gzip refused", "gzip;q=0, br", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, largeJSON, nil)
		}},
		{"small with length", "gzip", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "2")
			_, _ = io.WriteString(w, "{}")
		}},
		{"video", "gzip", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = io.WriteString(w, largeJSON)
		}},
		{"event stream", "gzip", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: scan\ndata: {}\n\n")
		}},
		{"partial content", "gzip", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/javascript")
			w.Header().Set("Content-Range", "bytes 0-9/100")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, largeJSON)
		}},
		{"already encoded", "gzip", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Encoding", "br")
			_, _ = io.WriteString(w, largeJSON)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveCompressed(t, tc.acceptEncoding, tc.handler)
			if got := rec.Header().Get("Content-Encoding"); got == "gzip" {
				t.Fatalf("Content-Encoding = gzip, want untouched")
			}
			if strings.HasPrefix(rec.Body.String(), "\x1f\x8b") {
				t.Errorf("body is gzip")
			}
		})
	}
}

func TestCompressResponsesFlushesWhatWasWritten(t *testing.T) {
	flushed := ""
	rec := serveCompressed(t, "gzip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"a":`)
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Fatal(err)
		}
		// 流した分だけで、まだ閉じていない gzip を読める。
		reader, err := gzip.NewReader(strings.NewReader(w.(*compressWriter).ResponseWriter.(*httptest.ResponseRecorder).Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		chunk := make([]byte, 16)
		n, _ := reader.Read(chunk)
		flushed = string(chunk[:n])
		_, _ = io.WriteString(w, `1}`)
	})
	if flushed != `{"a":` {
		t.Errorf("flushed = %q, want %q", flushed, `{"a":`)
	}
	if body := gunzip(t, rec.Body); body != `{"a":1}` {
		t.Errorf("body = %q", body)
	}
}
