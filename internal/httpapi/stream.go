package httpapi

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// cacheStream は動画本体に付ける値である。内容が同じでも別の利用者に
// 共有キャッシュさせないため public にしない（contracts/http-routes.md）。
const cacheStream = "private, max-age=0, must-revalidate"

// streamContentTypes は拡張子から Content-Type を決める表である。
//
// ffprobe の format_name ではなく拡張子を見るのは、format_name が
// mov,mp4,m4a,3gp,3g2,mj2 のようにまとめて返り mp4 と mov を区別できない
// ためである。再生可否の判定も拡張子を基準にしているので、判定と配信で
// 基準が揃う（R-103）。
var streamContentTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
}

// defaultStreamContentType は表に無い拡張子に使う。
const defaultStreamContentType = "application/octet-stream"

// StreamVideo は動画本体を配信する（GET /api/videos/{id}/stream）。
//
// Range の解釈・206・Content-Range・Accept-Ranges・416・If-Range は
// http.ServeContent に任せ、自前で組み立てない（R-105 / 技術選定文書 3.3）。
// Linux では io.Copy が sendfile に落ちるため、大きなファイルでもユーザー
// 空間のコピーが起きない。
//
// 再生できない形式（playable = false）でも配信自体は行う。ブラウザが再生
// できるかどうかと、ファイルを取得できるかは別の話である。
func (s *server) StreamVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}

	file, info, path, ok := s.openMediaFile(r, video)
	if !ok {
		// 実体を開けない理由（外を指している、消えた、通常ファイルでない）は
		// 応答で区別しない。403 と 404 を出し分けると、どのパスが存在するかを
		// 漏らすことになる（R-105）。
		s.notFound(w, "この動画の実体を開けません")
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", streamContentType(path))
	w.Header().Set("Cache-Control", cacheStream)

	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

// openMediaFile は配信してよい実体だけを開く（R-105 / contracts/http-routes.md）。
//
// DB の値をそのまま os.Open に渡す実装は、将来 DB へ書き込む経路が増えたときに
// 任意ファイル読み出しになりうる。次の3つをすべて満たす行だけを開く。
//
//  1. filepath.Clean 後のパスが登録済みroot + 区切り文字で始まる
//  2. filepath.EvalSymlinks 後のパスにも 1 が成り立つ
//  3. 通常ファイルである（ディレクトリ・デバイスファイルを開かない）
func (s *server) openMediaFile(r *http.Request, video domain.Video) (*os.File, os.FileInfo, string, bool) {
	type locationLibrary interface {
		VideoLocations(context.Context, int64) ([]domain.VideoLocation, error)
		ListMediaFolders(context.Context) ([]domain.MediaFolder, error)
	}
	library, ok := s.videos.(locationLibrary)
	if !ok {
		return nil, nil, "", false
	}
	locations, err := library.VideoLocations(r.Context(), video.ID)
	if err != nil {
		return nil, nil, "", false
	}
	folders, err := library.ListMediaFolders(r.Context())
	if err != nil {
		return nil, nil, "", false
	}
	for _, location := range locations {
		for _, folder := range folders {
			if file, info, ok := s.openLocation(video.ID, folder.Path, location.Path); ok {
				return file, info, location.Path, true
			}
		}
	}
	return nil, nil, "", false
}

func (s *server) openLocation(videoID int64, root, path string) (*os.File, os.FileInfo, bool) {
	cleaned := filepath.Clean(path)
	if !isInside(root, cleaned) {
		return nil, nil, false
	}

	// シンボリックリンクを辿った先にも同じ検証を掛ける。Clean だけでは
	// 根の外にあるファイルへ辿り着けてしまう。
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		s.logger.Debug("実体を辿れません", slog.Int64("video", videoID), slog.Any("error", err))
		return nil, nil, false
	}
	if !isInside(root, resolved) {
		s.logger.Warn("リンク先が配信の対象外です",
			slog.Int64("video", videoID), slog.String("path", path))
		return nil, nil, false
	}

	file, err := os.Open(resolved)
	if err != nil {
		s.logger.Debug("実体を開けません", slog.Int64("video", videoID), slog.Any("error", err))
		return nil, nil, false
	}

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, false
	}
	return file, info, true
}

// isInside は path が root の内側にあるかを返す。
//
// 区切り文字まで含めて比べるのは、"/media" と "/media-other" のように
// 接頭辞が一致するだけの別ディレクトリを内側と誤判定しないためである。
// root そのものは「内側のファイル」ではないので false になる。
func isInside(root, path string) bool {
	return root != path && domain.PathWithinRoot(root, path)
}

// streamContentType は拡張子から Content-Type を決める。
func streamContentType(path string) string {
	if contentType, ok := streamContentTypes[strings.ToLower(filepath.Ext(path))]; ok {
		return contentType
	}
	return defaultStreamContentType
}
