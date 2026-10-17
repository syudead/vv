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
	"testing/synctest"
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
	var phases []string
	checkGate := func(phase string) {
		assertUploadGateHeld(t, ingest.uploads)
		phases = append(phases, phase)
	}
	ingest.checkUpload = func(image []byte) error {
		checkGate("validate")
		return checkThumbnailUpload(image)
	}
	ingest.generator = &gateCheckingGenerator{Generator: generator, check: checkGate}

	if _, err := ingest.UploadThumbnailImage(context.Background(), video.ID, "a", []byte("text")); !errors.Is(err, domain.ErrThumbnailUploadInvalid) {
		t.Fatalf("err = %v", err)
	}
	if calls, _ := generator.snapshot(); len(calls) != 0 || len(store.rows) != 0 {
		t.Fatalf("断った画像で 呼び出し = %v, 行 = %v", calls, store.rows)
	}

	assertUploadGateAvailable(t, ingest.uploads)
	image := uploadPNG(t)
	saved, err := ingest.UploadThumbnailImage(context.Background(), video.ID, "a", image)
	if err != nil {
		t.Fatal(err)
	}
	assertUploadGateAvailable(t, ingest.uploads)
	if want := []string{"validate", "validate", "fit", "source"}; !slices.Equal(phases, want) {
		t.Fatalf("gate phases = %v, want %v", phases, want)
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
	ingest.checkUpload = func([]byte) error {
		markValidated()
		return nil
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
	assertUploadGateAvailable(t, ingest.uploads)
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

// The gate itself proves blocking, release and cancellation without image I/O.
func TestUploadGateSerializes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := newUploadGate()
		release, err := gate.acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		releaseFirst := sync.OnceFunc(release)
		defer releaseFirst()
		acquired := make(chan func(), 1)
		go func() {
			release, err := gate.acquire(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			acquired <- release
		}()
		synctest.Wait()
		select {
		case release := <-acquired:
			release()
			t.Fatal("second acquisition passed the held gate")
		default:
		}
		releaseFirst()
		releaseSecond := <-acquired
		assertUploadGateHeld(t, gate)
		releaseSecond()
		assertUploadGateAvailable(t, gate)
	})
}

func TestUploadGateHonoursContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := newUploadGate()
		release, err := gate.acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		releaseFirst := sync.OnceFunc(release)
		defer releaseFirst()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			release, err := gate.acquire(ctx)
			if release != nil {
				release()
			}
			done <- err
		}()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("acquire returned before cancellation: %v", err)
		default:
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("acquire: %v", err)
		}
		assertUploadGateHeld(t, gate)
		releaseFirst()
		assertUploadGateAvailable(t, gate)
	})
}

func assertUploadGateHeld(t *testing.T, gate uploadGate) {
	t.Helper()
	select {
	case gate <- struct{}{}:
		<-gate
		t.Fatal("upload gate is not held")
	default:
	}
}

func assertUploadGateAvailable(t *testing.T, gate uploadGate) {
	t.Helper()
	select {
	case gate <- struct{}{}:
		<-gate
	default:
		t.Fatal("upload gate is still held")
	}
}

// Observe both image conversions without changing the shared generator fake.
type gateCheckingGenerator struct {
	Generator
	check func(string)
}

func (g *gateCheckingGenerator) FitThumbnailImage(ctx context.Context, image []byte, aspect float64) ([]byte, error) {
	g.check("fit")
	return g.Generator.FitThumbnailImage(ctx, image, aspect)
}

func (g *gateCheckingGenerator) FitThumbnailSource(ctx context.Context, image []byte, aspect float64) ([]byte, error) {
	g.check("source")
	return g.Generator.FitThumbnailSource(ctx, image, aspect)
}
