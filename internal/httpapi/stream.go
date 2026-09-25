package httpapi

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// cacheStream は動画本体に付ける値である。内容が同じでも別の利用者に
// 共有キャッシュさせないため public にしない。
const cacheStream = "private, max-age=0, must-revalidate"

// streamContentTypes は拡張子から Content-Type を決める表である。
//
// ffprobe の format_name ではなく拡張子を見るのは、format_name が
// mov,mp4,m4a,3gp,3g2,mj2 のようにまとめて返り mp4 と mov を区別できない
// ためである。再生可否の判定も拡張子を基準にしているので、判定と配信で
// 基準が揃う。
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
// http.ServeContent に任せ、自前で組み立てない（技術選定文書 3.3）。
// Linux では io.Copy が sendfile に落ちるため、大きなファイルでもユーザー
// 空間のコピーが起きない。
//
// 再生できない形式（playable = false）でも配信自体は行う。ブラウザが再生
// できるかどうかと、ファイルを取得できるかは別の話である。
func (s *server) StreamVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	// ゲストとして処理する要求は、非公開にされたら打ち切れるよう台帳に載せる
	// （contracts/guest-api.md §6）。
	video, r, release, ok := s.lookupServedVideo(w, r, id)
	if !ok {
		return
	}
	defer release()

	if s.files == nil {
		s.internalError(w, "メディアファイルの読み出しが設定されていません", nil)
		return
	}
	file, info, path, ok := s.openMediaFile(r, video)
	if !ok {
		// 実体を開けない理由（外を指している、消えた、通常ファイルでない）は
		// 応答で区別しない。403 と 404 を出し分けると、どのパスが存在するかを
		// 漏らすことになる。
		s.notFound(w, "この動画の実体を開けません")
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", streamContentType(path))
	w.Header().Set("Cache-Control", cacheStream)

	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

// openMediaFile は、動画の所在のうち開いてよい実体を開く。開いてよいかは
// MediaFiles が判定し、ここは所在と登録フォルダを渡すだけである。返すパスは
// 所在のパス（辿る前）で、Content-Type とファイル名に使う。
func (s *server) openMediaFile(r *http.Request, video domain.Video) (*os.File, os.FileInfo, string, bool) {
	locations, err := s.videos.VideoLocations(r.Context(), video.ID)
	if err != nil {
		return nil, nil, "", false
	}
	roots, err := s.mediaFolderPaths(r)
	if err != nil {
		return nil, nil, "", false
	}
	for _, location := range locations {
		file, info, err := s.files.OpenMediaFile(roots, location.Path)
		if err == nil {
			return file, info, location.Path, true
		}
		if errors.Is(err, domain.ErrMediaFileOutsideRoot) {
			s.logger.Warn("リンク先が配信の対象外です",
				slog.Int64("video", video.ID), slog.String("path", location.Path))
		} else {
			s.logger.Debug("実体を開けません", slog.Int64("video", video.ID), slog.Any("error", err))
		}
	}
	return nil, nil, "", false
}

// mediaFolderPaths は登録フォルダのパスを返す。
func (s *server) mediaFolderPaths(r *http.Request) ([]string, error) {
	folders, err := s.videos.ListMediaFolders(r.Context())
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(folders))
	for _, folder := range folders {
		roots = append(roots, folder.Path)
	}
	return roots, nil
}

// streamContentType は拡張子から Content-Type を決める。
func streamContentType(path string) string {
	if contentType, ok := streamContentTypes[strings.ToLower(filepath.Ext(path))]; ok {
		return contentType
	}
	return defaultStreamContentType
}
