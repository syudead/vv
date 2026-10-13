package httpapi

import (
	"bytes"
	"context"
	"image/png"
	"net/http"
	"os"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 今のサムネイルから始める編集（specs/046-thumbnail-image-upload/contracts/screen-api.md の
// POST /api/videos/{id}/thumbnail-edits (changed)）を、本物の保存層の適用した画像で確かめる。

// applySource は動画 name に元の画像 source を持つ画像を適用したことを保存層へ直接記録する。
func (f *guestFixture) applySource(t *testing.T, name string, source []byte) {
	t.Helper()
	ctx := context.Background()
	video, err := f.env.db.Ingest().GetVideo(ctx, f.ids[name])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.db.Ingest().ApplyThumbnailImage(ctx, video.ID, video.ContentKey, domain.AppliedThumbnailImage{
		Image: []byte("jpeg"), Source: source, Origin: domain.ThumbnailImageUploaded,
	}); err != nil {
		t.Fatal(err)
	}
}

// 画像を適用していない動画は自動の位置のコマから始め、その位置を返す。適用した動画は保存した
// 元の画像の大きさで始め、位置を返さず、コマのルートはその PNG を返す。元の画像を読めなければ
// 409 thumbnail_frame_unavailable である。
func TestStartThumbnailEditFromThumbnail(t *testing.T) {
	f := newGuestFixture(t, true)
	f.saveConnection(t)

	rec := f.startEdit("a", `{"source":"thumbnail","positionMs":999999}`, f.owner)
	edit := decode[gen.ThumbnailEdit](t, rec)
	if rec.Code != http.StatusCreated || edit.Source != gen.ThumbnailEditSourceThumbnail ||
		edit.PositionMs == nil || *edit.PositionMs != 6_000 || edit.FrameWidth != 4 {
		t.Fatalf("start from the automatic frame = %d %s", rec.Code, rec.Body)
	}
	f.edits.mu.Lock()
	positions := slices.Clone(f.edits.positions)
	f.edits.mu.Unlock()
	if !slices.Equal(positions, []int64{6_000}) {
		t.Fatalf("extracted at %v", positions)
	}

	f.applySource(t, "a", editPNG(6, 5))
	rec = f.startEdit("a", `{"source":"thumbnail"}`, f.owner)
	edit = decode[gen.ThumbnailEdit](t, rec)
	if rec.Code != http.StatusCreated || edit.Source != gen.ThumbnailEditSourceThumbnail ||
		edit.PositionMs != nil || edit.FrameWidth != 6 || edit.FrameHeight != 5 {
		t.Fatalf("start from the stored source = %d %s", rec.Code, rec.Body)
	}
	frame := f.editRequest(http.MethodGet, edit.Id, "/frame", f.owner)
	config, err := png.DecodeConfig(bytes.NewReader(frame.Body.Bytes()))
	if frame.Code != http.StatusOK || frame.Header().Get("Content-Type") != "image/png" ||
		err != nil || config.Width != 6 || config.Height != 5 {
		t.Fatalf("frame = %d %v %+v %v", frame.Code, frame.Header(), config, err)
	}

	f.applySource(t, "a", []byte("not an image"))
	rec = f.startEdit("a", `{"source":"thumbnail"}`, f.owner)
	assertErrorBody(t, "unreadable source", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailFrameUnavailable})
}

// 動画のファイルを開けなくても、画像を適用していれば今のサムネイルから始められる（ui-design.md:
// プレイヤーが出ない動画でも始められる）。コマが要るときは従来どおり 404 file_unavailable である。
func TestStartThumbnailEditFromThumbnailWithoutFile(t *testing.T) {
	f := newGuestFixture(t, true)
	f.saveConnection(t)
	video := decode[gen.Video](t, f.env.get(f.videoPath("b", ""), f.owner))
	if err := os.Remove(video.Location.Path); err != nil {
		t.Fatal(err)
	}
	fileUnavailable := wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonFileUnavailable}
	rec := f.startEdit("b", `{"source":"thumbnail"}`, f.owner)
	assertErrorBody(t, "frame-backed thumbnail", rec.Code, rec.Body.Bytes(), fileUnavailable)

	f.applySource(t, "b", editPNG(6, 5))
	rec = f.startEdit("b", `{"source":"thumbnail"}`, f.owner)
	edit := decode[gen.ThumbnailEdit](t, rec)
	if rec.Code != http.StatusCreated || edit.Source != gen.ThumbnailEditSourceThumbnail ||
		edit.FrameWidth != 6 || edit.FrameHeight != 5 {
		t.Fatalf("start from the stored source without the file = %d %s", rec.Code, rec.Body)
	}
	rec = f.startEdit("b", `{"source":"frame","positionMs":1000}`, f.owner)
	assertErrorBody(t, "frame without the file", rec.Code, rec.Body.Bytes(), fileUnavailable)
}
