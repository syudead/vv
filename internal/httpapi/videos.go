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

// Keep the search-length limit aligned with OpenAPI maxLength.
const maxQueryLength = 100

// Keep the tag-filter limit aligned with OpenAPI maxItems.
const maxTagFilterCount = 16

// ListVideos uses cursor pagination and returns the total independently of the page.
func (s *server) ListVideos(w http.ResponseWriter, r *http.Request, params gen.ListVideosParams) {
	if s.videos == nil {
		s.internalError(w, "Library queries are not configured.", nil)
		return
	}

	audience := audienceFrom(r.Context())
	query, ok := s.parseVideoQuery(w, audience, videoQueryParams{
		query: params.Query, sort: params.Sort,
		seed: params.Seed, cursor: params.Cursor, limit: params.Limit, tag: params.Tag,
		favorite: params.Favorite, person: params.Person, organization: params.Organization,
		tagKind: params.TagKind, personRole: params.PersonRole,
	})
	if !ok {
		return
	}
	if params.Kind != nil {
		if !params.Kind.Valid() {
			s.invalidRequest(w, "kind must be all, file or virtual.")
			return
		}
		query.Kind = domain.VideoKind(*params.Kind)
	}

	page, err := s.videos.ListVideos(r.Context(), audience, query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		// Reject invalid cursors: restarting would make infinite scrolling repeat the first page.
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load the list.", err)
		return
	}

	s.writeVideoPage(w, r, page, s.registeredRoots(r.Context()))
}

// videoQueryParams is shared by listVideos and listLibrary.
type videoQueryParams struct {
	query        *string
	sort         *gen.VideoSort
	seed         *int64
	cursor       *string
	limit        *int
	tag          *[]int64
	favorite     *bool
	person       *int64
	organization *int64
	tagKind      *int64
	// personRole is accepted only by listVideos and requires person.
	personRole *gen.WorkPersonRole
}

// parseVideoQuery writes 400 and returns false for invalid or audience-restricted filters.
func (s *server) parseVideoQuery(w http.ResponseWriter, audience domain.Audience, params videoQueryParams) (domain.VideoQuery, bool) {
	query := domain.VideoQuery{Sort: domain.SortAddedDesc}

	filters, ok := s.parseListFilters(w, listFilterParams{sort: params.sort, seed: params.seed})
	if !ok {
		return domain.VideoQuery{}, false
	}
	query.Sort, query.Seed = filters.sort, filters.seed
	query.FavoriteOnly = params.favorite != nil && *params.favorite

	// Clamp the limit at the boundary so response metadata matches the query and schema maximum.
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
	if !s.checkAudienceQuery(w, audience, query) {
		return domain.VideoQuery{}, false
	}
	return query, true
}

// parseIDFilters accepts positive IDs without checking existence; unmatched IDs produce an empty list. Missing filters remain zero.
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
	if params.personRole != nil {
		role := domain.PersonRole(*params.personRole)
		if query.Person == 0 || !role.Valid() {
			s.invalidRequest(w, "personRole must be a known role and requires person.")
			return false
		}
		query.PersonRole = role
	}
	return true
}

// writeVideoPage includes progress, tags, and the selected location's folder when roots are available. Preserve MissingTagIDs in the response.
func (s *server) writeVideoPage(w http.ResponseWriter, r *http.Request, page domain.VideoPage, roots []domain.MediaFolder) {
	extras := s.extrasFor(r.Context(), page.Items)

	audience := audienceFrom(r.Context())
	payload := gen.VideoPage{Items: make([]gen.Video, 0, len(page.Items)), Total: page.Total}
	for _, view := range s.presentVideos(r.Context(), page.Items) {
		video := view.Video
		item := extras.item(view)
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

// checkAudienceQuery rejects guest filters that depend on owner data with 400.
func (s *server) checkAudienceQuery(w http.ResponseWriter, audience domain.Audience, query domain.VideoQuery) bool {
	if err := audience.CheckVideoQuery(query); err != nil {
		s.invalidRequestReason(w, reasonGuestFilterNotAllowed, "Sign in to filter by tags, people, or favorites, or to sort by last played or date favorited.")
		return false
	}
	return true
}

// forAudience removes owner-only data from guest responses, including absolute paths, errors that may contain paths, progress, tags, overrides, and work information. Guests receive the display name through title.
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
	video.WorkInfo = guestWorkInfo(video.WorkInfo)
	video.CardCredits = nil
	return video
}

// Guests read a video's description and links; nil when the video has neither.
func guestWorkInfo(info *gen.WorkInfo) *gen.WorkInfo {
	if info == nil || (info.Description == nil && len(info.Urls) == 0) {
		return nil
	}
	return &gen.WorkInfo{
		Description: info.Description, Urls: info.Urls,
		Dates: []gen.WorkDate{}, People: []gen.WorkPerson{}, Organizations: []gen.WorkOrganization{},
		ExternalIds: []gen.ExternalId{},
	}
}

// If registered roots cannot be read, omit folder metadata rather than fail the video list.
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

type listFilterParams struct {
	sort *gen.VideoSort
	seed *int64
}

type listFilters struct {
	sort domain.VideoSort
	seed int64
}

// Reject unknown sorts and invalid seeds instead of silently returning a list in a different order.
func (s *server) parseListFilters(w http.ResponseWriter, params listFilterParams) (listFilters, bool) {
	out := listFilters{sort: domain.SortAddedDesc}
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

// parseTagFilter enforces the ID limit; the store removes nonexistent IDs and returns them in missingTagIds.
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

// GetVideo uses group or smartFolder as the context in which the video was opened.
func (s *server) GetVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.GetVideoParams) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	// Only video details load work information; lists and related videos do not.
	video, err := s.workInfoForDetail(r.Context(), video)
	if err != nil {
		s.internalError(w, "Could not load the video.", err)
		return
	}
	// Additional media information is detail-only and identical for owners and guests.
	video, err = s.mediaInfoForDetail(r.Context(), video)
	if err != nil {
		s.internalError(w, "Could not load the video.", err)
		return
	}
	s.writeVideoDetailIn(w, r, video, videoContext(params.Group, params.SmartFolder))
}

// videoContext uses zero for omitted group and smart-folder IDs.
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

// writeVideoDetail uses the GET detail shape; without context, choose the group with the smallest ID.
func (s *server) writeVideoDetail(w http.ResponseWriter, r *http.Request, video domain.Video) {
	s.writeVideoDetailIn(w, r, video, domain.VideoContext{})
}

// writeVideoDetailIn uses the group or smart-folder context in opened.
func (s *server) writeVideoDetailIn(w http.ResponseWriter, r *http.Request, video domain.Video, opened domain.VideoContext) {
	payload, err := s.videoDetail(r, video, opened)
	if err != nil {
		s.internalError(w, "Could not load the video.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// videoDetail filters fields by audience and uses opened for context. If group or seek-preview lookup fails, return partial details plus the error so committed mutations can still respond successfully.
func (s *server) videoDetail(r *http.Request, video domain.Video, opened domain.VideoContext) (gen.Video, error) {
	var errs []error
	progress := s.progressFor(r.Context(), []domain.Video{video})
	tags := s.tagsFor(r.Context(), []domain.Video{video})
	payload := withTags(withProgress(s.apiVideo(r.Context(), video), progress, video.UserKey), tags, video.UserKey)

	// Locations and seek-preview state are detail-only to avoid sending unused paths for every list item. forAudience removes locations for guests.
	if !video.Virtual() {
		payload.Location = &gen.VideoLocation{Path: video.Path, Openable: s.canOpen(r)}
		payload.Folder = detailFolder(s.registeredRoots(r.Context()), video.Path)
		// Include additional media information only when GetVideo populated it.
		payload.MediaInfo = toAPIMediaInfo(video.MediaInfo)
	}
	// Include work information only when the caller loaded or saved it into video.
	payload.WorkInfo = toAPIWorkInfo(video.WorkInfo)
	// Lists replace a changed item with this detail, so it carries the card's credits too.
	if video.WorkInfo != nil {
		payload.CardCredits = toAPICardCredits(*video.WorkInfo)
	}
	// Detail context uses playback order and audience-visible counts; smart-folder context replaces group context.
	if s.catalog != nil {
		// Only position and count are needed; avoid loading member details.
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

// detailFolder includes the registered root's display name for breadcrumbs; return nil when the location lies outside all registered roots.
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

// progressFor batches reads to avoid per-video queries. Skip guests and tolerate read failures so the list remains available.
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

// tagsFor batches reads to avoid per-video queries. Skip guests and tolerate read failures so the list remains available.
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

// Use an empty tags array, never null.
func withTags(video gen.Video, tags map[string][]domain.VideoTag, contentKey string) gen.Video {
	refs := tags[contentKey]
	video.Tags = make([]gen.VideoTag, 0, len(refs))
	for _, ref := range refs {
		video.Tags = append(video.Tags, toAPIVideoTag(ref))
	}
	return video
}

// Omit absent progress to distinguish never-watched videos from videos rewound to zero.
func withProgress(video gen.Video, progress map[string]domain.Progress, contentKey string) gen.Video {
	found, ok := progress[contentKey]
	if !ok {
		return video
	}
	payload := toAPIProgress(found)
	video.Progress = &payload
	return video
}

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

// Omit unavailable values so unknown duration remains distinct from zero duration.
func toAPIVideo(view domain.VideoView) gen.Video {
	video := view.Video
	out := gen.Video{
		Id:             video.ID,
		Virtual:        video.Virtual(),
		Title:          video.Title,
		SizeBytes:      video.SizeBytes,
		AddedAt:        video.AddedAt,
		UpdatedAt:      video.EditedAt,
		FileCreatedAt:  video.FileCreatedAt,
		FileModifiedAt: video.MTime,
		Playable:       video.PlayableInBrowser(),
		ProbeState:     gen.VideoProbeState(video.ProbeState),
		ThumbnailState: gen.VideoThumbnailState(video.ThumbnailState),
		PreviewState:   gen.VideoPreviewState(video.PreviewState),
		Public:         video.Public,
		// Include bundle summaries for all members, counting only public versions for guests.
		Versions: apiVersionsRef(video.Versions),
	}
	favorite := video.Favorite
	out.Favorite = &favorite

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
	out.ContentVersion = contentVersionOf(video)
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
	// Only failed rows expose error codes; failures predating the upgrade may lack a code.
	if video.ProbeState == domain.ProbeStateFailed && video.ProbeErrorCode != "" {
		code := gen.ProbeErrorCode(video.ProbeErrorCode)
		out.ProbeErrorCode = &code
	}
	// Expose a thumbnail URL only after generation; clients render placeholders otherwise.
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
	if video.Virtual() {
		return withoutFileFields(out)
	}
	return out
}

// Content version is owner-only and is echoed with image uploads; a virtual video's is its whole user key, which
// its saves and deletion check (docs/design-docs/catalog-organization.md).
func contentVersionOf(video domain.Video) *string {
	if video.ContentKey == "" {
		return nil
	}
	version := domain.ContentVersion(video.ContentKey)
	return &version
}

// A virtual video has no file, so nothing a file supplies is sent and it is never playable.
func withoutFileFields(video gen.Video) gen.Video {
	video.FileTitle, video.DurationMs, video.Width, video.Height, video.DisplayAspectRatio = nil, nil, nil, nil, nil
	video.Container, video.VideoCodec, video.AudioCodec, video.UnplayableReason = nil, nil, nil, nil
	video.PreviewUrl, video.SeekThumbnailUrl, video.Versions = nil, nil, nil
	video.Playable = false
	return video
}

// VideoCatalog schedules missing previews for regeneration. Without a catalog, treat preview files as absent.
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

func (s *server) apiVideo(ctx context.Context, video domain.Video) gen.Video {
	return toAPIVideo(s.presentVideos(ctx, []domain.Video{video})[0])
}

func previewURL(video domain.Video) string {
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/preview?v=" +
		withBuild(video.ContentKey, video.Builds.Preview)
}

// Version thumbnail URLs by content-key prefix plus image generation (-g) or position revision (-r), and the forced
// rebuild count (-b). Never include the position itself because guests receive these URLs.
func thumbnailURL(video domain.Video) string {
	version := thumbnailVersion(video.ContentKey)
	switch {
	case video.ThumbnailImageRevision != 0:
		version += "-g" + strconv.FormatInt(video.ThumbnailImageRevision, 10)
	case video.ThumbnailPositionMs != nil:
		version += "-r" + strconv.FormatInt(video.ThumbnailRevision, 10)
	}
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/thumbnail?v=" +
		withBuild(version, video.Builds.Thumbnail)
}

func seekThumbnailURL(video domain.Video) string {
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/seek-thumbnail?v=" + seekThumbnailVersion(video)
}

// Sprite sheets use the same version as the seek-thumbnail manifest.
func seekThumbnailSheetURL(video domain.Video, sheet int) string {
	return "/api/videos/" + strconv.FormatInt(video.ID, 10) + "/seek-thumbnail/" + strconv.Itoa(sheet) +
		"?v=" + seekThumbnailVersion(video)
}

func seekThumbnailVersion(video domain.Video) string {
	return withBuild(thumbnailVersion(video.ContentKey), video.Builds.SeekThumbnail)
}

// withBuild appends a forced rebuild count, so a screen loads the rebuilt file; a never-rebuilt artifact keeps its URL.
func withBuild(version string, build int64) string {
	if build <= 0 {
		return version
	}
	return version + "-b" + strconv.FormatInt(build, 10)
}

// Use a content-key prefix as the thumbnail URL version for safe long-lived caching.
func thumbnailVersion(contentKey string) string {
	if len(contentKey) > domain.ContentVersionLength {
		return contentKey[:domain.ContentVersionLength]
	}
	return contentKey
}

// Collection reads keep their saved order until a temporary sort is explicitly supplied.
func (s *server) parseCollectionQuery(w http.ResponseWriter, audience domain.Audience, params videoQueryParams) (domain.VideoQuery, bool) {
	query, ok := s.parseVideoQuery(w, audience, params)
	if params.sort == nil {
		query.Sort = ""
	}
	return query, ok
}
