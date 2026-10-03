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

// デスクトップ版の入口の引数で run を起動すると、保存した許可が待ち受けに効き、
// /api/settings/network がつながる（404 でなく、ゲストには 401）。
func TestDesktopRunOptionsApplySavedLANAccess(t *testing.T) {
	// 起動前確認は ffmpeg・ffprobe の有無だけを見る。この試験は待ち受けと API を
	// 確かめるので、ffmpeg の無いホスト（CI の Checks）でも動くよう代わりを置く。
	stubMediaCommands(t)
	dataDir := t.TempDir()
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Migrate(context.Background(), db); err != nil {
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

// stubMediaCommands は起動前確認を通すだけの ffmpeg・ffprobe を PATH に置き、
// ホストに入っている本物を使わない。代わりは何もせず失敗で終わる。
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
