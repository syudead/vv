package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
)

// 生成物（サムネイル・シークプレビュー・ホバープレビュー）の成功の応答に付ける
// キャッシュの指示と ETag（specs/016-single-account-auth/contracts/guest-api.md §5、
// plan.md Structural Decisions 15）。

// setRevalidate は、使うたびにサーバーへ確かめさせる指示と ETag を付ける。
func setRevalidate(w http.ResponseWriter, etag string) {
	w.Header().Set("Cache-Control", cacheRevalidate)
	w.Header().Set("ETag", etag)
}

// ETag は内容が変われば必ず変わるものから作る。更新時刻と大きさは、作り直した
// 生成物でも前と同じになりうるので使わない。

// digestETag は、生成のときに記録した内容のダイジェストから ETag を作る。
// 中身を読み直さずに済む（ホバープレビュー）。
func digestETag(kind, digest string) string {
	return hashETag([]byte(kind + "\x00" + digest))
}

// readerETag は内容を読み切って ETag を作り、読む位置を先頭へ戻す。小さな
// 生成物（サムネイル）に使う。
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

// bytesETag は内容そのものから ETag を作る。
func bytesETag(body []byte) string { return hashETag(body) }

func hashETag(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// etagMatches は If-None-Match の値が etag に当たるかを返す（RFC 9110 §13.1.2 の弱い比較）。
func etagMatches(ifNoneMatch, etag string) bool {
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}
