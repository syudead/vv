package artifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// key は実際の形（"<16進>:<サイズ>"）の content key である。
const key = "ab12cd34:5678"

// 既存の MDM_DATA_DIR にある生成物のパスである。frame0・frame1 はスプライトに
// する前の形式（個別 JPEG）のシーク用プレビューである。規則を通さず文字列で書く。
// 規則の実装からパスを作ると、規則を変えたときにテストも一緒に変わり、
// 既存の生成物が読めなくなったことに気付けない。
func existingLayout(root string) (thumbnail, frame0, frame1, preview, manifest string) {
	return filepath.Join(root, "ab", "ab12cd34_5678.jpg"),
		filepath.Join(root, "seek", "ab", "ab12cd34_5678", "000000.jpg"),
		filepath.Join(root, "seek", "ab", "ab12cd34_5678", "000001.jpg"),
		filepath.Join(root, "preview", "ab", "ab12cd34_5678.mp4"),
		filepath.Join(root, "preview", "ab", "ab12cd34_5678.mp4.sha256")
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func manifestFor(t *testing.T, payload []byte, size int64) []byte {
	t.Helper()
	digest := sha256.Sum256(payload)
	return fmt.Appendf(nil, `{"version":1,"size":%d,"sha256":%q}`+"\n", size, hex.EncodeToString(digest[:]))
}

// sheetJPEG は width × height の JPEG を返す。
func sheetJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// spriteLayout は 2 シートにまたがる 150 コマの配置である。
var spriteLayout = domain.NewSeekSpriteLayout(150 * 5000)

// sheetWriter は受け取ったディレクトリへ、1 コマ 32 × 18 のシートを sheets 枚書く
// 生成の代わりである。
func sheetWriter(t *testing.T, sheets int) func(string) error {
	return func(dir string) error {
		for i := range sheets {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d.jpg", i)), sheetJPEG(t, 320, 180), 0o644); err != nil {
				return err
			}
		}
		return nil
	}
}

// seekDirOf は key のシーク用プレビューの置き場（既存の並べ方）である。
func seekDirOf(root string) string {
	return filepath.Join(root, "seek", "ab", "ab12cd34_5678")
}

// writer は受け取ったパスへ data を書く生成の代わりである。
func writer(data []byte) func(string) error {
	return func(output string) error { return os.WriteFile(output, data, 0o644) }
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func temporaryEntries(t *testing.T, root string) []string {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(root, ".tmp", "*"))
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// この変更より前に作った生成物は、そのままの場所で確認・配信・削除できる。
func TestReadsExistingLayout(t *testing.T) {
	root := t.TempDir()
	thumbnail, frame0, frame1, preview, manifest := existingLayout(root)
	payload := []byte("preview-mp4")
	writeFile(t, thumbnail, []byte("jpeg"))
	writeFile(t, frame0, []byte("frame0"))
	writeFile(t, frame1, []byte("frame1"))
	writeFile(t, preview, payload)
	writeFile(t, manifest, manifestFor(t, payload, int64(len(payload))))
	store := New(root)

	if !store.PreviewAvailable(key) {
		t.Fatal("既存の生成物を無いと答えた")
	}
	// 配置情報の無い旧形式のシーク用プレビューは未完成で、配信しない。
	if store.SeekThumbnailsAvailable(key) {
		t.Fatal("旧形式のシーク用プレビューを完成と答えた")
	}
	if _, err := store.SeekSprite(key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("旧形式の配置情報の誤り = %v", err)
	}
	file, err := store.ThumbnailFile(key)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := io.ReadAll(file); string(data) != "jpeg" {
		t.Fatalf("サムネイル = %q", data)
	}
	_ = file.Close()
	file, digest, err := store.PreviewFile(key)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := io.ReadAll(file); string(data) != string(payload) {
		t.Fatalf("プレビュー = %q", data)
	}
	_ = file.Close()
	if sum := sha256.Sum256(payload); digest != hex.EncodeToString(sum[:]) {
		t.Fatalf("プレビューのダイジェスト = %q", digest)
	}

	// 別の内容の生成物には触れない。
	other := filepath.Join(root, "cd", "cd00_1.jpg")
	writeFile(t, other, []byte("x"))
	if err := store.RemoveContent(key); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{thumbnail, filepath.Dir(frame0), preview, manifest} {
		if exists(path) {
			t.Errorf("%s が残っている", path)
		}
	}
	if !exists(other) {
		t.Error("別の内容の生成物が消えた")
	}
	// 2回目は何も無いが失敗しない。
	if err := store.RemoveContent(key); err != nil {
		t.Fatal(err)
	}
}

// 公開した生成物は既存と同じ場所に置かれる。
func TestPublishesIntoExistingLayout(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	ctx := context.Background()

	if err := store.PublishThumbnail(key, writer([]byte("jpeg"))); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishSeekThumbnails(key, spriteLayout, sheetWriter(t, 2)); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishPreview(ctx, key, writer([]byte("mp4")), nil); err != nil {
		t.Fatal(err)
	}

	thumbnail, _, _, preview, manifest := existingLayout(root)
	seek := seekDirOf(root)
	for _, path := range []string{
		thumbnail, preview, manifest,
		filepath.Join(seek, "000.jpg"), filepath.Join(seek, "001.jpg"), filepath.Join(seek, "sprite.json"),
	} {
		if !exists(path) {
			t.Errorf("%s に公開されていない", path)
		}
	}
	data, err := os.ReadFile(manifest)
	if err != nil || string(data) != string(manifestFor(t, []byte("mp4"), 3)) {
		t.Fatalf("manifest = %q, %v", data, err)
	}
	if entries := temporaryEntries(t, root); len(entries) != 0 {
		t.Fatalf("一時置き場に残っている: %v", entries)
	}
}

// 置き場の外や置き場自身の名前を指しうる content key からはパスを作らない。
func TestRejectsKeysOutsideTheStore(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "thumbnails")
	sentinel := filepath.Join(parent, "sentinel")
	writeFile(t, sentinel, []byte("x"))
	writeFile(t, filepath.Join(root, "seek", "keep"), []byte("x"))
	writeFile(t, filepath.Join(root, ".tmp", "keep"), []byte("x"))
	store := New(root)
	ctx := context.Background()

	for _, bad := range []string{"", ".", "..", "..ab", ".tmp", "../x", `..\x`} {
		if store.PreviewAvailable(bad) || store.SeekThumbnailsAvailable(bad) {
			t.Errorf("%q: あると答えた", bad)
		}
		if _, err := store.ThumbnailFile(bad); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: サムネイルの誤り = %v", bad, err)
		}
		if _, _, err := store.PreviewFile(bad); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: プレビューの誤り = %v", bad, err)
		}
		if _, err := store.SeekSprite(bad); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: シークの誤り = %v", bad, err)
		}
		if _, err := store.SeekSpriteSheet(bad, 0); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: シートの誤り = %v", bad, err)
		}
		if err := store.RemoveContent(bad); err != nil {
			t.Errorf("%q: 削除の誤り = %v", bad, err)
		}
		called := false
		write := func(string) error { called = true; return nil }
		if store.PublishThumbnail(bad, write) == nil || store.PublishSeekThumbnails(bad, spriteLayout, write) == nil ||
			store.PublishPreview(ctx, bad, write, nil) == nil || called {
			t.Errorf("%q: 公開した", bad)
		}
	}
	for _, path := range []string{sentinel, filepath.Join(root, "seek", "keep"), filepath.Join(root, ".tmp", "keep")} {
		if !exists(path) {
			t.Errorf("%s が消えた", path)
		}
	}
}

// 根が設定されていなければ、どの生成物も無く、公開もしない。
func TestEmptyRootHasNothing(t *testing.T) {
	store := New("")
	if store.PreviewAvailable(key) || store.SeekThumbnailsAvailable(key) {
		t.Fatal("根が無いのにあると答えた")
	}
	if _, err := store.ThumbnailFile(key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("誤り = %v", err)
	}
	for _, err := range []error{
		store.PublishThumbnail(key, writer([]byte("x"))),
		store.PublishSeekThumbnails(key, spriteLayout, sheetWriter(t, 2)),
		store.PublishPreview(context.Background(), key, writer([]byte("x")), nil),
	} {
		if !errors.Is(err, errNotConfigured) {
			t.Fatalf("根が無いときの誤り = %v", err)
		}
	}
	if err := store.RemoveTemporary(); err != nil {
		t.Fatal(err)
	}
}

// 表示と配信は、manifest があり大きさが一致するプレビューだけを完成とみなす。
func TestPreviewCompleteness(t *testing.T) {
	payload := []byte("preview-mp4")
	tests := []struct {
		name     string
		mutate   func(t *testing.T, preview, manifest string)
		complete bool
	}{
		{name: "揃っている", mutate: func(*testing.T, string, string) {}, complete: true},
		{name: "MP4 が無い", mutate: func(t *testing.T, preview, _ string) { _ = os.Remove(preview) }},
		{name: "manifest が無い", mutate: func(t *testing.T, _, manifest string) { _ = os.Remove(manifest) }},
		{name: "MP4 が空", mutate: func(t *testing.T, preview, _ string) { writeFile(t, preview, nil) }},
		{name: "大きさが違う", mutate: func(t *testing.T, preview, _ string) { writeFile(t, preview, []byte("short")) }},
		{name: "manifest の大きさが違う", mutate: func(t *testing.T, _, manifest string) {
			writeFile(t, manifest, manifestFor(t, payload, 99))
		}},
		{name: "manifest の版が違う", mutate: func(t *testing.T, _, manifest string) {
			writeFile(t, manifest, []byte(`{"version":2,"size":11,"sha256":"`+hex.EncodeToString(make([]byte, 32))+`"}`))
		}},
		{name: "MP4 が空で manifest も大きさ 0", mutate: func(t *testing.T, preview, manifest string) {
			writeFile(t, preview, nil)
			writeFile(t, manifest, manifestFor(t, nil, 0))
		}},
		{name: "manifest が壊れている", mutate: func(t *testing.T, _, manifest string) { writeFile(t, manifest, []byte("{")) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			_, _, _, preview, manifest := existingLayout(root)
			writeFile(t, preview, payload)
			writeFile(t, manifest, manifestFor(t, payload, int64(len(payload))))
			tt.mutate(t, preview, manifest)
			store := New(root)

			if got := store.PreviewAvailable(key); got != tt.complete {
				t.Fatalf("PreviewAvailable = %v, want %v", got, tt.complete)
			}
			file, _, err := store.PreviewFile(key)
			if tt.complete {
				if err != nil {
					t.Fatal(err)
				}
				_ = file.Close()
			} else if !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("PreviewFile の誤り = %v", err)
			}
		})
	}
}

// 生成のときは、digest まで一致する既存のプレビューだけを採用する。
func TestPublishPreviewAdoptsOnlyVerifiedAsset(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	_, _, _, preview, manifest := existingLayout(root)
	payload := []byte("preview-mp4")
	writeFile(t, preview, payload)
	writeFile(t, manifest, manifestFor(t, payload, int64(len(payload))))
	store := New(root)

	called := false
	if err := store.PublishPreview(ctx, key, func(string) error { called = true; return nil }, nil); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("完成したプレビューを作り直した")
	}

	// 大きさは同じで中身が壊れている。
	writeFile(t, preview, []byte("preview-mpX"))
	if err := store.PublishPreview(ctx, key, writer([]byte("regenerated")), nil); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(preview); string(data) != "regenerated" {
		t.Fatalf("壊れたプレビューを採用した: %q", data)
	}
	if verifyPreview(preview, manifest) != nil {
		t.Fatal("作り直したプレビューが manifest と一致しない")
	}
}

// 生成に失敗したり、公開の直前に元が変わっていたりしたら、何も公開せず、
// 一時置き場にも残さない。
func TestPublishFailureLeavesNothing(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("ffmpeg exited 1")
	failing := func(output string) error {
		_ = os.WriteFile(output, []byte("partial"), 0o644)
		return failure
	}
	tests := []struct {
		name    string
		publish func(*Store) error
		want    error
	}{
		{name: "サムネイル", want: failure, publish: func(s *Store) error { return s.PublishThumbnail(key, failing) }},
		{name: "空のサムネイル", publish: func(s *Store) error { return s.PublishThumbnail(key, writer(nil)) }},
		{name: "シーク", want: failure, publish: func(s *Store) error {
			return s.PublishSeekThumbnails(key, spriteLayout, func(dir string) error {
				return failing(filepath.Join(dir, "000.jpg"))
			})
		}},
		{name: "シートが無いシーク", publish: func(s *Store) error {
			return s.PublishSeekThumbnails(key, spriteLayout, func(string) error { return nil })
		}},
		{name: "シートが足りないシーク", publish: func(s *Store) error {
			return s.PublishSeekThumbnails(key, spriteLayout, sheetWriter(t, 1))
		}},
		{name: "JPEG でないシーク", publish: func(s *Store) error {
			return s.PublishSeekThumbnails(key, spriteLayout, func(dir string) error {
				for _, name := range []string{"000.jpg", "001.jpg"} {
					if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
						return err
					}
				}
				return nil
			})
		}},
		{name: "格子に割り切れないシーク", publish: func(s *Store) error {
			return s.PublishSeekThumbnails(key, spriteLayout, func(dir string) error {
				for _, name := range []string{"000.jpg", "001.jpg"} {
					if err := os.WriteFile(filepath.Join(dir, name), sheetJPEG(t, 325, 180), 0o644); err != nil {
						return err
					}
				}
				return nil
			})
		}},
		{name: "プレビュー", want: failure, publish: func(s *Store) error {
			return s.PublishPreview(ctx, key, failing, nil)
		}},
		{name: "元が変わったプレビュー", want: domain.ErrPreviewStale, publish: func(s *Store) error {
			return s.PublishPreview(ctx, key, writer([]byte("mp4")), func(context.Context) (bool, error) { return false, nil })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			err := tt.publish(New(root))
			if err == nil || (tt.want != nil && !errors.Is(err, tt.want)) {
				t.Fatalf("誤り = %v, want %v", err, tt.want)
			}
			thumbnail, frame0, _, preview, manifest := existingLayout(root)
			for _, path := range []string{thumbnail, filepath.Dir(frame0), preview, manifest} {
				if exists(path) {
					t.Errorf("%s に公開した", path)
				}
			}
			if entries := temporaryEntries(t, root); len(entries) != 0 {
				t.Errorf("一時置き場に残っている: %v", entries)
			}
		})
	}
}

// シーク用プレビューは、配置情報のある完成した置き場があれば作り直さない。
// サムネイルは呼ばれるたびに置き換える。
func TestPublishReusesCompletedSeekThumbnails(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	if err := store.PublishSeekThumbnails(key, spriteLayout, sheetWriter(t, 2)); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := store.PublishSeekThumbnails(key, spriteLayout, func(string) error { called = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("完成したシーク用プレビューを作り直した")
	}

	thumbnail, _, _, _, _ := existingLayout(root)
	writeFile(t, thumbnail, []byte("old"))
	if err := store.PublishThumbnail(key, writer([]byte("new"))); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(thumbnail); string(data) != "new" {
		t.Fatalf("サムネイル = %q", data)
	}
}

// 生成途中の成果物は1か所にまとまっていて、起動時にそこだけを消せる。
// 公開した生成物には触れず、生成途中のものは確認にも配信にも見えない。
func TestRemoveTemporaryKeepsPublishedArtifacts(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	if err := store.PublishThumbnail("kept:1", writer([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	var visible []bool
	err := store.PublishPreview(context.Background(), key, func(output string) error {
		if err := os.WriteFile(output, []byte("mp4"), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(output+".sha256", manifestFor(t, []byte("mp4"), 3), 0o644); err != nil {
			return err
		}
		visible = append(visible, store.PreviewAvailable(key))
		if _, _, err := store.PreviewFile(key); err == nil {
			visible = append(visible, true)
		}
		return errors.New("stop here")
	}, nil)
	if err == nil || slices.Contains(visible, true) {
		t.Fatalf("生成途中のプレビューが見えた: %v, %v", visible, err)
	}
	writeFile(t, filepath.Join(root, ".tmp", "preview-left", "preview.mp4"), []byte("途中"))

	if err := store.RemoveTemporary(); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(root, ".tmp")) {
		t.Error("生成途中の成果物が残っている")
	}
	if !exists(filepath.Join(root, "ke", "kept_1.jpg")) {
		t.Error("公開した生成物が消えた")
	}
	if err := store.RemoveTemporary(); err != nil {
		t.Fatal(err)
	}
}

// 同じ置き場を使う別の Store（別のプロセスに相当）でも、プレビューの公開は
// 直列になり、後の側は先に公開されたものを採用する。
func TestPublishPreviewSerializesAcrossStores(t *testing.T) {
	root := t.TempDir()
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- New(root).PublishPreview(context.Background(), key, writer([]byte("mp4")),
			func(context.Context) (bool, error) {
				close(entered)
				<-release
				return true, nil
			})
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("先の公開が確認まで進まない")
	}
	secondWrote := make(chan struct{}, 1)
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- New(root).PublishPreview(context.Background(), key, func(output string) error {
			secondWrote <- struct{}{}
			return os.WriteFile(output, []byte("mp4"), 0o644)
		}, nil)
	}()
	select {
	case <-secondWrote:
		t.Fatal("先の公開が錠を持つ間に、後の側が生成を始めた")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondWrote:
		t.Fatal("後の側が完成したプレビューを作り直した")
	default:
	}
	if !New(root).PreviewAvailable(key) {
		t.Fatal("公開したプレビューが無い")
	}
}

// 公開したスプライトの配置情報は、配置とシート 000.jpg の寸法から決まり、
// シートは番号で読める。
func TestSeekSpriteDescribesPublishedSheets(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	if err := store.PublishSeekThumbnails(key, spriteLayout, sheetWriter(t, 2)); err != nil {
		t.Fatal(err)
	}
	if !store.SeekThumbnailsAvailable(key) {
		t.Fatal("公開したスプライトを未完成と答えた")
	}
	sprite, err := store.SeekSprite(key)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.SeekSprite{SeekSpriteLayout: spriteLayout, FrameWidth: 32, FrameHeight: 18}
	if sprite != want {
		t.Fatalf("配置情報 = %+v, want %+v", sprite, want)
	}
	data, err := os.ReadFile(filepath.Join(seekDirOf(root), "sprite.json"))
	if err != nil {
		t.Fatal(err)
	}
	const wantJSON = `{"version":1,"intervalMs":5000,"frameCount":150,"columns":10,"rows":10,` +
		`"frameWidth":32,"frameHeight":18,"sheetCount":2}` + "\n"
	if string(data) != wantJSON {
		t.Fatalf("sprite.json = %s", data)
	}
	for sheet := range 2 {
		image, err := store.SeekSpriteSheet(key, sheet)
		if err != nil || !bytes.Equal(image, sheetJPEG(t, 320, 180)) {
			t.Fatalf("シート %d = %d バイト, %v", sheet, len(image), err)
		}
	}
	for _, sheet := range []int{-1, 2, domain.SeekSpriteMaxSheets} {
		if _, err := store.SeekSpriteSheet(key, sheet); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("シート %d の誤り = %v", sheet, err)
		}
	}
}

// 公開は、旧形式の個別 JPEG や配置情報の無い壊れた置き場を消して、シートと
// sprite.json に置き換える。
func TestPublishSeekSpriteReplacesIncompleteDirectory(t *testing.T) {
	for name, prepare := range map[string]func(t *testing.T, root string){
		"旧形式": func(t *testing.T, root string) {
			_, frame0, frame1, _, _ := existingLayout(root)
			writeFile(t, frame0, []byte("frame0"))
			writeFile(t, frame1, []byte("frame1"))
		},
		"配置情報が無い": func(t *testing.T, root string) {
			writeFile(t, filepath.Join(seekDirOf(root), "000.jpg"), []byte("partial"))
		},
		"配置情報が壊れている": func(t *testing.T, root string) {
			writeFile(t, filepath.Join(seekDirOf(root), "000.jpg"), sheetJPEG(t, 320, 180))
			writeFile(t, filepath.Join(seekDirOf(root), "sprite.json"), []byte(`{"version":2}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			prepare(t, root)
			store := New(root)
			if store.SeekThumbnailsAvailable(key) {
				t.Fatal("未完成の置き場を完成と答えた")
			}
			called := false
			if err := store.PublishSeekThumbnails(key, spriteLayout, func(dir string) error {
				called = true
				return sheetWriter(t, 2)(dir)
			}); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("未完成の置き場を採用した")
			}
			entries, err := os.ReadDir(seekDirOf(root))
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			if want := []string{"000.jpg", "001.jpg", "sprite.json"}; !slices.Equal(names, want) {
				t.Fatalf("置き場の中身 = %v, want %v", names, want)
			}
			if !store.SeekThumbnailsAvailable(key) {
				t.Fatal("置き換えたスプライトを未完成と答えた")
			}
			if entries := temporaryEntries(t, root); len(entries) != 0 {
				t.Fatalf("一時置き場に残っている: %v", entries)
			}
		})
	}
}

// 形の違う配置情報は、無いものではなく読めないものとして返す（配信は 500）。
func TestSeekSpriteRejectsMalformedDescription(t *testing.T) {
	for name, body := range map[string]string{
		"壊れている":     "{",
		"版が違う":      `{"version":2,"intervalMs":5000,"frameCount":1,"columns":10,"rows":10,"frameWidth":2,"frameHeight":2,"sheetCount":1}`,
		"枚数が食い違う":   `{"version":1,"intervalMs":5000,"frameCount":150,"columns":10,"rows":10,"frameWidth":2,"frameHeight":2,"sheetCount":1}`,
		"間隔が短い":     `{"version":1,"intervalMs":1000,"frameCount":1,"columns":10,"rows":10,"frameWidth":2,"frameHeight":2,"sheetCount":1}`,
		"コマが上限を超える": `{"version":1,"intervalMs":5000,"frameCount":700,"columns":10,"rows":10,"frameWidth":2,"frameHeight":2,"sheetCount":7}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(seekDirOf(root), "sprite.json"), []byte(body))
			store := New(root)
			if store.SeekThumbnailsAvailable(key) {
				t.Fatal("形の違う配置情報を完成と答えた")
			}
			if _, err := store.SeekSprite(key); err == nil || errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("誤り = %v", err)
			}
		})
	}
}

// RemoveContent はスプライトの置き場を丸ごと消す。
func TestRemoveContentRemovesSeekSprite(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	if err := store.PublishSeekThumbnails(key, spriteLayout, sheetWriter(t, 2)); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveContent(key); err != nil {
		t.Fatal(err)
	}
	if exists(seekDirOf(root)) || store.SeekThumbnailsAvailable(key) {
		t.Fatal("スプライトが残っている")
	}
}
