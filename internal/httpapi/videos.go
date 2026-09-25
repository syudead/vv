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

// maxTagFilterCount は一覧の絞り込みに使えるタグの id の最大個数である。
// api/openapi.yaml の tag パラメータの maxItems と同じ値で、越える要求は
// invalid_request にする（specs/014-video-tags/contracts/tags-api.md §5）。
const maxTagFilterCount = 16

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

	filters, ok := s.parseListFilters(w, listFilterParams{
		watch: params.Watch, playable: params.Playable, sort: params.Sort, seed: params.Seed,
	})
	if !ok {
		return
	}
	query.Watch, query.PlayableOnly, query.Sort, query.Seed = filters.watch, filters.playableOnly, filters.sort, filters.seed

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

	if query.Query, ok = s.parseSearchQuery(w, params.Query); !ok {
		return
	}
	if query.TagIDs, ok = s.parseTagFilter(w, params.Tag); !ok {
		return
	}
	audience := audienceFrom(r.Context())
	if !s.checkAudienceQuery(w, audience, query) {
		return
	}

	page, err := s.videos.ListVideos(r.Context(), audience, query)
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

	s.writeVideoPage(w, r, page, s.registeredRoots(r.Context()))
}

// writeVideoPage は一覧1ページを応答に書く。各項目には再生位置・タグと、一覧に
// 出す所在の置かれたフォルダ（Video.folder）を載せる。roots が無ければ folder は
// 省く。page.MissingTagIDs があれば応答にもそのまま載せる
// （contracts/tags-api.md §5）。
func (s *server) writeVideoPage(w http.ResponseWriter, r *http.Request, page domain.VideoPage, roots []domain.MediaFolder) {
	progress := s.progressFor(r.Context(), page.Items)
	tags := s.tagsFor(r.Context(), page.Items)

	audience := audienceFrom(r.Context())
	payload := gen.VideoPage{Items: make([]gen.Video, 0, len(page.Items)), Total: page.Total}
	for _, view := range s.presentVideos(r.Context(), page.Items) {
		video := view.Video
		item := withTags(withProgress(toAPIVideo(view), progress, video.ContentKey), tags, video.ContentKey)
		if folder, ok := domain.LocateVideoFolder(roots, video.Path); ok {
			item.Folder = &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
		}
		payload.Items = append(payload.Items, forAudience(audience, item))
	}
	if page.NextCursor != "" {
		next := page.NextCursor
		payload.NextCursor = &next
	}
	if len(page.MissingTagIDs) > 0 {
		missing := page.MissingTagIDs
		payload.MissingTagIds = &missing
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// checkAudienceQuery は、見る人がこの一覧の条件を使えるかを確かめる。ゲストが所有者の
// データに依る条件を指定したら 400 を書いて false を返す（contracts/guest-api.md §3）。
func (s *server) checkAudienceQuery(w http.ResponseWriter, audience domain.Audience, query domain.VideoQuery) bool {
	if err := audience.CheckVideoQuery(query); err != nil {
		s.invalidRequest(w, "視聴状態・再生日時の並べ替え・タグの絞り込みは、ログインしてから使えます")
		return false
	}
	return true
}

// forAudience は応答に載せる動画を見る人に合わせる。ゲストには所在（絶対パス）・
// 再生位置・読み取りの誤り（絶対パスを含みうる）を出さず、タグを空の配列にする
// （contracts/guest-api.md §1、親 Issue 要件 18）。所有者にはそのまま返す。
func forAudience(audience domain.Audience, video gen.Video) gen.Video {
	if audience.IsOwner() {
		return video
	}
	video.Location = nil
	video.Progress = nil
	video.ProbeError = nil
	video.Tags = []gen.VideoTag{}
	return video
}

// registeredRoots は Video.folder を作るための登録フォルダの一覧を引く。
//
// 引けなかった場合は一覧を諦めず、folder を省く。folder は置き場所の手がかりで
// あって、無いと動画を見渡せなくなるものではない（progressFor と同じ扱い）。
func (s *server) registeredRoots(ctx context.Context) []domain.MediaFolder {
	var list func(context.Context) ([]domain.MediaFolder, error)
	switch {
	case s.folders != nil:
		list = s.folders.ListMediaFolders
	case s.mediaFolders != nil:
		list = s.mediaFolders.ListMediaFolders
	default:
		return nil
	}
	roots, err := list(ctx)
	if err != nil {
		s.logger.Warn("登録フォルダを読み出せませんでした", slog.Any("error", err))
		return nil
	}
	return roots
}

// listFilterParams は2つの一覧の経路が共通に受ける絞り込みと並び順である。
type listFilterParams struct {
	watch    *gen.WatchFilter
	playable *bool
	sort     *gen.VideoSort
	seed     *int64
}

type listFilters struct {
	watch        domain.WatchFilter
	playableOnly bool
	sort         domain.VideoSort
	seed         int64
}

// parseListFilters は絞り込みと並び順を検査して写す。未知の値と範囲外の seed は
// 400 を書いて false を返す（contracts/list-api.md §5）。黙って既定へ戻さないのは、
// 画面の送り間違いを、条件と違う一覧として見せないためである。
func (s *server) parseListFilters(w http.ResponseWriter, params listFilterParams) (listFilters, bool) {
	out := listFilters{watch: domain.WatchAll, sort: domain.SortAddedDesc}
	if params.watch != nil {
		watch := domain.WatchFilter(*params.watch)
		if !watch.Valid() {
			s.invalidRequest(w, "視聴状態の値が不明です")
			return listFilters{}, false
		}
		out.watch = watch
	}
	if params.playable != nil {
		out.playableOnly = *params.playable
	}
	if params.sort != nil {
		sort := domain.VideoSort(*params.sort)
		if !sort.Valid() {
			s.invalidRequest(w, "並び順の値が不明です")
			return listFilters{}, false
		}
		out.sort = sort
	}
	if params.seed != nil {
		seed := *params.seed
		if seed < 0 || seed > domain.MaxShuffleSeed {
			s.invalidRequest(w, "並びの種は 0 以上 2147483647 以下にしてください")
			return listFilters{}, false
		}
		out.seed = seed
	}
	return out, true
}

// parseSearchQuery は検索語の長さを検査する。越えていれば 400 を書いて false を返す。
func (s *server) parseSearchQuery(w http.ResponseWriter, query *string) (string, bool) {
	if query == nil {
		return "", true
	}
	if len([]rune(*query)) > maxQueryLength {
		s.invalidRequest(w, "検索語は 100 文字までにしてください")
		return "", false
	}
	return *query, true
}

// parseTagFilter は絞り込みに使うタグの id の個数を検査する。17 個以上は 400 を
// 書いて false を返す（contracts/tags-api.md §5）。存在しない id は誤りにせず、
// 保存層（existingTagIDs）が落として応答の missingTagIds に返す。
func (s *server) parseTagFilter(w http.ResponseWriter, tag *[]int64) ([]int64, bool) {
	if tag == nil {
		return nil, true
	}
	if len(*tag) > maxTagFilterCount {
		s.invalidRequest(w, "タグでの絞り込みは 16 個までにしてください")
		return nil, false
	}
	return *tag, true
}

// GetVideo は動画1件の詳細を返す（GET /api/videos/{id}）。
func (s *server) GetVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}

	progress := s.progressFor(r.Context(), []domain.Video{video})
	tags := s.tagsFor(r.Context(), []domain.Video{video})
	payload := withTags(withProgress(s.apiVideo(r.Context(), video), progress, video.ContentKey), tags, video.ContentKey)

	// 所在とシーク用プレビューの状態は、動画1件の応答にだけ載せる。一覧に載せると、
	// 画面が使わない絶対パスを1ページ 60 件ぶん毎回送ることになる。所在は
	// forAudience がゲストの応答から外す。
	payload.Location = &gen.VideoLocation{Path: video.Path, Openable: s.canOpen(r)}
	payload.Folder = detailFolder(s.registeredRoots(r.Context()), video.Path)
	if video.HasSeekThumbnail() && s.catalog != nil {
		state, err := s.catalog.SeekThumbnailState(r.Context(), video)
		if err != nil {
			s.internalError(w, "動画を取得できませんでした", err)
			return
		}
		apiState := gen.VideoSeekThumbnailState(state)
		payload.SeekThumbnailState = &apiState
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, forAudience(audienceFrom(r.Context()), payload), s.logger)
}

// detailFolder は動画1件の応答に載せる、代表の所在が置かれたフォルダを作る。
// 再生画面の見出しのパンくずに使うので、一覧と違って登録フォルダの表示名も載せる。
// 所在がどの登録フォルダにも含まれない（登録を外した直後など）ときは nil を返す。
func detailFolder(roots []domain.MediaFolder, locationPath string) *gen.VideoFolder {
	folder, ok := domain.LocateVideoFolder(roots, locationPath)
	if !ok {
		return nil
	}
	out := &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
	for _, root := range roots {
		if root.ID == folder.RootID {
			name := domain.FolderName(root.Path, "")
			out.RootName = &name
			break
		}
	}
	return out
}

// progressFor は動画たちの再生位置をまとめて引く。1件ずつ引くと、60 件の
// 一覧で 60 回の問い合わせになる。
//
// 引けなかった場合は一覧を諦めない。再生位置は一覧に「あると嬉しい」情報で
// あって、無いと動画を見渡せなくなるものではない。
//
// ゲストには再生位置を出さないので、読みもしない（contracts/guest-api.md §1）。
func (s *server) progressFor(ctx context.Context, videos []domain.Video) map[string]domain.Progress {
	if s.playback == nil || len(videos) == 0 || !audienceFrom(ctx).IsOwner() {
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

// tagsFor は動画たちのタグをまとめて引く（progressFor と同じ形。Plan の
// Structural Decisions 5・14）。1件ずつ引くと、60 件の一覧で 60 回の問い合わせに
// なる。
//
// 引けなかった場合は一覧を諦めない。タグは「あると嬉しい」情報であって、
// 無いと動画を見渡せなくなるものではない。
//
// ゲストにはタグを出さないので、読みもしない（contracts/guest-api.md §1）。
func (s *server) tagsFor(ctx context.Context, videos []domain.Video) map[string][]domain.VideoTag {
	if s.tags == nil || len(videos) == 0 || !audienceFrom(ctx).IsOwner() {
		return nil
	}

	keys := make([]string, 0, len(videos))
	for _, video := range videos {
		if video.ContentKey != "" {
			keys = append(keys, video.ContentKey)
		}
	}

	tags, err := s.tags.TagsByContentKeys(ctx, keys)
	if err != nil {
		s.logger.Warn("項目のタグを読み出せませんでした", slog.Any("error", err))
		return nil
	}
	return tags
}

// withTags はタグを載せる。タグが1つも無い動画は空の配列にする（null にしない。
// contracts/tags-api.md §1）。
func withTags(video gen.Video, tags map[string][]domain.VideoTag, contentKey string) gen.Video {
	refs := tags[contentKey]
	video.Tags = make([]gen.VideoTag, 0, len(refs))
	for _, ref := range refs {
		video.Tags = append(video.Tags, gen.VideoTag{
			Id: ref.ID, Name: ref.Name, Manual: ref.Manual, FromFolder: ref.FromFolder,
		})
	}
	return video
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

	video, err := s.videos.GetVideo(r.Context(), audienceFrom(r.Context()), id)
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

// toAPIVideo は応答に載せる動画を契約の形へ写す。
//
// 取得できていない値は省略する。0 で埋めると、一覧で「尺が 0 の動画」と
// 「尺が分からない動画」を区別できなくなる。
func toAPIVideo(view domain.VideoView) gen.Video {
	video := view.Video
	out := gen.Video{
		Id:             video.ID,
		Title:          video.Title,
		SizeBytes:      video.SizeBytes,
		AddedAt:        video.AddedAt,
		Playable:       video.PlayableInBrowser(),
		ProbeState:     gen.VideoProbeState(video.ProbeState),
		ThumbnailState: gen.VideoThumbnailState(video.ThumbnailState),
		PreviewState:   gen.VideoPreviewState(video.PreviewState),
		Public:         video.Public,
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
	if video.DisplayAspectRatio != nil {
		out.DisplayAspectRatio = video.DisplayAspectRatio
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
	if video.HasSeekThumbnail() {
		url := seekThumbnailURL(video)
		out.SeekThumbnailUrl = &url
	}
	if video.PreviewState == domain.PreviewStateDone && view.PreviewAvailable {
		url := previewURL(video)
		out.PreviewUrl = &url
	}

	return out
}

// presentVideos は動画たちを応答に載せる形にする。消えたプレビューの作り直しの
// 予約はアプリケーション層（VideoCatalog）が行う。catalog が無ければ、
// プレビューのファイルは無いものとして扱う。
func (s *server) presentVideos(ctx context.Context, videos []domain.Video) []domain.VideoView {
	if s.catalog != nil {
		return s.catalog.PresentVideos(ctx, videos)
	}
	views := make([]domain.VideoView, 0, len(videos))
	for _, video := range videos {
		views = append(views, domain.VideoView{Video: video})
	}
	return views
}

// apiVideo は動画1件を契約の形へ写す。
func (s *server) apiVideo(ctx context.Context, video domain.Video) gen.Video {
	return toAPIVideo(s.presentVideos(ctx, []domain.Video{video})[0])
}

func previewURL(video domain.Video) string {
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/preview?v=" + video.ContentKey
}

// thumbnailURL はサムネイルの取得先を組み立てる。版は content_key の先頭で、
// 内容が変われば URL も変わる。
func thumbnailURL(video domain.Video) string {
	version := thumbnailVersion(video.ContentKey)
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/thumbnail?v=" + version
}

func seekThumbnailURL(video domain.Video) string {
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/seek-thumbnail?v=" +
		thumbnailVersion(video.ContentKey)
}

func thumbnailVersion(contentKey string) string {
	if len(contentKey) > thumbnailVersionLength {
		return contentKey[:thumbnailVersionLength]
	}
	return contentKey
}
