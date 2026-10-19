package httpapi

import (
	"bytes"
	"context"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

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

func (f *guestFixture) startEditFromImage(t *testing.T, name, part string, image []byte, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	file, err := writer.CreateFormFile(part, "framing.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(image); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, f.videoPath(name, "/thumbnail-edits/image"), &buf)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.env.handler.ServeHTTP(rec, r)
	return rec
}

// A preview's framing starts a session without the media file, and its frame is the sent image.
func TestStartThumbnailEditFromImage(t *testing.T) {
	f := newGuestFixture(t, true)
	f.saveConnection(t)
	assertUnauthenticated(t, "guest", f.startEditFromImage(t, "b", "image", editPNG(6, 5)))
	video := decode[gen.Video](t, f.env.get(f.videoPath("b", ""), f.owner))
	if err := os.Remove(video.Location.Path); err != nil {
		t.Fatal(err)
	}

	rec := f.startEditFromImage(t, "b", "image", editPNG(6, 5), f.owner)
	edit := decode[gen.ThumbnailEdit](t, rec)
	if rec.Code != http.StatusCreated || edit.Source != gen.ThumbnailEditSourceImage ||
		edit.PositionMs != nil || edit.FrameWidth != 6 || edit.FrameHeight != 5 {
		t.Fatalf("start from an image = %d %s", rec.Code, rec.Body)
	}
	frame := f.editRequest(http.MethodGet, edit.Id, "/frame", f.owner)
	if frame.Code != http.StatusOK || !bytes.Equal(frame.Body.Bytes(), editPNG(6, 5)) {
		t.Fatalf("frame = %d", frame.Code)
	}

	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	rec = f.startEditFromImage(t, "b", "image", []byte("not an image"), f.owner)
	assertErrorBody(t, "unreadable image", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonThumbnailUploadInvalid})
	rec = f.startEditFromImage(t, "b", "reference", editPNG(6, 5), f.owner)
	assertErrorBody(t, "another part", rec.Code, rec.Body.Bytes(), invalid)
	rec = f.startEdit("a", `{"source":"image"}`, f.owner)
	assertErrorBody(t, "image through the JSON start", rec.Code, rec.Body.Bytes(), invalid)
	if rec := f.editRequest(http.MethodGet, edit.Id, "", f.owner); rec.Code != http.StatusOK {
		t.Fatalf("a refused start ended the session: %d", rec.Code)
	}
}
