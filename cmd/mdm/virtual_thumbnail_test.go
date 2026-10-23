package main

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// A virtual video's thumbnail through real stores, ingestion, artifacts and routes: uploaded, served, republished
// when its file is missing, removed, and released with the video.
func TestVirtualVideoThumbnailFromUploadedImage(t *testing.T) {
	f := newThumbnailImageFixture(t)
	virtual, err := f.db.VirtualVideos().Create(f.ctx, "Moonlight")
	if err != nil {
		t.Fatal(err)
	}
	f.videoID = virtual.ID
	key := virtual.ContentKey
	if url := thumbnailURLString(f.video()); url != "" {
		t.Fatalf("画像の無いバーチャルな動画の thumbnailUrl = %q", url)
	}

	picture := virtualUploadPNG(t)
	uploaded := f.decodeVideo(f.upload(key, picture))
	want := "fitted:" + string(picture)
	if uploaded.ThumbnailImage == nil || *uploaded.ThumbnailImage != gen.Uploaded {
		t.Fatalf("thumbnailImage = %v", uploaded.ThumbnailImage)
	}
	if got := f.servedThumbnail(uploaded); got != want {
		t.Fatalf("配ったサムネイル = %q, want アップロードを合わせた画像", got)
	}

	position := f.serve(http.MethodPut, f.videoPath("/thumbnail-position"), `{"positionMs":null}`)
	if position.Code != http.StatusConflict || !bytes.Contains(position.Body.Bytes(), []byte(`"virtual_video"`)) {
		t.Fatalf("フレーム位置: status = %d: %s", position.Code, position.Body)
	}

	// Startup republishes a missing file from the stored image.
	if err := os.Remove(f.thumbnailFilePath(key)); err != nil {
		t.Fatal(err)
	}
	if err := f.ingest.RepublishVirtualThumbnails(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.servedThumbnail(f.video()); got != want {
		t.Fatalf("再公開したサムネイル = %q", got)
	}

	removed := f.decodeVideo(f.serve(http.MethodDelete, f.videoPath("/thumbnail-image"), ""))
	if removed.ThumbnailUrl != nil || removed.ThumbnailImage != nil || f.artifacts.ThumbnailAvailable(key) {
		t.Fatalf("取り除いたあと: thumbnailUrl = %v, ファイル = %v", removed.ThumbnailUrl, f.artifacts.ThumbnailAvailable(key))
	}

	f.decodeVideo(f.upload(key, picture))
	if err := f.db.VirtualVideos().Delete(f.ctx, virtual.ID, key); err != nil {
		t.Fatal(err)
	}
	// The store's deletion event names the key; cmd/mdm routes it to ReleaseArtifacts.
	f.ingest.ReleaseArtifacts(domain.ContentUnreferenced{ContentKeys: []string{key}})
	f.ingest.Wait()
	if f.artifacts.ThumbnailAvailable(key) {
		t.Fatal("削除したバーチャルな動画のサムネイルが残った")
	}
}

func (f *thumbnailImageFixture) upload(contentVersion string, picture []byte) *httptest.ResponseRecorder {
	f.t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("contentVersion", contentVersion); err != nil {
		f.t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image", "cover.png")
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := part.Write(picture); err != nil {
		f.t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		f.t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, f.videoPath("/thumbnail-image"), &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f *thumbnailImageFixture) servedThumbnail(video gen.Video) string {
	f.t.Helper()
	url := thumbnailURLString(video)
	if url == "" {
		f.t.Fatal("thumbnailUrl が無い")
	}
	rec := f.serve(http.MethodGet, url, "")
	if rec.Code != http.StatusOK {
		f.t.Fatalf("GET %s: status = %d: %s", url, rec.Code, rec.Body)
	}
	return rec.Body.String()
}

func (f *thumbnailImageFixture) thumbnailFilePath(key string) string {
	f.t.Helper()
	file, err := f.artifacts.ThumbnailFile(key)
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	return file.Name()
}

func virtualUploadPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
