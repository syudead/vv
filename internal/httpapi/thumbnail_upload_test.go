package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// 手元の画像のアップロードの経路（specs/046-thumbnail-image-upload/contracts/screen-api.md）は、
// 本物の認証・保存層と app.Ingest の確かめと適用で確かめる。ffmpeg の合わせと生成物の置き場だけが
// 偽である。

// uploadFakes は app.Ingest のアップロードを包む。before があれば、経路が動画の内容を読んだあと、
// アップロードを渡す前に呼ぶ。
type uploadFakes struct {
	ingest *app.Ingest

	mu        sync.Mutex
	before    func()
	fitErr    error
	published map[string][]byte
}

func newUploadFakes(t *testing.T, db *store.DB) *uploadFakes {
	t.Helper()
	u := &uploadFakes{published: map[string][]byte{}}
	dir := t.TempDir()
	u.ingest = app.NewIngest(app.IngestOptions{
		Store: db.Ingest(), Generator: uploadGenerator{u: u}, Artifacts: uploadArtifacts{u: u, dir: dir},
	})
	return u
}

func (u *uploadFakes) set(apply func(*uploadFakes)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	apply(u)
}

func (u *uploadFakes) publishedCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.published)
}

func (u *uploadFakes) UploadThumbnailImage(ctx context.Context, videoID int64, contentKey string, image []byte) (domain.Video, error) {
	u.mu.Lock()
	before := u.before
	u.mu.Unlock()
	if before != nil {
		before()
	}
	return u.ingest.UploadThumbnailImage(ctx, videoID, contentKey, image)
}

// uploadGenerator は ffmpeg の代わりに決まった JPEG を返す。ほかの生成は呼ばれない。
type uploadGenerator struct {
	app.Generator
	u *uploadFakes
}

func (g uploadGenerator) FitThumbnailImage(context.Context, []byte, float64) ([]byte, error) {
	return g.fit("fitted")
}

func (g uploadGenerator) FitThumbnailSource(context.Context, []byte, float64) ([]byte, error) {
	return g.fit("source")
}

func (g uploadGenerator) fit(name string) ([]byte, error) {
	g.u.mu.Lock()
	defer g.u.mu.Unlock()
	if g.u.fitErr != nil {
		return nil, g.u.fitErr
	}
	return []byte(name), nil
}

// uploadArtifacts は公開した代表サムネイルを内容ごとに覚える置き場である。
type uploadArtifacts struct {
	app.ArtifactStore
	u   *uploadFakes
	dir string
}

func (a uploadArtifacts) PublishThumbnail(contentKey string, write func(string) error) error {
	output := filepath.Join(a.dir, contentKey+".jpg")
	if err := write(output); err != nil {
		return err
	}
	data, err := os.ReadFile(output)
	if err != nil {
		return err
	}
	a.u.mu.Lock()
	defer a.u.mu.Unlock()
	a.u.published[contentKey] = data
	return nil
}

func (uploadArtifacts) StashThumbnail(string) (func() error, func(), error) {
	return func() error { return nil }, func() {}, nil
}

func (uploadArtifacts) RemoveContent(string) error {
	return nil
}

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

// upload は image を動画 name のサムネイルとして送る。ctx は要求の文脈である。
func (f *guestFixture) uploadWith(
	ctx context.Context, name string, body io.Reader, contentType string, cookies ...*http.Cookie,
) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, f.videoPath(name, "/thumbnail-image"), body)
	r.Header.Set("Content-Type", contentType)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.env.handler.ServeHTTP(rec, r)
	return rec
}

func (f *guestFixture) upload(t *testing.T, name string, image []byte, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := uploadBody(t, image)
	return f.uploadWith(context.Background(), name, body, contentType, cookies...)
}

// appliedImage は動画 name の内容に記録された画像の行の代表サムネイルを返す。無ければ nil。
func (f *guestFixture) appliedImage(t *testing.T, name string) []byte {
	t.Helper()
	data, err := f.env.db.Ingest().ThumbnailImage(context.Background(), "content-key-"+name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// assertNothingWritten は画像の行も代表サムネイルのファイルも書いていないことを確かめる。
func (f *guestFixture) assertNothingWritten(t *testing.T, label string) {
	t.Helper()
	if image := f.appliedImage(t, "a"); image != nil || f.uploads.publishedCount() != 0 {
		t.Errorf("%s: 書いた: 行 %q, 公開 %d 件", label, image, f.uploads.publishedCount())
	}
}

var uploadStillWebP = []byte("RIFF\x1a\x00\x00\x00WEBPVP8L\x0d\x00\x00\x00\x2f\x00\x00\x00\x10\x07\x10\x11\x11\x88\x88\xfe\x07\x00")

func uploadTestPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 6))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func uploadTestJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 64, 48)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// PNG・JPEG・WebP はそれぞれ 200 で、thumbnailImage: "uploaded" と新しい版の thumbnailUrl を返す。
// 合わせた画像を公開し、同じものを記録する。
func TestUploadThumbnailImageFormats(t *testing.T) {
	for _, tc := range []struct {
		name  string
		image func(t *testing.T) []byte
	}{
		{name: "PNG", image: uploadTestPNG},
		{name: "JPEG", image: uploadTestJPEG},
		{name: "WebP", image: func(*testing.T) []byte { return uploadStillWebP }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGuestFixture(t, true)
			before := decode[gen.Video](t, f.env.get(f.videoPath("a", ""), f.owner))

			rec := f.upload(t, "a", tc.image(t), f.owner)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body)
			}
			video := decode[gen.Video](t, rec)
			if video.ThumbnailImage == nil || *video.ThumbnailImage != gen.Uploaded {
				t.Errorf("thumbnailImage = %v", video.ThumbnailImage)
			}
			if got := thumbnailURLOf(video); got == thumbnailURLOf(before) || !strings.Contains(got, "-g") {
				t.Errorf("thumbnailUrl = %q, 前 = %q", got, thumbnailURLOf(before))
			}
			if video.Location == nil {
				t.Errorf("応答が詳細と同じ形でない: %+v", video)
			}
			if got := f.appliedImage(t, "a"); string(got) != "fitted" {
				t.Errorf("記録した画像 = %q", got)
			}
		})
	}
}

// 画像として読めないものは 400 thumbnail_upload_invalid、上限を越えるものは越えた上限を添えた
// 400 thumbnail_upload_too_large で、どれも何も書かない。
func TestUploadThumbnailImageRejects(t *testing.T) {
	f := newGuestFixture(t, true)
	jpegImage := uploadTestJPEG(t)
	animated := []byte("RIFF\x00\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	animated = append(animated, uploadStillWebP[12:]...)
	binary.LittleEndian.PutUint32(animated[4:8], uint32(len(animated)-8))
	// 見出しだけが 12000×11000 を名乗る PNG。
	huge := uploadTestPNG(t)
	binary.BigEndian.PutUint32(huge[16:20], 12000)
	binary.BigEndian.PutUint32(huge[20:24], 11000)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))

	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonThumbnailUploadInvalid}
	for _, tc := range []struct {
		name  string
		image []byte
		want  wantError
	}{
		{name: "テキスト", image: []byte("just some text"), want: invalid},
		{name: "途中で切れた JPEG", image: jpegImage[:len(jpegImage)/2], want: invalid},
		{name: "アニメーションの WebP", image: animated, want: invalid},
		{name: "12000×11000", image: huge, want: wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
			reason: reasonThumbnailUploadTooLarge, limit: domain.ThumbnailUploadMaxPixels,
		}},
		{name: "64 MiB を越える", image: make([]byte, domain.ThumbnailUploadMaxBytes+1), want: wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
			reason: reasonThumbnailUploadTooLarge, limit: domain.ThumbnailUploadMaxBytes,
		}},
	} {
		rec := f.upload(t, "a", tc.image, f.owner)
		assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), tc.want)
		f.assertNothingWritten(t, tc.name)
	}

	// image の無い本文とほかの部分は形の誤りである。
	for name, extra := range map[string][]string{"image なし": nil, "ほかの部分": {"prompt"}} {
		var image []byte
		if name == "ほかの部分" {
			image = uploadTestPNG(t)
		}
		body, contentType := uploadBody(t, image, extra...)
		rec := f.uploadWith(context.Background(), "a", body, contentType, f.owner)
		assertErrorBody(t, name, rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
		f.assertNothingWritten(t, name)
	}

	// contentVersion の無い本文と 64 バイトを越える contentVersion も形の誤りである。
	long := strings.Repeat("x", 65)
	for name, version := range map[string]*string{"contentVersion なし": nil, "長い contentVersion": &long} {
		body, contentType := uploadBodyVersion(t, uploadTestPNG(t), version)
		rec := f.uploadWith(context.Background(), "a", body, contentType, f.owner)
		assertErrorBody(t, name, rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
		f.assertNothingWritten(t, name)
	}
}

// プレビューを開いたあとに内容が替わった動画（contentVersion が今の内容と違う）には適用しない。
func TestUploadThumbnailImageRefusesStaleContentVersion(t *testing.T) {
	f := newGuestFixture(t, true)
	stale := "content-old-"
	body, contentType := uploadBodyVersion(t, uploadTestPNG(t), &stale)
	rec := f.uploadWith(context.Background(), "a", body, contentType, f.owner)
	assertErrorBody(t, "古い内容", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonContentChanged})
	f.assertNothingWritten(t, "古い内容")
}

// ちょうど 64 MiB の image 部分は大きさで断られない（後ろを埋めた PNG は最後まで読める）。
func TestUploadThumbnailImageExactly64MiB(t *testing.T) {
	f := newGuestFixture(t, true)
	image := make([]byte, domain.ThumbnailUploadMaxBytes)
	copy(image, uploadTestPNG(t))
	rec := f.upload(t, "a", image, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
}

// 本文を送り終えたあとに接続が切れても、確かめと適用は最後まで行う。
func TestUploadThumbnailImageSurvivesDisconnect(t *testing.T) {
	f := newGuestFixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.uploads.set(func(u *uploadFakes) { u.before = cancel })

	body, contentType := uploadBody(t, uploadTestPNG(t))
	f.uploadWith(ctx, "a", body, contentType, f.owner)
	if got := f.appliedImage(t, "a"); string(got) != "fitted" {
		t.Fatalf("切断後に適用されていない: %q", got)
	}
}

// 本文が届く前に切れた要求は適用しない。
func TestUploadThumbnailImageCutOffBody(t *testing.T) {
	f := newGuestFixture(t, true)
	body, contentType := uploadBody(t, uploadTestPNG(t))
	cut := io.MultiReader(bytes.NewReader(body.Bytes()[:body.Len()/2]), errReader{errors.New("connection reset")})
	rec := f.uploadWith(context.Background(), "a", cut, contentType, f.owner)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d: %s", rec.Code, rec.Body)
	}
	f.assertNothingWritten(t, "途中で切れた本文")

	// image 部分を読んでいる途中で切れても同じである。
	body, contentType = uploadBody(t, uploadTestPNG(t))
	start := bytes.Index(body.Bytes(), []byte("\x89PNG"))
	cut = io.MultiReader(bytes.NewReader(body.Bytes()[:start+4]), errReader{errors.New("connection reset")})
	rec = f.uploadWith(context.Background(), "a", cut, contentType, f.owner)
	assertErrorBody(t, "image の途中で切れた本文", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	f.assertNothingWritten(t, "image の途中で切れた本文")
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// 要求の始めに読んだあとで内容の変わった動画は 409 content_changed、ffmpeg が合わせられなければ
// 409 thumbnail_image_invalid で、どちらも何も書かない。無い動画は 404 video_not_found である。
func TestUploadThumbnailImageConflicts(t *testing.T) {
	f := newGuestFixture(t, true)

	f.uploads.set(func(u *uploadFakes) { u.fitErr = domain.ErrThumbnailImageInvalid })
	rec := f.upload(t, "a", uploadTestPNG(t), f.owner)
	assertErrorBody(t, "合わせの失敗", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailImageInvalid})
	f.assertNothingWritten(t, "合わせの失敗")
	f.uploads.set(func(u *uploadFakes) { u.fitErr = nil })

	// 経路が内容を読んだあと、同じ所在のファイルが別の内容に変わる（スキャンが付け替える）。
	f.uploads.set(func(u *uploadFakes) {
		u.before = func() {
			video, err := f.env.db.Ingest().GetVideo(context.Background(), f.ids["a"])
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := f.env.db.ScanIndex().UpsertVideo(context.Background(), domain.VideoFile{
				Path: video.Path, Title: "a", ContentKey: "content-key-changed", SizeBytes: 1,
				MTime: video.MTime, AddedAt: video.AddedAt, Container: "mp4",
			}); err != nil {
				t.Error(err)
			}
		}
	})
	rec = f.upload(t, "a", uploadTestPNG(t), f.owner)
	assertErrorBody(t, "内容の変化", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonContentChanged})
	// 所在が別の内容になった動画は、消えたか別の内容を持つ。
	if after, err := f.env.db.Ingest().GetVideo(context.Background(), f.ids["a"]); err == nil && after.ContentKey == "content-key-a" {
		t.Fatal("前提: 内容が変わっていない")
	}
	f.assertNothingWritten(t, "内容の変化")
	if data, err := f.env.db.Ingest().ThumbnailImage(context.Background(), "content-key-changed"); err != nil || data != nil {
		t.Errorf("変わった内容に書いた: %q, %v", data, err)
	}

	missing := httptest.NewRecorder()
	body, contentType := uploadBody(t, uploadTestPNG(t))
	r := httptest.NewRequest(http.MethodPost, "/api/videos/9999/thumbnail-image", body)
	r.Header.Set("Content-Type", contentType)
	r.AddCookie(f.owner)
	f.env.handler.ServeHTTP(missing, r)
	assertErrorBody(t, "無い動画", missing.Code, missing.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound})
}

// ゲストは 401 で、何も書かない。
func TestUploadThumbnailImageIsOwnerOnly(t *testing.T) {
	f := newGuestFixture(t, true)
	assertUnauthenticated(t, "Cookie なし", f.upload(t, "a", uploadTestPNG(t)))
	f.assertNothingWritten(t, "ゲスト")
}

// image 部分の外で本文全体が上限を越えても（ここでは最初の区切りの前の長い前置き）、
// 400 thumbnail_upload_too_large に image の上限を添え、何も書かない。
func TestUploadThumbnailImageBodyOverLimitOutsideImage(t *testing.T) {
	f := newGuestFixture(t, true)
	line := strings.Repeat("x", 1022) + "\r\n"
	preamble := strings.Repeat(line, int(thumbnailUploadMaxBody)/len(line)+2)
	body, contentType := uploadBody(t, uploadTestPNG(t))
	rec := f.uploadWith(context.Background(), "a", io.MultiReader(strings.NewReader(preamble), body), contentType, f.owner)
	assertErrorBody(t, "前置きで越える本文", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
		reason: reasonThumbnailUploadTooLarge, limit: domain.ThumbnailUploadMaxBytes,
	})
	f.assertNothingWritten(t, "前置きで越える本文")
}

// 長さの分からない本文は 1 件で本文の上限ぶんの枠を取るので、確かめと適用を待つ件数が枠を
// 使い切っている間、次の 1 件は本文を読まずに待つ。1 件が終われば次が進む。
func TestUploadThumbnailImageBodyBudget(t *testing.T) {
	f := newGuestFixture(t, true)
	started := make(chan struct{}, 8)
	release := make(chan struct{}, 8)
	f.uploads.set(func(u *uploadFakes) {
		u.before = func() {
			started <- struct{}{}
			<-release
		}
	})
	send := func() <-chan int {
		done := make(chan int, 1)
		body, contentType := uploadBody(t, uploadTestPNG(t))
		go func() {
			// MultiReader で包むと、要求は長さの分からない本文になる。
			done <- f.uploadWith(context.Background(), "a", io.MultiReader(body), contentType, f.owner).Code
		}()
		return done
	}
	waitStarted := func(label string) {
		t.Helper()
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: アップロードが本文を読み終えない", label)
		}
	}

	var done []<-chan int
	for range thumbnailUploadBufferBudget / thumbnailUploadMaxBody {
		done = append(done, send())
		waitStarted("枠の中")
	}
	done = append(done, send())
	select {
	case <-started:
		t.Fatal("枠を越えるアップロードが本文を読んだ")
	case <-time.After(100 * time.Millisecond):
	}

	release <- struct{}{}
	waitStarted("枠が空いたあと")
	for range done[1:] {
		release <- struct{}{}
	}
	for _, d := range done {
		if code := <-d; code != http.StatusOK {
			t.Errorf("status = %d", code)
		}
	}
}

// 本文が枠から取るのは、上限以下の Content-Length ならその長さ、無いか上限を越えれば本文の上限である。
func TestThumbnailUploadWeight(t *testing.T) {
	for _, tc := range []struct {
		length int64
		want   int64
	}{
		{length: 1024, want: 1024},
		{length: thumbnailUploadMaxBody, want: thumbnailUploadMaxBody},
		{length: thumbnailUploadMaxBody + 1, want: thumbnailUploadMaxBody},
		{length: -1, want: thumbnailUploadMaxBody},
	} {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.ContentLength = tc.length
		if got := thumbnailUploadWeight(r); got != tc.want {
			t.Errorf("Content-Length %d: weight = %d, want %d", tc.length, got, tc.want)
		}
	}
}
