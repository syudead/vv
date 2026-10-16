package httpapi

import (
	"context"
	"net/http"
	"slices"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetRelatedVideos は関連動画を返す（GET /api/videos/{id}/related）。
//
// 並べ方と問い合わせの流れはアプリケーション層（VideoCatalog）が持つ。ここは
// ページの指定を読み、動画を引き、並んだ結果を契約の形へ写すだけである。グループの
// メンバーなら、最初のページにこの動画を中ほどに置いたグループのメンバーの窓も関連動画と
// 同じ形で載せる。
func (s *server) GetRelatedVideos(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.GetRelatedVideosParams) {
	opened := videoContext(params.Group, params.SmartFolder)
	page := domain.RelatedPage{Limit: domain.DefaultRelatedPageSize, GroupID: opened.GroupID, SmartFolderID: opened.SmartFolderID}
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > domain.MaxRelatedPageSize {
			s.invalidRequest(w, "limit must be between 1 and 100.")
			return
		}
		page.Limit = *params.Limit
	}
	if params.Cursor != nil {
		cursor, err := domain.ParseRelatedCursor(*params.Cursor)
		if err != nil {
			s.invalidRequest(w, "cursor is not a value returned as nextCursor.")
			return
		}
		page.After = &cursor
	}
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if s.catalog == nil {
		s.internalError(w, "Related video queries are not configured.", nil)
		return
	}
	audience := audienceFrom(r.Context())

	related, err := s.catalog.RelatedVideos(r.Context(), audience, video, page)
	if err != nil {
		s.internalError(w, "Could not load related videos.", err)
		return
	}

	// グループのメンバーにも関連動画と同じく再生位置とタグを載せるので、まとめて引く。
	shown := related.Items
	if related.Group != nil {
		shown = append(slices.Clone(related.Items), related.Group.Members...)
	}
	present := s.presenter(r.Context(), audience, shown)
	payload := gen.RelatedVideos{Items: present(r.Context(), related.Items)}
	if group := related.Group; group != nil {
		payload.Group = &gen.RelatedGroup{
			Id:     group.ID,
			Name:   group.Name,
			Items:  present(r.Context(), group.Members),
			Offset: group.Offset,
			Total:  group.Total(),
		}
	}
	if related.NextCursor != nil {
		cursor := related.NextCursor.String()
		payload.NextCursor = &cursor
	}
	if related.NextID != 0 {
		next := related.NextID
		payload.NextId = &next
	}
	if related.PrevID != 0 {
		prev := related.PrevID
		payload.PrevId = &prev
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// ListVideoGroupMembers は動画の文脈のグループの動画を、再生順の範囲で返す
// （GET /api/videos/{id}/group-members、specs/048-groups/research.md R-9）。関連動画の group の
// 窓の外を、画面がスクロールに合わせて読む。
func (s *server) ListVideoGroupMembers(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.ListVideoGroupMembersParams) {
	window := domain.GroupWindow{Limit: domain.GroupMemberWindow}
	if params.Offset != nil {
		if *params.Offset < 0 {
			s.invalidRequest(w, "offset must be 0 or more.")
			return
		}
		window.Offset = *params.Offset
	}
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > domain.MaxGroupMemberPage {
			s.invalidRequest(w, "limit must be between 1 and 200.")
			return
		}
		window.Limit = *params.Limit
	}
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if s.catalog == nil {
		s.internalError(w, "Related video queries are not configured.", nil)
		return
	}
	audience := audienceFrom(r.Context())
	group, grouped, err := s.catalog.VideoGroup(r.Context(), audience, video, videoContext(params.Group, params.SmartFolder), window)
	if err != nil {
		s.internalError(w, "Could not load the group members.", err)
		return
	}
	if !grouped {
		s.notFound(w, "The video is not in a group.")
		return
	}
	present := s.presenter(r.Context(), audience, group.Members)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.GroupMemberPage{
		Items:  present(r.Context(), group.Members),
		Offset: group.Offset,
		Total:  group.Total(),
	}, s.logger)
}

// presenter は動画たちを応答の形にする関数を返す。再生位置とタグは shown の分をまとめて
// 引いておく。
func (s *server) presenter(ctx context.Context, audience domain.Audience, shown []domain.Video) func(context.Context, []domain.Video) []gen.Video {
	progress := s.progressFor(ctx, shown)
	tags := s.tagsFor(ctx, shown)
	return func(ctx context.Context, videos []domain.Video) []gen.Video {
		out := make([]gen.Video, 0, len(videos))
		for _, view := range s.presentVideos(ctx, videos) {
			item := withTags(withProgress(toAPIVideo(view), progress, view.Video.UserKey), tags, view.Video.UserKey)
			out = append(out, forAudience(audience, item))
		}
		return out
	}
}

// videoContextRefs は動画 videoID の文脈 group を、動画の応答の group か smartFolder のどちらか一方に
// する。文脈がスマートフォルダなら smartFolder、グループなら group である。
func videoContextRefs(group domain.VideoGroup, videoID int64) (*gen.VideoGroupRef, *gen.VideoSmartFolderRef) {
	position, count := group.Position(videoID), group.Total()
	if group.SmartFolder {
		return nil, &gen.VideoSmartFolderRef{Id: group.ID, Name: group.Name, Position: position, Count: count}
	}
	return &gen.VideoGroupRef{
		Id: group.ID, Folder: apiGroupFolder(group.Folder), Name: group.Name, Position: position, Count: count,
	}, nil
}

// apiGroupFolder は文脈のグループのフォルダを応答の形にする。自動グループのフォルダが登録
// フォルダの下に無いときと、手で作ったグループでは nil である。
func apiGroupFolder(folder *domain.VideoFolder) *gen.VideoFolder {
	if folder == nil {
		return nil
	}
	return &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
}
