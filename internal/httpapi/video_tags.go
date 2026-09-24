package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 動画へのタグの付け外し・要約・「すべて選択」用の全件 id
// （specs/014-video-tags/contracts/tags-api.md §4・§5）。
//
// 付け外し・要約は internal/store.TagStore（s.tags）、全件の id は
// internal/store.LibraryStore（s.videos）を、httpapi が直接呼ぶ。internal/app は
// 通さない（Plan の Structural Decisions 14）。

// maxVideoTagsIDs は付け外し・要約が受け付ける videoIds の最大件数である。
// api/openapi.yaml の VideoTagsRequest/VideoTagsSummaryRequest の maxItems と
// 同じ値（contracts/tags-api.md §4）。
const maxVideoTagsIDs = 20000

// UpdateVideoTags は動画へタグを付ける・外す（POST /api/video-tags）。
func (s *server) UpdateVideoTags(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoTagsRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if !validVideoTagsIDs(body.VideoIds) {
		s.invalidRequest(w, "videoIdsは1件以上20000件以下で指定してください")
		return
	}
	tagID, tagName, ok := parseTagInput(body.Tag)
	if !ok {
		s.invalidRequest(w, "tagはidとnameのどちらか一方だけを指定してください")
		return
	}
	if s.tags == nil {
		s.internalError(w, "タグの経路が設定されていません", nil)
		return
	}

	var (
		ref     domain.TagRef
		applied int
		err     error
	)
	switch body.Action {
	case gen.Add:
		if tagID != nil {
			ref, applied, err = s.tags.AttachTagByID(r.Context(), body.VideoIds, *tagID)
		} else {
			ref, applied, err = s.tags.AttachTagByName(r.Context(), body.VideoIds, *tagName)
		}
	case gen.Remove:
		// 取り外しは id での指定だけを受け付ける。画面が外す候補はいつも付いている
		// タグで、id を持っているからである（contracts/tags-api.md §4）。
		if tagID == nil {
			s.invalidRequest(w, "取り外しはtag.idで指定してください")
			return
		}
		ref, applied, err = s.tags.DetachTag(r.Context(), body.VideoIds, *tagID)
	default:
		s.invalidRequest(w, "actionの値が不明です")
		return
	}
	if err != nil {
		s.writeVideoTagsError(w, err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.VideoTagsResponse{
		Tag:     gen.TagRef{Id: ref.ID, Name: ref.Name},
		Applied: applied,
	}, s.logger)
}

// SummarizeVideoTags は選んだ動画に付いたタグの要約を返す
// （POST /api/video-tags/summary）。
func (s *server) SummarizeVideoTags(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoTagsSummaryRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if !validVideoTagsIDs(body.VideoIds) {
		s.invalidRequest(w, "videoIdsは1件以上20000件以下で指定してください")
		return
	}
	if s.tags == nil {
		s.internalError(w, "タグの経路が設定されていません", nil)
		return
	}

	summary, err := s.tags.Summary(r.Context(), body.VideoIds)
	if err != nil {
		s.internalError(w, "タグの要約を取得できませんでした", err)
		return
	}

	items := make([]gen.VideoTagsSummaryItem, 0, len(summary.Items))
	for _, item := range summary.Items {
		items = append(items, gen.VideoTagsSummaryItem{
			Tag:   gen.TagRef{Id: item.Tag.ID, Name: item.Tag.Name},
			Count: item.Count,
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.VideoTagsSummary{Total: summary.Total, Items: items}, s.logger)
}

// ListVideoIds は絞り込みに合う動画の全件の id を返す（GET /api/videos/ids、
// 「すべて選択」用。contracts/tags-api.md §5）。パラメータは listVideos の
// query・watch・playable・tag と同じ。
func (s *server) ListVideoIds(w http.ResponseWriter, r *http.Request, params gen.ListVideoIdsParams) {
	if s.videos == nil {
		s.internalError(w, "一覧の問い合わせ先が設定されていません", nil)
		return
	}

	query := domain.VideoQuery{}
	filters, ok := s.parseListFilters(w, listFilterParams{watch: params.Watch, playable: params.Playable})
	if !ok {
		return
	}
	query.Watch, query.PlayableOnly = filters.watch, filters.playableOnly
	if query.Query, ok = s.parseSearchQuery(w, params.Query); !ok {
		return
	}
	if query.TagIDs, ok = s.parseTagFilter(w, params.Tag); !ok {
		return
	}

	ids, missingTagIDs, err := s.videos.VideoIDs(r.Context(), query)
	if err != nil {
		s.internalError(w, "idを取得できませんでした", err)
		return
	}

	payload := gen.VideoIdsResponse{Ids: ids}
	if len(missingTagIDs) > 0 {
		payload.MissingTagIds = &missingTagIDs
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// validVideoTagsIDs は videoIds が1件以上20000件以下かを確かめる
// （contracts/tags-api.md §4）。
func validVideoTagsIDs(videoIDs []int64) bool {
	return len(videoIDs) >= 1 && len(videoIDs) <= maxVideoTagsIDs
}

// parseTagInput は TagInput が id と name のちょうど一方を持つかを確かめる
// （contracts/tags-api.md §1）。どちらも無いか両方あれば ok = false を返す。
func parseTagInput(tag gen.TagInput) (id *int64, name *string, ok bool) {
	if (tag.Id == nil) == (tag.Name == nil) {
		return nil, nil, false
	}
	return tag.Id, tag.Name, true
}

// writeVideoTagsError は付け外しの保存層の誤りを応答へ写す。
func (s *server) writeVideoTagsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidTagName):
		s.invalidRequest(w, err.Error())
	case errors.Is(err, domain.ErrTagNotFound):
		s.writeError(w, http.StatusNotFound, codeTagNotFound, "タグが見つかりません")
	default:
		s.internalError(w, "タグを変更できませんでした", err)
	}
}
