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

// compressMinBytes は、長さが分かっている応答を圧縮する最小の大きさである。これより
// 小さい応答は、圧縮の手間と gzip の枠の分だけ損になる。
const compressMinBytes = 1024

// compressibleTypes は圧縮する応答の Content-Type（引数を除く）である。JSON の API の
// 応答と、画面のファイル（HTML・JS・CSS・SVG など）に限る。動画・画像・字幕・変化の
// 知らせ（text/event-stream）は圧縮しない。動画と画像はもう圧縮されており、変化の知らせは
// 届いたその場で読まれなければならないからである。
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

// compressResponses は、gzip を受け付けるクライアント（Accept-Encoding）への応答のうち、
// compressibleTypes の型で compressMinBytes 以上（長さが分からなければ大きさを問わない）の
// ものを gzip で圧縮する（issue 674）。受け付けないクライアントには、今までどおり圧縮せずに返す。
// 部分の応答（206）・本文の無い応答・すでに符号化された応答は手を付けない。
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

// acceptsGzip は Accept-Encoding が gzip を受け付けるか（q=0 で断っていないか）を返す。
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

// compressWriter は、応答の頭を書く時点の状態で圧縮するかを決め、圧縮するなら本文を
// gzip を通して書く。
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
			// net/http と同じく、書く前に型を決める。決めないと圧縮した本文から型を
			// 推し量ってしまう。
			w.Header().Set("Content-Type", http.DetectContentType(p))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

// ReadFrom は、圧縮しないときに下の ResponseWriter の io.ReaderFrom へ中継する
// （errorCacheWriter.ReadFrom と同じ理由で、sendfile を使えるようにする）。
func (w *compressWriter) ReadFrom(src io.Reader) (int64, error) {
	if !w.decided {
		// 頭の判断を Write に任せる。最初の塊で型が決まる。
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

// close は gzip の末尾を書き、書き手を使い回しに戻す。
func (w *compressWriter) close() {
	if w.gz == nil {
		return
	}
	_ = w.gz.Close()
	w.gz.Reset(io.Discard)
	gzipWriters.Put(w.gz)
	w.gz = nil
}
