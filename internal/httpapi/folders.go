package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Folders are derived from location paths, not persisted; reads receive the audience.
type Folders interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	FolderLocations(ctx context.Context, audience domain.Audience, dir string) ([]domain.FolderLocation, error)
	HasFolderLocations(ctx context.Context, audience domain.Audience, dir string) (bool, error)
	ListFolderVideos(ctx context.Context, audience domain.Audience, q domain.FolderVideoQuery) (domain.VideoPage, error)
}

const folderNotFoundMessage = "Folder not found."

func (s *server) ListRootFolders(w http.ResponseWriter, r *http.Request) {
	if s.folders == nil {
		s.internalError(w, "Folder queries are not configured.", nil)
		return
	}
	roots, err := s.folders.ListMediaFolders(r.Context())
	if err != nil {
		s.folderError(w, r, "Could not load folders.", err)
		return
	}

	audience := audienceFrom(r.Context())
	summaries := make([]domain.FolderSummary, 0, len(roots))
	for _, root := range roots {
		locations, err := s.folders.FolderLocations(r.Context(), audience, root.Path)
		if err != nil {
			s.folderError(w, r, "Could not load folders.", err)
			return
		}
		listing, _ := domain.SummarizeFolder(root, "", locations)
		if !audience.IsOwner() && !listingHasVideos(listing) {
			continue
		}
		summaries = append(summaries, listing.Folder)
	}
	domain.SortFolders(summaries)
	groupings, err := s.folderGroupings(r.Context(), audience, summaries)
	if err != nil {
		s.folderError(w, r, "Could not load folders.", err)
		return
	}

	folders := s.apiFolders(r.Context(), audience, summaries, groupings)
	if s.mergesLists(r.Context(), domain.VideoQuery{}) {
		folders = s.withPeerRoots(r, folders)
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.RootFolderListing{Folders: folders}, s.logger)
}

func (s *server) GetFolder(w http.ResponseWriter, r *http.Request, rootID gen.FolderRootId, params gen.GetFolderParams) {
	if domain.IsRemoteID(rootID) {
		s.forwardRemoteFolder(w, r, rootID, "", r.URL.Query(), rewriteAs(rewriteRemoteFolderListing))
		return
	}
	root, rel, ok := s.resolveFolderRoot(w, r, rootID, params.Path)
	if !ok {
		return
	}
	audience := audienceFrom(r.Context())
	locations, err := s.folders.FolderLocations(r.Context(), audience, domain.FolderDir(root.Path, rel))
	if err != nil {
		s.folderError(w, r, "Could not load folders.", err)
		return
	}
	listing, found := domain.SummarizeFolder(root, rel, locations)
	// Hide roots without public videos from guests to prevent enumerating registered root IDs.
	if !found || (!audience.IsOwner() && !listingHasVideos(listing)) {
		s.notFoundReason(w, reasonFolderNotFound, folderNotFoundMessage)
		return
	}

	// Read current and child-folder grouping from the same snapshot.
	groupings, err := s.folderGroupings(r.Context(), audience, append([]domain.FolderSummary{listing.Folder}, listing.Folders...))
	if err != nil {
		s.folderError(w, r, "Could not load folders.", err)
		return
	}
	var folderGrouping *gen.FolderGrouping
	var childGroupings []*gen.FolderGrouping
	if groupings != nil {
		folderGrouping, childGroupings = groupings[0], groupings[1:]
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.FolderListing{
		Folder:  s.apiFolder(r.Context(), audience, listing.Folder, folderGrouping),
		Folders: s.apiFolders(r.Context(), audience, listing.Folders, childGroupings),
	}, s.logger)
}

// Folder absence takes precedence over scope/query validation and always yields 404.
func (s *server) ListFolderVideos(w http.ResponseWriter, r *http.Request, rootID gen.FolderRootId, params gen.ListFolderVideosParams) {
	if domain.IsRemoteID(rootID) {
		s.listRemoteFolderVideos(w, r, rootID, params)
		return
	}
	roots, root, rel, ok := s.resolveFolderRootIn(w, r, rootID, params.Path)
	if !ok {
		return
	}

	query := domain.FolderVideoQuery{
		Dir: domain.FolderDir(root.Path, rel), Scope: domain.FolderScopeDirect,
	}
	audience := audienceFrom(r.Context())
	if rel != "" || !audience.IsOwner() {
		found, err := s.folders.HasFolderLocations(r.Context(), audience, query.Dir)
		if err != nil {
			s.folderError(w, r, "Could not load the folder's videos.", err)
			return
		}
		if !found {
			s.notFoundReason(w, reasonFolderNotFound, folderNotFoundMessage)
			return
		}
	}
	if params.Scope != nil {
		scope := domain.FolderScope(*params.Scope)
		if !scope.Valid() {
			s.invalidRequest(w, "Unknown search scope.")
			return
		}
		query.Scope = scope
	}
	filters, ok := s.parseVideoQuery(w, audience, folderVideoQueryParams(params))
	if !ok {
		return
	}
	query.VideoQuery = filters

	page, err := s.folders.ListFolderVideos(r.Context(), audience, query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reopen the folder.")
		return
	case err != nil:
		s.folderError(w, r, "Could not load the folder's videos.", err)
		return
	}

	s.writeVideoPage(w, r, page, roots)
}

func folderVideoQueryParams(params gen.ListFolderVideosParams) videoQueryParams {
	return videoQueryParams{
		query: params.Query, sort: params.Sort, seed: params.Seed, cursor: params.Cursor,
		limit: params.Limit, tag: params.Tag, favorite: params.Favorite, person: params.Person,
		organization: params.Organization, tagKind: params.TagKind, personRole: params.PersonRole,
		tagName: params.TagName, tagKindName: params.TagKindName, personName: params.PersonName,
		organizationName: params.OrganizationName,
	}
}

func (s *server) resolveFolderRoot(w http.ResponseWriter, r *http.Request, rootID int64, path *string) (domain.MediaFolder, string, bool) {
	_, root, rel, ok := s.resolveFolderRootIn(w, r, rootID, path)
	return root, rel, ok
}

func (s *server) resolveFolderRootIn(w http.ResponseWriter, r *http.Request, rootID int64, path *string) ([]domain.MediaFolder, domain.MediaFolder, string, bool) {
	if s.folders == nil {
		s.internalError(w, "Folder queries are not configured.", nil)
		return nil, domain.MediaFolder{}, "", false
	}
	rel := ""
	if path != nil {
		rel = *path
	}
	if err := domain.ValidateFolderPath(rel); err != nil {
		s.invalidRequestReason(w, reasonInvalidFolderPath,
			"Invalid folder path. Empty segments, leading or trailing /, ., and .. are not allowed.")
		return nil, domain.MediaFolder{}, "", false
	}

	roots, err := s.folders.ListMediaFolders(r.Context())
	if err != nil {
		s.folderError(w, r, "Could not load folders.", err)
		return nil, domain.MediaFolder{}, "", false
	}
	for _, root := range roots {
		if root.ID == rootID {
			return roots, root, rel, true
		}
	}
	s.notFoundReason(w, reasonFolderNotFound, folderNotFoundMessage)
	return nil, domain.MediaFolder{}, "", false
}

// Derived child folders always contain locations, so a direct video or child folder proves visible descendants exist.
func listingHasVideos(listing domain.FolderListing) bool {
	return listing.Folder.VideoCount > 0 || len(listing.Folders) > 0
}

func (s *server) apiFolders(ctx context.Context, audience domain.Audience, folders []domain.FolderSummary, groupings []*gen.FolderGrouping) []gen.FolderSummary {
	out := make([]gen.FolderSummary, 0, len(folders))
	for i, folder := range folders {
		var grouping *gen.FolderGrouping
		if groupings != nil {
			grouping = groupings[i]
		}
		out = append(out, s.apiFolder(ctx, audience, folder, grouping))
	}
	return out
}

// Preview presentation verifies current files and schedules missing-artifact regeneration, matching video listings.
func (s *server) folderPreviews(ctx context.Context, videos []domain.Video) []gen.FolderPreview {
	views := s.presentVideos(ctx, videos)
	previews := make([]gen.FolderPreview, 0, len(views))
	for _, view := range views {
		item := gen.FolderPreview{
			VideoId:      view.Video.ID,
			ThumbnailUrl: thumbnailURL(view.Video),
		}
		if view.Video.PreviewState == domain.PreviewStateDone && view.PreviewAvailable {
			url := previewURL(view.Video)
			item.PreviewUrl = &url
		}
		previews = append(previews, item)
	}
	return previews
}

func previewVideos(previews []domain.FolderPreview) []domain.Video {
	videos := make([]domain.Video, 0, len(previews))
	for _, preview := range previews {
		videos = append(videos, domain.Video{
			ID: preview.VideoID, ContentKey: preview.ContentKey, PreviewState: preview.PreviewState,
			ThumbnailPositionMs: preview.ThumbnailPositionMs, ThumbnailRevision: preview.ThumbnailRevision,
			ThumbnailImageRevision: preview.ThumbnailImageRevision, Builds: preview.Builds,
		})
	}
	return videos
}

// Guest folder responses omit absolute root paths and grouping settings.
func (s *server) apiFolder(ctx context.Context, audience domain.Audience, folder domain.FolderSummary, grouping *gen.FolderGrouping) gen.FolderSummary {
	previews := s.folderPreviews(ctx, previewVideos(folder.Previews))
	out := gen.FolderSummary{
		RootId:      folder.RootID,
		Path:        folder.Path,
		Name:        folder.Name,
		VideoCount:  folder.VideoCount,
		FolderCount: folder.FolderCount,
		Previews:    previews,
	}
	if audience.IsOwner() {
		rootPath := folder.RootPath
		out.RootPath = &rootPath
		out.Grouping = grouping
	}
	return out
}

// Canceled requests produce neither an error response nor a log entry because the client has left.
func (s *server) folderError(w http.ResponseWriter, r *http.Request, message string, err error) {
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	s.internalError(w, message, err)
}
