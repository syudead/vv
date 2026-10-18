package opener

import (
	"errors"
	"slices"
	"testing"
)

func TestNewOpenerResolvesCommandPerEnvironment(t *testing.T) {
	cases := []struct {
		name      string
		goos      string
		env       map[string]string
		installed map[string]string
		want      string
	}{
		{name: "windows", goos: "windows", installed: map[string]string{"explorer.exe": `C:\Windows\explorer.exe`}, want: `C:\Windows\explorer.exe`},
		{name: "windows without explorer", goos: "windows", want: ""},
		{name: "macOS", goos: "darwin", installed: map[string]string{"open": "/usr/bin/open"}, want: "/usr/bin/open"},
		{name: "macOS without open", goos: "darwin", want: ""},
		{name: "linux with X11", goos: "linux", env: map[string]string{"DISPLAY": ":0"},
			installed: map[string]string{"xdg-open": "/usr/bin/xdg-open"}, want: "/usr/bin/xdg-open"},
		{name: "linux with Wayland", goos: "linux", env: map[string]string{"WAYLAND_DISPLAY": "wayland-0"},
			installed: map[string]string{"xdg-open": "/usr/bin/xdg-open"}, want: "/usr/bin/xdg-open"},
		{name: "linux without display", goos: "linux",
			installed: map[string]string{"xdg-open": "/usr/bin/xdg-open"}, want: ""},
		{name: "linux with display but no xdg-open", goos: "linux", env: map[string]string{"DISPLAY": ":0"}, want: ""},
		{name: "freebsd follows xdg-open", goos: "freebsd", env: map[string]string{"DISPLAY": ":0"},
			installed: map[string]string{"xdg-open": "/usr/local/bin/xdg-open"}, want: "/usr/local/bin/xdg-open"},
		// macOS and Windows do not require display environment variables.
		{name: "macOS ignores DISPLAY", goos: "darwin", env: map[string]string{},
			installed: map[string]string{"open": "/usr/bin/open"}, want: "/usr/bin/open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string { return tc.env[key] }
			lookPath := func(name string) (string, error) {
				if path, ok := tc.installed[name]; ok {
					return path, nil
				}
				return "", errors.New("not found")
			}
			got := newOpener(tc.goos, getenv, lookPath, nil)
			if got.Command() != tc.want {
				t.Errorf("command = %q, want %q", got.Command(), tc.want)
			}
			if got.Available() != (tc.want != "") {
				t.Errorf("available = %v, want %v", got.Available(), tc.want != "")
			}
		})
	}
}

func TestOpenPassesPathAsSingleArgument(t *testing.T) {
	var gotName string
	var gotArgs []string
	start := func(name string, args ...string) error {
		gotName, gotArgs = name, args
		return nil
	}
	lookPath := func(string) (string, error) { return "/usr/bin/open", nil }
	opener := newOpener("darwin", func(string) string { return "" }, lookPath, start)

	path := "/media/a b; rm -rf ~/$(x).mp4"
	if err := opener.Open(path); err != nil {
		t.Fatal(err)
	}
	if gotName != "/usr/bin/open" || !slices.Equal(gotArgs, []string{path}) {
		t.Fatalf("started %q %q", gotName, gotArgs)
	}
}

func TestOpenReportsUnavailableAndStartFailure(t *testing.T) {
	unavailable := newOpener("linux", func(string) string { return "" },
		func(string) (string, error) { return "/usr/bin/xdg-open", nil }, nil)
	if err := unavailable.Open("/media/a.mp4"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}

	boom := errors.New("fork failed")
	failing := newOpener("darwin", func(string) string { return "" },
		func(string) (string, error) { return "/usr/bin/open", nil },
		func(string, ...string) error { return boom })
	if err := failing.Open("/media/a.mp4"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want start failure", err)
	}
}

func TestStartDetached(t *testing.T) {
	if err := startDetached("/nonexistent/opener-test-command"); err == nil {
		t.Fatal("存在しないコマンドの起動が成功した")
	}
}
