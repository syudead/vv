package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/store"
)

func TestDesktopRunOptionsApplySavedLANAccess(t *testing.T) {
	// Only startup tool detection is needed; stub binaries keep this wiring test independent of host
	// installations.
	stubMediaCommands(t)
	dataDir := t.TempDir()
	db, err := store.OpenMigratedForTest(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Settings().SaveLANAccess(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	port := freePort(t)
	listening := make(chan struct{})
	stop := make(chan struct{})
	opts := desktopRunOptions(
		Config{Addr: net.JoinHostPort("127.0.0.1", port), DataDir: dataDir, LogLevel: defaultLogLevel},
		io.Discard,
		func() { close(listening) },
		nil,
		stop,
	)
	if !opts.Desktop {
		t.Fatal("デスクトップ版の引数で Desktop が偽")
	}
	server := startServer(func() error { return run(opts) })
	t.Cleanup(func() {
		close(stop)
		select {
		case <-server.Done():
			if err := server.Err(); err != nil {
				t.Errorf("run = %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("run が止まらない")
		}
	})

	select {
	case <-listening:
	case <-server.Done():
		t.Fatalf("run が待ち受けの前に終わった: %v", server.Err())
	case <-time.After(10 * time.Second):
		t.Fatal("待ち受けが始まらない")
	}

	if got := opts.Listener.Addr(); !strings.HasPrefix(got, "0.0.0.0:") {
		t.Errorf("保存値が真のときの待ち受け = %q", got)
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/api/settings/network")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("ゲストの GET /api/settings/network = %d, want 401", resp.StatusCode)
	}
}

// Stub commands pass executable discovery but fail if actually invoked.
func stubMediaCommands(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		file, content := name, "#!/bin/sh\nexit 1\n"
		if runtime.GOOS == "windows" {
			file, content = name+".bat", "@exit /b 1\r\n"
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}
