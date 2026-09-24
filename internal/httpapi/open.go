package httpapi

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// OpenVideoFile は代表の所在をサーバーの PC の既定アプリで開く
// （POST /api/videos/{id}/open）。
//
// 開くのはサーバーが持っているその動画の代表の所在だけで、要求からパスは
// 受け取らない。判定は 404 → 409 open_unavailable → 403 → 409 file_missing →
// 500 の順である。起動できない環境かどうかはサーバー全体で決まる事実なので、
// 要求元を問わず先に返す。
func (s *server) OpenVideoFile(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if s.opener == nil || !s.opener.Available() {
		s.writeError(w, http.StatusConflict, codeOpenUnavailable, "このサーバーではファイルを開けません")
		return
	}
	if !loopbackRequest(r) {
		s.writeError(w, http.StatusForbidden, codeForbidden, "ファイルはサーバーと同じ PC からだけ開けます")
		return
	}
	path, ok, err := s.openablePath(r, video.Path)
	if err != nil {
		s.internalError(w, "登録フォルダを取得できませんでした", err)
		return
	}
	if !ok {
		s.writeError(w, http.StatusConflict, codeFileMissing, "ファイルが見つかりません。移動または削除された可能性があります")
		return
	}
	if err := s.opener.Open(path); err != nil {
		s.internalError(w, "ファイルを開けませんでした", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// openablePath は、代表の所在が登録フォルダの内側にある通常ファイルを指すときだけ
// シンボリックリンクを辿った先のパスを返す。
//
// 配信（openLocation）と同じく、DB の値をそのまま OS へ渡さない。Clean 後の
// パスと、シンボリックリンクを辿った先の両方が登録フォルダの内側にあることを
// 確かめる。既定アプリで開くのは配信より強い操作なので、同じ検証を省かない。
// 外を指している場合も「見つからない」と同じに扱い、どのパスが存在するかを漏らさない。
// 確かめたパスと開くパスを揃えるため、辿った先のパスを opener へ渡す。
func (s *server) openablePath(r *http.Request, path string) (string, bool, error) {
	folders, err := s.videos.ListMediaFolders(r.Context())
	if err != nil {
		return "", false, err
	}
	cleaned := filepath.Clean(path)
	for _, folder := range folders {
		if !isInside(folder.Path, cleaned) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(cleaned)
		if err != nil || !isInside(folder.Path, resolved) {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		return resolved, true, nil
	}
	return "", false, nil
}

// canOpen は location.openable を決める。POST /api/videos/{id}/open の 403 と
// 409 open_unavailable に当たらないときだけ true である。
func (s *server) canOpen(r *http.Request) bool {
	return s.opener != nil && s.opener.Available() && loopbackRequest(r)
}

// loopbackRequest は要求がループバックから、ループバックの Host で来たかを返す。
//
// 要求元（RemoteAddr）だけでは足りない。既存の POST の同一オリジン確認は Origin と
// Host を比べるだけなので、DNS rebinding で 127.0.0.1 を指させた他サイトの
// ページを通してしまう。Host をループバックの名前に限ってこれを塞ぐ。
// X-Forwarded-For は信用しない。
func loopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	return loopbackHostName(r.Host)
}

// loopbackHostName は Host が localhost・127.0.0.1・[::1] のいずれかかを返す。
// ポートは問わない。
func loopbackHostName(hostHeader string) bool {
	name := hostHeader
	if host, _, err := net.SplitHostPort(hostHeader); err == nil {
		name = host
	} else {
		name = strings.TrimSuffix(strings.TrimPrefix(name, "["), "]")
	}
	switch strings.ToLower(name) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
