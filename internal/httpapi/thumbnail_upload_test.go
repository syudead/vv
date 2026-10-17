package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// uploadContentVersion は試験の動画の内容（content-key-<名前>）の版で、どの動画にも合う。
const uploadContentVersion = "content-key-"

// uploadBody は image を image 部分、uploadContentVersion を contentVersion 部分（extra があればその
// 部分も）にした multipart の本文と Content-Type を返す。
func uploadBody(t *testing.T, image []byte, extra ...string) (*bytes.Buffer, string) {
	t.Helper()
	version := uploadContentVersion
	return uploadBodyVersion(t, image, &version, extra...)
}

// uploadBodyVersion は uploadBody の contentVersion を version にする。nil なら送らない。
func uploadBodyVersion(t *testing.T, image []byte, version *string, extra ...string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if version != nil {
		if err := writer.WriteField("contentVersion", *version); err != nil {
			t.Fatal(err)
		}
	}
	if image != nil {
		part, err := writer.CreateFormFile("image", "photo")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(image); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range extra {
		if err := writer.WriteField(name, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, writer.FormDataContentType()
}

// thumbnailUploadFunc supplies application results without image decoding or storage.
type thumbnailUploadFunc func(context.Context, int64, string, []byte) (domain.Video, error)

func (f thumbnailUploadFunc) UploadThumbnailImage(ctx context.Context, id int64, key string, image []byte) (domain.Video, error) {
	return f(ctx, id, key, image)
}

func uploadVideo() domain.Video {
	video := sampleVideo(1, "a")
	video.ContentKey = "content-key-a"
	return video
}

func newUploadHandler(t *testing.T, upload thumbnailUploadFunc) http.Handler {
	t.Helper()
	return newTestServer(t, Options{
		Videos:           &fakeLibrary{videos: map[int64]domain.Video{1: uploadVideo()}},
		ThumbnailUploads: upload,
	})
}

func sendUpload(ctx context.Context, handler http.Handler, body io.Reader, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/videos/1/thumbnail-image", body)
	r.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	return rec
}

func TestUploadThumbnailImagePassesImageAndWritesDetail(t *testing.T) {
	image := []byte("image supplied to the application")
	calls := 0
	handler := newUploadHandler(t, func(ctx context.Context, id int64, key string, got []byte) (domain.Video, error) {
		calls++
		if id != 1 || key != "content-key-a" || !bytes.Equal(got, image) {
			t.Errorf("upload(%d, %q, %q)", id, key, got)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("application context has no deadline")
		}
		saved := uploadVideo()
		saved.ThumbnailImageOrigin = domain.ThumbnailImageUploaded
		saved.ThumbnailImageRevision = 2
		return saved, nil
	})
	body, contentType := uploadBody(t, image)
	rec := sendUpload(context.Background(), handler, body, contentType)
	if rec.Code != http.StatusOK || calls != 1 {
		t.Fatalf("status = %d, calls = %d: %s", rec.Code, calls, rec.Body)
	}
	video := decode[gen.Video](t, rec)
	if video.ThumbnailImage == nil || *video.ThumbnailImage != gen.Uploaded || video.Location == nil ||
		!strings.Contains(thumbnailURLOf(video), "-g2") {
		t.Errorf("detail = %+v", video)
	}
}

func TestUploadThumbnailImageMapsApplicationErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want wantError
	}{
		{"invalid", domain.ErrThumbnailUploadInvalid, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonThumbnailUploadInvalid}},
		{"bytes", &domain.ThumbnailUploadTooLargeError{Limit: domain.ThumbnailUploadMaxBytes}, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonThumbnailUploadTooLarge, limit: domain.ThumbnailUploadMaxBytes}},
		{"pixels", &domain.ThumbnailUploadTooLargeError{Limit: domain.ThumbnailUploadMaxPixels}, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonThumbnailUploadTooLarge, limit: domain.ThumbnailUploadMaxPixels}},
		{"changed", domain.ErrContentChanged, wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonContentChanged}},
		{"fit", domain.ErrThumbnailImageInvalid, wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailImageInvalid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			handler := newUploadHandler(t, func(context.Context, int64, string, []byte) (domain.Video, error) {
				calls++
				return domain.Video{}, tc.err
			})
			body, contentType := uploadBody(t, []byte("image"))
			rec := sendUpload(context.Background(), handler, body, contentType)
			assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), tc.want)
			if calls != 1 {
				t.Errorf("application calls = %d, want 1", calls)
			}
		})
	}
}

func TestUploadThumbnailImageRejectsMalformedParts(t *testing.T) {
	calls := 0
	handler := newUploadHandler(t, func(context.Context, int64, string, []byte) (domain.Video, error) {
		calls++
		return uploadVideo(), nil
	})
	long := strings.Repeat("x", thumbnailUploadMaxContentVersion+1)
	for _, tc := range []struct {
		name    string
		image   []byte
		version *string
		extra   []string
	}{
		{"missing image", nil, new(uploadContentVersion), nil},
		{"missing version", []byte("image"), nil, nil},
		{"long version", []byte("image"), &long, nil},
		{"unknown part", []byte("image"), new(uploadContentVersion), []string{"prompt"}},
		{"duplicate image", []byte("image"), new(uploadContentVersion), []string{"image"}},
	} {
		body, contentType := uploadBodyVersion(t, tc.image, tc.version, tc.extra...)
		rec := sendUpload(context.Background(), handler, body, contentType)
		assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}
	if calls != 0 {
		t.Errorf("malformed requests reached application %d times", calls)
	}
}

func TestUploadThumbnailImageRefusesStaleContentVersion(t *testing.T) {
	calls := 0
	handler := newUploadHandler(t, func(context.Context, int64, string, []byte) (domain.Video, error) {
		calls++
		return uploadVideo(), nil
	})
	stale := "content-old-"
	body, contentType := uploadBodyVersion(t, []byte("image"), &stale)
	rec := sendUpload(context.Background(), handler, body, contentType)
	assertErrorBody(t, "stale version", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonContentChanged})
	if calls != 0 {
		t.Errorf("stale request reached application %d times", calls)
	}
}

func TestUploadThumbnailImageSurvivesDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	handler := newUploadHandler(t, func(appCtx context.Context, _ int64, _ string, _ []byte) (domain.Video, error) {
		calls++
		cancel()
		if appCtx.Err() != nil {
			t.Errorf("application cancelled with request: %v", appCtx.Err())
		}
		return uploadVideo(), nil
	})
	body, contentType := uploadBody(t, []byte("image"))
	rec := sendUpload(ctx, handler, body, contentType)
	if rec.Code != http.StatusOK || calls != 1 {
		t.Fatalf("status = %d, calls = %d", rec.Code, calls)
	}
}

func TestUploadThumbnailImageCutOffBody(t *testing.T) {
	calls := 0
	handler := newUploadHandler(t, func(context.Context, int64, string, []byte) (domain.Video, error) {
		calls++
		return uploadVideo(), nil
	})
	for _, cutInImage := range []bool{false, true} {
		body, contentType := uploadBody(t, []byte("test image"))
		end := body.Len() / 2
		if cutInImage {
			end = bytes.Index(body.Bytes(), []byte("test image")) + 4
		}
		cut := io.MultiReader(bytes.NewReader(body.Bytes()[:end]), errReader{errors.New("connection reset")})
		rec := sendUpload(context.Background(), handler, cut, contentType)
		assertErrorBody(t, "cut off", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}
	if calls != 0 {
		t.Errorf("incomplete request reached application %d times", calls)
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestUploadThumbnailImageIsOwnerOnly(t *testing.T) {
	calls := 0
	handler := newTestServer(t, Options{Auth: guestEventsAuth{}, ThumbnailUploads: thumbnailUploadFunc(func(context.Context, int64, string, []byte) (domain.Video, error) {
		calls++
		return uploadVideo(), nil
	})})
	body, contentType := uploadBody(t, []byte("image"))
	assertUnauthenticated(t, "guest upload", sendUpload(context.Background(), handler, body, contentType))
	if calls != 0 {
		t.Errorf("guest reached application %d times", calls)
	}
}

func TestUploadThumbnailImageMissingVideo(t *testing.T) {
	calls := 0
	handler := newUploadHandler(t, func(context.Context, int64, string, []byte) (domain.Video, error) {
		calls++
		return uploadVideo(), nil
	})
	body, contentType := uploadBody(t, []byte("image"))
	r := httptest.NewRequest(http.MethodPost, "/api/videos/9999/thumbnail-image", body)
	r.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	assertErrorBody(t, "missing video", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound})
	if calls != 0 {
		t.Errorf("missing video reached application %d times", calls)
	}
}

// synctest waits for the fifth request to block on the buffer budget, not for a timer.
func TestUploadThumbnailImageBodyBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{}, 5)
		release := make(chan struct{})
		defer close(release)
		handler := newUploadHandler(t, func(context.Context, int64, string, []byte) (domain.Video, error) {
			started <- struct{}{}
			<-release
			return uploadVideo(), nil
		})
		send := func() <-chan int {
			body, contentType := uploadBody(t, []byte("image"))
			done := make(chan int, 1)
			go func() {
				done <- sendUpload(context.Background(), handler, io.MultiReader(body), contentType).Code
			}()
			return done
		}
		var done []<-chan int
		for range thumbnailUploadBufferBudget / thumbnailUploadMaxBody {
			done = append(done, send())
			<-started
		}
		done = append(done, send())
		synctest.Wait()
		select {
		case <-started:
			t.Fatal("request exceeded the buffer budget")
		default:
		}
		release <- struct{}{}
		<-started
		for range done[1:] {
			release <- struct{}{}
		}
		for _, completed := range done {
			if code := <-completed; code != http.StatusOK {
				t.Errorf("status = %d", code)
			}
		}
	})
}

func TestThumbnailUploadWeight(t *testing.T) {
	for _, tc := range []struct{ length, want int64 }{
		{1024, 1024}, {thumbnailUploadMaxBody, thumbnailUploadMaxBody},
		{thumbnailUploadMaxBody + 1, thumbnailUploadMaxBody}, {-1, thumbnailUploadMaxBody},
	} {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.ContentLength = tc.length
		if got := thumbnailUploadWeight(r); got != tc.want {
			t.Errorf("Content-Length %d: weight = %d, want %d", tc.length, got, tc.want)
		}
	}
}
