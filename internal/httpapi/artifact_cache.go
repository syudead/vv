package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strconv"
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

// fileETag は置き場のファイルの ETag を作る。生成物の種類・content_key・更新時刻・
// 大きさのどれかが変われば変わる。content_key をそのまま出さないよう、ハッシュにする。
func fileETag(kind, contentKey string, info os.FileInfo) string {
	return hashETag([]byte(strings.Join([]string{
		kind, contentKey,
		strconv.FormatInt(info.ModTime().UnixNano(), 10),
		strconv.FormatInt(info.Size(), 10),
	}, "\x00")))
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
