package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
)

func setRevalidate(w http.ResponseWriter, etag string) {
	w.Header().Set("Cache-Control", cacheRevalidate)
	w.Header().Set("ETag", etag)
}

// Use content identity for ETags because rebuilt artifacts can retain size and modification time.

// Reuse generation-time digests to avoid rereading preview bytes.
func digestETag(kind, digest string) string {
	return hashETag([]byte(kind + "\x00" + digest))
}

// readerETag consumes small artifacts and rewinds to the beginning.
func readerETag(content io.ReadSeeker) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, content); err != nil {
		return "", err
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return hashETag(h.Sum(nil)), nil
}

func bytesETag(body []byte) string { return hashETag(body) }

func hashETag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// Use weak If-None-Match comparison per RFC 9110 §13.1.2.
func etagMatches(ifNoneMatch, etag string) bool {
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}
