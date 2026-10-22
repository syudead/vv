package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// The store's SQL contract is tested in internal/store; this fake records what the handlers pass to it.
type fakeVirtualVideos struct {
	video domain.Video
	err   error
	calls []virtualCall
}

type virtualCall struct {
	op             string
	videoID        int64
	contentVersion string
	title          string
	info           domain.WorkInfo
}

func (f *fakeVirtualVideos) Create(_ context.Context, title string) (domain.Video, error) {
	f.calls = append(f.calls, virtualCall{op: "create", title: title})
	return f.video, f.err
}

func (f *fakeVirtualVideos) SaveTitle(_ context.Context, videoID int64, contentVersion, title string) (domain.Video, error) {
	f.calls = append(f.calls, virtualCall{op: "title", videoID: videoID, contentVersion: contentVersion, title: title})
	return f.video, f.err
}

func (f *fakeVirtualVideos) SaveDetails(
	_ context.Context, videoID int64, contentVersion, title string, info domain.WorkInfo,
) (domain.Video, error) {
	f.calls = append(f.calls, virtualCall{
		op: "details", videoID: videoID, contentVersion: contentVersion, title: title, info: info,
	})
	return f.video, f.err
}

func (f *fakeVirtualVideos) Delete(_ context.Context, videoID int64, contentVersion string) error {
	f.calls = append(f.calls, virtualCall{op: "delete", videoID: videoID, contentVersion: contentVersion})
	return f.err
}

// Fields as the store reads a projected row: no location, no media, and the file title equal to the title.
func virtualVideo(id int64, title string) domain.Video {
	key := domain.VirtualKeyPrefix + strconv.FormatInt(id, 10)
	created := time.Unix(1_757_000_000, 0)
	return domain.Video{
		ID: id, Title: title, FileTitle: title, ContentKey: key, UserKey: key,
		AddedAt: created, FileCreatedAt: created, EditedAt: created,
		ProbeState: domain.ProbeStatePending, ThumbnailState: domain.ThumbnailStatePending,
		PreviewState: domain.PreviewStatePending,
	}
}

func virtualServer(t *testing.T, store *fakeVirtualVideos, videos ...domain.Video) http.Handler {
	t.Helper()
	byID := map[int64]domain.Video{}
	for _, video := range videos {
		byID[video.ID] = video
	}
	return newTestServer(t, Options{Videos: &fakeLibrary{videos: byID}, VirtualVideos: store,
		Overrides: unusedOverrides{}, WorkInfo: unusedWorkInfo{}})
}

// A virtual video's save must never reach the file-video stores.
type unusedOverrides struct{ OverrideStore }

type unusedWorkInfo struct{ WorkInfo }

func (unusedWorkInfo) ByUserKeys(context.Context, []string) (map[string]domain.WorkInfo, error) {
	return nil, nil
}

func TestVirtualVideoResponseOmitsFileFields(t *testing.T) {
	video := virtualVideo(5, "Moonlight")
	handler := virtualServer(t, &fakeVirtualVideos{}, video)

	rec := do(t, handler, http.MethodGet, "/api/videos/5")
	got := decode[map[string]json.RawMessage](t, rec)
	for _, absent := range []string{"location", "folder", "fileTitle", "durationMs", "unplayableReason", "previewUrl",
		"seekThumbnailUrl", "versions", "mediaInfo"} {
		if _, ok := got[absent]; ok {
			t.Errorf("%s が入っている: %s", absent, rec.Body)
		}
	}
	for field, want := range map[string]string{
		"virtual": "true", "contentVersion": `"virtual:5"`, "playable": "false", "sizeBytes": "0",
		"fileCreatedAt": `"2025-09-04T15:33:20Z"`,
	} {
		if string(got[field]) != want {
			t.Errorf("%s = %s, want %s", field, got[field], want)
		}
	}

	file := decode[gen.Video](t, do(t, virtualServer(t, &fakeVirtualVideos{}, sampleVideo(1, "clip")), http.MethodGet, "/api/videos/1"))
	if file.Virtual || file.ContentVersion == nil || *file.ContentVersion != "abcdef012345" {
		t.Errorf("ファイルのある動画 = virtual %v, contentVersion %v, want false と先頭 12 文字", file.Virtual, file.ContentVersion)
	}
}

func TestCreateVirtualVideo(t *testing.T) {
	store := &fakeVirtualVideos{video: virtualVideo(5, "Moonlight")}
	handler := virtualServer(t, store)

	rec := jsonRequest(t, handler, http.MethodPost, "/api/virtual-videos", `{"title":" Moonlight "}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Video](t, rec); !got.Virtual || got.Id != 5 || got.Title != "Moonlight" {
		t.Errorf("作った動画 = %+v", got)
	}
	if !reflect.DeepEqual(store.calls, []virtualCall{{op: "create", title: " Moonlight "}}) {
		t.Errorf("ストアへの呼び出し = %+v", store.calls)
	}

	store.err = domain.ErrInvalidVirtualTitle
	rec = jsonRequest(t, handler, http.MethodPost, "/api/virtual-videos", `{"title":"  "}`)
	assertErrorBody(t, "空のタイトル", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidTitle,
	})
	assertStatus(t, "title が無い", jsonRequest(t, handler, http.MethodPost, "/api/virtual-videos", `{}`), http.StatusBadRequest)
}

func TestDeleteVideo(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want wantError
	}{
		{"消した", nil, wantError{status: http.StatusNoContent}},
		{"無い", domain.ErrNotFound, wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound}},
		{"版が違う", domain.ErrContentChanged, wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonContentChanged}},
		{"ファイルがある", domain.ErrNotVirtual, wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonNotVirtual}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeVirtualVideos{err: tc.err}
			rec := do(t, virtualServer(t, store), http.MethodDelete, "/api/videos/5?contentVersion=virtual%3A5")
			if tc.err == nil {
				assertStatus(t, tc.name, rec, http.StatusNoContent)
			} else {
				assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), tc.want)
			}
			if !reflect.DeepEqual(store.calls, []virtualCall{{op: "delete", videoID: 5, contentVersion: "virtual:5"}}) {
				t.Errorf("ストアへの呼び出し = %+v", store.calls)
			}
		})
	}

	store := &fakeVirtualVideos{}
	assertStatus(t, "contentVersion が無い", do(t, virtualServer(t, store), http.MethodDelete, "/api/videos/5"), http.StatusBadRequest)
	if len(store.calls) != 0 {
		t.Errorf("版の無い削除がストアに届いた: %+v", store.calls)
	}
}

func TestSaveVirtualVideoDetails(t *testing.T) {
	video := virtualVideo(5, "Moonlight")
	store := &fakeVirtualVideos{video: video}
	handler := virtualServer(t, store, video)

	rec := jsonRequest(t, handler, http.MethodPut, "/api/videos/5/details",
		`{"displayName":"Sunrise","contentVersion":"virtual:5","workInfo":{"description":"夜明け","urls":["https://example.com/a"]}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if len(store.calls) != 1 {
		t.Fatalf("ストアへの呼び出し = %+v", store.calls)
	}
	call := store.calls[0]
	if call.op != "details" || call.videoID != 5 || call.contentVersion != "virtual:5" || call.title != "Sunrise" ||
		call.info.Description != "夜明け" || !slices.Equal(call.info.URLs, []string{"https://example.com/a"}) {
		t.Errorf("ストアへの呼び出し = %+v", call)
	}

	rec = jsonRequest(t, handler, http.MethodPut, "/api/videos/5/display-name", `{"displayName":"Dusk","contentVersion":"virtual:5"}`)
	if rec.Code != http.StatusOK || !reflect.DeepEqual(store.calls[1], virtualCall{op: "title", videoID: 5, contentVersion: "virtual:5", title: "Dusk"}) {
		t.Errorf("表示名の保存 = %d %+v: %s", rec.Code, store.calls[1:], rec.Body)
	}
}

func TestSaveVirtualVideoErrors(t *testing.T) {
	video := virtualVideo(5, "Moonlight")
	bodies := map[string]func(title, version string) string{
		"details":      func(title, version string) string { return `{"displayName":` + title + version + `,"workInfo":{}}` },
		"display-name": func(title, version string) string { return `{"displayName":` + title + version + `}` },
	}
	withVersion := `,"contentVersion":"virtual:5"`
	for _, tc := range []struct {
		name      string
		title     string
		version   string
		storeErr  error
		want      wantError
		wantField string
		reaches   bool
	}{
		{"空のタイトル", `" "`, withVersion, nil,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidTitle}, "displayName", false},
		{"版が無い", `"x"`, "", nil, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "", false},
		{"消された", `"x"`, withVersion, domain.ErrNotFound,
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound}, "", true},
		{"版が違う", `"x"`, withVersion, domain.ErrContentChanged,
			wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonContentChanged}, "", true},
	} {
		for endpoint, bodyOf := range bodies {
			t.Run(tc.name+"/"+endpoint, func(t *testing.T) {
				store := &fakeVirtualVideos{err: tc.storeErr}
				body := bodyOf(tc.title, tc.version)
				rec := jsonRequest(t, virtualServer(t, store, video), http.MethodPut, "/api/videos/5/"+endpoint, body)
				assertErrorBody(t, body, rec.Code, rec.Body.Bytes(), tc.want)
				if got := decode[gen.Error](t, rec); endpoint == "details" && valueOrZero(got.Field) != tc.wantField {
					t.Errorf("field = %v, want %q", got.Field, tc.wantField)
				}
				if reached := len(store.calls) > 0; reached != tc.reaches {
					t.Errorf("ストアに届いた = %v, want %v", reached, tc.reaches)
				}
			})
		}
	}
}

// A screen that read a deleted virtual video must not write into the file video that reused its row id.
func TestSaveWithVirtualVersionRejectsFileVideo(t *testing.T) {
	want := wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonContentChanged}
	for endpoint, body := range map[string]string{
		"details":      `{"displayName":"x","contentVersion":"virtual:5","workInfo":{}}`,
		"display-name": `{"displayName":"x","contentVersion":"virtual:5"}`,
	} {
		store := &fakeVirtualVideos{}
		rec := jsonRequest(t, virtualServer(t, store, sampleVideo(5, "clip")), http.MethodPut, "/api/videos/5/"+endpoint, body)
		assertErrorBody(t, endpoint, rec.Code, rec.Body.Bytes(), want)
		if len(store.calls) != 0 {
			t.Errorf("%s: ストアに届いた: %+v", endpoint, store.calls)
		}
	}
}

// Writes stay owner-only at the auth boundary, which answers a guest before any store call.
func TestGuestCannotWriteVirtualVideos(t *testing.T) {
	store := &fakeVirtualVideos{}
	handler := newTestServer(t, Options{Videos: &fakeLibrary{}, VirtualVideos: store, Auth: guestEventsAuth{}})
	for _, tc := range []struct{ method, target, body string }{
		{http.MethodPost, "/api/virtual-videos", `{"title":"x"}`},
		{http.MethodDelete, "/api/videos/5?contentVersion=virtual%3A5", ""},
		{http.MethodPut, "/api/videos/5/details", `{"displayName":"x","contentVersion":"virtual:5","workInfo":{}}`},
		{http.MethodPut, "/api/videos/5/display-name", `{"displayName":"x","contentVersion":"virtual:5"}`},
	} {
		assertUnauthenticated(t, tc.method+" "+tc.target, jsonRequest(t, handler, tc.method, tc.target, tc.body))
	}
	if len(store.calls) != 0 {
		t.Errorf("ゲストの書き込みがストアに届いた: %+v", store.calls)
	}
}

func TestListVideosKind(t *testing.T) {
	library := &fakeLibrary{}
	handler := newTestServer(t, Options{Videos: library})

	assertStatus(t, "file", do(t, handler, http.MethodGet, "/api/videos?kind=file"), http.StatusOK)
	if library.lastQuery.Kind != domain.VideoKindFile {
		t.Errorf("Kind = %q, want file", library.lastQuery.Kind)
	}
	assertStatus(t, "既定", do(t, handler, http.MethodGet, "/api/videos"), http.StatusOK)
	if library.lastQuery.Kind != "" {
		t.Errorf("既定の Kind = %q, want すべて", library.lastQuery.Kind)
	}
	assertStatus(t, "知らない種類", do(t, handler, http.MethodGet, "/api/videos?kind=folder"), http.StatusBadRequest)
}

// Every version-bundle operation answers 409 virtual_video when the store rejects a virtual video.
func TestBundleOperationsRejectVirtualVideos(t *testing.T) {
	handler := newTestServer(t, Options{Versions: virtualVersions{}, Organizer: virtualOrganizer{}})
	want := wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonVirtualVideo}
	for _, tc := range []struct{ target, body string }{
		{"/api/video-bundles", `{"videoIds":[1,5],"representativeId":1}`},
		{"/api/organize/bundle", `{"videoIds":[5],"target":1}`},
		{"/api/version-candidates/dismiss", `{"videoIds":[1,5]}`},
		{"/api/videos/5/make-representative", ""},
		{"/api/videos/5/unbundle", ""},
	} {
		rec := jsonRequest(t, handler, http.MethodPost, tc.target, tc.body)
		assertErrorBody(t, tc.target, rec.Code, rec.Body.Bytes(), want)
	}
}

// The versions list of an unbundled virtual video is the video alone, and it has no file location either.
func TestVirtualVideoVersionsOmitLocation(t *testing.T) {
	handler := newTestServer(t, Options{Versions: virtualVersions{}})
	rec := do(t, handler, http.MethodGet, "/api/videos/5/versions")
	assertStatus(t, "versions", rec, http.StatusOK)
	got := decode[struct {
		Items []map[string]json.RawMessage `json:"items"`
	}](t, rec)
	if len(got.Items) != 1 {
		t.Fatalf("items = %s", rec.Body)
	}
	for _, absent := range []string{"location", "folder"} {
		if _, ok := got.Items[0][absent]; ok {
			t.Errorf("%s が入っている: %s", absent, rec.Body)
		}
	}
}

type virtualVersions struct{ VersionStore }

func (virtualVersions) Versions(_ context.Context, _ domain.Audience, id int64) (domain.VideoVersions, error) {
	return domain.VideoVersions{RepresentativeID: id, Items: []domain.Video{virtualVideo(id, "Moonlight")}}, nil
}

func (virtualVersions) Bundle(context.Context, []int64, int64) (domain.VideoVersions, error) {
	return domain.VideoVersions{}, domain.ErrVirtualVideo
}

func (virtualVersions) MakeRepresentative(context.Context, int64) (domain.VideoVersions, error) {
	return domain.VideoVersions{}, domain.ErrVirtualVideo
}

func (virtualVersions) Unbundle(context.Context, int64) (domain.Video, error) {
	return domain.Video{}, domain.ErrVirtualVideo
}

func (virtualVersions) Dismiss(context.Context, [2]int64) error { return domain.ErrVirtualVideo }

// A virtual video has no frames, so the frame position and every thumbnail edit start answer 409 virtual_video
// before reaching the application, which these fakes would panic on.
func TestFrameThumbnailEndpointsRejectVirtualVideos(t *testing.T) {
	handler := newTestServer(t, Options{
		Videos:          &fakeLibrary{videos: map[int64]domain.Video{7: virtualVideo(7, "Moonlight")}},
		ThumbnailPicker: unusedThumbnailPicker{}, ThumbnailEdits: unusedThumbnailEdits{},
	})
	want := wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonVirtualVideo}
	for _, tc := range []struct{ method, target, body string }{
		{http.MethodPut, "/api/videos/7/thumbnail-position", `{"positionMs":null}`},
		{http.MethodPost, "/api/videos/7/thumbnail-edits", `{"source":"thumbnail"}`},
		{http.MethodPost, "/api/videos/7/thumbnail-edits/image", `{}`},
	} {
		rec := jsonRequest(t, handler, tc.method, tc.target, tc.body)
		assertErrorBody(t, tc.target, rec.Code, rec.Body.Bytes(), want)
	}
}

type unusedThumbnailPicker struct{ ThumbnailPicker }

type unusedThumbnailEdits struct{ ThumbnailEdits }

type virtualOrganizer struct{ Organizer }

func (virtualOrganizer) Bundle(context.Context, []int64, int64) (domain.OrganizeResult, error) {
	return domain.OrganizeResult{}, domain.ErrVirtualVideo
}
