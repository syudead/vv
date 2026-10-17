package httpapi

import (
	"context"
	"errors"
	"fmt"
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
// invalid_request にする。
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
		s.internalError(w, "Library queries are not configured.", nil)
		return
	}

	audience := audienceFrom(r.Context())
	query, ok := s.parseVideoQuery(w, audience, videoQueryParams{
		query: params.Query, watch: params.Watch, playable: params.Playable, sort: params.Sort,
		seed: params.Seed, cursor: params.Cursor, limit: params.Limit, tag: params.Tag,
		favorite: params.Favorite, person: params.Person, organization: params.Organization,
		addedWithin: params.AddedWithin, minDuration: params.MinDuration, maxDuration: params.MaxDuration,
		tagKind: params.TagKind,
	})
	if !ok {
		return
	}

	page, err := s.videos.ListVideos(r.Context(), audience, query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		// 黙って先頭から返さない。無限スクロールが巻き戻って同じ内容を
		// 延々と表示することになる。
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load the list.", err)
		return
	}

	s.writeVideoPage(w, r, page, s.registeredRoots(r.Context()))
}

// videoQueryParams は一覧の経路（listVideos・listLibrary）が共通に受けるパラメータである。
type videoQueryParams struct {
	query    *string
	watch    *gen.WatchFilter
	playable *bool
	sort     *gen.VideoSort
	seed     *int64
	cursor   *string
	limit    *int
	tag      *[]int64
	favorite *bool
	// person と organization は人物・団体の id による絞り込み。
	person       *int64
	organization *int64
	// addedWithin は追加日の日数、minDuration と maxDuration は長さの範囲（秒）である
	//
	addedWithin *int
	minDuration *int
	maxDuration *int
	// tagKind はタグの種類の id による絞り込み。
	tagKind *int64
}

// parseVideoQuery は一覧のパラメータを検査して問い合わせにする。誤りなら 400 を書いて
// false を返す。見る人が使えない条件も 400 にする（checkAudienceQuery）。
func (s *server) parseVideoQuery(w http.ResponseWriter, audience domain.Audience, params videoQueryParams) (domain.VideoQuery, bool) {
	query := domain.VideoQuery{Sort: domain.SortAddedDesc}

	filters, ok := s.parseListFilters(w, listFilterParams{
		watch: params.watch, playable: params.playable, sort: params.sort, seed: params.seed,
	})
	if !ok {
		return domain.VideoQuery{}, false
	}
	query.Watch, query.PlayableOnly, query.Sort, query.Seed = filters.watch, filters.playableOnly, filters.sort, filters.seed
	query.FavoriteOnly = params.favorite != nil && *params.favorite

	// 件数は入口で丸める。ここで確定させておくと、応答の件数と問い合わせの
	// 条件が一致し、「limit=1000 を渡したのに 200 件しか来ない」理由が
	// 契約（api/openapi.yaml の maximum）だけで説明できる。
	query.Limit = domain.DefaultLimit
	if params.limit != nil {
		limit := *params.limit
		if limit < 1 {
			s.invalidRequest(w, "limit must be at least 1.")
			return domain.VideoQuery{}, false
		}
		query.Limit = min(limit, domain.MaxLimit)
	}

	if params.cursor != nil {
		query.Cursor = *params.cursor
	}

	if query.Query, ok = s.parseSearchQuery(w, params.query); !ok {
		return domain.VideoQuery{}, false
	}
	if query.TagIDs, ok = s.parseTagFilter(w, params.tag); !ok {
		return domain.VideoQuery{}, false
	}
	if !s.parseIDFilters(w, params, &query) {
		return domain.VideoQuery{}, false
	}
	if !s.parseAddedAndLength(w, params, &query) {
		return domain.VideoQuery{}, false
	}
	if !s.checkAudienceQuery(w, audience, query) {
		return domain.VideoQuery{}, false
	}
	return query, true
}

// parseIDFilters は一覧の person・organization・tagKind を検査して query に書く。指定が無ければ
// 0 のままにする。正の整数でない値は 400 を書いて false を返す。実体の有無は確かめない（当たらなければ
// 空の一覧になる、docs/design-docs/catalog-organization.md、
// docs/design-docs/catalog-organization.md の List parameter `tagKind`）。
func (s *server) parseIDFilters(w http.ResponseWriter, params videoQueryParams, query *domain.VideoQuery) bool {
	for _, filter := range []struct {
		name  string
		value *int64
		out   *int64
	}{
		{"person", params.person, &query.Person},
		{"organization", params.organization, &query.Organization},
		{"tagKind", params.tagKind, &query.TagKindID},
	} {
		if filter.value == nil {
			continue
		}
		if *filter.value < 1 {
			s.invalidRequest(w, filter.name+" must be a positive integer.")
			return false
		}
		*filter.out = *filter.value
	}
	return true
}

// parseAddedAndLength は一覧の addedWithin・minDuration・maxDuration を検査して query に写す。
// 範囲の外の値と、minDuration が maxDuration を越える組は 400 を書いて false を返す。ゲストも使える。
func (s *server) parseAddedAndLength(w http.ResponseWriter, params videoQueryParams, query *domain.VideoQuery) bool {
	switch err := domain.CheckAddedAndLength(params.addedWithin, params.minDuration, params.maxDuration); {
	case errors.Is(err, domain.ErrInvalidAddedWithin):
		s.invalidRequest(w, fmt.Sprintf("addedWithin must be between 1 and %d.", domain.MaxAddedWithinDays))
		return false
	case err != nil:
		s.invalidRequest(w, fmt.Sprintf(
			"minDuration and maxDuration must be between 0 and %d, and minDuration must not exceed maxDuration.",
			domain.MaxDurationBoundSec))
		return false
	}
	if params.addedWithin != nil {
		query.AddedWithinDays = *params.addedWithin
	}
	query.MinDurationSec, query.MaxDurationSec = params.minDuration, params.maxDuration
	return true
}

// writeVideoPage は一覧1ページを応答に書く。各項目には再生位置・タグと、一覧に
// 出す所在の置かれたフォルダ（Video.folder）を載せる。roots が無ければ folder は
// 省く。page.MissingTagIDs があれば応答にもそのまま載せる
func (s *server) writeVideoPage(w http.ResponseWriter, r *http.Request, page domain.VideoPage, roots []domain.MediaFolder) {
	progress := s.progressFor(r.Context(), page.Items)
	tags := s.tagsFor(r.Context(), page.Items)

	audience := audienceFrom(r.Context())
	payload := gen.VideoPage{Items: make([]gen.Video, 0, len(page.Items)), Total: page.Total}
	for _, view := range s.presentVideos(r.Context(), page.Items) {
		video := view.Video
		item := withTags(withProgress(toAPIVideo(view), progress, video.UserKey), tags, video.UserKey)
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
// データに依る条件を指定したら 400 を書いて false を返す。
func (s *server) checkAudienceQuery(w http.ResponseWriter, audience domain.Audience, query domain.VideoQuery) bool {
	if err := audience.CheckVideoQuery(query); err != nil {
		s.invalidRequestReason(w, reasonGuestFilterNotAllowed, "Sign in to filter by watch status, tags, or favorites, or to sort by last played or date favorited.")
		return false
	}
	return true
}

// forAudience は応答に載せる動画を見る人に合わせる。ゲストには所在（絶対パス）・
// 再生位置・読み取りの誤りとそのコード（誤りは絶対パスを含みうる）を出さず、タグを空の配列にする。ファイル名由来の題名・表示名・代表サムネイルの
// 位置と適用した画像の有無も出さず、ゲストは title で表示名だけを受け取る。作品情報も出さない
//
// 所有者にはそのまま返す。
func forAudience(audience domain.Audience, video gen.Video) gen.Video {
	if audience.IsOwner() {
		return video
	}
	video.Location = nil
	video.Progress = nil
	video.ProbeError = nil
	video.ProbeErrorCode = nil
	video.Tags = []gen.VideoTag{}
	video.FileTitle = nil
	video.DisplayName = nil
	video.ThumbnailPositionMs = nil
	video.ThumbnailImage = nil
	video.ContentVersion = nil
	video.Favorite = nil
	video.WorkInfo = nil
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
		s.logger.Warn("could not read media folders", slog.Any("error", err))
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
// 400 を書いて false を返す。黙って既定へ戻さないのは、
// 画面の送り間違いを、条件と違う一覧として見せないためである。
func (s *server) parseListFilters(w http.ResponseWriter, params listFilterParams) (listFilters, bool) {
	out := listFilters{watch: domain.WatchAll, sort: domain.SortAddedDesc}
	if params.watch != nil {
		watch := domain.WatchFilter(*params.watch)
		if !watch.Valid() {
			s.invalidRequest(w, "Unknown watch filter.")
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
			s.invalidRequest(w, "Unknown sort order.")
			return listFilters{}, false
		}
		out.sort = sort
	}
	if params.seed != nil {
		seed := *params.seed
		if seed < 0 || seed > domain.MaxShuffleSeed {
			s.invalidRequest(w, fmt.Sprintf("seed must be between 0 and %d.", domain.MaxShuffleSeed))
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
		s.invalidRequestLimit(w, reasonSearchTooLong, maxQueryLength,
			fmt.Sprintf("Search text must be at most %d characters.", maxQueryLength))
		return "", false
	}
	return *query, true
}

// parseTagFilter は絞り込みに使うタグの id の個数を検査する。17 個以上は 400 を
// 書いて false を返す。存在しない id は誤りにせず、
// 保存層（existingTagIDs）が落として応答の missingTagIds に返す。
func (s *server) parseTagFilter(w http.ResponseWriter, tag *[]int64) ([]int64, bool) {
	if tag == nil {
		return nil, true
	}
	if len(*tag) > maxTagFilterCount {
		s.invalidRequestLimit(w, reasonTooManyTagFilters, maxTagFilterCount,
			fmt.Sprintf("Filter by at most %d tags.", maxTagFilterCount))
		return nil, false
	}
	return *tag, true
}

// GetVideo は動画1件の詳細を返す（GET /api/videos/{id}）。group は動画を開いたグループ、smartFolder
// は動画を開いたスマートフォルダで、応答の group か smartFolder をその文脈で答える
func (s *server) GetVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.GetVideoParams) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	// 作品情報は動画 1 件の応答にだけ載せる。一覧・関連動画・ライブラリの項目は読まない。
	video, err := s.workInfoForDetail(r.Context(), video)
	if err != nil {
		s.internalError(w, "Could not load the video.", err)
		return
	}
	// 追加の情報も動画 1 件の応答にだけ載せ、所有者とゲストに同じ値を返す
	//
	video, err = s.mediaInfoForDetail(r.Context(), video)
	if err != nil {
		s.internalError(w, "Could not load the video.", err)
		return
	}
	s.writeVideoDetailIn(w, r, video, videoContext(params.Group, params.SmartFolder))
}

// videoContext は問い合わせの group と smartFolder を動画を開いた文脈にする。省かれた方は 0 である。
func videoContext(group *gen.VideoGroupContext, smartFolder *gen.VideoSmartFolderContext) domain.VideoContext {
	var opened domain.VideoContext
	if group != nil {
		opened.GroupID = *group
	}
	if smartFolder != nil {
		opened.SmartFolderID = *smartFolder
	}
	return opened
}

// writeVideoDetail は動画1件の詳細を GET /api/videos/{id} の形で書く。表示名の設定の
// 応答も同じ形にする。group は
// 文脈の無い動画と同じく、id のいちばん小さいグループで答える。
func (s *server) writeVideoDetail(w http.ResponseWriter, r *http.Request, video domain.Video) {
	s.writeVideoDetailIn(w, r, video, domain.VideoContext{})
}

// writeVideoDetailIn は writeVideoDetail と同じ応答を、group か smartFolder を動画を開いた文脈 opened
// で答えて書く。
func (s *server) writeVideoDetailIn(w http.ResponseWriter, r *http.Request, video domain.Video, opened domain.VideoContext) {
	payload, err := s.videoDetail(r, video, opened)
	if err != nil {
		s.internalError(w, "Could not load the video.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// videoDetail は動画1件の詳細を GET /api/videos/{id} の形にする。要求の見る人に合わせて項目を絞り、
// group か smartFolder は動画を開いた文脈 opened で答える。作品情報の取得の結果の反映の応答も、
// これを video に入れる。
//
// グループかシーク用プレビューの状態を読めなければ、その項目を除いた詳細と誤りを返す。書き込みが確定した
// 後の応答は、誤りを記録して除いた詳細を返せる。
func (s *server) videoDetail(r *http.Request, video domain.Video, opened domain.VideoContext) (gen.Video, error) {
	var errs []error
	progress := s.progressFor(r.Context(), []domain.Video{video})
	tags := s.tagsFor(r.Context(), []domain.Video{video})
	payload := withTags(withProgress(s.apiVideo(r.Context(), video), progress, video.UserKey), tags, video.UserKey)

	// 所在とシーク用プレビューの状態は、動画1件の応答にだけ載せる。一覧に載せると、
	// 画面が使わない絶対パスを1ページ 60 件ぶん毎回送ることになる。所在は
	// forAudience がゲストの応答から外す。
	payload.Location = &gen.VideoLocation{Path: video.Path, Openable: s.canOpen(r)}
	payload.Folder = detailFolder(s.registeredRoots(r.Context()), video.Path)
	// 作品情報は、読んだ（または書いた）経路が video に埋めたときだけ載せる。
	payload.WorkInfo = toAPIWorkInfo(video.WorkInfo)
	// 追加の情報も、GET /api/videos/{id} が video に埋めたときだけ載せる。
	payload.MediaInfo = toAPIMediaInfo(video.MediaInfo)
	// グループも動画1件の応答にだけ載せる。文脈のグループの再生順で数え、ゲストには公開の
	// 動画だけで数えたものが返る。文脈がスマートフォルダなら
	// group の代わりに smartFolder をフォルダの再生順で載せる。
	if s.catalog != nil {
		// 位置と本数だけを使うので、メンバーの詳細は読まない。
		group, grouped, err := s.catalog.VideoGroup(r.Context(), audienceFrom(r.Context()), video, opened, domain.GroupWindow{})
		if err != nil {
			errs = append(errs, err)
		}
		if err == nil && grouped {
			payload.Group, payload.SmartFolder = videoContextRefs(group, video.ID)
		}
	}
	if video.HasSeekThumbnail() && s.catalog != nil {
		state, err := s.catalog.SeekThumbnailState(r.Context(), video)
		if err != nil {
			errs = append(errs, err)
		} else {
			apiState := gen.VideoSeekThumbnailState(state)
			payload.SeekThumbnailState = &apiState
		}
	}
	return forAudience(audienceFrom(r.Context()), payload), errors.Join(errs...)
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
// ゲストには再生位置を出さないので、読みもしない。
func (s *server) progressFor(ctx context.Context, videos []domain.Video) map[string]domain.Progress {
	if s.playback == nil || len(videos) == 0 || !audienceFrom(ctx).IsOwner() {
		return nil
	}

	keys := make([]string, 0, len(videos))
	for _, video := range videos {
		if video.UserKey != "" {
			keys = append(keys, video.UserKey)
		}
	}

	progress, err := s.playback.ProgressByContentKeys(ctx, keys)
	if err != nil {
		s.logger.Warn("could not read playback progress", slog.Any("error", err))
		return nil
	}
	return progress
}

// tagsFor は動画たちのタグをまとめて引く（progressFor と同じ形）。1件ずつ引くと、60 件の一覧で 60 回の問い合わせに
// なる。
//
// 引けなかった場合は一覧を諦めない。タグは「あると嬉しい」情報であって、
// 無いと動画を見渡せなくなるものではない。
//
// ゲストにはタグを出さないので、読みもしない。
func (s *server) tagsFor(ctx context.Context, videos []domain.Video) map[string][]domain.VideoTag {
	if s.tags == nil || len(videos) == 0 || !audienceFrom(ctx).IsOwner() {
		return nil
	}

	keys := make([]string, 0, len(videos))
	for _, video := range videos {
		if video.UserKey != "" {
			keys = append(keys, video.UserKey)
		}
	}

	tags, err := s.tags.TagsByContentKeys(ctx, keys)
	if err != nil {
		s.logger.Warn("could not read item tags", slog.Any("error", err))
		return nil
	}
	return tags
}

// withTags はタグを載せる。タグが1つも無い動画は空の配列にする（null にしない）。
func withTags(video gen.Video, tags map[string][]domain.VideoTag, contentKey string) gen.Video {
	refs := tags[contentKey]
	video.Tags = make([]gen.VideoTag, 0, len(refs))
	for _, ref := range refs {
		video.Tags = append(video.Tags, toAPIVideoTag(ref))
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
		s.internalError(w, "Library queries are not configured.", nil)
		return domain.Video{}, false
	}

	video, err := s.videos.GetVideo(r.Context(), audienceFrom(r.Context()), id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
		return domain.Video{}, false
	case err != nil:
		s.internalError(w, "Could not load the video.", err)
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
		UpdatedAt:      video.EditedAt,
		FileCreatedAt:  video.FileCreatedAt,
		Playable:       video.PlayableInBrowser(),
		ProbeState:     gen.VideoProbeState(video.ProbeState),
		ThumbnailState: gen.VideoThumbnailState(video.ThumbnailState),
		PreviewState:   gen.VideoPreviewState(video.PreviewState),
		Public:         video.Public,
		// 集まりの要約は、集まりのメンバーならどの応答の項目にも載せる。ゲストには公開のバージョン
		// だけで数えたものが返る。
		Versions: apiVersionsRef(video.Versions),
	}
	// お気に入りは所有者の応答にだけ入る。ゲストの応答からは forAudience が外す
	//
	favorite := video.Favorite
	out.Favorite = &favorite

	// 上書きの項目。title は有効な
	// 題名で、ファイル名由来の題名・表示名・代表サムネイルの位置は forAudience がゲストの
	// 応答から外す。
	if video.FileTitle != "" {
		fileTitle := video.FileTitle
		out.FileTitle = &fileTitle
	}
	if video.DisplayName != "" {
		displayName := video.DisplayName
		out.DisplayName = &displayName
	}
	if video.ThumbnailPositionMs != nil {
		position := *video.ThumbnailPositionMs
		out.ThumbnailPositionMs = &position
	}
	// 内容の版はアップロードに添えて送り返す値で、所有者の応答にだけ入る
	// （docs/design-docs/thumbnail-editing.md「Uploaded images」）。
	if video.ContentKey != "" {
		contentVersion := thumbnailVersion(video.ContentKey)
		out.ContentVersion = &contentVersion
	}
	// 適用した画像の出どころも所有者の応答にだけ入る。
	if video.ThumbnailImageRevision != 0 && video.ThumbnailImageOrigin != "" {
		origin := gen.VideoThumbnailImage(video.ThumbnailImageOrigin)
		out.ThumbnailImage = &origin
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
	// コードは failed の行だけに出す。アップグレード前の失敗にはコードが無い
	//
	if video.ProbeState == domain.ProbeStateFailed && video.ProbeErrorCode != "" {
		code := gen.ProbeErrorCode(video.ProbeErrorCode)
		out.ProbeErrorCode = &code
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
// 内容が変われば URL も変わる。所有者が画像を適用していれば、版に画像の改版番号を
// 足して `<内容鍵の先頭>-g<改版番号>` にする。
// 適用していなくて位置を指定していれば、位置の改版番号を足して `<内容鍵の先頭>-r<改版番号>`
// にし、位置を記録し直すたびに URL を変える。
// ゲストにも渡る URL なので、位置の値は入れない。
func thumbnailURL(video domain.Video) string {
	version := thumbnailVersion(video.ContentKey)
	switch {
	case video.ThumbnailImageRevision != 0:
		version += "-g" + strconv.FormatInt(video.ThumbnailImageRevision, 10)
	case video.ThumbnailPositionMs != nil:
		version += "-r" + strconv.FormatInt(video.ThumbnailRevision, 10)
	}
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/thumbnail?v=" + version
}

func seekThumbnailURL(video domain.Video) string {
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/seek-thumbnail?v=" +
		thumbnailVersion(video.ContentKey)
}

// seekThumbnailSheetURL はスプライトのシート sheet の取得先を組み立てる。版は
// 配置情報の URL（seekThumbnailURL）と同じく内容由来である。
func seekThumbnailSheetURL(video domain.Video, sheet int) string {
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/seek-thumbnail/" + strconv.Itoa(sheet) +
		"?v=" + thumbnailVersion(video.ContentKey)
}

func thumbnailVersion(contentKey string) string {
	if len(contentKey) > thumbnailVersionLength {
		return contentKey[:thumbnailVersionLength]
	}
	return contentKey
}
