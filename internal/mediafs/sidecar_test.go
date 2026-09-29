package mediafs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// sidecarTree は登録フォルダ media/ に動画と隣のファイルを置き、別のフォルダと
// 外側にも字幕を置く。
func sidecarTree(t *testing.T) (root, video, outsideVideo string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "media")
	for _, dir := range []string{
		filepath.Join(root, "show"), filepath.Join(root, "show", "movie.ja"),
		filepath.Join(root, "other"), filepath.Join(base, "outside"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	video = filepath.Join(root, "show", "movie.mp4")
	outsideVideo = filepath.Join(base, "outside", "movie.mp4")
	files := map[string]string{
		video:                                    "video",
		filepath.Join(root, "show", "movie.srt"): "1\n00:00:01,000 --> 00:00:02,000\nhi\n",
		filepath.Join(root, "show", "movie.ja.vtt"):  "WEBVTT\n",
		filepath.Join(root, "other", "movie.en.srt"): "other",
		filepath.Join(root, "x.srt"):                 "parent",
		outsideVideo:                                 "video",
		filepath.Join(base, "outside", "movie.srt"):  "outside",
		filepath.Join(base, "secret.srt"):            "secret",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, video, outsideVideo
}

func sidecarNames(entries []domain.SidecarEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	slices.Sort(names)
	return names
}

func TestListSidecarFilesListsRegularFilesNextToTheVideo(t *testing.T) {
	root, video, _ := sidecarTree(t)
	symlink(t, filepath.Join(filepath.Dir(root), "secret.srt"), filepath.Join(root, "show", "movie.link.srt"))
	entries, err := New().ListSidecarFiles([]string{root}, video)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"movie.ja.vtt", "movie.mp4", "movie.srt"}
	if got := sidecarNames(entries); !slices.Equal(got, want) {
		t.Fatalf("names = %q, want %q (no subfolders, no symlinks)", got, want)
	}
	for _, entry := range entries {
		if entry.Name == "movie.srt" && entry.Size != int64(len("1\n00:00:01,000 --> 00:00:02,000\nhi\n")) {
			t.Fatalf("size of movie.srt = %d", entry.Size)
		}
	}
}

func TestListSidecarFilesUsesTheLocationFolderOfALinkedVideo(t *testing.T) {
	root, video, _ := sidecarTree(t)
	link := filepath.Join(root, "other", "linked.mp4")
	symlink(t, video, link)
	entries, err := New().ListSidecarFiles([]string{root}, link)
	if err != nil {
		t.Fatal(err)
	}
	// 所在（link のあるフォルダ）の隣を探す。辿った先のフォルダではない。
	if got := sidecarNames(entries); !slices.Equal(got, []string{"movie.en.srt"}) {
		t.Fatalf("names = %q, want the files next to the link", got)
	}
}

func TestSidecarFilesRejectVideosOutsideTheRoot(t *testing.T) {
	root, video, outsideVideo := sidecarTree(t)
	escape := filepath.Join(root, "escape.mp4")
	symlink(t, outsideVideo, escape)
	for _, path := range []string{outsideVideo, escape, filepath.Join(root, "show", "missing.mp4"), filepath.Join(root, "show")} {
		if _, err := New().ListSidecarFiles([]string{root}, path); !errors.Is(err, domain.ErrMediaFileUnavailable) {
			t.Errorf("ListSidecarFiles(%q) = %v, want ErrMediaFileUnavailable", path, err)
		}
		if file, _, err := New().OpenSidecarFile([]string{root}, path, "movie.srt"); !errors.Is(err, domain.ErrMediaFileUnavailable) {
			if file != nil {
				_ = file.Close()
			}
			t.Errorf("OpenSidecarFile(%q) = %v, want ErrMediaFileUnavailable", path, err)
		}
	}
	if _, err := New().ListSidecarFiles(nil, video); !errors.Is(err, domain.ErrMediaFileUnavailable) {
		t.Errorf("ListSidecarFiles without roots = %v, want ErrMediaFileUnavailable", err)
	}
}

func TestSidecarFilesRejectAVideoThatCannotBeOpened(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions do not stop this user from opening files")
	}
	root, video, _ := sidecarTree(t)
	if err := os.Chmod(video, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(video, 0o600) })
	// stat はできても開けない所在は、配信（OpenMediaFile）と同じく字幕も探さない。
	if _, err := New().ListSidecarFiles([]string{root}, video); !errors.Is(err, domain.ErrMediaFileUnavailable) {
		t.Fatalf("ListSidecarFiles = %v, want ErrMediaFileUnavailable", err)
	}
}

func TestOpenSidecarFileOpensOnlyListedNames(t *testing.T) {
	root, video, _ := sidecarTree(t)
	symlink(t, filepath.Join(filepath.Dir(root), "secret.srt"), filepath.Join(root, "show", "movie.link.srt"))
	file, info, err := New().OpenSidecarFile([]string{root}, video, "movie.srt")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil || string(body) != "1\n00:00:01,000 --> 00:00:02,000\nhi\n" || info.Size() != int64(len(body)) {
		t.Fatalf("read %q, %v (size %d)", body, err, info.Size())
	}
	refused := []string{
		"../x.srt", "../other/movie.en.srt", "movie.en.srt", "movie.ja", "movie.link.srt",
		"", ".", "..", "MOVIE.SRT", filepath.Join(root, "show", "movie.srt"), "./movie.srt",
	}
	for _, name := range refused {
		file, _, err := New().OpenSidecarFile([]string{root}, video, name)
		if file != nil {
			_ = file.Close()
		}
		if !errors.Is(err, domain.ErrMediaFileUnavailable) {
			t.Errorf("OpenSidecarFile(%q) = %v, want ErrMediaFileUnavailable", name, err)
		}
	}
}
