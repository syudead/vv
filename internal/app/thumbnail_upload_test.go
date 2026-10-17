package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 手元の画像のアップロード。

// stillWebP は 1×1 の可逆の WebP である。
var stillWebP = []byte("RIFF\x1a\x00\x00\x00WEBPVP8L\x0d\x00\x00\x00\x2f\x00\x00\x00\x10\x07\x10\x11\x11\x88\x88\xfe\x07\x00")

func uploadPNG(t *testing.T) []byte {
	t.Helper()
	return testPNG(t, 8, 6)
}

func uploadJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 64, 48)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// animatedWebPImage は、拡張形式の見出しでアニメーションの印を立てた WebP である。
func animatedWebPImage() []byte {
	chunks := []byte("VP8X\x0a\x00\x00\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	chunks = append(chunks, []byte("ANIM\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00")...)
	chunks = append(chunks, stillWebP[12:]...)
	data := append([]byte("RIFF\x00\x00\x00\x00WEBP"), chunks...)
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	return data
}

// 確かめは PNG・JPEG・静止画の WebP を通し、ほかの形式・途中で切れた画像・アニメーションの WebP を
// 読めないもの、画素数か大きさの上限を越える画像をその上限を添えた大きすぎるものとして断る。
func TestCheckThumbnailUpload(t *testing.T) {
	jpegImage := uploadJPEG(t)
	for _, tc := range []struct {
		name      string
		image     []byte
		want      error
		wantLimit int
	}{
		{name: "PNG", image: uploadPNG(t)},
		{name: "JPEG", image: jpegImage},
		{name: "WebP", image: stillWebP},
		{name: "テキスト", image: []byte("just some text"), want: domain.ErrThumbnailUploadInvalid},
		{name: "途中で切れた JPEG", image: jpegImage[:len(jpegImage)/2], want: domain.ErrThumbnailUploadInvalid},
		{name: "アニメーションの WebP", image: animatedWebPImage(), want: domain.ErrThumbnailUploadInvalid},
		{name: "12000×11000", image: pngHeaderSized(t, 12000, 11000),
			want: domain.ErrThumbnailUploadTooLarge, wantLimit: domain.ThumbnailUploadMaxPixels},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkThumbnailUpload(tc.image)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			var tooLarge *domain.ThumbnailUploadTooLargeError
			if tc.wantLimit != 0 && (!errors.As(err, &tooLarge) || tooLarge.Limit != tc.wantLimit) {
				t.Fatalf("err = %v, want limit %d", err, tc.wantLimit)
			}
		})
	}
}

func TestCheckThumbnailUploadSize(t *testing.T) {
	for _, size := range []int{0, domain.ThumbnailUploadMaxBytes - 1, domain.ThumbnailUploadMaxBytes} {
		if err := checkThumbnailUploadSize(size); err != nil {
			t.Errorf("size %d: %v", size, err)
		}
	}
	err := checkThumbnailUploadSize(domain.ThumbnailUploadMaxBytes + 1)
	var tooLarge *domain.ThumbnailUploadTooLargeError
	if !errors.As(err, &tooLarge) || tooLarge.Limit != domain.ThumbnailUploadMaxBytes {
		t.Fatalf("over limit: %v", err)
	}
}

// 編集の画像の確かめは、アップロードと読み方を共有したまま 8192×8192 の上限を保つ。
func TestCheckThumbnailEditImageKeepsItsPixelCap(t *testing.T) {
	if err := checkThumbnailEditImage(pngHeaderSized(t, 8193, 8192)); !errors.Is(err, errThumbnailEditImageUnreadable) {
		t.Fatalf("err = %v", err)
	}
}

// 確かめた画像は出どころ uploaded で適用する。断った画像は合わせも記録もしない。
func TestUploadThumbnailImageAppliesAsUploaded(t *testing.T) {
	video := sizedVideo(1, "a", 1920, 1080)
	store := newFakeIngestStore(video)
	generator := &fakeGenerator{thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)

	if _, err := ingest.UploadThumbnailImage(context.Background(), video.ID, "a", []byte("text")); !errors.Is(err, domain.ErrThumbnailUploadInvalid) {
		t.Fatalf("err = %v", err)
	}
	if calls, _ := generator.snapshot(); len(calls) != 0 || len(store.rows) != 0 {
		t.Fatalf("断った画像で 呼び出し = %v, 行 = %v", calls, store.rows)
	}

	image := uploadPNG(t)
	saved, err := ingest.UploadThumbnailImage(context.Background(), video.ID, "a", image)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ThumbnailImageOrigin != domain.ThumbnailImageUploaded || store.rows["a"].Origin != domain.ThumbnailImageUploaded {
		t.Fatalf("出どころ = %q / %q", saved.ThumbnailImageOrigin, store.rows["a"].Origin)
	}
	if !bytes.Equal(store.rows["a"].Image, append([]byte("fitted:"), image...)) {
		t.Fatal("記録した画像がアップロードを合わせたものでない")
	}

	if _, err := ingest.UploadThumbnailImage(context.Background(), video.ID, "gone", image); !errors.Is(err, domain.ErrContentChanged) {
		t.Fatalf("内容の変わった動画: err = %v", err)
	}
}

// uploadChecks はアップロードの確かめを記録し、止めておく偽物である。block にある画像の確かめは、
// 始めたことを started で知らせ、release が閉じるまで終わらない。
type uploadChecks struct {
	mu      sync.Mutex
	checked []string
	started map[string]chan struct{}
	release map[string]chan struct{}
}

func newUploadChecks(block ...string) *uploadChecks {
	c := &uploadChecks{started: map[string]chan struct{}{}, release: map[string]chan struct{}{}}
	for _, image := range block {
		c.started[image] = make(chan struct{})
		c.release[image] = make(chan struct{})
	}
	return c
}

func (c *uploadChecks) check(image []byte) error {
	name := string(image)
	c.mu.Lock()
	c.checked = append(c.checked, name)
	c.mu.Unlock()
	if started, ok := c.started[name]; ok {
		close(started)
		<-c.release[name]
	}
	return nil
}

func (c *uploadChecks) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.checked...)
}

func uploadAsync(ingest *Ingest, videoID int64, key, image string) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := ingest.UploadThumbnailImage(context.Background(), videoID, key, []byte(image))
		done <- err
	}()
	return done
}

// fitted は画像 image を合わせ始めたかを返す。
func fitted(generator *fakeGenerator, image string) bool {
	generator.mu.Lock()
	defer generator.mu.Unlock()
	return slices.Contains(generator.fitImages, image)
}

// 1 件目が合わせている間、別の動画への 2 件目の確かめは門を待つ。
func TestUploadCheckWaitsForAnotherUploadsFit(t *testing.T) {
	store := newFakeIngestStore(probedVideo(1, "a"), probedVideo(2, "b"))
	hold := newHoldGeneration("fit:one")
	generator := &fakeGenerator{gate: hold.gate, thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)
	checks := newUploadChecks()
	ingest.checkUpload = checks.check

	first := uploadAsync(ingest, 1, "a", "one")
	<-hold.started["fit:one"]
	second := uploadAsync(ingest, 2, "b", "two")
	time.Sleep(50 * time.Millisecond)
	if got := checks.snapshot(); !slices.Equal(got, []string{"one"}) {
		t.Fatalf("1 件目の合わせの間に 2 件目を確かめた: %v", got)
	}

	close(hold.release["fit:one"])
	for _, done := range []<-chan error{first, second} {
		if err := waitJob(t, done); err != nil {
			t.Fatal(err)
		}
	}
	if got := checks.snapshot(); !slices.Equal(got, []string{"one", "two"}) {
		t.Fatalf("確かめ = %v", got)
	}
}

// 自分の動画の錠を待つアップロードは門を持たないので、その間に別の動画へのアップロードが確かめに
// 入れる。錠を取ったあとの合わせは、確かめている 1 件目が門を放すまで待つ。
func TestUploadWaitingOnLockDoesNotHoldGate(t *testing.T) {
	store := newFakeIngestStore(probedVideo(1, "a"), probedVideo(2, "b"))
	hold := newHoldGeneration("thumbnail-at")
	generator := &fakeGenerator{gate: hold.gate, thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)
	checks := newUploadChecks("one")
	ingest.checkUpload = checks.check

	// 動画 b の錠を位置の指定が持ったままにする。
	position := make(chan error, 1)
	go func() {
		_, err := ingest.SetThumbnailPosition(context.Background(), 2, "/media/show/b.mp4", "/media/show/b.mp4", int64Ptr(5_000))
		position <- err
	}()
	<-hold.started["thumbnail-at"]
	// 2 件目（動画 b）は確かめを終えて錠を待つ。
	second := uploadAsync(ingest, 2, "b", "two")
	waitUntil(t, func() bool { return slices.Contains(checks.snapshot(), "two") })

	// 錠を待つ 2 件目は門を持たないので、1 件目（動画 a）が確かめに入れる。
	first := uploadAsync(ingest, 1, "a", "one")
	select {
	case <-checks.started["one"]:
	case <-time.After(2 * time.Second):
		t.Fatal("錠を待つアップロードが門を持っている")
	}

	// 錠が空いても、2 件目の合わせは 1 件目の確かめが終わるまで待つ。
	close(hold.release["thumbnail-at"])
	if err := waitJob(t, position); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if fitted(generator, "two") {
		t.Fatal("1 件目の確かめの間に 2 件目を合わせた")
	}

	close(checks.release["one"])
	for _, done := range []<-chan error{first, second} {
		if err := waitJob(t, done); err != nil {
			t.Fatal(err)
		}
	}
	if store.rows["b"].Origin != domain.ThumbnailImageUploaded {
		t.Fatalf("動画 b の行 = %+v", store.rows["b"])
	}
}

// 位置の指定の後ろで錠を待ったアップロードは、あとで確定するので最後に勝つ。位置は記録されたまま
// 残る。
func TestUploadAfterPositionChangeWins(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	hold := newHoldGeneration("thumbnail-at")
	generator := &fakeGenerator{gate: hold.gate, thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)
	checks := newUploadChecks()
	ingest.checkUpload = checks.check

	position := make(chan error, 1)
	go func() {
		_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(5_000))
		position <- err
	}()
	<-hold.started["thumbnail-at"]
	upload := uploadAsync(ingest, video.ID, "a", "photo")
	waitUntil(t, func() bool { return slices.Contains(checks.snapshot(), "photo") })
	time.Sleep(20 * time.Millisecond)

	close(hold.release["thumbnail-at"])
	for _, done := range []<-chan error{position, upload} {
		if err := waitJob(t, done); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := store.GetVideo(context.Background(), video.ID)
	if got.ThumbnailImageOrigin != domain.ThumbnailImageUploaded || store.rows["a"].Origin != domain.ThumbnailImageUploaded {
		t.Fatalf("アップロードが残っていない: %+v", got)
	}
	if got.ThumbnailPositionMs == nil || *got.ThumbnailPositionMs != 5_000 {
		t.Fatalf("位置 = %v", got.ThumbnailPositionMs)
	}
}

// uploadLockWaitContext observes cancellation checks after image validation.
// The upload gate checks Done before validation; the generation lock checks it
// afterwards. Only the latter signals that the request reached the lock wait.
type uploadLockWaitContext struct {
	context.Context
	validated <-chan struct{}
	waiting   chan struct{}
	once      sync.Once
}

func (c *uploadLockWaitContext) Done() <-chan struct{} {
	select {
	case <-c.validated:
		c.once.Do(func() { close(c.waiting) })
	default:
	}
	return c.Context.Done()
}

// 錠を待っているアップロードを取り消すと、錠が空くのを待たずに取り消しの誤りを返し、
// 何も書かない。待つのをやめたあとも錠は空き、後の位置の指定とアップロードは進む。
func TestUploadCancelsWhileWaitingOnLock(t *testing.T) {
	video := probedVideo(1, "a")
	store := newFakeIngestStore(video)
	hold := newHoldGeneration("thumbnail-at")
	generator := &fakeGenerator{gate: hold.gate, thumbnailDir: t.TempDir()}
	ingest, _ := newTestIngest(store, generator)
	validated := make(chan struct{})
	markValidated := sync.OnceFunc(func() { close(validated) })
	checks := newUploadChecks()
	ingest.checkUpload = func(image []byte) error {
		err := checks.check(image)
		markValidated()
		return err
	}
	releasePosition := sync.OnceFunc(func() { close(hold.release["thumbnail-at"]) })
	defer releasePosition()

	position := make(chan error, 1)
	go func() {
		_, err := ingest.SetThumbnailPosition(context.Background(), video.ID, video.Path, video.Path, int64Ptr(5_000))
		position <- err
	}()
	<-hold.started["thumbnail-at"]

	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &uploadLockWaitContext{Context: parent, validated: validated, waiting: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, err := ingest.UploadThumbnailImage(ctx, video.ID, "a", []byte("photo"))
		done <- err
	}()
	select {
	case <-ctx.waiting:
	case err := <-done:
		t.Fatalf("upload returned before waiting on the lock: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("upload did not reach the lock's cancellation check")
	}
	cancel()
	if err := waitJob(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if fitted(generator, "photo") || store.rows["a"].Image != nil {
		t.Fatalf("取り消したアップロードが書いた: %+v", store.rows["a"])
	}

	releasePosition()
	if err := waitJob(t, position); err != nil {
		t.Fatal(err)
	}
	if err := waitJob(t, uploadAsync(ingest, video.ID, "a", "later")); err != nil {
		t.Fatal(err)
	}
	if store.rows["a"].Origin != domain.ThumbnailImageUploaded {
		t.Fatalf("行 = %+v", store.rows["a"])
	}
}

// 門を待つ間に文脈が終われば、その誤りを返して何もしない。
func TestUploadGateHonoursContext(t *testing.T) {
	gate := newUploadGate()
	release, err := gate.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gate.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("条件が満たされない")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
