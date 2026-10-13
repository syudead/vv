package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// サムネイルの編集のセッション（specs/045-comfyui-thumbnails/data-model.md の Edit session、
// research.md R-4・R-11）を、偽の ImageEditor と偽の時計で確かめる。

type fakeImageEditor struct {
	mu        sync.Mutex
	next      int
	starts    []domain.ImageEditInput
	startErr  error
	state     domain.ThumbnailEditState
	failure   *domain.ImageEditError
	readJobs  []domain.ImageEditJob
	cancels   []string
	cancelErr error
	// cancelHold が正なら、次の cancelHold 回の Cancel は、cancelEntered に実行の id を送ってから
	// cancelRelease が閉じるまで返らない。
	cancelHold    int
	cancelEntered chan string
	cancelRelease chan struct{}
	// cancelsActive は返っていない Cancel の数、startsWhileCancelling はその間に来た Start の数である。
	cancelsActive         int
	startsWhileCancelling int
	output                []byte
	outputErr             error
	outputs               int
	progress              *domain.ImageEditProgress
}

func (f *fakeImageEditor) Start(_ context.Context, conn domain.ComfyUISettings, input domain.ImageEditInput) (domain.ImageEditJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, input)
	if f.cancelsActive > 0 {
		f.startsWhileCancelling++
	}
	if f.startErr != nil {
		return domain.ImageEditJob{}, f.startErr
	}
	f.next++
	f.state = domain.ThumbnailEditQueued
	return domain.ImageEditJob{Connection: conn, ID: "job-" + strconv.Itoa(f.next), Output: "9"}, nil
}

func (f *fakeImageEditor) State(_ context.Context, job domain.ImageEditJob, _ domain.ThumbnailEditState) (domain.ThumbnailEditState, *domain.ImageEditError, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readJobs = append(f.readJobs, job)
	return f.state, f.failure, nil
}

func (f *fakeImageEditor) Cancel(_ context.Context, job domain.ImageEditJob, _ domain.ThumbnailEditState) error {
	f.mu.Lock()
	f.cancels = append(f.cancels, job.ID)
	f.cancelsActive++
	hold := f.cancelHold > 0
	if hold {
		f.cancelHold--
	}
	err := f.cancelErr
	f.mu.Unlock()
	if hold {
		f.cancelEntered <- job.ID
		<-f.cancelRelease
	}
	f.mu.Lock()
	f.cancelsActive--
	f.mu.Unlock()
	return err
}

func (f *fakeImageEditor) Output(context.Context, domain.ImageEditJob) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outputs++
	return f.output, f.outputErr
}

func (f *fakeImageEditor) Progress(domain.ImageEditJob) *domain.ImageEditProgress {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.progress == nil {
		return nil
	}
	progress := *f.progress
	return &progress
}

func (f *fakeImageEditor) set(state domain.ThumbnailEditState, failure *domain.ImageEditError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state, f.failure = state, failure
}

func (f *fakeImageEditor) cancelled() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cancels...)
}

type fakeFrames struct {
	mu    sync.Mutex
	frame domain.ThumbnailFrame
	// missing はコマが取れない位置である。
	missing map[int64]bool
	// auto は AutoThumbnailPositions が返す位置である。
	auto []int64
	// positions は FrameAt に渡された位置である。
	positions []int64
}

func (f *fakeFrames) FrameAt(_ context.Context, _ string, positionMs int64) (domain.ThumbnailFrame, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.positions = append(f.positions, positionMs)
	if f.frame.PNG == nil || f.missing[positionMs] {
		return domain.ThumbnailFrame{}, errors.New("no frame")
	}
	return f.frame, nil
}

func (f *fakeFrames) AutoThumbnailPositions(int64) []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.auto...)
}

func (f *fakeFrames) extracted() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.positions...)
}

// fakeThumbnailImages は内容ごとに適用した画像（代表サムネイル image と元の画像 source）と
// 指定した位置を持つ。
type fakeThumbnailImages struct {
	mu       sync.Mutex
	image    map[string][]byte
	source   map[string][]byte
	position map[string]*int64
	err      error
}

func (f *fakeThumbnailImages) CurrentThumbnailSelection(
	_ context.Context, contentKey string,
) (domain.CurrentThumbnailSelection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	image := f.source[contentKey]
	if image == nil {
		image = f.image[contentKey]
	}
	return domain.CurrentThumbnailSelection{Image: image, PositionMs: f.position[contentKey]}, f.err
}

// setPosition は内容 contentKey に指定した位置を置く。nil なら解除する。
func (f *fakeThumbnailImages) setPosition(contentKey string, positionMs *int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.position == nil {
		f.position = map[string]*int64{}
	}
	f.position[contentKey] = positionMs
}

// set は内容 contentKey に適用した画像を置く。image が nil なら外す（元に戻す操作）。
func (f *fakeThumbnailImages) set(contentKey string, image, source []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.image == nil {
		f.image, f.source = map[string][]byte{}, map[string][]byte{}
	}
	f.image[contentKey], f.source[contentKey] = image, source
}

type fakeApplier struct {
	err     error
	applied [][]byte
	// origins は適用に渡された出どころである。
	origins []domain.ThumbnailImageOrigin
	// entered と release があれば、適用はそこで止まり、release が閉じるまで返らない。
	entered chan struct{}
	release chan struct{}
}

func (f *fakeApplier) ApplyThumbnailImage(
	_ context.Context, videoID int64, _ string, image []byte, origin domain.ThumbnailImageOrigin,
) (domain.Video, error) {
	if f.entered != nil {
		close(f.entered)
		<-f.release
	}
	if f.err != nil {
		return domain.Video{}, f.err
	}
	f.applied = append(f.applied, image)
	f.origins = append(f.origins, origin)
	return domain.Video{ID: videoID}, nil
}

type editsFixture struct {
	edits    *ThumbnailEdits
	editor   *fakeImageEditor
	settings *fakeComfyUIStore
	applier  *fakeApplier
	clock    *fakeClock
	frames   *fakeFrames
	images   *fakeThumbnailImages
}

func newEditsFixture(t *testing.T) *editsFixture {
	t.Helper()
	f := &editsFixture{
		editor:   &fakeImageEditor{output: testPNG(t, 8, 6)},
		settings: &fakeComfyUIStore{settings: domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: "http://a:8188"}},
		applier:  &fakeApplier{},
		clock:    &fakeClock{now: time.Unix(1_700_000_000, 0)},
		frames: &fakeFrames{
			frame: domain.ThumbnailFrame{PNG: testPNG(t, 8, 6), Width: 8, Height: 6}, auto: []int64{6_000, 0},
		},
		images: &fakeThumbnailImages{},
	}
	f.edits = NewThumbnailEdits(ThumbnailEditsOptions{
		Settings: f.settings, Editor: f.editor, Frames: f.frames, Images: f.images, Applier: f.applier,
		Now: f.clock.Now,
	})
	return f
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// testWebP は 1×1 の可逆の WebP である。
var testWebP = []byte("RIFF\x1a\x00\x00\x00WEBPVP8L\x0d\x00\x00\x00\x2f\x00\x00\x00\x10\x07\x10\x11\x11\x88\x88\xfe\x07\x00")

// corruptWebP は、RIFF とチャンクの長さも VP8L の見出しも正しいまま、画像の符号だけが壊れた
// WebP である。チャンクの並びと見出しを見るだけでは見分けられない。
func corruptWebP() []byte {
	data := append([]byte(nil), testWebP...)
	data[26] = 0
	return data
}

// shortWebP は、VP8L の符号が途中で終わり、RIFF とチャンクの長さをその短さに合わせた WebP である。
func shortWebP() []byte {
	payload := testWebP[20:27]
	data := []byte("RIFF\x14\x00\x00\x00WEBPVP8L\x07\x00\x00\x00")
	data = append(data, payload...)
	return append(data, 0)
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// truncatedJPEG は、見出しは読めるが走査の途中で切れた JPEG を返す。
func truncatedJPEG(t *testing.T) []byte {
	t.Helper()
	data := testJPEG(t)
	return data[:len(data)-len(data)/4]
}

// truncatedPNG は data の IDAT の途中から後を切り落とした PNG を返す。見出しは読める。
func truncatedPNG(t *testing.T, data []byte) []byte {
	t.Helper()
	at := bytes.Index(data, []byte("IDAT"))
	if at < 0 {
		t.Fatal("no IDAT")
	}
	return data[:at+8]
}

// hugePNGHeader は、見出しだけが 100000×100000 を名乗る PNG を返す。
func hugePNGHeader(t *testing.T) []byte {
	t.Helper()
	return pngHeaderSized(t, 100_000, 100_000)
}

// pngHeaderSized は、見出しだけが width×height を名乗る 1×1 の PNG を返す。
func pngHeaderSized(t *testing.T, width, height uint32) []byte {
	t.Helper()
	data := testPNG(t, 1, 1)
	ihdr := bytes.Index(data, []byte("IHDR"))
	binary.BigEndian.PutUint32(data[ihdr+4:], width)
	binary.BigEndian.PutUint32(data[ihdr+8:], height)
	binary.BigEndian.PutUint32(data[ihdr+17:], crc32.ChecksumIEEE(data[ihdr:ihdr+17]))
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	return data
}

func editVideo(id int64) domain.Video {
	duration := int64(60_000)
	return domain.Video{ID: id, ContentKey: "key-" + strconv.FormatInt(id, 10), ProbeState: domain.ProbeStateDone, DurationMs: &duration}
}

func (f *editsFixture) start(t *testing.T, videoID int64) domain.ThumbnailEdit {
	t.Helper()
	edit, err := f.edits.Start(context.Background(), editVideo(videoID), "/media/a.mp4", frameStart(1_000))
	if err != nil {
		t.Fatal(err)
	}
	return edit
}

// frameStart は positionMs のコマから始める要求である。
func frameStart(positionMs int64) domain.ThumbnailEditStart {
	return domain.ThumbnailEditStart{Source: domain.ThumbnailEditFromFrame, PositionMs: &positionMs}
}

func (f *editsFixture) run(t *testing.T, id string) domain.ThumbnailEdit {
	t.Helper()
	edit, err := f.edits.StartRun(context.Background(), id, domain.ThumbnailEditRunInput{Instruction: "remove the caption"})
	if err != nil {
		t.Fatal(err)
	}
	return edit
}

func (f *editsFixture) get(t *testing.T, id string) domain.ThumbnailEdit {
	t.Helper()
	edit, err := f.edits.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return edit
}

func (f *editsFixture) wantEnded(t *testing.T, id string) {
	t.Helper()
	if _, err := f.edits.Get(context.Background(), id); !errors.Is(err, domain.ErrThumbnailEditNotFound) {
		t.Fatalf("Get after the end = %v, want ErrThumbnailEditNotFound", err)
	}
}

func wantState(t *testing.T, edit domain.ThumbnailEdit, state domain.ThumbnailEditState) {
	t.Helper()
	if edit.State != state {
		t.Fatalf("state = %s, want %s", edit.State, state)
	}
}

// ready から始まり、実行は queued・preparing・running を経て succeeded になり、出力を取る。
// 終わった実行はどれも再実行で queued に戻り、succeeded から適用するとセッションが終わる。
func TestThumbnailEditHappyPath(t *testing.T) {
	f := newEditsFixture(t)
	edit := f.start(t, 1)
	wantState(t, edit, domain.ThumbnailEditReady)
	if edit.Elapsed != nil || edit.FrameWidth != 8 || edit.FrameHeight != 6 || len(edit.ID) != 32 {
		t.Fatalf("ready = %+v", edit)
	}

	edit = f.run(t, edit.ID)
	wantState(t, edit, domain.ThumbnailEditQueued)
	for _, state := range []domain.ThumbnailEditState{domain.ThumbnailEditPreparing, domain.ThumbnailEditRunning} {
		f.clock.Advance(2 * time.Second)
		f.editor.set(state, nil)
		wantState(t, f.get(t, edit.ID), state)
	}
	f.editor.progress = &domain.ImageEditProgress{Value: 2, Max: 4}
	f.clock.Advance(2 * time.Second)
	f.editor.set(domain.ThumbnailEditSucceeded, nil)
	done := f.get(t, edit.ID)
	wantState(t, done, domain.ThumbnailEditSucceeded)
	if !done.HasResult || done.Elapsed == nil || *done.Elapsed != 6*time.Second ||
		done.Progress == nil || *done.Progress != (domain.ImageEditProgress{Value: 2, Max: 4}) {
		t.Fatalf("succeeded = %+v", done)
	}
	f.clock.Advance(time.Minute)
	if later := f.get(t, edit.ID); *later.Elapsed != 6*time.Second {
		t.Errorf("the elapsed time of an ended run moved: %v", *later.Elapsed)
	}
	if result, err := f.edits.Result(context.Background(), edit.ID); err != nil || !bytes.Equal(result, f.editor.output) {
		t.Fatalf("Result = %q, %v", result, err)
	}

	video, err := f.edits.Apply(context.Background(), edit.ID)
	if err != nil || video.ID != 1 || len(f.applier.applied) != 1 || !bytes.Equal(f.applier.applied[0], f.editor.output) {
		t.Fatalf("Apply = %+v, %v, applied %q", video, err, f.applier.applied)
	}
	if !slices.Equal(f.applier.origins, []domain.ThumbnailImageOrigin{domain.ThumbnailImageGenerated}) {
		t.Fatalf("出どころ = %v, want generated", f.applier.origins)
	}
	f.wantEnded(t, edit.ID)
	if len(f.editor.cancelled()) != 0 {
		t.Errorf("an ended run was cancelled: %v", f.editor.cancelled())
	}
}

// queued・preparing・running のどれからも、提供元の誤りか 10 分の時間切れで failed に、持ち主の
// 中止で cancelled になる。queued から running へ直に進むこともある。
func TestThumbnailEditRunEndings(t *testing.T) {
	from := []domain.ThumbnailEditState{domain.ThumbnailEditQueued, domain.ThumbnailEditPreparing, domain.ThumbnailEditRunning}
	endings := map[string]func(*testing.T, *editsFixture, string) domain.ThumbnailEdit{
		"error": func(t *testing.T, f *editsFixture, id string) domain.ThumbnailEdit {
			t.Helper()
			f.editor.set(domain.ThumbnailEditFailed, &domain.ImageEditError{Reason: domain.ImageEditRunFailed, Message: "boom"})
			return f.get(t, id)
		},
		"time limit": func(t *testing.T, f *editsFixture, id string) domain.ThumbnailEdit {
			t.Helper()
			// 読み続けるので、読まれないことでは終わらない。
			for range 9 {
				f.clock.Advance(time.Minute)
				wantState(t, f.get(t, id), f.editor.state)
			}
			f.clock.Advance(time.Minute)
			return f.get(t, id)
		},
		"owner cancel": func(t *testing.T, f *editsFixture, id string) domain.ThumbnailEdit {
			t.Helper()
			edit, err := f.edits.Cancel(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			return edit
		},
	}
	for _, state := range from {
		for name, end := range endings {
			t.Run(string(state)+" "+name, func(t *testing.T) {
				f := newEditsFixture(t)
				id := f.start(t, 1).ID
				f.run(t, id)
				f.editor.set(state, nil)
				wantState(t, f.get(t, id), state)

				edit := end(t, f, id)
				switch name {
				case "owner cancel":
					wantState(t, edit, domain.ThumbnailEditCancelled)
				case "time limit":
					wantState(t, edit, domain.ThumbnailEditFailed)
					if edit.Error == nil || edit.Error.Reason != domain.ImageEditTimedOut {
						t.Fatalf("error = %+v, want timed_out", edit.Error)
					}
				default:
					wantState(t, edit, domain.ThumbnailEditFailed)
					if edit.Error == nil || edit.Error.Reason != domain.ImageEditRunFailed || edit.Error.Message != "boom" {
						t.Fatalf("error = %+v", edit.Error)
					}
				}
				if name != "error" && len(f.editor.cancelled()) != 1 {
					t.Errorf("cancels at the provider = %v, want 1", f.editor.cancelled())
				}
				if edit.HasResult {
					t.Error("an unsuccessful run has a result")
				}
				if _, err := f.edits.Apply(context.Background(), id); !errors.Is(err, domain.ErrThumbnailEditNoResult) {
					t.Errorf("Apply = %v, want ErrThumbnailEditNoResult", err)
				}

				// 再実行で queued に戻る。
				wantState(t, f.run(t, id), domain.ThumbnailEditQueued)
			})
		}
	}
}

// 提供元が中止を報告した実行は cancelled になる。実行中でない中止は今の姿を返す。
func TestThumbnailEditProviderCancelAndIdleCancel(t *testing.T) {
	f := newEditsFixture(t)
	id := f.start(t, 1).ID
	edit, err := f.edits.Cancel(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	wantState(t, edit, domain.ThumbnailEditReady)

	f.run(t, id)
	f.editor.set(domain.ThumbnailEditCancelled, nil)
	wantState(t, f.get(t, id), domain.ThumbnailEditCancelled)
	if len(f.editor.cancelled()) != 0 {
		t.Errorf("an ended run was cancelled: %v", f.editor.cancelled())
	}
}

// 成功を報告しても出力を取れなければ、その理由で failed になる。
func TestThumbnailEditOutputFailure(t *testing.T) {
	f := newEditsFixture(t)
	id := f.start(t, 1).ID
	f.run(t, id)
	f.editor.outputErr = &domain.ImageEditError{Reason: domain.ImageEditNoOutput}
	f.editor.set(domain.ThumbnailEditSucceeded, nil)
	edit := f.get(t, id)
	wantState(t, edit, domain.ThumbnailEditFailed)
	if edit.Error == nil || edit.Error.Reason != domain.ImageEditNoOutput || edit.HasResult {
		t.Fatalf("edit = %+v", edit)
	}
}

// 出力が届かなければ前の状態のまま次の読み取りで取り直し、取れたら succeeded になる。
func TestThumbnailEditOutputFetchRetries(t *testing.T) {
	for name, fetchErr := range map[string]error{
		"unreachable": &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: "cannot reach ComfyUI"},
		"timeout":     context.DeadlineExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			f := newEditsFixture(t)
			id := f.start(t, 1).ID
			f.run(t, id)
			f.editor.set(domain.ThumbnailEditRunning, nil)
			f.get(t, id)
			f.editor.outputErr = fetchErr
			f.editor.set(domain.ThumbnailEditSucceeded, nil)
			edit := f.get(t, id)
			wantState(t, edit, domain.ThumbnailEditRunning)
			if edit.Error != nil || edit.HasResult {
				t.Fatalf("after a failed fetch = %+v", edit)
			}

			f.editor.outputErr = nil
			edit = f.get(t, id)
			wantState(t, edit, domain.ThumbnailEditSucceeded)
			if !edit.HasResult || f.editor.outputs != 2 {
				t.Fatalf("after the retry = %+v, fetches %d", edit, f.editor.outputs)
			}
		})
	}
}

// 出力が読めない画像なら、succeeded にせず no_output で failed にする。
// 提供元の WebP の出力は、最後まで読めれば適用できる結果になり、符号が壊れていれば no_output になる。
func TestThumbnailEditWebPOutput(t *testing.T) {
	for name, c := range map[string]struct {
		output []byte
		want   domain.ThumbnailEditState
	}{
		"valid":   {testWebP, domain.ThumbnailEditSucceeded},
		"corrupt": {corruptWebP(), domain.ThumbnailEditFailed},
	} {
		t.Run(name, func(t *testing.T) {
			f := newEditsFixture(t)
			id := f.start(t, 1).ID
			f.run(t, id)
			f.editor.output = c.output
			f.editor.set(domain.ThumbnailEditSucceeded, nil)
			edit := f.get(t, id)
			wantState(t, edit, c.want)
			if edit.HasResult != (c.want == domain.ThumbnailEditSucceeded) {
				t.Fatalf("edit = %+v", edit)
			}
		})
	}
}

func TestThumbnailEditUndecodableOutput(t *testing.T) {
	f := newEditsFixture(t)
	id := f.start(t, 1).ID
	f.run(t, id)
	f.editor.output = []byte("<html>bad gateway</html>")
	f.editor.set(domain.ThumbnailEditSucceeded, nil)
	edit := f.get(t, id)
	wantState(t, edit, domain.ThumbnailEditFailed)
	if edit.Error == nil || edit.Error.Reason != domain.ImageEditNoOutput || edit.HasResult {
		t.Fatalf("edit = %+v", edit)
	}
	if _, err := f.edits.Result(context.Background(), id); !errors.Is(err, domain.ErrThumbnailEditNoResult) {
		t.Fatalf("Result = %v", err)
	}
}

// 5 分読まれないセッションは、提供元で実行を中止してから終わる。片付けでも、次の読み取りでも同じ。
func TestThumbnailEditIdleExpiry(t *testing.T) {
	for _, sweep := range []bool{true, false} {
		t.Run("sweep "+strconv.FormatBool(sweep), func(t *testing.T) {
			f := newEditsFixture(t)
			id := f.start(t, 1).ID
			f.run(t, id)
			f.editor.set(domain.ThumbnailEditRunning, nil)
			f.clock.Advance(domain.ThumbnailEditIdleLimit - time.Second)
			f.edits.Sweep(context.Background())
			wantState(t, f.get(t, id), domain.ThumbnailEditRunning)

			f.clock.Advance(domain.ThumbnailEditIdleLimit)
			if sweep {
				f.edits.Sweep(context.Background())
				if got := f.editor.cancelled(); len(got) != 1 || got[0] != "job-1" {
					t.Fatalf("cancels after the sweep = %v", got)
				}
			}
			f.wantEnded(t, id)
			if got := f.editor.cancelled(); len(got) != 1 {
				t.Fatalf("cancels = %v, want job-1 once", got)
			}
			for _, call := range []func() error{
				func() error { _, err := f.edits.Frame(context.Background(), id); return err },
				func() error { _, err := f.edits.Result(context.Background(), id); return err },
				func() error { _, err := f.edits.Cancel(context.Background(), id); return err },
				func() error { _, err := f.edits.Apply(context.Background(), id); return err },
				func() error {
					_, err := f.edits.StartRun(context.Background(), id, domain.ThumbnailEditRunInput{})
					return err
				},
			} {
				if err := call(); !errors.Is(err, domain.ErrThumbnailEditNotFound) {
					t.Errorf("an ended session answered %v", err)
				}
			}
		})
	}
}

// 片付けは、読まれ続けていても送ってから 10 分終わらない実行を中止して timed_out にする。
func TestThumbnailEditSweepTimesOutRun(t *testing.T) {
	f := newEditsFixture(t)
	id := f.start(t, 1).ID
	f.run(t, id)
	f.editor.set(domain.ThumbnailEditQueued, nil)
	for range 9 {
		f.clock.Advance(time.Minute)
		f.edits.Sweep(context.Background())
		wantState(t, f.get(t, id), domain.ThumbnailEditQueued)
	}
	f.clock.Advance(time.Minute)
	f.edits.Sweep(context.Background())
	if got := f.editor.cancelled(); len(got) != 1 {
		t.Fatalf("cancels = %v", got)
	}
	edit := f.get(t, id)
	wantState(t, edit, domain.ThumbnailEditFailed)
	if edit.Error == nil || edit.Error.Reason != domain.ImageEditTimedOut || *edit.Elapsed != domain.ThumbnailEditRunLimit {
		t.Fatalf("edit = %+v", edit)
	}
}

// 同じ動画で 2 つ目の編集を始めると、1 つ目の実行を中止して 1 つ目を終える。別の動画は残る。
func TestThumbnailEditSecondEditReplacesFirst(t *testing.T) {
	f := newEditsFixture(t)
	first := f.start(t, 1).ID
	other := f.start(t, 2).ID
	f.run(t, first)
	f.editor.set(domain.ThumbnailEditRunning, nil)
	f.get(t, first)

	second := f.start(t, 1).ID
	if second == first {
		t.Fatal("the second edit reused the id")
	}
	if got := f.editor.cancelled(); len(got) != 1 || got[0] != "job-1" {
		t.Fatalf("cancels = %v, want job-1", got)
	}
	f.wantEnded(t, first)
	wantState(t, f.get(t, second), domain.ThumbnailEditReady)
	wantState(t, f.get(t, other), domain.ThumbnailEditReady)
}

// 同じ動画の新しい編集は、1 つ目で進んでいる適用が終わるまで載らない。1 つ目の id は、その後
// どの要求にも 404 である。
func TestThumbnailEditSecondEditWaitsForApply(t *testing.T) {
	f := newEditsFixture(t)
	first := f.start(t, 1).ID
	f.run(t, first)
	f.editor.set(domain.ThumbnailEditSucceeded, nil)
	f.get(t, first)
	f.applier.entered = make(chan struct{})
	f.applier.release = make(chan struct{})

	applied := make(chan error, 1)
	go func() {
		_, err := f.edits.Apply(context.Background(), first)
		applied <- err
	}()
	<-f.applier.entered
	started := make(chan domain.ThumbnailEdit, 1)
	go func() {
		started <- f.start(t, 1)
	}()

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		f.edits.mu.Lock()
		active := f.edits.byVideo[1]
		f.edits.mu.Unlock()
		if active != first {
			t.Fatalf("the second edit %s became active while the first was applying", active)
		}
		select {
		case <-started:
			t.Fatal("the second edit started while the first was applying")
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}

	close(f.applier.release)
	if err := <-applied; err != nil {
		t.Fatalf("Apply = %v", err)
	}
	second := <-started
	f.wantEnded(t, first)
	wantState(t, f.get(t, second.ID), domain.ThumbnailEditReady)
}

// 実行中に新しい実行を始めると、今の実行を中止してから送る。
func TestThumbnailEditRunAgainCancelsRunningRun(t *testing.T) {
	f := newEditsFixture(t)
	id := f.start(t, 1).ID
	f.run(t, id)
	f.editor.set(domain.ThumbnailEditRunning, nil)
	f.get(t, id)
	edit := f.run(t, id)
	wantState(t, edit, domain.ThumbnailEditQueued)
	if got := f.editor.cancelled(); len(got) != 1 || got[0] != "job-1" {
		t.Fatalf("cancels = %v, want job-1", got)
	}
	if *edit.Elapsed != 0 {
		t.Errorf("elapsed of the new run = %v", *edit.Elapsed)
	}
}

// 実行の途中で接続先を変えても、その実行は送ったときの接続先で問い合わせる。次の実行は新しい接続先を使う。
func TestThumbnailEditRunKeepsConnectionSnapshot(t *testing.T) {
	f := newEditsFixture(t)
	id := f.start(t, 1).ID
	original := f.settings.settings
	f.run(t, id)
	f.settings.settings = domain.ComfyUISettings{Provider: domain.ComfyUICloud, APIKey: "new"}
	f.editor.set(domain.ThumbnailEditRunning, nil)
	f.get(t, id)
	if _, err := f.edits.Cancel(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	f.editor.mu.Lock()
	read := f.editor.readJobs[len(f.editor.readJobs)-1]
	f.editor.mu.Unlock()
	if read.Connection != original {
		t.Fatalf("the run read %+v, want the snapshot %+v", read.Connection, original)
	}

	f.run(t, id)
	f.get(t, id)
	f.editor.mu.Lock()
	read = f.editor.readJobs[len(f.editor.readJobs)-1]
	f.editor.mu.Unlock()
	if read.Connection.Provider != domain.ComfyUICloud {
		t.Fatalf("the next run read %+v, want the new connection", read.Connection)
	}
}

// マスクと参考画像は何も上げる前に確かめる。接続先が消されていれば始めない。
func TestThumbnailEditRunRejectsInputs(t *testing.T) {
	f := newEditsFixture(t)
	id := f.start(t, 1).ID
	mask := testPNG(t, 8, 6)
	reference := testPNG(t, 64, 48)
	for _, c := range []struct {
		name  string
		input domain.ThumbnailEditRunInput
		want  error
	}{
		{"mask of another size", domain.ThumbnailEditRunInput{Mask: testPNG(t, 4, 6)}, domain.ErrThumbnailEditMaskInvalid},
		{"mask that is not a PNG", domain.ThumbnailEditRunInput{Mask: []byte("jpeg")}, domain.ErrThumbnailEditMaskInvalid},
		{"reference that is not an image", domain.ThumbnailEditRunInput{Reference: []byte("text")}, domain.ErrThumbnailEditReferenceInvalid},
		{"reference that is too large", domain.ThumbnailEditRunInput{Reference: make([]byte, domain.ThumbnailEditMaxReferenceBytes+1)}, domain.ErrThumbnailEditReferenceInvalid},
		{"truncated mask", domain.ThumbnailEditRunInput{Mask: truncatedPNG(t, mask)}, domain.ErrThumbnailEditMaskInvalid},
		{"truncated PNG reference", domain.ThumbnailEditRunInput{Reference: truncatedPNG(t, reference)}, domain.ErrThumbnailEditReferenceInvalid},
		{"truncated JPEG reference", domain.ThumbnailEditRunInput{Reference: truncatedJPEG(t)}, domain.ErrThumbnailEditReferenceInvalid},
		{"forged WebP header", domain.ThumbnailEditRunInput{Reference: []byte("RIFF\x05\x00\x00\x00WEBPx")}, domain.ErrThumbnailEditReferenceInvalid},
		{"truncated WebP reference", domain.ThumbnailEditRunInput{Reference: testWebP[:len(testWebP)-6]}, domain.ErrThumbnailEditReferenceInvalid},
		{"WebP without an image", domain.ThumbnailEditRunInput{Reference: []byte("RIFF\x0c\x00\x00\x00WEBPEXIF\x00\x00\x00\x00")}, domain.ErrThumbnailEditReferenceInvalid},
		{"WebP with a corrupt bitstream", domain.ThumbnailEditRunInput{Reference: corruptWebP()}, domain.ErrThumbnailEditReferenceInvalid},
		{"WebP whose bitstream ends early", domain.ThumbnailEditRunInput{Reference: shortWebP()}, domain.ErrThumbnailEditReferenceInvalid},
		{"reference too large to decode", domain.ThumbnailEditRunInput{Reference: hugePNGHeader(t)}, domain.ErrThumbnailEditReferenceInvalid},
	} {
		if _, err := f.edits.StartRun(context.Background(), id, c.input); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if len(f.editor.starts) != 0 {
		t.Fatalf("an invalid input was sent: %d", len(f.editor.starts))
	}
	wantState(t, f.get(t, id), domain.ThumbnailEditReady)

	for name, reference := range map[string][]byte{"PNG": reference, "JPEG": testJPEG(t), "WebP": testWebP} {
		edit, err := f.edits.StartRun(context.Background(), id, domain.ThumbnailEditRunInput{Mask: mask, Reference: reference})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		wantState(t, edit, domain.ThumbnailEditQueued)
	}

	f.settings.settings = domain.ComfyUISettings{}
	if _, err := f.edits.StartRun(context.Background(), id, domain.ThumbnailEditRunInput{}); !errors.Is(err, domain.ErrThumbnailEditNotReady) {
		t.Fatalf("without a connection: %v", err)
	}
}

// 提供元が中止を受け付けなければ、画面には cancelled を返すが、実行は追い続けて片付けのたびに
// 中止をやり直す。中止が済むまで、その動画の新しい実行は送らずに unreachable で失敗させる。
func TestThumbnailEditFailedCancelIsRetried(t *testing.T) {
	f := newEditsFixture(t)
	ctx := context.Background()
	id := f.start(t, 1).ID
	f.run(t, id)
	f.editor.set(domain.ThumbnailEditRunning, nil)
	f.get(t, id)
	f.editor.mu.Lock()
	f.editor.cancelErr = errors.New("the status endpoint timed out")
	f.editor.mu.Unlock()

	edit, err := f.edits.Cancel(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	wantState(t, edit, domain.ThumbnailEditCancelled)

	_, err = f.edits.StartRun(ctx, id, domain.ThumbnailEditRunInput{Instruction: "again"})
	var failure *domain.ImageEditError
	if !errors.As(err, &failure) || failure.Reason != domain.ImageEditUnreachable {
		t.Fatalf("a run while job-1 may still be active = %v", err)
	}
	if len(f.editor.starts) != 1 {
		t.Fatalf("a replacement was submitted while job-1 was unconfirmed: %d starts", len(f.editor.starts))
	}
	wantState(t, f.get(t, id), domain.ThumbnailEditFailed)
	if got := f.editor.cancelled(); len(got) != 2 || got[1] != "job-1" {
		t.Fatalf("cancels = %v, want job-1 twice", got)
	}

	f.edits.Sweep(ctx)
	if got := f.editor.cancelled(); len(got) != 3 || got[2] != "job-1" {
		t.Fatalf("cancels after the sweep = %v", got)
	}
	f.editor.mu.Lock()
	f.editor.cancelErr = nil
	f.editor.mu.Unlock()
	f.edits.Sweep(ctx)
	f.edits.Sweep(ctx)
	if got := f.editor.cancelled(); len(got) != 4 {
		t.Fatalf("cancels after the confirmed cancel = %v, want no more retries", got)
	}

	edit = f.run(t, id)
	wantState(t, edit, domain.ThumbnailEditQueued)
	if len(f.editor.starts) != 2 {
		t.Fatalf("starts = %d, want the run submitted once job-1 is cancelled", len(f.editor.starts))
	}
}

// 片付けがある実行の中止をやり直している間は、同じ動画の新しい実行を送らない。Comfy Cloud の
// interrupt はアカウントの実行中の仕事をすべて止めるので、遅れて届いた片付けのやり直しが新しい
// 実行を止めうる。StartRun は片付けのやり直しの終わりを待ち、同じ実行を重ねて中止しない。
func TestThumbnailEditRunWaitsForInFlightCancelRetry(t *testing.T) {
	f := newEditsFixture(t)
	ctx := context.Background()
	id := f.start(t, 1).ID
	f.run(t, id)
	f.editor.set(domain.ThumbnailEditRunning, nil)
	f.get(t, id)
	f.editor.mu.Lock()
	f.editor.cancelErr = errors.New("the status endpoint timed out")
	f.editor.mu.Unlock()
	if _, err := f.edits.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}

	f.editor.mu.Lock()
	f.editor.cancelErr = nil
	f.editor.cancelHold = 1
	f.editor.cancelEntered = make(chan string, 1)
	f.editor.cancelRelease = make(chan struct{})
	f.editor.mu.Unlock()
	swept := make(chan struct{})
	go func() {
		defer close(swept)
		f.edits.Sweep(ctx)
	}()
	if got := <-f.editor.cancelEntered; got != "job-1" {
		t.Fatalf("the sweep retried %s", got)
	}

	type result struct {
		edit domain.ThumbnailEdit
		err  error
	}
	ran := make(chan result, 1)
	go func() {
		edit, err := f.edits.StartRun(ctx, id, domain.ThumbnailEditRunInput{Instruction: "again"})
		ran <- result{edit, err}
	}()
	select {
	case r := <-ran:
		close(f.editor.cancelRelease)
		<-swept
		t.Fatalf("the run returned while the sweep was still cancelling job-1: %+v, %v", r.edit, r.err)
	case <-time.After(100 * time.Millisecond):
	}
	close(f.editor.cancelRelease)
	<-swept
	r := <-ran
	if r.err != nil {
		t.Fatal(r.err)
	}
	wantState(t, r.edit, domain.ThumbnailEditQueued)
	f.editor.mu.Lock()
	defer f.editor.mu.Unlock()
	if f.editor.startsWhileCancelling != 0 || len(f.editor.starts) != 2 {
		t.Fatalf("starts = %d, while a cancel was in flight = %d", len(f.editor.starts), f.editor.startsWhileCancelling)
	}
	if got := f.editor.cancels; len(got) != 2 {
		t.Fatalf("cancels = %v, want job-1 cancelled once by the sweep after the first attempt", got)
	}
}

// 片付けは、別々の実行の中止を同時にやり直す。1 つずつ待つと、届かない提供元の実行が多いほど
// 次の片付け（読まれないセッションと時間切れの実行）が遅れる。
func TestThumbnailEditSweepRetriesCancelsConcurrently(t *testing.T) {
	f := newEditsFixture(t)
	ctx := context.Background()
	f.editor.mu.Lock()
	f.editor.cancelErr = errors.New("unreachable")
	f.editor.mu.Unlock()
	for _, videoID := range []int64{1, 2} {
		id := f.start(t, videoID).ID
		f.run(t, id)
		f.edits.Discard(ctx, id)
	}

	f.editor.mu.Lock()
	f.editor.cancelErr = nil
	f.editor.cancelHold = 2
	f.editor.cancelEntered = make(chan string, 2)
	f.editor.cancelRelease = make(chan struct{})
	f.editor.mu.Unlock()
	swept := make(chan struct{})
	go func() {
		defer close(swept)
		f.edits.Sweep(ctx)
	}()
	for range 2 {
		select {
		case <-f.editor.cancelEntered:
		case <-time.After(5 * time.Second):
			close(f.editor.cancelRelease)
			<-swept
			t.Fatal("the sweep retried one cancel at a time")
		}
	}
	close(f.editor.cancelRelease)
	<-swept
	f.edits.Sweep(ctx)
	if got := f.editor.cancelled(); len(got) != 4 {
		t.Fatalf("cancels = %v, want each job retried once and then none", got)
	}
}

// 終えたセッションの実行も中止をやり直し、1 時間たっても済まなければ諦める。
func TestThumbnailEditFailedCancelGivesUp(t *testing.T) {
	f := newEditsFixture(t)
	ctx := context.Background()
	id := f.start(t, 1).ID
	f.run(t, id)
	f.editor.mu.Lock()
	f.editor.cancelErr = errors.New("unreachable")
	f.editor.mu.Unlock()
	f.edits.Discard(ctx, id)
	f.wantEnded(t, id)

	f.edits.Sweep(ctx)
	if got := f.editor.cancelled(); len(got) != 2 {
		t.Fatalf("cancels = %v, want a retry after the discard", got)
	}
	f.clock.Advance(thumbnailEditCancelRetryLimit)
	f.edits.Sweep(ctx)
	f.edits.Sweep(ctx)
	if got := f.editor.cancelled(); len(got) != 3 {
		t.Fatalf("cancels = %v, want one last retry and then none", got)
	}
}

// 提供元が受け付けなければ、セッションは同じ理由の failed になり、誤りを返す。
func TestThumbnailEditRunSubmitFailure(t *testing.T) {
	f := newEditsFixture(t)
	id := f.start(t, 1).ID
	f.editor.startErr = &domain.ImageEditError{Reason: domain.ImageEditUnreachable}
	_, err := f.edits.StartRun(context.Background(), id, domain.ThumbnailEditRunInput{})
	var failure *domain.ImageEditError
	if !errors.As(err, &failure) || failure.Reason != domain.ImageEditUnreachable {
		t.Fatalf("err = %v", err)
	}
	edit := f.get(t, id)
	wantState(t, edit, domain.ThumbnailEditFailed)
	if edit.Error == nil || edit.Error.Reason != domain.ImageEditUnreachable {
		t.Fatalf("edit = %+v", edit)
	}
	if _, err := f.edits.Cancel(context.Background(), id); err != nil || len(f.editor.cancelled()) != 0 {
		t.Fatalf("cancel of a run never submitted = %v, %v", err, f.editor.cancelled())
	}
}

// 始めるときは接続先・内容・位置・コマを確かめる。
func TestThumbnailEditStartRejects(t *testing.T) {
	f := newEditsFixture(t)
	ctx := context.Background()
	noKey := editVideo(1)
	noKey.ContentKey = ""
	for _, c := range []struct {
		name     string
		video    domain.Video
		position int64
		want     error
	}{
		{"no content", noKey, 0, domain.ErrNotFound},
		{"past the end", editVideo(1), 60_000, domain.ErrThumbnailPositionOutOfRange},
	} {
		if _, err := f.edits.Start(ctx, c.video, "/a", frameStart(c.position)); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	f.frames.frame = domain.ThumbnailFrame{}
	if _, err := f.edits.Start(ctx, editVideo(1), "/a", frameStart(0)); !errors.Is(err, domain.ErrThumbnailFrameUnavailable) {
		t.Errorf("no frame: %v", err)
	}
	f.settings.settings = domain.ComfyUISettings{}
	if _, err := f.edits.Start(ctx, editVideo(1), "/a", frameStart(0)); !errors.Is(err, domain.ErrThumbnailEditNotReady) {
		t.Errorf("no connection: %v", err)
	}
}

// 適用で動画が別の内容になっていればセッションは終わり、画像を読めなければ残る。
func TestThumbnailEditApplyFailures(t *testing.T) {
	f := newEditsFixture(t)
	id := f.start(t, 1).ID
	f.run(t, id)
	f.editor.set(domain.ThumbnailEditSucceeded, nil)
	f.get(t, id)

	f.applier.err = domain.ErrThumbnailImageInvalid
	if _, err := f.edits.Apply(context.Background(), id); !errors.Is(err, domain.ErrThumbnailImageInvalid) {
		t.Fatalf("Apply = %v", err)
	}
	wantState(t, f.get(t, id), domain.ThumbnailEditSucceeded)

	f.applier.err = domain.ErrContentChanged
	if _, err := f.edits.Apply(context.Background(), id); !errors.Is(err, domain.ErrContentChanged) {
		t.Fatalf("Apply = %v", err)
	}
	f.wantEnded(t, id)
}

// 捨てると実行を中止してセッションを終える。終わったセッションや無い id を捨てても何も起きない。
// 止めるときは進んでいる実行をすべて中止する。
func TestThumbnailEditDiscardAndClose(t *testing.T) {
	f := newEditsFixture(t)
	discarded := f.start(t, 1).ID
	f.run(t, discarded)
	f.edits.Discard(context.Background(), discarded)
	f.wantEnded(t, discarded)
	f.edits.Discard(context.Background(), discarded)
	f.edits.Discard(context.Background(), "missing")
	if got := f.editor.cancelled(); len(got) != 1 {
		t.Fatalf("cancels = %v", got)
	}

	ready := f.start(t, 2).ID
	f.edits.Discard(context.Background(), ready)
	f.wantEnded(t, ready)

	running := f.start(t, 3).ID
	f.run(t, running)
	f.edits.Close(context.Background())
	f.wantEnded(t, running)
	if got := f.editor.cancelled(); len(got) != 2 || got[1] != "job-2" {
		t.Fatalf("cancels after close = %v", got)
	}
}
