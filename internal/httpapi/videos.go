package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// maxQueryLength は検索語に許す長さである。api/openapi.yaml の
// maxLength と同じ値で、越える要求は誤りとして断る。
const maxQueryLength = 100

// thumbnailVersionLength はサムネイルの URL に付ける版の長さである。
// content_key の先頭を使う。内容が変われば版も変わるので、長期キャッシュを
// 安全に効かせられる。
const thumbnailVersionLength = 12

// ListVideos は動画の一覧を返す（GET /api/videos）。
//
// ページングはカーソル方式で、総件数はページとは独立に返る。
func (s *server) ListVideos(w http.ResponseWriter, r *http.Request, params gen.ListVideosParams) {
	if s.videos == nil {
		s.internalError(w, "一覧の問い合わせ先が設定されていません", nil)
		return
	}

	query := domain.VideoQuery{Sort: domain.SortAddedDesc}

	if params.Sort != nil {
		sort := domain.VideoSort(*params.Sort)
		if !sort.Valid() {
			s.invalidRequest(w, "並び順は addedDesc か titleAsc を指定してください")
			return
		}
		query.Sort = sort
	}

	// 件数は入口で丸める。ここで確定させておくと、応答の件数と問い合わせの
	// 条件が一致し、「limit=1000 を渡したのに 200 件しか来ない」理由が
	// 契約（api/openapi.yaml の maximum）だけで説明できる。
	query.Limit = domain.DefaultLimit
	if params.Limit != nil {
		limit := *params.Limit
		if limit < 1 {
			s.invalidRequest(w, "1ページの件数は 1 以上を指定してください")
			return
		}
		query.Limit = min(limit, domain.MaxLimit)
	}

	if params.Cursor != nil {
		query.Cursor = *params.Cursor
	}

	if params.Query != nil {
		if len([]rune(*params.Query)) > maxQueryLength {
			s.invalidRequest(w, "検索語は 100 文字までにしてください")
			return
		}
		query.Query = *params.Query
	}

	page, err := s.videos.ListVideos(r.Context(), query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		// 黙って先頭から返さない。無限スクロールが巻き戻って同じ内容を
		// 延々と表示することになる。
		s.invalidRequest(w, "読み込み位置を解釈できません。一覧を開き直してください")
		return
	case err != nil:
		s.internalError(w, "一覧を取得できませんでした", err)
		return
	}

	progress := s.progressFor(r.Context(), page.Items)

	payload := gen.VideoPage{Items: make([]gen.Video, 0, len(page.Items)), Total: page.Total}
	for _, video := range page.Items {
		payload.Items = append(payload.Items, withProgress(toAPIVideo(video), progress, video.ContentKey))
	}
	if page.NextCursor != "" {
		next := page.NextCursor
		payload.NextCursor = &next
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// GetVideo は動画1件の詳細を返す（GET /api/videos/{id}）。
func (s *server) GetVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}

	progress := s.progressFor(r.Context(), []domain.Video{video})

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, withProgress(toAPIVideo(video), progress, video.ContentKey), s.logger)
}

// progressFor は動画たちの再生位置をまとめて引く。1件ずつ引くと、60 件の
// 一覧で 60 回の問い合わせになる。
//
// 引けなかった場合は一覧を諦めない。再生位置は一覧に「あると嬉しい」情報で
// あって、無いと動画を見渡せなくなるものではない。
func (s *server) progressFor(ctx context.Context, videos []domain.Video) map[string]domain.Progress {
	if s.playback == nil || len(videos) == 0 {
		return nil
	}

	keys := make([]string, 0, len(videos))
	for _, video := range videos {
		if video.ContentKey != "" {
			keys = append(keys, video.ContentKey)
		}
	}

	progress, err := s.playback.ProgressByContentKeys(ctx, keys)
	if err != nil {
		s.logger.Warn("再生位置を読み出せませんでした", slog.Any("error", err))
		return nil
	}
	return progress
}

// withProgress は再生位置を載せる。記録の無い動画では省略する。
//
// 「記録が無い」ことを位置 0 で表さないのは、先頭まで戻した動画と一度も
// 見ていない動画を、一覧で区別できなくなるためである。
func withProgress(video gen.Video, progress map[string]domain.Progress, contentKey string) gen.Video {
	found, ok := progress[contentKey]
	if !ok {
		return video
	}
	payload := toAPIProgress(found)
	video.Progress = &payload
	return video
}

// lookupVideo は id から動画を引く。見つからなければ応答を書いて false を返す。
func (s *server) lookupVideo(w http.ResponseWriter, r *http.Request, id int64) (domain.Video, bool) {
	if s.videos == nil {
		s.internalError(w, "一覧の問い合わせ先が設定されていません", nil)
		return domain.Video{}, false
	}

	video, err := s.videos.GetVideo(r.Context(), id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "その動画はありません")
		return domain.Video{}, false
	case err != nil:
		s.internalError(w, "動画を取得できませんでした", err)
		return domain.Video{}, false
	}
	return video, true
}

// toAPIVideo は domain.Video を契約の形へ写す。
//
// 取得できていない値は省略する。0 で埋めると、一覧で「尺が 0 の動画」と
// 「尺が分からない動画」を区別できなくなる。
func toAPIVideo(video domain.Video) gen.Video {
	out := gen.Video{
		Id:             video.ID,
		Title:          video.Title,
		SizeBytes:      video.SizeBytes,
		AddedAt:        video.AddedAt,
		Playable:       video.PlayableInBrowser(),
		ProbeState:     gen.VideoProbeState(video.ProbeState),
		ThumbnailState: gen.VideoThumbnailState(video.ThumbnailState),
	}

	if video.DurationMs != nil {
		out.DurationMs = video.DurationMs
	}
	if video.Width != nil {
		out.Width = video.Width
	}
	if video.Height != nil {
		out.Height = video.Height
	}
	if video.Container != "" {
		container := video.Container
		out.Container = &container
	}
	if video.VideoCodec != "" {
		codec := video.VideoCodec
		out.VideoCodec = &codec
	}
	if video.AudioCodec != "" {
		codec := video.AudioCodec
		out.AudioCodec = &codec
	}
	if video.UnplayableReason != "" {
		reason := gen.VideoUnplayableReason(video.UnplayableReason)
		out.UnplayableReason = &reason
	}
	if video.ProbeError != "" {
		reason := video.ProbeError
		out.ProbeError = &reason
	}
	// サムネイルは生成済みのときだけ URL を出す。未生成の動画も一覧には
	// 並べ、クライアントは枠だけを描く。
	if video.HasThumbnail() {
		url := thumbnailURL(video)
		out.ThumbnailUrl = &url
	}

	return out
}

// thumbnailURL はサムネイルの取得先を組み立てる。版は content_key の先頭で、
// 内容が変われば URL も変わる。
func thumbnailURL(video domain.Video) string {
	version := video.ContentKey
	if len(version) > thumbnailVersionLength {
		version = version[:thumbnailVersionLength]
	}
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/thumbnail?v=" + version
}
