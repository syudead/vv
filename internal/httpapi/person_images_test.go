package httpapi

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

func newPersonImageFixture(t *testing.T) *creditFixture {
	t.Helper()
	return newPersonImageFixtureChecking(t, nil)
}

func newPersonImageFixtureChecking(t *testing.T, faces app.FaceImageCheck) *creditFixture {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		return Options{Credits: db.Credits(), PersonImages: app.NewPersonImages(db.Credits(), app.NewIngest(app.IngestOptions{}), faces)}
	})
	return &creditFixture{env: env, owner: env.setup()}
}

// refusingFaceCheck refuses every upload with its error, as the face check does for a face it cannot use.
type refusingFaceCheck struct{ err error }

func (c refusingFaceCheck) CheckFaceImage(context.Context, domain.PersonImage) error { return c.err }

func (f *creditFixture) putPersonImage(t *testing.T, id int64) *httptest.ResponseRecorder {
	t.Helper()
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewGray(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "face.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(picture.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return f.env.serve(authRequest{
		method: http.MethodPut, target: creditPath("/api/people", id, "/image"), body: body.String(),
		cookies: []*http.Cookie{f.owner}, header: map[string]string{"Content-Type": writer.FormDataContentType()},
	})
}

func uploadedRevision(t *testing.T, label string, rec *httptest.ResponseRecorder) int64 {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", label, rec.Code, rec.Body)
	}
	person := decode[gen.Person](t, rec)
	if person.ImageRevision == nil {
		t.Fatalf("%s: imageRevision が無い", label)
	}
	return *person.ImageRevision
}

func TestPersonImageRevisionsAcrossUploadAndDelete(t *testing.T) {
	f := newPersonImageFixture(t)
	person := f.createPerson(t, map[string]any{"name": "山田"})
	imagePath := creditPath("/api/people", person.Id, "/image")

	first := uploadedRevision(t, "1 回目のアップロード", f.putPersonImage(t, person.Id))
	got := f.env.get(imagePath, f.owner)
	etag := got.Header().Get("ETag")
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "image/jpeg" ||
		got.Header().Get("Cache-Control") != "private, no-cache" || etag == "" {
		t.Fatalf("GET: status = %d, headers = %v", got.Code, got.Header())
	}
	revalidated := f.env.serve(authRequest{method: http.MethodGet, target: imagePath + "?v=1",
		cookies: []*http.Cookie{f.owner}, header: map[string]string{"If-None-Match": etag}})
	if revalidated.Code != http.StatusNotModified {
		t.Errorf("同じ ETag の GET: status = %d, want 304", revalidated.Code)
	}

	deleted := f.owned(http.MethodDelete, imagePath, "")
	if deleted.Code != http.StatusOK || decode[gen.Person](t, deleted).ImageRevision != nil {
		t.Fatalf("DELETE: status = %d: %s", deleted.Code, deleted.Body)
	}
	gone := f.env.get(imagePath, f.owner)
	assertErrorBody(t, "DELETE 後の GET", gone.Code, gone.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonImageNotFound})

	second := uploadedRevision(t, "削除後のアップロード", f.putPersonImage(t, person.Id))
	if second <= first {
		t.Errorf("削除後の revision = %d, 前の revision %d と別の値になっていない", second, first)
	}
	reused := f.env.serve(authRequest{method: http.MethodGet, target: imagePath,
		cookies: []*http.Cookie{f.owner}, header: map[string]string{"If-None-Match": etag}})
	if reused.Code != http.StatusOK {
		t.Errorf("新しい顔に古い ETag: status = %d, want 200", reused.Code)
	}
}

func TestPersonImageErrors(t *testing.T) {
	f := newPersonImageFixture(t)
	person := f.createPerson(t, map[string]any{"name": "山田"})
	missing := creditPath("/api/people", person.Id+100, "/image")
	notFound := wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound}
	rec := f.env.get(missing, f.owner)
	assertErrorBody(t, "無い人物の GET", rec.Code, rec.Body.Bytes(), notFound)
	rec = f.putPersonImage(t, person.Id+100)
	assertErrorBody(t, "無い人物の PUT", rec.Code, rec.Body.Bytes(), notFound)

	rec = f.env.serve(authRequest{method: http.MethodPut, target: creditPath("/api/people", person.Id, "/image"),
		body:    "--b\r\nContent-Disposition: form-data; name=\"image\"; filename=\"a\"\r\n\r\nnot an image\r\n--b--\r\n",
		cookies: []*http.Cookie{f.owner}, header: map[string]string{"Content-Type": "multipart/form-data; boundary=b"}})
	assertErrorBody(t, "画像でない", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonPersonImageInvalid})
}

func TestRefusedFacesAnswer400AndStoreNothing(t *testing.T) {
	for _, tc := range []struct {
		err    error
		reason gen.ErrorReason
	}{
		{domain.ErrPersonImageNoFace, gen.ErrorReasonPersonImageNoFace},
		{domain.ErrPersonImageManyFaces, gen.ErrorReasonPersonImageManyFaces},
	} {
		t.Run(string(tc.reason), func(t *testing.T) {
			f := newPersonImageFixtureChecking(t, refusingFaceCheck{tc.err})
			person := f.createPerson(t, map[string]any{"name": "山田"})
			rec := f.putPersonImage(t, person.Id)
			assertErrorBody(t, "PUT", rec.Code, rec.Body.Bytes(),
				wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: tc.reason})
			rec = f.env.get(creditPath("/api/people", person.Id, "/image"), f.owner)
			assertErrorBody(t, "GET", rec.Code, rec.Body.Bytes(),
				wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonImageNotFound})
		})
	}
}

func TestPersonImageBodyLimitIs413WithTheLimit(t *testing.T) {
	srv := &server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	if _, ok := srv.readUploadImagePart(rec, errReader{&http.MaxBytesError{Limit: thumbnailUploadMaxBody}}, srv.personImageError); ok {
		t.Fatal("oversized body was accepted")
	}
	assertErrorBody(t, "body limit", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusRequestEntityTooLarge, code: gen.ErrorCodeInvalidRequest,
		reason: gen.ErrorReasonPersonImageTooLarge, limit: domain.ThumbnailUploadMaxBytes,
	})
}

func TestPersonImageRoutesAreOwnerOnly(t *testing.T) {
	handler := newTestServer(t, Options{Auth: guestEventsAuth{}})
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, "/api/people/1/image", nil))
		assertUnauthenticated(t, method, rec)
	}
}
