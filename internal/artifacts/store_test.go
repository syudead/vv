package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

// 既存の MDM_DATA_DIR にある生成物のパスである。規則を通さず文字列で書く。
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

	if !store.PreviewAvailable(key) || !store.SeekThumbnailsAvailable(key) {
		t.Fatal("既存の生成物を無いと答えた")
	}
	file, err := store.ThumbnailFile(key)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := io.ReadAll(file); string(data) != "jpeg" {
		t.Fatalf("サムネイル = %q", data)
	}
	_ = file.Close()
	file, err = store.PreviewFile(key)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := io.ReadAll(file); string(data) != string(payload) {
		t.Fatalf("プレビュー = %q", data)
	}
	_ = file.Close()
	for position, want := range map[int64]string{0: "frame0", 4999: "frame0", 5000: "frame1", 9999: "frame1", -1: "frame0"} {
		data, err := store.SeekThumbnail(key, position)
		if err != nil || string(data) != want {
			t.Fatalf("位置 %d = %q, %v, want %q", position, data, err, want)
		}
	}
	if _, err := store.SeekThumbnail(key, 10_000); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("無いフレームの誤り = %v", err)
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
	var pattern string
	if err := store.PublishSeekThumbnails(key, func(p string) error {
		pattern = p
		for i := range 2 {
			if err := os.WriteFile(fmt.Sprintf(p, i), []byte("frame"), 0o644); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(pattern) != "%06d.jpg" {
		t.Fatalf("連番の型 = %q", pattern)
	}
	if err := store.PublishPreview(ctx, key, writer([]byte("mp4")), nil); err != nil {
		t.Fatal(err)
	}

	thumbnail, frame0, frame1, preview, manifest := existingLayout(root)
	for _, path := range []string{thumbnail, frame0, frame1, preview, manifest} {
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
		if _, err := store.PreviewFile(bad); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: プレビューの誤り = %v", bad, err)
		}
		if _, err := store.SeekThumbnail(bad, 0); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: シークの誤り = %v", bad, err)
		}
		if err := store.RemoveContent(bad); err != nil {
			t.Errorf("%q: 削除の誤り = %v", bad, err)
		}
		called := false
		write := func(string) error { called = true; return nil }
		if store.PublishThumbnail(bad, write) == nil || store.PublishSeekThumbnails(bad, write) == nil ||
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
		store.PublishSeekThumbnails(key, writer([]byte("x"))),
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
			file, err := store.PreviewFile(key)
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
			return s.PublishSeekThumbnails(key, func(p string) error { return failing(fmt.Sprintf(p, 0)) })
		}},
		{name: "フレームが無いシーク", publish: func(s *Store) error {
			return s.PublishSeekThumbnails(key, func(string) error { return nil })
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

// シーク用プレビューは、完成した置き場があれば作り直さない。サムネイルは
// 呼ばれるたびに置き換える。
func TestPublishReusesCompletedSeekThumbnails(t *testing.T) {
	root := t.TempDir()
	_, frame0, _, _, _ := existingLayout(root)
	writeFile(t, frame0, []byte("old"))
	store := New(root)
	called := false
	if err := store.PublishSeekThumbnails(key, func(string) error { called = true; return nil }); err != nil {
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
		if _, err := store.PreviewFile(key); err == nil {
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
