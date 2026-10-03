package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/store"
)

// デスクトップ版の入口の引数で run を起動すると、保存した許可が待ち受けに効き、
// /api/settings/network がつながる（404 でなく、ゲストには 401）。
func TestDesktopRunOptionsApplySavedLANAccess(t *testing.T) {
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
