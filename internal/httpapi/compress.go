package httpapi

import (
	"compress/gzip"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Small responses cost more gzip overhead than they save.
const compressMinBytes = 1024

// Compress only textual API/SPA formats; media is already compressed and SSE must be delivered immediately.
var compressibleTypes = map[string]bool{
	"application/json":          true,
	"application/manifest+json": true,
	"text/html":                 true,
	"text/css":                  true,
	"text/javascript":           true,
	"application/javascript":    true,
	"image/svg+xml":             true,
	"text/plain":                true,
}

var gzipWriters = sync.Pool{New: func() any {
	writer, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
	return writer
}}

// Skip partial, bodyless, and already-encoded responses; unknown-length compressible responses do not require a size threshold.
func compressResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if r.Method == http.MethodHead || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		cw := &compressWriter{ResponseWriter: w}
		defer cw.close()
		next.ServeHTTP(cw, r)
	})
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(coding), "gzip") {
			continue
		}
		for _, param := range strings.Split(params, ";") {
			name, value, _ := strings.Cut(strings.TrimSpace(param), "=")
			if strings.EqualFold(strings.TrimSpace(name), "q") {
				q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				return err == nil && q > 0
			}
		}
		return true
	}
	return false
}

type compressWriter struct {
	http.ResponseWriter
	decided bool
	gz      *gzip.Writer
}

func (w *compressWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *compressWriter) WriteHeader(status int) {
	if !w.decided {
		w.decided = true
		if w.shouldCompress(status) {
			header := w.Header()
			header.Del("Content-Length")
			header.Set("Content-Encoding", "gzip")
			gz := gzipWriters.Get().(*gzip.Writer)
			gz.Reset(w.ResponseWriter)
			w.gz = gz
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *compressWriter) shouldCompress(status int) bool {
	if status < http.StatusOK || status == http.StatusNoContent || status == http.StatusPartialContent ||
		status == http.StatusNotModified {
		return false
	}
	header := w.Header()
	if header.Get("Content-Encoding") != "" || header.Get("Content-Range") != "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || !compressibleTypes[mediaType] {
		return false
	}
	if length := header.Get("Content-Length"); length != "" {
		n, err := strconv.ParseInt(length, 10, 64)
		if err == nil && n < compressMinBytes {
			return false
		}
	}
	return true
}

func (w *compressWriter) Write(p []byte) (int, error) {
	if !w.decided {
		if w.Header().Get("Content-Type") == "" {
			// Determine Content-Type before compression so sniffing does not inspect gzip bytes.
			w.Header().Set("Content-Type", http.DetectContentType(p))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

// Forward uncompressed ReaderFrom calls to preserve sendfile.
func (w *compressWriter) ReadFrom(src io.Reader) (int64, error) {
	if !w.decided {
		return io.Copy(writerOnly{w}, src)
	}
	if w.gz != nil {
		return io.Copy(w.gz, src)
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(src)
	}
	return io.Copy(writerOnly{w.ResponseWriter}, src)
}

func (w *compressWriter) Flush() {
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *compressWriter) close() {
	if w.gz == nil {
		return
	}
	_ = w.gz.Close()
	w.gz.Reset(io.Discard)
	gzipWriters.Put(w.gz)
	w.gz = nil
}
