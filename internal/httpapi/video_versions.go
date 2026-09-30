package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 同じ動画の別バージョンの集まり（specs/030-video-versions/contracts/screen-api.md §0〜§4、
// research.md R-8）。集まりは動画の id で指し、集まりの id は応答に出さない。どの操作も
// 1 つの取引で済むので internal/app は通さず、internal/store の *VersionStore を直接呼ぶ。
// 確定後の /api/events の video は、保存層が発行する domain.VideoBundleChanged から流れる
// （events.go）。

// VersionStore は集まりの保存先である。internal/store の *VersionStore がこれを満たす。
type VersionStore interface {
	// Versions は videoID の動画の集まりのうち、見る人に見せてよいバージョンを実効の代表を
	// 先頭に返す。集まりに属さなければその 1 本。見せられなければ domain.ErrNotFound。
	Versions(ctx context.Context, audience domain.Audience, videoID int64) (domain.VideoVersions, error)
	// Bundle は videoIDs を 1 つの集まりに束ね、representativeID を代表にする。引けない id が
	// あれば domain.ErrNotFound、2 本未満なら domain.ErrTooFewVersions、代表が含まれなければ
	// domain.ErrRepresentativeNotSelected で、何も変えない。
	Bundle(ctx context.Context, videoIDs []int64, representativeID int64) (domain.VideoVersions, error)
	// MakeRepresentative は videoID の動画をその集まりの代表にする。集まりに属さなければ
	// domain.ErrNotBundled、引けなければ domain.ErrNotFound。
	MakeRepresentative(ctx context.Context, videoID int64) (domain.VideoVersions, error)
	// Unbundle は videoID の動画をその集まりから外し、外した動画を返す。集まりに属さなければ
	// domain.ErrNotBundled、引けなければ domain.ErrNotFound。
	Unbundle(ctx context.Context, videoID int64) (domain.Video, error)
}

// maxBundleVideoIDs は束ねる経路が受け付ける videoIds の最大件数である。
// POST /api/video-tags と同じ上限（contracts/screen-api.md §2）。
const maxBundleVideoIDs = maxVideoTagsIDs

// ListVideoVersions は動画の集まりの全バージョンを返す（GET /api/videos/{id}/versions）。
func (s *server) ListVideoVersions(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	versions, err := s.versions.Versions(r.Context(), audienceFrom(r.Context()), id)
	if !s.checkVersionsError(w, err, "Could not load the versions.") {
		return
	}
	s.writeVideoVersions(w, r, versions)
}

// BundleVideos は動画を 1 つの集まりに束ねる（POST /api/video-bundles）。
func (s *server) BundleVideos(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoBundleRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if len(body.VideoIds) > maxBundleVideoIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxBundleVideoIDs,
			fmt.Sprintf("Bundle at most %d videos at a time.", maxBundleVideoIDs))
		return
	}
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	versions, err := s.versions.Bundle(r.Context(), body.VideoIds, body.RepresentativeId)
	if !s.checkVersionsError(w, err, "Could not bundle the videos.") {
		return
	}
	s.writeVideoVersions(w, r, versions)
}

// MakeRepresentativeVersion は動画をその集まりの代表にする
// （POST /api/videos/{id}/make-representative）。
func (s *server) MakeRepresentativeVersion(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	versions, err := s.versions.MakeRepresentative(r.Context(), id)
	if !s.checkVersionsError(w, err, "Could not change the version shown in the library.") {
		return
	}
	s.writeVideoVersions(w, r, versions)
}

// UnbundleVideo は動画をその集まりから外す（POST /api/videos/{id}/unbundle）。応答は
// GET /api/videos/{id} と同じ形の外した動画である。
func (s *server) UnbundleVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	video, err := s.versions.Unbundle(r.Context(), id)
	if !s.checkVersionsError(w, err, "Could not unbundle the video.") {
		return
	}
	s.writeVideoDetail(w, r, video)
}

// checkVersionsError は集まりの保存層の誤りを契約の応答へ写す（contracts/screen-api.md §1〜§4）。
// 誤りが無ければ true を返し、あれば応答を書いて false を返す。
func (s *server) checkVersionsError(w http.ResponseWriter, err error, message string) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
	case errors.Is(err, domain.ErrTooFewVersions):
		s.invalidRequestReason(w, reasonTooFewVideos, "Select at least two videos to bundle.")
	case errors.Is(err, domain.ErrRepresentativeNotSelected):
		s.invalidRequestReason(w, reasonRepresentativeNotSelected, "representativeId must be one of videoIds.")
	case errors.Is(err, domain.ErrNotBundled):
		s.invalidRequestReason(w, reasonNotBundled, "The video is not bundled with other videos.")
	default:
		s.internalError(w, message, err)
	}
	return false
}

// writeVideoVersions は集まりの全バージョンを VideoVersions の形で書く。各項目は
// GET /api/videos/{id} と同じく、再生位置・タグ・所在・置かれたフォルダ（登録フォルダの
// 表示名つき）・集まりの要約を持ち、見る人に合わせて省く（forAudience）。グループと
// シーク用プレビューの状態は、項目ごとに問い合わせが要るので載せない。
func (s *server) writeVideoVersions(w http.ResponseWriter, r *http.Request, versions domain.VideoVersions) {
	ctx := r.Context()
	audience := audienceFrom(ctx)
	progress := s.progressFor(ctx, versions.Items)
	tags := s.tagsFor(ctx, versions.Items)
	roots := s.registeredRoots(ctx)
	openable := s.canOpen(r)

	payload := gen.VideoVersions{RepresentativeId: versions.RepresentativeID, Items: make([]gen.Video, 0, len(versions.Items))}
	for _, view := range s.presentVideos(ctx, versions.Items) {
		video := view.Video
		item := withTags(withProgress(toAPIVideo(view), progress, video.UserKey), tags, video.UserKey)
		item.Location = &gen.VideoLocation{Path: video.Path, Openable: openable}
		item.Folder = detailFolder(roots, video.Path)
		item.Versions = apiVersionsRef(video.Versions)
		payload.Items = append(payload.Items, forAudience(audience, item))
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// apiVersionsRef は集まりの要約を契約の形へ写す。集まりのメンバーでなければ nil。
func apiVersionsRef(ref *domain.VideoVersionsRef) *gen.VideoVersionsRef {
	if ref == nil {
		return nil
	}
	return &gen.VideoVersionsRef{Count: ref.Count, RepresentativeId: ref.RepresentativeID}
}
