package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type LibraryItems interface {
	ListLibrary(ctx context.Context, audience domain.Audience, q domain.VideoQuery) (domain.LibraryPage, error)
	// LibraryIDs shares list filters but ignores sorting, cursor, and limit, returning group playback members separately.
	LibraryIDs(ctx context.Context, q domain.VideoQuery) (selection domain.LibrarySelection, missingTagIDs []int64, err error)
	// Missing and invisible groups both return ErrNotFound.
	Group(ctx context.Context, audience domain.Audience, id int64, trail []int64) (domain.GroupPage, error)
	GroupItems(ctx context.Context, audience domain.Audience, id int64, cursor string, limit int) (domain.GroupPage, error)
	ListGroups(ctx context.Context, audience domain.Audience, query, cursor string, limit int) (domain.GroupListPage, error)
}

func (s *server) ListLibrary(w http.ResponseWriter, r *http.Request, params gen.ListLibraryParams) {
	if s.library == nil {
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
	// 棚に無い動画のみはライブラリの項目の一覧だけが受け、ゲストも使える（docs/design-docs/home.md）。
	query.Unshelved = params.Unshelved != nil && *params.Unshelved

	page, err := s.library.ListLibrary(r.Context(), audience, query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load the list.", err)
		return
	}

	items, err := s.presentLibraryItems(r.Context(), audience, page.Roots, page.Items)
	if err != nil {
		s.internalError(w, "Could not load the list.", err)
		return
	}
	payload := gen.LibraryPage{Items: items, Total: page.Total}
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

func (s *server) ListLibraryIds(w http.ResponseWriter, r *http.Request, params gen.ListLibraryIdsParams) {
	if s.library == nil {
		s.internalError(w, "Library queries are not configured.", nil)
		return
	}
	query, ok := s.parseIDsQuery(w, videoQueryParams{
		query: params.Query, watch: params.Watch, playable: params.Playable, tag: params.Tag,
		favorite: params.Favorite, person: params.Person, organization: params.Organization,
		addedWithin: params.AddedWithin, minDuration: params.MinDuration, maxDuration: params.MaxDuration,
		tagKind: params.TagKind,
	})
	if !ok {
		return
	}
	selection, missingTagIDs, err := s.library.LibraryIDs(r.Context(), query)
	if err != nil {
		s.internalError(w, "Could not load the ids.", err)
		return
	}
	s.writeVideoIDs(w, selection.AllVideoIDs(), missingTagIDs, libraryGroupIDs(selection.Groups), selection.SmartFolderIDs)
}

func libraryGroupIDs(groups []domain.LibraryGroupSelection) []gen.LibraryGroupIds {
	if len(groups) == 0 {
		return nil
	}
	out := make([]gen.LibraryGroupIds, 0, len(groups))
	for _, group := range groups {
		ids := group.VideoIDs
		if ids == nil {
			ids = []int64{}
		}
		out = append(out, gen.LibraryGroupIds{Id: group.ID, VideoIds: ids})
	}
	return out
}

func (s *server) presentLibraryItems(ctx context.Context, audience domain.Audience, roots []domain.MediaFolder, items []domain.LibraryItem) ([]gen.LibraryItem, error) {
	var shown, tagged []domain.Video
	for _, item := range items {
		if item.Video != nil {
			shown = append(shown, *item.Video)
			tagged = append(tagged, *item.Video)
		} else if item.Group != nil {
			tagged = append(tagged, item.Group.Members...)
		}
	}
	lookup := s.itemLookups(ctx, shown, tagged)
	out := make([]gen.LibraryItem, 0, len(items))
	for _, item := range items {
		if item.SmartFolder != nil {
			folder := toAPISmartFolderSummary(audience, *item.SmartFolder)
			out = append(out, gen.LibraryItem{Kind: gen.LibraryItemKindSmartFolder, SmartFolder: &folder})
			continue
		}
		if item.Video != nil {
			video := lookup.video(ctx, audience, roots, *item.Video)
			out = append(out, gen.LibraryItem{Kind: gen.LibraryItemKindVideo, Video: &video})
			continue
		}
		group, ok := lookup.group(ctx, audience, roots, *item.Group)
		if !ok {
			return nil, fmt.Errorf("group %d has no video", item.Group.ID)
		}
		out = append(out, gen.LibraryItem{Kind: gen.LibraryItemKindGroup, Group: &group})
	}
	return out, nil
}

type itemLookup struct {
	s        *server
	progress map[string]domain.Progress
	tags     map[string][]domain.VideoTag
}

// Skip progress/tag reads entirely for guests.
func (s *server) itemLookups(ctx context.Context, shown, tagged []domain.Video) itemLookup {
	return itemLookup{s: s, progress: s.progressFor(ctx, shown), tags: s.tagsFor(ctx, tagged)}
}

func (l itemLookup) video(ctx context.Context, audience domain.Audience, roots []domain.MediaFolder, video domain.Video) gen.Video {
	view := l.s.presentVideos(ctx, []domain.Video{video})[0]
	item := withTags(withProgress(toAPIVideo(view), l.progress, video.UserKey), l.tags, video.UserKey)
	if folder, ok := domain.LocateVideoFolder(roots, video.Path); ok {
		item.Folder = &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
	}
	return forAudience(audience, item)
}

// Guest groups omit watch data and tags; automatic folders are exposed only under registered roots.
func (l itemLookup) group(ctx context.Context, audience domain.Audience, roots []domain.MediaFolder, group domain.LibraryGroup) (gen.LibraryGroup, bool) {
	card, ok := l.card(ctx, audience, roots, group)
	if !ok {
		return gen.LibraryGroup{}, false
	}
	ids := make([]int64, 0, len(group.Members))
	for _, member := range group.Members {
		ids = append(ids, member.ID)
	}
	out := gen.LibraryGroup{
		Id: card.Id, Folder: card.Folder, Name: card.Name, VideoCount: card.VideoCount, WatchedCount: card.WatchedCount,
		WatchState: (*gen.LibraryGroupWatchState)(card.WatchState), DurationMs: card.DurationMs, SizeBytes: card.SizeBytes,
		AddedAt: card.AddedAt, LastPlayedAt: card.LastPlayedAt, Previews: card.Previews, OpenVideoId: card.OpenVideoId,
		Favorite: card.Favorite, VideoIds: ids, Tags: []gen.VideoTag{},
	}
	if audience.IsOwner() {
		for _, tag := range unionMemberTags(group.Members, l.tags) {
			out.Tags = append(out.Tags, toAPIVideoTag(tag))
		}
	}
	return out, true
}

// card はグループのカードの値を応答の形にする（ホームの棚の項目。docs/design-docs/home.md）。
// 見せる動画が無ければ false を返す。folder は自動グループのフォルダが登録フォルダ roots の下にあるときだけ
// 載せる（docs/design-docs/catalog-organization.md）。ゲストには視聴の値を載せない（data-model.md §7）。
func (l itemLookup) card(ctx context.Context, audience domain.Audience, roots []domain.MediaFolder, group domain.LibraryGroup) (gen.LibraryGroupCard, bool) {
	if group.VideoCount == 0 {
		return gen.LibraryGroupCard{}, false
	}
	out := gen.LibraryGroupCard{
		Id:          group.ID,
		Name:        group.Name,
		VideoCount:  group.VideoCount,
		DurationMs:  group.DurationMs,
		SizeBytes:   group.SizeBytes,
		AddedAt:     group.AddedAt,
		Previews:    l.s.folderPreviews(ctx, previewVideos(group.Previews)),
		OpenVideoId: group.OpenVideoID,
	}
	if group.Path != "" {
		if folder, ok := domain.LocateFolder(roots, group.Path); ok {
			out.Folder = &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
		}
	}
	if !audience.IsOwner() {
		return out, true
	}
	state := gen.LibraryGroupCardValuesWatchState(group.WatchState)
	watched := group.WatchedCount
	out.WatchState, out.WatchedCount = &state, &watched
	favorite := group.Favorite
	out.Favorite = &favorite
	if group.LastPlayedAt != nil {
		played := *group.LastPlayedAt
		out.LastPlayedAt = &played
	}
	return out, true
}

func unionMemberTags(members []domain.Video, tags map[string][]domain.VideoTag) []domain.VideoTag {
	index := map[int64]int{}
	var out []domain.VideoTag
	for _, member := range members {
		for _, tag := range tags[member.UserKey] {
			if i, ok := index[tag.ID]; ok {
				out[i].Manual = out[i].Manual || tag.Manual
				out[i].FromFolder = out[i].FromFolder || tag.FromFolder
				continue
			}
			index[tag.ID] = len(out)
			out = append(out, tag)
		}
	}
	domain.SortVideoTags(out)
	return out
}
