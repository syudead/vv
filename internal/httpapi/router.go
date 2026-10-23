package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const contentTypeJSON = "application/json; charset=utf-8"

type Pinger interface {
	Ping(context.Context) error
}

// Library reads honor the audience from the auth boundary; MediaFiles, not HTTP handlers, validates
// opened/resolved locations against registered roots.
type Library interface {
	ListVideos(ctx context.Context, audience domain.Audience, q domain.VideoQuery) (domain.VideoPage, error)
	GetVideo(ctx context.Context, audience domain.Audience, id int64) (domain.Video, error)
	VideoLocations(ctx context.Context, videoID int64) ([]domain.VideoLocation, error)
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	// TranscodeProbe returns raw persisted data or nil; handlers validate reuse with
	// domain.TranscodeProbeUsable.
	TranscodeProbe(ctx context.Context, videoID int64) (*domain.StoredTranscodeProbe, error)
	KeyframeSpacing(ctx context.Context, videoID int64) (domain.KeyframeProbe, error)
}

// Playback survives deleted video rows through content keys and writes history atomically when
// supplied, deduplicating PlaybackID.
type Playback interface {
	SaveProgress(
		ctx context.Context, contentKey string, progress domain.Progress, play *domain.Play,
	) (domain.Progress, error)
	ProgressByContentKeys(ctx context.Context, contentKeys []string) (map[string]domain.Progress, error)
}

// StartScan returns the current scan without restarting it; started identifies transactional
// creation.
type Scans interface {
	StartScan(ctx context.Context) (scan domain.Scan, started bool, err error)
	// StartRescan returns ErrScanRunning while a manual scan or force rescan runs, ErrNoMediaFolders without
	// folders and ErrNotFound for a missing folder or video.
	StartRescan(ctx context.Context, scope domain.RescanScope) (domain.Scan, error)
	CurrentScan(ctx context.Context) (domain.Scan, error)
	// ListScanIssues returns ErrNotFound before any scan and ErrInvalidCursor for malformed cursors.
	ListScanIssues(ctx context.Context, cursor string, limit int) (domain.ScanIssuePage, error)
}

type MediaFolders interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	AddMediaFolder(ctx context.Context, path string) (domain.MediaFolder, error)
	ReplaceMediaFolder(ctx context.Context, id, expectedVersion int64, path string) (domain.MediaFolder, error)
	DeleteMediaFolder(ctx context.Context, id, expectedVersion int64) error
}

// Tag operations each complete in one store transaction without an app use case.
type Tags interface {
	ListTags(ctx context.Context, query domain.TagListQuery) (domain.TagPage, error)
	CreateTag(ctx context.Context, name string) (domain.Tag, error)
	RenameTag(ctx context.Context, id int64, name string) (domain.Tag, error)
	DeleteTag(ctx context.Context, id int64) error
	MergeTags(ctx context.Context, targetID int64, sourceIDs []int64) (domain.TagMergeOutcome, error)
	AddSynonym(ctx context.Context, tagID int64, name string, mergeTagID *int64) (domain.Tag, error)

	RemoveSynonym(ctx context.Context, tagID int64, name string) (domain.Tag, error)

	AttachTagByID(ctx context.Context, videoIDs []int64, tagID int64) (domain.TagRef, int, error)
	AttachTagByName(ctx context.Context, videoIDs []int64, name string) (domain.TagRef, int, error)
	DetachTag(ctx context.Context, videoIDs []int64, tagID int64) (domain.TagRef, int, error)
	Summary(ctx context.Context, videoIDs []int64) (domain.TagSummary, error)
	// TagsByContentKeys uses Video.UserKey for shared bundle data.
	TagsByContentKeys(ctx context.Context, contentKeys []string) (map[string][]domain.VideoTag, error)
	// ApplyVideoTags atomically applies named tags through the external API, returning
	// VideoRefNotFoundError for missing videos or TagNameAtError for invalid names. With tentative
	// set, create unknown names as tentative tags and skip rejected names.
	ApplyVideoTags(ctx context.Context, videos []domain.VideoRef, action domain.VideoTagsAction, names []string, tentative bool) (domain.VideoTagsOutcome, error)

	ConfirmTag(ctx context.Context, id int64) (domain.Tag, error)
	RejectTag(ctx context.Context, id int64) (string, error)
	ListRejectedTagNames(ctx context.Context, cursor string, limit int) (domain.RejectedTagNamePage, error)

	ForgetRejectedTagName(ctx context.Context, name string) (bool, error)

	// BatchTags rejects nonexistent kind IDs without writes.
	BatchTags(ctx context.Context, req domain.TagBatchRequest, ids []int64) (domain.TagBatchOutcome, error)
	TagImpact(ctx context.Context, action domain.TagImpactAction, ids []int64) (domain.TagImpact, error)

	ListTagKinds(ctx context.Context) ([]domain.TagKind, error)
	CreateTagKind(ctx context.Context, name string) (domain.TagKind, error)
	RenameTagKind(ctx context.Context, id int64, name string) (domain.TagKind, error)
	SetTagKindCardFirst(ctx context.Context, id int64, cardFirst bool) (domain.TagKind, error)
	DeleteTagKind(ctx context.Context, id int64) error
}

// Transcoder produces fragmented MP4 per request; Start returns with initial data and probes into
// Probed only when request.Probe is nil. StartupDeadline covers both probing and initial output;
// call Wait exactly once after successful Start and Stop on disconnection.
type Transcoder interface {
	Start(ctx context.Context, request domain.LiveTranscodeRequest) (domain.LiveTranscode, error)
}

// TranscodeSettings selects live encoding modes and rejects unavailable modes; implemented by
// app.TranscodeSettings.
type TranscodeSettings interface {
	Current() domain.TranscodeEncoding
	// Selecting unavailable hardware leaves saved settings unchanged.
	Select(ctx context.Context, choice domain.EncoderChoice) (domain.TranscodeEncoding, error)
}

// NetworkSettings controls Windows desktop LAN access; app.NetworkSettings owns listener
// replacement and persistence ordering.
type NetworkSettings interface {
	Current() domain.NetworkSettings
	// Listener reopen failures wrap ErrListenFailed and preserve saved settings.
	SetLANAccess(ctx context.Context, allowed bool) (domain.NetworkSettings, error)
}

// PluginProxySettings supplies the work-info plugin proxy URL; implemented by
// app.WorkInfoPluginProxy.
type PluginProxySettings interface {
	URL(ctx context.Context) (string, error)

	SetURL(ctx context.Context, raw string) (string, error)
}

// AutoImportSettings supplies auto-import preferences and watcher state; implemented by
// app.AutoImport.
type AutoImportSettings interface {
	Status() domain.AutoImportStatus
	// SetEnabled changes watches without starting scans and is a no-op for the current value.
	SetEnabled(ctx context.Context, enabled bool) (domain.AutoImportStatus, error)
}

type VersionJudging interface {
	Enabled(ctx context.Context) (bool, error)

	SaveEnabled(ctx context.Context, enabled bool) error
}

// VersionJudgeReview is app.VersionJudgeReview.
type VersionJudgeReview interface {
	Rejudge(ctx context.Context, videoIDs [2]int64) error
	// JudgementFrame returns a JPEG; a missing entry, frame or seek preview returns ErrNotFound.
	JudgementFrame(ctx context.Context, entryID int64, index int) ([]byte, error)
}

type TranscodeProbeWriter interface {
	SaveTranscodeProbe(ctx context.Context, videoID int64, source domain.FileStamp, probe domain.TranscodeProbe) error
}

// Artifacts reports absent, incomplete, or in-progress files as fs.ErrNotExist.
type ArtifactReader interface {
	// The caller closes the thumbnail file.
	ThumbnailFile(contentKey string) (*os.File, error)
	// The caller closes the poster file.
	PosterFile(contentKey string) (*os.File, error)
	// PreviewFile returns the stored SHA-256 digest with the file; the caller closes it.
	PreviewFile(contentKey string) (*os.File, string, error)
	// Invalid sprite metadata returns an error distinct from fs.ErrNotExist.
	SeekSprite(contentKey string) (domain.SeekSprite, error)
	// Sheet indices are zero-based.
	SeekSpriteSheet(contentKey string, sheet int) ([]byte, error)
}

// VideoCatalog owns artifact rebuild scheduling, derived seek-preview state, and related-video
// assembly; handlers only parse and present.
type VideoCatalog interface {
	// PresentVideos preserves input order.
	PresentVideos(ctx context.Context, videos []domain.Video) []domain.VideoView
	SeekThumbnailState(ctx context.Context, video domain.Video) (domain.SeekThumbnailState, error)
	RelatedVideos(ctx context.Context, audience domain.Audience, video domain.Video, page domain.RelatedPage) (domain.RelatedVideos, error)
	// VideoGroup returns visible playback order and reads detail only inside window; false means no
	// group context.
	VideoGroup(ctx context.Context, audience domain.Audience, video domain.Video, opened domain.VideoContext, window domain.GroupWindow) (domain.VideoGroup, bool, error)

	RetryProbe(ctx context.Context, video domain.Video) error
}

// MediaFiles validates regular files and both original/resolved paths within registered roots.
type MediaFiles interface {
	OpenMediaFile(roots []string, path string) (*os.File, os.FileInfo, error)

	ResolveMediaFile(roots []string, path string) (string, error)

	ListDirectories(path string) (domain.DirectoryListing, error)

	DirectoryRoots() domain.DirectoryListing
	// ListSidecarFiles applies media-opening rules first, distinguishing unavailable media from
	// unreadable directories.
	ListSidecarFiles(roots []string, path string) ([]domain.SidecarEntry, error)
	// OpenSidecarFile accepts only exact names returned by ListSidecarFiles.
	OpenSidecarFile(roots []string, path, name string) (*os.File, os.FileInfo, error)
}

// SubtitleConverter outputs UTF-8 WebVTT with offsetMs subtracted from cue times; unreadable input
// wraps ErrSubtitleUnreadable.
type SubtitleConverter interface {
	Convert(src []byte, format domain.SubtitleFormat, offsetMs int64) ([]byte, error)
}

// FileOpener launches the server host default application; opener.Opener determines capability at
// startup.
type FileOpener interface {
	Available() bool
	Open(path string) error
}

// Options wires route dependencies; missing services normally yield 500, with read omissions and
// fallbacks documented below.
type Options struct {
	Build domain.BuildInfo
	// Nil Pinger means storage is unreachable.
	Pinger Pinger

	Videos Library
	// Nil Playback makes writes fail and omits progress from reads.
	Playback Playback

	Scans Scans

	MediaFolders MediaFolders

	Tags Tags

	Credits Credits
	// Nil WorkInfo makes writes fail and omits workInfo from video responses.
	WorkInfo WorkInfo
	// Nil MediaInfo omits mediaInfo from video responses.
	MediaInfo MediaInfoReader

	Visibility Visibility
	// Nil VideoEdits makes the bulk edit routes fail with 500.
	VideoEdits VideoEdits

	Favorites Favorites

	Playlists Playlists

	PlaylistPlayback PlaylistPlaybackReader

	WatchHistory WatchHistory

	Activity Activity

	Overrides OverrideStore

	ThumbnailPicker ThumbnailPicker

	Versions VersionStore

	Folders Folders
	// Nil FolderGroups makes mutations fail and omits grouping from folder reads.
	FolderGroups FolderGroups

	Groups Groups

	SmartFolders SmartFolders

	Collections Collections

	Organizer Organizer

	Library LibraryItems

	Transcoder Transcoder
	// Nil TranscodeSettings makes settings routes fail but uses software encoding.
	TranscodeSettings TranscodeSettings
	// Only desktop builds supply NetworkSettings; nil yields 404.
	NetworkSettings NetworkSettings

	PluginProxy PluginProxySettings

	AutoImport AutoImportSettings

	AutoTagging AutoTagging
	Onboarding  Onboarding
	// Nil FaceRecognition makes the face recognition settings routes fail with 500.
	FaceRecognition FaceRecognition
	// Nil FaceAnalysis makes the face analysis runs route fail with 500.
	FaceAnalysis   FaceAnalysis
	VersionJudging VersionJudging
	VersionReview  VersionJudgeReview

	Ollama OllamaConnection

	ComfyUI ComfyUIConnection

	ThumbnailEdits ThumbnailEdits

	ThumbnailUploads ThumbnailUploads
	// Nil PersonImages makes the person image routes fail with 500.
	PersonImages PersonImages

	WorkInfoPlugins WorkInfoPlugins
	// Nil TranscodeProbes disables persistence, requiring uncached videos to be probed on every
	// transcode.
	TranscodeProbes TranscodeProbeWriter
	// Nil Artifacts yields 404 for thumbnails/hover previews and 500 for seek previews.
	Artifacts ArtifactReader
	// Nil Catalog makes related/reprobe routes fail and omits preview URLs and seek-preview state.
	Catalog VideoCatalog
	// Nil Opener means opening files is unavailable.
	Opener FileOpener
	// Nil Files yields 500, not 404, so missing wiring cannot masquerade as missing media.
	Files MediaFiles

	Subtitles SubtitleConverter

	Events *Events

	Assets fs.FS
	// Nil Logger uses slog's default.
	Logger *slog.Logger
	// Nil Auth fails nonpublic/auth routes with 500 and never grants owner access.
	Auth Authenticator

	APITokens APITokens

	ExternalVideos ExternalVideos
	// SessionRecheck defaults to thirty seconds when zero.
	SessionRecheck time.Duration
	// Nil Now uses time.Now for cookie Max-Age.
	Now func() time.Time
	// Empty TrustedProxies ignores forwarding headers; configure with MDM_TRUSTED_PROXIES.
	TrustedProxies []netip.Prefix
}

// Generated interface conformance catches missing implementations after schema changes.
type server struct {
	// favoritesMaxBody overrides the default byte limit for this server instance.
	favoritesMaxBody int64

	build        domain.BuildInfo
	pinger       Pinger
	videos       Library
	playback     Playback
	scans        Scans
	mediaFolders MediaFolders
	tags         Tags
	credits      Credits
	workInfo     WorkInfo
	mediaInfo    MediaInfoReader
	visibility   Visibility
	videoEdits   VideoEdits
	favorites    Favorites
	playlists    Playlists

	playlistPlayback PlaylistPlaybackReader
	watchHistory     WatchHistory
	activity         Activity
	overrides        OverrideStore
	thumbnails       ThumbnailPicker
	versions         VersionStore
	folders          Folders
	folderGroups     FolderGroups
	groups           Groups
	smartFolders     SmartFolders
	collections      Collections
	organizer        Organizer
	library          LibraryItems
	transcoder       Transcoder

	transcodeSettings TranscodeSettings
	networkSettings   NetworkSettings
	pluginProxy       PluginProxySettings
	autoImport        AutoImportSettings
	autoTagging       AutoTagging
	faceRecognition   FaceRecognition
	faceAnalysis      FaceAnalysis
	onboarding        Onboarding
	versionJudging    VersionJudging
	versionReview     VersionJudgeReview
	ollama            OllamaConnection
	comfyUI           ComfyUIConnection
	thumbnailEdits    ThumbnailEdits
	thumbnailUploads  ThumbnailUploads
	personImages      PersonImages
	workInfoPlugins   WorkInfoPlugins
	// thumbnailUploadBuffers bounds aggregate concurrent upload memory.
	thumbnailUploadBuffers *semaphore.Weighted

	transcodeProbes TranscodeProbeWriter

	transcodeStarts *transcodeStarts
	artifacts       ArtifactReader
	catalog         VideoCatalog
	opener          FileOpener
	files           MediaFiles
	subtitles       SubtitleConverter
	events          *Events
	logger          *slog.Logger
	auth            Authenticator
	apiTokens       APITokens
	externalVideos  ExternalVideos
	sessions        *sessionLedger

	guests *guestLedger
	now    func() time.Time

	trustedProxies trustedProxies
}

// NewRouter registers generated UI/external APIs and Bearer-only MCP, returns JSON 404 for unknown
// API paths, and falls back to the SPA elsewhere.
func NewRouter(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()

	// ServeMux prefers generated method-specific routes over the broad /api/ fallback.
	mux.Handle("/api/", http.HandlerFunc(apiNotFound))
	mux.Handle("/", newSPAHandler(opts.Assets, logger))

	srv := &server{
		build:                  opts.Build,
		pinger:                 opts.Pinger,
		videos:                 opts.Videos,
		playback:               opts.Playback,
		scans:                  opts.Scans,
		mediaFolders:           opts.MediaFolders,
		tags:                   opts.Tags,
		credits:                opts.Credits,
		workInfo:               opts.WorkInfo,
		mediaInfo:              opts.MediaInfo,
		visibility:             opts.Visibility,
		videoEdits:             opts.VideoEdits,
		favorites:              opts.Favorites,
		playlists:              opts.Playlists,
		playlistPlayback:       opts.PlaylistPlayback,
		watchHistory:           opts.WatchHistory,
		activity:               opts.Activity,
		overrides:              opts.Overrides,
		thumbnails:             opts.ThumbnailPicker,
		versions:               opts.Versions,
		folders:                opts.Folders,
		folderGroups:           opts.FolderGroups,
		groups:                 opts.Groups,
		smartFolders:           opts.SmartFolders,
		collections:            opts.Collections,
		organizer:              opts.Organizer,
		library:                opts.Library,
		transcoder:             opts.Transcoder,
		transcodeProbes:        opts.TranscodeProbes,
		transcodeSettings:      opts.TranscodeSettings,
		networkSettings:        opts.NetworkSettings,
		pluginProxy:            opts.PluginProxy,
		autoImport:             opts.AutoImport,
		autoTagging:            opts.AutoTagging,
		faceRecognition:        opts.FaceRecognition,
		faceAnalysis:           opts.FaceAnalysis,
		onboarding:             opts.Onboarding,
		versionJudging:         opts.VersionJudging,
		versionReview:          opts.VersionReview,
		ollama:                 opts.Ollama,
		comfyUI:                opts.ComfyUI,
		thumbnailEdits:         opts.ThumbnailEdits,
		thumbnailUploads:       opts.ThumbnailUploads,
		personImages:           opts.PersonImages,
		workInfoPlugins:        opts.WorkInfoPlugins,
		thumbnailUploadBuffers: newThumbnailUploadBuffers(),
		transcodeStarts:        newTranscodeStarts(),
		artifacts:              opts.Artifacts,
		catalog:                opts.Catalog,
		opener:                 opts.Opener,
		files:                  opts.Files,
		subtitles:              opts.Subtitles,
		events:                 opts.Events,
		logger:                 logger,
		auth:                   opts.Auth,
		apiTokens:              opts.APITokens,
		externalVideos:         opts.ExternalVideos,
		sessions:               newSessionLedger(opts.SessionRecheck, logger),
		guests:                 newGuestLedger(),
		now:                    opts.Now,
		// Copy trusted proxies so caller mutations cannot change access decisions.
		trustedProxies: slices.Clone(opts.TrustedProxies),
	}
	if srv.now == nil {
		srv.now = time.Now
	}
	if opts.Auth != nil {
		srv.sessions.check = func(ctx context.Context, token string) (bool, error) {
			_, valid, err := opts.Auth.CheckSession(ctx, token)
			return valid, err
		}
	}

	registerExternalAPI(mux, srv)
	mux.Handle(mcpPath, newMCPHandler(srv))
	generated := gen.HandlerWithOptions(srv, gen.StdHTTPServerOptions{
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			w.Header().Set("Cache-Control", cacheNoStore)
			writeJSON(w, http.StatusBadRequest, gen.Error{
				Code:    codeInvalidRequest,
				Message: err.Error(),
			}, logger)
		},
	})
	return compressResponses(noStoreOnError(srv.authBoundary(srv.mutationBoundary(generated))))
}

// Set no-store at write time for all 4xx/5xx, including ServeContent Range errors that bypass
// writeError after success headers were set.
func noStoreOnError(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&errorCacheWriter{ResponseWriter: w}, r)
	})
}

type errorCacheWriter struct {
	http.ResponseWriter
}

func (w *errorCacheWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *errorCacheWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Forward ReaderFrom so ServeContent/io.Copy can use sendfile instead of 32KB userspace copies.
func (w *errorCacheWriter) ReadFrom(src io.Reader) (int64, error) {
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(src)
	}
	// Expose only Write to avoid recursing into this wrapper's ReadFrom.
	return io.Copy(writerOnly{w.ResponseWriter}, src)
}

type writerOnly struct {
	io.Writer
}

func (w *errorCacheWriter) WriteHeader(status int) {
	if status >= http.StatusBadRequest {
		w.Header().Set("Cache-Control", cacheNoStore)
	}
	w.ResponseWriter.WriteHeader(status)
}

func (s *server) mutationBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			// Origin checks protect automatic cookie credentials; Bearer routes ignore cookies and need no
			// Origin restriction.
			if class, _ := classifyRequest(r); class != accessBearer && !s.acceptsSameOrigin(w, r) {
				return
			}
		}
		if requiresJSONBody(r) {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" {
				s.invalidRequest(w, "Content-Type must be application/json.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func requiresJSONBody(r *http.Request) bool {
	return requiresCreditJSONBody(r.Method, r.URL.Path) || isWorkInfoPluginJSONBody(r) || isWorkInfoFetchJSONPost(r) ||
		isActivityUndoPost(r) || requiresOtherJSONBody(r)
}

func requiresOtherJSONBody(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost:
		return requiresPostJSONBody(r.URL.Path)
	case http.MethodPut:
		return requiresPutJSONBody(r.URL.Path)
	case http.MethodPatch:
		return requiresPatchJSONBody(r.URL.Path)
	}
	return false
}

func requiresPostJSONBody(path string) bool {
	switch path {
	case "/api/media-folders", "/api/scans", "/api/scans/rescan", "/api/tags", "/api/tags/batch", "/api/tags/impact", "/api/tag-kinds",
		"/api/video-tags", "/api/video-tags/summary", "/api/video-edits", "/api/video-edits/summary",
		"/api/video-bundles", "/api/version-candidates/dismiss", "/api/version-candidates/rejudge", "/api/auth/setup", "/api/auth/login", "/api/api-tokens", "/api/v1/video-tags",
		"/api/v1/video-display-names", "/api/v1/video-thumbnails", "/api/v1/tags/merge", "/api/v1/tags/rename",
		"/api/v1/tags/synonyms", "/api/v1/tags/batch", "/api/auto-tagging/runs", "/api/face-recognition/runs", "/api/onboarding/interests", "/api/playlists", "/api/groups", "/api/smart-folders", "/api/collections/delete",
		"/api/organize/reorder", "/api/organize/group", "/api/organize/move", "/api/organize/bundle", "/api/organize/merge", "/api/organize/undo",
		"/api/v1/video-work-info", "/api/v1/people", "/api/v1/people/update", "/api/v1/people/merge",
		"/api/v1/people/delete", "/api/v1/organizations", "/api/v1/organizations/update",
		"/api/v1/organizations/merge", "/api/v1/organizations/delete",
		"/api/v1/tag-kinds", "/api/v1/tag-kinds/update", "/api/v1/tag-kinds/delete":
		return true
	}
	if suffix, ok := strings.CutPrefix(path, "/api/videos/"); ok {
		id, rest, found := strings.Cut(suffix, "/")
		return found && id != "" && rest == "thumbnail-edits"
	}

	if suffix, ok := strings.CutPrefix(path, "/api/playlists/"); ok {
		segments := strings.Split(suffix, "/")
		switch len(segments) {
		case 2:
			return segments[0] != "" && segments[1] == "items"
		case 4:
			return segments[0] != "" && segments[1] == "items" && segments[2] != "" && segments[3] == "move"
		}
		return false
	}

	if suffix, ok := strings.CutPrefix(path, "/api/groups/"); ok {
		return groupItemsBodyPath(strings.Split(suffix, "/"))
	}
	if id, ok := strings.CutPrefix(path, "/api/tags/"); ok {
		rest, found := strings.CutSuffix(id, "/merge")
		if found {
			return rest != "" && !strings.Contains(rest, "/")
		}
		rest, found = strings.CutSuffix(id, "/synonyms")
		return found && rest != "" && !strings.Contains(rest, "/")
	}
	return false
}

var putJSONBodyPaths = map[string]bool{
	"/api/video-visibility":          true,
	"/api/favorites":                 true,
	"/api/settings/transcoding":      true,
	"/api/settings/network":          true,
	"/api/settings/plugin-proxy":     true,
	"/api/settings/auto-import":      true,
	"/api/settings/auto-tagging":     true,
	"/api/settings/face-recognition": true,
	"/api/settings/ollama":           true,
	"/api/settings/comfyui":          true,
	"/api/settings/version-judging":  true,
	"/api/onboarding":                true,
}

func requiresPutJSONBody(path string) bool {
	if putJSONBodyPaths[path] {
		return true
	}
	if suffix, ok := strings.CutPrefix(path, "/api/folders/"); ok {
		id, rest, found := strings.Cut(suffix, "/")
		return found && id != "" && rest == "grouping"
	}
	if suffix, ok := strings.CutPrefix(path, "/api/tag-kinds/"); ok {
		id, rest, found := strings.Cut(suffix, "/")
		return found && id != "" && rest == "card-first"
	}
	if id, ok := strings.CutPrefix(path, "/api/media-folders/"); ok {
		return id != "" && !strings.Contains(id, "/")
	}
	if suffix, ok := strings.CutPrefix(path, "/api/videos/"); ok {
		id, rest, found := strings.Cut(suffix, "/")
		return found && id != "" && isVideoJSONPut(rest)
	}
	return false
}

func requiresPatchJSONBody(path string) bool {
	// PATCH /api/playlists/{id}。
	if id, ok := strings.CutPrefix(path, "/api/playlists/"); ok {
		return id != "" && !strings.Contains(id, "/")
	}
	// PATCH /api/groups/{id}。
	if id, ok := strings.CutPrefix(path, "/api/groups/"); ok {
		return id != "" && !strings.Contains(id, "/")
	}
	// cards is a literal route, not an ID, and has no PATCH operation.
	if id, ok := strings.CutPrefix(path, "/api/smart-folders/"); ok {
		return id != "" && id != "cards" && !strings.Contains(id, "/")
	}
	// PATCH /api/tag-kinds/{id}。
	if id, ok := strings.CutPrefix(path, "/api/tag-kinds/"); ok {
		return id != "" && !strings.Contains(id, "/")
	}
	// rejected-names, batch, and impact are literal routes without PATCH operations.
	if id, ok := strings.CutPrefix(path, "/api/tags/"); ok {
		switch id {
		case "", "rejected-names", "batch", "impact":
			return false
		}
		return !strings.Contains(id, "/")
	}
	return false
}

func isWorkInfoPluginJSONBody(r *http.Request) bool {
	suffix, ok := strings.CutPrefix(r.URL.Path, "/api/work-info-plugins/")
	if !ok {
		return false
	}
	id, rest, found := strings.Cut(suffix, "/")
	if !found || id == "" {
		return false
	}
	switch r.Method {
	case http.MethodPut:
		return rest == "approval" || rest == "settings" || rest == "auto-lookup"
	case http.MethodPost:
		return rest == "search" || rest == "person-search"
	}
	return false
}

func isWorkInfoFetchJSONPost(r *http.Request) bool {
	suffix, ok := strings.CutPrefix(r.URL.Path, "/api/videos/")
	if r.Method != http.MethodPost || !ok {
		return false
	}
	segments := strings.Split(suffix, "/")
	switch len(segments) {
	case 2:
		return segments[0] != "" && segments[1] == "work-info-fetches"
	case 4:
		return segments[0] != "" && segments[1] == "work-info-fetches" && segments[2] != "" && segments[3] == "apply"
	}
	return false
}

func groupItemsBodyPath(segments []string) bool {
	if len(segments) < 2 || segments[0] == "" || segments[1] != "items" {
		return false
	}
	switch len(segments) {
	case 2:
		return true
	case 3:
		return segments[2] == "remove"
	case 4:
		return segments[2] != "" && segments[3] == "move"
	}
	return false
}

func isVideoJSONPut(rest string) bool {
	switch rest {
	case "progress", "display-name", "thumbnail-position", "work-info", "details":
		return true
	}
	return false
}

func requiresCreditJSONBody(method, path string) bool {
	var rest string
	found := false
	for _, prefix := range []string{"/api/people", "/api/organizations"} {
		if suffix, ok := strings.CutPrefix(path, prefix); ok {
			rest, found = suffix, true
			break
		}
	}
	if !found {
		return false
	}
	if rest == "" {
		return method == http.MethodPost
	}
	segments := strings.Split(strings.TrimPrefix(rest, "/"), "/")
	if !strings.HasPrefix(rest, "/") || segments[0] == "" {
		return false
	}
	switch len(segments) {
	case 1:
		return method == http.MethodPatch
	case 2:
		return method == http.MethodPost && (segments[1] == "merge" || isPersonPluginFetch(path, segments))
	case 3:
		return method == http.MethodPost && strings.HasPrefix(path, "/api/people/") &&
			segments[1] == "appearances" && (segments[2] == "split" || segments[2] == "move")
	case 4:
		return method == http.MethodPost && isPersonPluginFetch(path, segments) && segments[2] != "" &&
			segments[3] == "apply"
	}
	return false
}

// isPersonPluginFetch reports a person's plugin-fetches path; organizations have none.
func isPersonPluginFetch(path string, segments []string) bool {
	return strings.HasPrefix(path, "/api/people/") && segments[1] == "plugin-fetches"
}

// Unknown API paths must return JSON 404, never SPA HTML 200 that obscures routing errors as JSON
// parse failures.
func apiNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusNotFound, gen.Error{
		Code:    codeNotFound,
		Message: "No such API route: " + r.URL.Path,
	}, slog.Default())
}

func writeJSON(w http.ResponseWriter, status int, payload any, logger *slog.Logger) {
	body, err := json.Marshal(payload)
	if err != nil {
		logger.Error("cannot encode the response as JSON", slog.Any("error", err))
		http.Error(w, `{"code":"internal","message":"Could not build the response."}`,
			http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	if _, err := w.Write(append(body, '\n')); err != nil {
		logger.Debug("could not write the response", slog.Any("error", err))
	}
}

const (
	// Mutable API data must not remain in intermediary caches.
	cacheNoStore = "no-store"
	// Artifact responses use private/no-cache so logout or visibility changes force reauthorization
	// while unchanged ETags allow 304 responses.
	cacheRevalidate = "private, no-cache"
)

// Error codes come from api/openapi.yaml and task generate; aliases only shorten call sites.
const (
	codeNotFound                    = gen.ErrorCodeNotFound
	codeInvalidRequest              = gen.ErrorCodeInvalidRequest
	codeInternal                    = gen.ErrorCodeInternal
	codeForbidden                   = gen.ErrorCodeForbidden
	codeInvalidMediaDirectory       = gen.ErrorCodeInvalidMediaDirectory
	codeUnsupportedMediaDirectory   = gen.ErrorCodeUnsupportedMediaDirectory
	codeMediaFolderNotFound         = gen.ErrorCodeMediaFolderNotFound
	codeOverlappingMediaDirectories = gen.ErrorCodeOverlappingMediaDirectories
	codeScanInProgress              = gen.ErrorCodeScanInProgress
	codeConflict                    = gen.ErrorCodeConflict
	codeMediaFoldersNotConfigured   = gen.ErrorCodeMediaFoldersNotConfigured
	codeDirectoryUnavailable        = gen.ErrorCodeDirectoryUnavailable
	codeProbeNotFailed              = gen.ErrorCodeProbeNotFailed
	codeOpenUnavailable             = gen.ErrorCodeOpenUnavailable
	codeFileMissing                 = gen.ErrorCodeFileMissing
	codeTagNotFound                 = gen.ErrorCodeTagNotFound
	codeInvalidSmartRule            = gen.ErrorCodeInvalidSmartRule
	codeSmartFolderNotFound         = gen.ErrorCodeSmartFolderNotFound
	codeSmartFolderCycle            = gen.ErrorCodeSmartFolderCycle
	codeTagNameTaken                = gen.ErrorCodeTagNameTaken
	codeTagMergeRequired            = gen.ErrorCodeTagMergeRequired
	codeTagNotTentative             = gen.ErrorCodeTagNotTentative
	codeProviderFailed              = gen.ErrorCodeProviderFailed
	codePluginTooLarge              = gen.ErrorCodePluginTooLarge
	codePluginChanged               = gen.ErrorCodePluginChanged
	codePluginUnusable              = gen.ErrorCodePluginUnusable
	codePluginNotApproved           = gen.ErrorCodePluginNotApproved
	codePluginSettingsMissing       = gen.ErrorCodePluginSettingsMissing
	codePluginFailed                = gen.ErrorCodePluginFailed
	codePluginTimedOut              = gen.ErrorCodePluginTimedOut
	codePluginBusy                  = gen.ErrorCodePluginBusy
	codePluginKindMismatch          = gen.ErrorCodePluginKindMismatch
	codeOperationUnsupported        = gen.ErrorCodeOperationUnsupported
	codeWorkInfoFetchStale          = gen.ErrorCodeWorkInfoFetchStale
	codePersonExternalIDMissing     = gen.ErrorCodePersonExternalIdMissing
	codeActivityNotRetryable        = gen.ErrorCodeActivityNotRetryable
	codeActivityNotUndoable         = gen.ErrorCodeActivityNotUndoable
)

// Error reasons likewise come from the schema; aliases distinguish generated unprefixed constants.
const (
	reasonNameIsTag                     = gen.ErrorReasonNameIsTag
	reasonNameIsSynonym                 = gen.ErrorReasonNameIsSynonym
	reasonUsernameLength                = gen.ErrorReasonUsernameLength
	reasonPasswordLength                = gen.ErrorReasonPasswordLength
	reasonTagNameEmpty                  = gen.ErrorReasonTagNameEmpty
	reasonTagNameControlCharacters      = gen.ErrorReasonTagNameControlCharacters
	reasonTagNameTooLong                = gen.ErrorReasonTagNameTooLong
	reasonMergeSameTag                  = gen.ErrorReasonMergeSameTag
	reasonSearchTooLong                 = gen.ErrorReasonSearchTooLong
	reasonTooManyTagFilters             = gen.ErrorReasonTooManyTagFilters
	reasonTooManyVideos                 = gen.ErrorReasonTooManyVideos
	reasonTooManyTags                   = gen.ErrorReasonTooManyTags
	reasonGuestFilterNotAllowed         = gen.ErrorReasonGuestFilterNotAllowed
	reasonInvalidCursor                 = gen.ErrorReasonInvalidCursor
	reasonInvalidFolderPath             = gen.ErrorReasonInvalidFolderPath
	reasonRelativeDirectoryPath         = gen.ErrorReasonRelativeDirectoryPath
	reasonVideoNotFound                 = gen.ErrorReasonVideoNotFound
	reasonFolderNotFound                = gen.ErrorReasonFolderNotFound
	reasonNoScan                        = gen.ErrorReasonNoScan
	reasonDirectoryNotFound             = gen.ErrorReasonDirectoryNotFound
	reasonFileUnavailable               = gen.ErrorReasonFileUnavailable
	reasonMediaFoldersChanged           = gen.ErrorReasonMediaFoldersChanged
	reasonGroupTargetNotFound           = gen.ErrorReasonGroupTargetNotFound
	reasonGroupCycle                    = gen.ErrorReasonGroupCycle
	reasonProbeInfoMissing              = gen.ErrorReasonProbeInfoMissing
	reasonSeekPreviewGenerating         = gen.ErrorReasonSeekPreviewGenerating
	reasonTranscodeUnavailable          = gen.ErrorReasonTranscodeUnavailable
	reasonCrossOrigin                   = gen.ErrorReasonCrossOrigin
	reasonOpenNotLocal                  = gen.ErrorReasonOpenNotLocal
	reasonEncoderUnavailable            = gen.ErrorReasonEncoderUnavailable
	reasonAPITokenNameEmpty             = gen.ErrorReasonApiTokenNameEmpty
	reasonAPITokenNameControlCharacters = gen.ErrorReasonApiTokenNameControlCharacters
	reasonAPITokenNameTooLong           = gen.ErrorReasonApiTokenNameTooLong
	reasonSubtitleUnavailable           = gen.ErrorReasonSubtitleUnavailable
	reasonDisplayNameControlCharacters  = gen.ErrorReasonDisplayNameControlCharacters
	reasonDisplayNameTooLong            = gen.ErrorReasonDisplayNameTooLong
	reasonDurationUnknown               = gen.ErrorReasonDurationUnknown
	reasonThumbnailPositionOutOfRange   = gen.ErrorReasonThumbnailPositionOutOfRange
	reasonThumbnailFrameUnavailable     = gen.ErrorReasonThumbnailFrameUnavailable
	reasonTooFewVideos                  = gen.ErrorReasonTooFewVideos
	reasonRepresentativeNotSelected     = gen.ErrorReasonRepresentativeNotSelected
	reasonNotBundled                    = gen.ErrorReasonNotBundled
	reasonListenFailed                  = gen.ErrorReasonListenFailed
	reasonComfyUIURLInvalid             = gen.ErrorReasonComfyuiUrlInvalid
	reasonComfyUIAPIKeyMissing          = gen.ErrorReasonComfyuiApiKeyMissing
	reasonThumbnailEditNotFound         = gen.ErrorReasonThumbnailEditNotFound
	reasonThumbnailEditNotReady         = gen.ErrorReasonThumbnailEditNotReady
	reasonThumbnailEditMaskInvalid      = gen.ErrorReasonThumbnailEditMaskInvalid
	reasonThumbnailEditReferenceInvalid = gen.ErrorReasonThumbnailEditReferenceInvalid
	reasonThumbnailEditNoResult         = gen.ErrorReasonThumbnailEditNoResult
	reasonThumbnailImageInvalid         = gen.ErrorReasonThumbnailImageInvalid
	reasonContentChanged                = gen.ErrorReasonContentChanged
	reasonThumbnailUploadTooLarge       = gen.ErrorReasonThumbnailUploadTooLarge
	reasonThumbnailUploadInvalid        = gen.ErrorReasonThumbnailUploadInvalid
	reasonPersonImageNotFound           = gen.ErrorReasonPersonImageNotFound
	reasonPersonImageInvalid            = gen.ErrorReasonPersonImageInvalid
	reasonPersonImageTooLarge           = gen.ErrorReasonPersonImageTooLarge
	reasonUndoConflict                  = gen.ErrorReasonUndoConflict
	reasonUndoExpired                   = gen.ErrorReasonUndoExpired
	reasonTagKindNotFound               = gen.ErrorReasonTagKindNotFound
	reasonTagKindNameTaken              = gen.ErrorReasonTagKindNameTaken
)

// Error messages must be English and exclude arbitrary OS/external-program output.
func (s *server) writeError(w http.ResponseWriter, status int, code gen.ErrorCode, message string) {
	s.writeErrorBody(w, status, gen.Error{Code: code, Message: message})
}

func (s *server) writeReasonError(w http.ResponseWriter, status int, code gen.ErrorCode, reason gen.ErrorReason, message string) {
	s.writeErrorBody(w, status, gen.Error{Code: code, Reason: &reason, Message: message})
}

func (s *server) writeErrorBody(w http.ResponseWriter, status int, body gen.Error) {
	// Centralize no-store on errors to prevent stale failures after state changes.
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, body, s.logger)
}

// Use the same 404 for missing and inaccessible media to avoid disclosing existence.
func (s *server) notFound(w http.ResponseWriter, message string) {
	s.writeError(w, http.StatusNotFound, codeNotFound, message)
}

func (s *server) invalidRequest(w http.ResponseWriter, message string) {
	s.writeError(w, http.StatusBadRequest, codeInvalidRequest, message)
}

func (s *server) invalidRequestReason(w http.ResponseWriter, reason gen.ErrorReason, message string) {
	s.writeReasonError(w, http.StatusBadRequest, codeInvalidRequest, reason, message)
}

// Limits come from server constants so the UI need not duplicate them.
func (s *server) invalidRequestLimit(w http.ResponseWriter, reason gen.ErrorReason, limit int, message string) {
	s.writeErrorBody(w, http.StatusBadRequest, gen.Error{
		Code:    codeInvalidRequest,
		Reason:  &reason,
		Limit:   &limit,
		Message: message,
	})
}

func (s *server) notFoundReason(w http.ResponseWriter, reason gen.ErrorReason, message string) {
	s.writeReasonError(w, http.StatusNotFound, codeNotFound, reason, message)
}

func (s *server) conflictReason(w http.ResponseWriter, reason gen.ErrorReason, message string) {
	s.writeReasonError(w, http.StatusConflict, codeConflict, reason, message)
}

// Log internal causes without exposing them in responses.
func (s *server) internalError(w http.ResponseWriter, message string, err error) {
	s.logger.Error(message, slog.Any("error", err))
	s.writeError(w, http.StatusInternalServerError, codeInternal, message)
}
