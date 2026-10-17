package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 適用したサムネイルの画像の取り消しと、thumbnailImage・thumbnailUrl の版
// は、本物の認証・保存層で確かめる。
// 画像の生成だけを fakeThumbnailPicker が代わりにする。

func (f *guestFixture) revertThumbnailImage(name string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{
		method: http.MethodDelete, target: f.videoPath(name, "/thumbnail-image"), cookies: cookies,
	})
}

// applyImage は動画 name に生成画像を適用したことを保存層へ直接記録する（適用の経路は編集の
// セッションのユニットが作る）。
func (f *guestFixture) applyImage(t *testing.T, name string) {
	t.Helper()
	f.applyImageOf(t, name, domain.ThumbnailImageGenerated)
}

// applyImageOf は動画 name に出どころ origin の画像を適用したことを保存層へ直接記録する。
func (f *guestFixture) applyImageOf(t *testing.T, name string, origin domain.ThumbnailImageOrigin) {
	t.Helper()
	ctx := context.Background()
	video, err := f.env.db.Ingest().GetVideo(ctx, f.ids[name])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.db.Ingest().ApplyThumbnailImage(ctx, video.ID, video.ContentKey, domain.AppliedThumbnailImage{
		Image: []byte("jpeg"), Source: []byte("source"), Origin: origin,
	}); err != nil {
		t.Fatal(err)
	}
}

// 画像を適用すると、所有者の応答に thumbnailImage: "generated" が入り、thumbnailUrl の版が
// `-g<改版番号>` になる。ゲストは同じ URL を受け取り、thumbnailImage を受け取らない。
// 取り消し（DELETE）は 200 で thumbnailImage の無い Video を返し、URL が元に戻る。
// 画像が無くても 200 である。
func TestRevertThumbnailImage(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	original := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner))
	if original.ThumbnailImage != nil {
		t.Fatalf("前提: thumbnailImage = %v", *original.ThumbnailImage)
	}
	// 所有者の応答の内容の版は thumbnailUrl の版の先頭と同じである。
	if original.ContentVersion == nil || !strings.Contains(thumbnailURLOf(original), "?v="+*original.ContentVersion) {
		t.Errorf("contentVersion = %v, thumbnailUrl = %q", original.ContentVersion, thumbnailURLOf(original))
	}

	f.applyImage(t, "a")
	applied := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner))
	if applied.ThumbnailImage == nil || *applied.ThumbnailImage != gen.Generated {
		t.Errorf("適用後の thumbnailImage = %v", applied.ThumbnailImage)
	}
	appliedURL := thumbnailURLOf(applied)
	if appliedURL == thumbnailURLOf(original) || !strings.Contains(appliedURL, "-g") {
		t.Errorf("適用後の thumbnailUrl = %q, 前 = %q", appliedURL, thumbnailURLOf(original))
	}
	guest := env.get(f.videoPath("a", ""))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(guest.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	assertGuestVideo(t, "ゲストの詳細", raw)
	var guestURL string
	if err := json.Unmarshal(raw["thumbnailUrl"], &guestURL); err != nil {
		t.Fatal(err)
	}
	if guestURL != appliedURL {
		t.Errorf("ゲストの thumbnailUrl = %q, 所有者 = %q", guestURL, appliedURL)
	}

	rec := f.revertThumbnailImage("a", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("取り消し: status = %d: %s", rec.Code, rec.Body)
	}
	reverted := decode[gen.Video](t, rec)
	if reverted.ThumbnailImage != nil || thumbnailURLOf(reverted) != thumbnailURLOf(original) {
		t.Errorf("取り消しの応答: thumbnailImage = %v, thumbnailUrl = %q, want %q",
			reverted.ThumbnailImage, thumbnailURLOf(reverted), thumbnailURLOf(original))
	}
	if reverted.Location == nil || len(reverted.Tags) != 1 {
		t.Errorf("取り消しの応答が詳細と同じ形でない: %+v", reverted)
	}

	if rec := f.revertThumbnailImage("a", f.owner); rec.Code != http.StatusOK {
		t.Errorf("画像の無い取り消し: status = %d: %s", rec.Code, rec.Body)
	}
}

// アップロードした画像を適用していれば、所有者の応答の thumbnailImage は "uploaded" になる。
func TestThumbnailImageOriginUploaded(t *testing.T) {
	f := newGuestFixture(t, true)
	f.applyImageOf(t, "a", domain.ThumbnailImageUploaded)
	video := decode[gen.Video](t, f.env.get(f.videoPath("a", ""), f.owner))
	if video.ThumbnailImage == nil || *video.ThumbnailImage != gen.Uploaded {
		t.Fatalf("thumbnailImage = %v, want uploaded", video.ThumbnailImage)
	}
}

// ゲストの取り消しは 401、生成の失敗は 409 で画像が残り、無い動画は 404 になる。
func TestRevertThumbnailImageRejects(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	f.applyImage(t, "a")

	assertUnauthenticated(t, "Cookie なしの取り消し", f.revertThumbnailImage("a"))

	f.thumbnails.setFail(errors.New("ffmpeg: no frame"))
	rec := f.revertThumbnailImage("a", f.owner)
	assertErrorBody(t, "生成の失敗", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailFrameUnavailable,
	})
	after := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner))
	if after.ThumbnailImage == nil || *after.ThumbnailImage != gen.Generated {
		t.Errorf("生成の失敗で画像が外れた")
	}
	f.thumbnails.setFail(nil)

	f.thumbnails.setStale(true)
	rec = f.revertThumbnailImage("a", f.owner)
	assertErrorBody(t, "付け替わった所在", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonFileUnavailable,
	})
	f.thumbnails.setStale(false)

	missing := env.serve(authRequest{
		method: http.MethodDelete, target: "/api/videos/9999/thumbnail-image", cookies: []*http.Cookie{f.owner},
	})
	assertErrorBody(t, "無い動画", missing.Code, missing.Body.Bytes(), wantError{
		status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound,
	})
}
