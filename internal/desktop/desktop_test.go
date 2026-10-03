package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		port int
		ok   bool
	}{
		{name: "none", args: nil, port: DefaultPort, ok: true},
		{name: "port", args: []string{"--port", "47881"}, port: 47881, ok: true},
		{name: "port with equals", args: []string{"--port=1"}, port: 1, ok: true},
		{name: "highest port", args: []string{"--port", "65535"}, port: 65535, ok: true},
		{name: "zero", args: []string{"--port", "0"}},
		{name: "too large", args: []string{"--port", "65536"}},
		{name: "not a number", args: []string{"--port", "http"}},
		{name: "missing value", args: []string{"--port"}},
		{name: "unknown flag", args: []string{"--data", "C:\\data"}},
		{name: "account command", args: []string{"account", "reset-password"}},
		{name: "extra argument", args: []string{"--port", "47881", "x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port, err := ParseArgs(tc.args)
			if tc.ok {
				if err != nil || port != tc.port {
					t.Fatalf("ParseArgs(%q) = %d, %v; want %d", tc.args, port, err, tc.port)
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseArgs(%q) = %d; want an error", tc.args, port)
			}
		})
	}
}

func TestResolvePaths(t *testing.T) {
	base := t.TempDir()
	paths, err := ResolvePaths(base)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "VVMDM")
	want := Paths{
		Root:            root,
		Data:            filepath.Join(root, "data"),
		WebView2:        filepath.Join(root, "webview2"),
		Logs:            filepath.Join(root, "logs"),
		LogFile:         filepath.Join(root, "logs", "vvmdm.log"),
		PreviousLogFile: filepath.Join(root, "logs", "vvmdm.1.log"),
	}
	if paths != want {
		t.Fatalf("ResolvePaths = %+v; want %+v", paths, want)
	}

	for _, bad := range []string{"", "relative"} {
		if _, err := ResolvePaths(bad); err == nil {
			t.Errorf("ResolvePaths(%q) succeeded", bad)
		}
	}
}

func TestPrepareDirsCreatesEveryFolder(t *testing.T) {
	paths, err := ResolvePaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := PrepareDirs(paths); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{paths.Root, paths.Data, paths.WebView2, paths.Logs} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		// 書けるかの確認に使ったファイルを残さない。データの下は子のフォルダだけ。
		for _, entry := range entries {
			if !entry.IsDir() {
				t.Errorf("%s holds %s", dir, entry.Name())
			}
		}
	}
}

func TestPrepareDirsNamesTheUnwritableFolder(t *testing.T) {
	base := t.TempDir()
	// VVMDM をファイルにすると、その下のフォルダを作れない（root でも同じ）。
	blocker := filepath.Join(base, "VVMDM")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := ResolvePaths(base)
	if err != nil {
		t.Fatal(err)
	}
	err = PrepareDirs(paths)
	var unwritable *UnwritableDirError
	if !errors.As(err, &unwritable) {
		t.Fatalf("PrepareDirs = %v; want *UnwritableDirError", err)
	}
	if unwritable.Dir != paths.Root {
		t.Fatalf("Dir = %q; want %q", unwritable.Dir, paths.Root)
	}
}

func TestOpenLogKeepsOnlyThePreviousRun(t *testing.T) {
	paths, err := ResolvePaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := PrepareDirs(paths); err != nil {
		t.Fatal(err)
	}
	for _, run := range []string{"first", "second", "third"} {
		file, err := OpenLog(paths)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString(run); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	assertContent(t, paths.LogFile, "third")
	assertContent(t, paths.PreviousLogFile, "second")
}

// 今回のログを移せなかったときに、前回のログを消してしまわない。移せない場合は
// 今回のログの場所を中身のあるフォルダにして作る（Windows で別のプロセスが
// ログを開いたままのときに相当する）。
func TestOpenLogKeepsThePreviousRunWhenRotationFails(t *testing.T) {
	paths, err := ResolvePaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := PrepareDirs(paths); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PreviousLogFile, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.LogFile, "busy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if file, err := OpenLog(paths); err == nil {
		_ = file.Close()
		t.Fatal("OpenLog succeeded; want an error when the current log cannot be moved")
	}
	assertContent(t, paths.PreviousLogFile, "previous")
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q; want %q", path, got, want)
	}
}

func TestCheckBundle(t *testing.T) {
	base := t.TempDir()
	app := filepath.Join(base, "apps", "VVMDM-1.0.0-windows-amd64")
	temp := filepath.Join(base, "Temp")
	exe := filepath.Join(app, "VVMDM.exe")
	bundled := map[string]bool{
		filepath.Join(app, "ffmpeg", "ffmpeg.exe"):  true,
		filepath.Join(app, "ffmpeg", "ffprobe.exe"): true,
	}
	exists := func(path string) bool { return bundled[path] }

	if err := CheckBundle(exe, temp, exists); err != nil {
		t.Fatalf("extracted folder: %v", err)
	}

	// エクスプローラーで zip の中の exe を開くと、exe だけが一時フォルダへ展開される。
	fromZip := filepath.Join(temp, "Temp1_VVMDM.zip", "VVMDM.exe")
	if err := CheckBundle(fromZip, temp, exists); !errors.Is(err, ErrNotExtracted) {
		t.Fatalf("under temp: %v", err)
	}
	// 一時フォルダの判定は大文字と小文字を区別しない。
	upper := filepath.Join(base, "TEMP", "x", "VVMDM.exe")
	if err := CheckBundle(upper, temp, func(string) bool { return true }); !errors.Is(err, ErrNotExtracted) {
		t.Fatalf("under temp with other case: %v", err)
	}
	// 名前が一時フォルダで始まるだけの隣のフォルダは対象外。
	sibling := filepath.Join(base, "Temporary", "VVMDM.exe")
	if err := CheckBundle(sibling, temp, func(string) bool { return true }); err != nil {
		t.Fatalf("sibling of temp: %v", err)
	}

	delete(bundled, filepath.Join(app, "ffmpeg", "ffprobe.exe"))
	if err := CheckBundle(exe, temp, exists); !errors.Is(err, ErrNotExtracted) {
		t.Fatalf("missing ffprobe: %v", err)
	}
}

func TestPrependPath(t *testing.T) {
	sep := string(os.PathListSeparator)
	if got := PrependPath("ff", "a"+sep+"b"); got != "ff"+sep+"a"+sep+"b" {
		t.Fatalf("PrependPath = %q", got)
	}
	if got := PrependPath("ff", ""); got != "ff" {
		t.Fatalf("PrependPath with empty PATH = %q", got)
	}
}

func TestMessagesNameTheLogExceptForTheUnwritableFolder(t *testing.T) {
	const logFile = `C:\Users\me\AppData\Local\VVMDM\logs\vvmdm.log`
	cause := errors.New("database disk image is malformed")
	for name, text := range map[string]string{
		"not extracted": MessageNotExtracted(logFile),
		"no webview2":   MessageNoWebView2(logFile),
		"database":      MessageDatabase(cause, logFile),
		"port":          MessagePortInUse(47880, logFile),
		"failed":        MessageFailed(cause, logFile),
	} {
		if !strings.Contains(text, logFile) {
			t.Errorf("%s: %q does not name the log", name, text)
		}
	}
	if text := MessageUnwritable(`C:\locked`); !strings.Contains(text, `C:\locked`) || strings.Contains(text, "log") {
		t.Errorf("unwritable: %q", text)
	}
	if text := MessagePortInUse(47880, logFile); !strings.Contains(text, "47880") || !strings.Contains(text, "--port 47881") {
		t.Errorf("port: %q", text)
	}
	if text := MessageNoWebView2(logFile); !strings.Contains(text, WebView2URL) {
		t.Errorf("webview2: %q", text)
	}
	if text := MessageDatabase(cause, logFile); !strings.Contains(text, cause.Error()) {
		t.Errorf("database: %q", text)
	}
	if text := MessageBadArgs(errors.New("unsupported arguments: -x")); !strings.Contains(text, UsageText) {
		t.Errorf("bad args: %q", text)
	}
}

// 二重起動のミューテックスは、同じ利用者・同じデータの置き場（大文字と小文字を
// 区別しない）で同じ名前になり、利用者かデータの置き場が違えば別の名前になる。
// global はセッションをまたぎ、local はセッションごとの名前空間に置く。
func TestInstanceNames(t *testing.T) {
	const sid = "S-1-5-21-1-2-3-1001"
	const root = `C:\Users\me\AppData\Local\VVMDM`
	global, local := InstanceNames(sid, root)
	if !strings.HasPrefix(global, `Global\VVMDM-`+sid+"-") || !strings.HasPrefix(local, `Local\VVMDM-`+sid+"-") {
		t.Fatalf("InstanceNames = %q, %q", global, local)
	}
	if strings.TrimPrefix(global, `Global\`) != strings.TrimPrefix(local, `Local\`) {
		t.Fatalf("global %q and local %q differ beyond the namespace", global, local)
	}
	if same, _ := InstanceNames(sid, strings.ToUpper(root)); same != global {
		t.Errorf("case of the data folder changed the name: %q, %q", same, global)
	}
	if other, _ := InstanceNames("S-1-5-21-1-2-3-1002", root); other == global {
		t.Errorf("another user got the same name %q", other)
	}
	if other, _ := InstanceNames(sid, `D:\VVMDM`); other == global {
		t.Errorf("another data folder got the same name %q", other)
	}
	if len(global) > 260 {
		t.Errorf("the name is longer than MAX_PATH: %d", len(global))
	}
}
