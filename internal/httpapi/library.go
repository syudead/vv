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
	Group(ctx context.Context, audience domain.Audience, id int64, trail []int64, query string) (domain.GroupPage, error)
	GroupItems(ctx context.Context, audience domain.Audience, id int64, cursor string, limit int, query string) (domain.GroupPage, error)
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

	page, err := s.library.ListLibrary(r.Context(), audience, query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load the list.", err)
		return
	}

	// Use roots from the item read snapshot so concurrent unregistering cannot remove groups already counted by totals/cursors.
	roots := page.Roots
	// Fetch progress only for video items to avoid argument limits; group watch state is precomputed, but tag unions require all members.
	var shown, tagged []domain.Video
	for _, item := range page.Items {
		if item.Video != nil {
			shown = append(shown, *item.Video)
			tagged = append(tagged, *item.Video)
		} else if item.Group != nil {
			tagged = append(tagged, item.Group.Members...)
		}
	}
	lookup := s.itemLookups(r.Context(), shown, tagged)

	payload := gen.LibraryPage{Items: make([]gen.LibraryItem, 0, len(page.Items)), Total: page.Total}
	for _, item := range page.Items {
		if item.SmartFolder != nil {
			// Smart-folder items contain summaries; cards are fetched separately.
			folder := toAPISmartFolderSummary(audience, *item.SmartFolder)
			payload.Items = append(payload.Items, gen.LibraryItem{Kind: gen.LibraryItemKindSmartFolder, SmartFolder: &folder})
			continue
		}
		if item.Video != nil {
			video := lookup.video(r.Context(), audience, roots, *item.Video)
			payload.Items = append(payload.Items, gen.LibraryItem{Kind: gen.LibraryItemKindVideo, Video: &video})
			continue
		}
		group, ok := lookup.group(r.Context(), audience, roots, *item.Group)
		if !ok {
			// Fail on groups without visible videos instead of silently dropping items and invalidating totals/cursors.
			s.internalError(w, "Could not load the list.", fmt.Errorf("group %d has no video", item.Group.ID))
			return
		}
		payload.Items = append(payload.Items, gen.LibraryItem{Kind: gen.LibraryItemKindGroup, Group: &group})
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
	if len(group.Members) == 0 {
		return gen.LibraryGroup{}, false
	}
	ids := make([]int64, 0, len(group.Members))
	for _, member := range group.Members {
		ids = append(ids, member.ID)
	}
	out := gen.LibraryGroup{
		Id:          group.ID,
		Name:        group.Name,
		VideoCount:  len(group.Members),
		DurationMs:  group.DurationMs,
		SizeBytes:   group.SizeBytes,
		AddedAt:     group.AddedAt,
		Previews:    l.s.folderPreviews(ctx, groupPreviewMembers(group.Members)),
		OpenVideoId: group.OpenVideoID,
		VideoIds:    ids,
		Tags:        []gen.VideoTag{},
	}
	if group.Path != "" {
		if folder, ok := domain.LocateFolder(roots, group.Path); ok {
			out.Folder = &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
		}
	}
	if !audience.IsOwner() {
		return out, true
	}
	state := gen.LibraryGroupWatchState(group.WatchState)
	watched := group.WatchedCount
	out.WatchState, out.WatchedCount = &state, &watched
	favorite := group.Favorite
	out.Favorite = &favorite
	if group.LastPlayedAt != nil {
		played := *group.LastPlayedAt
		out.LastPlayedAt = &played
	}
	for _, tag := range unionMemberTags(group.Members, l.tags) {
		out.Tags = append(out.Tags, toAPIVideoTag(tag))
	}
	return out, true
}

func groupPreviewMembers(members []domain.Video) []domain.Video {
	out := make([]domain.Video, 0, domain.MaxFolderPreviews)
	for _, member := range members {
		if len(out) == domain.MaxFolderPreviews {
			break
		}
		if member.HasThumbnail() {
			out = append(out, member)
		}
	}
	return out
}

// unionMemberTags merges provenance flags with OR across members.
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
