package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/store"
)

// openSettingsStore は移行を済ませた DB の設定の保存先を返す。
func openSettingsStore(t *testing.T) *store.SettingsStore {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db.Settings()
}

// 起動時は保存値が真なら 0.0.0.0、偽か行が無ければ 127.0.0.1 で待ち受ける（R-14）。
func TestStartNetworkSettingsChoosesListenAddressFromSavedValue(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range []struct {
		label string
		saved *bool
		want  string
	}{
		{"行が無い", nil, "127.0.0.1:47880"},
		{"偽", new(false), "127.0.0.1:47880"},
		{"真", new(true), "0.0.0.0:47880"},
	} {
		settingsStore := openSettingsStore(t)
		if tc.saved != nil {
			if err := settingsStore.SaveLANAccess(context.Background(), *tc.saved); err != nil {
				t.Fatal(err)
			}
		}
		addr, settings, err := startNetworkSettings(context.Background(), settingsStore, newReopenableListener(), "127.0.0.1:47880", logger)
		if err != nil {
			t.Fatalf("%s: %v", tc.label, err)
		}
		if addr != tc.want {
			t.Errorf("%s: 待ち受け = %q, want %q", tc.label, addr, tc.want)
		}
		if settings == nil {
			t.Fatalf("%s: 設定が無い", tc.label)
		}
	}
}

// addresses にはループバックと IPv6 を入れない。
func TestLANIPv4ExcludesLoopbackAndIPv6(t *testing.T) {
	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
		&net.IPNet{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},
		&net.IPNet{IP: net.ParseIP("192.168.1.20"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
		&net.IPAddr{IP: net.ParseIP("10.0.0.5")},
	}
	got := lanIPv4(addrs)
	want := []netip.Addr{netip.MustParseAddr("192.168.1.20"), netip.MustParseAddr("10.0.0.5")}
	if !slices.Equal(got, want) {
		t.Fatalf("lanIPv4 = %v, want %v", got, want)
	}

	own, err := lanAddresses()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range own {
		if addr.IsLoopback() || !addr.Is4() {
			t.Errorf("この機械のアドレスにループバックか IPv6 が入った: %v", addr)
		}
	}
}

// 本物の待ち受けと DB で切り替えると、許可中だけ LAN のアドレスから接続でき、addresses が
// 入る。やめると LAN のアドレスからの新しい接続は拒まれる（受け入れ条件 7）。
func TestNetworkSettingsSwitchesRealListener(t *testing.T) {
	lanIP := ownNonLoopbackIPv4(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	settingsStore := openSettingsStore(t)
	listener := newReopenableListener()
	port := freePort(t)

	addr, settings, err := startNetworkSettings(context.Background(), settingsStore, listener, net.JoinHostPort("127.0.0.1", port), logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Listen(addr); err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}), ReadHeaderTimeout: readHeaderTimeout}
	listener.Serve(srv)
	t.Cleanup(func() {
		_ = listener.Close()
		_ = srv.Close()
	})

	lanURL := "http://" + net.JoinHostPort(lanIP, port) + "/"
	if err := getOK(lanURL); err == nil {
		t.Fatal("許可していないのに LAN のアドレスから接続できた")
	}
	if current := settings.Current(); current.LANAccess || len(current.Addresses) != 0 {
		t.Fatalf("許可していないときの設定 = %+v", current)
	}

	allowed, err := settings.SetLANAccess(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := listener.Addr(); !strings.HasPrefix(got, "0.0.0.0:") {
		t.Errorf("許可したあとの待ち受け = %q", got)
	}
	if err := getOK(lanURL); err != nil {
		t.Errorf("許可したのに LAN のアドレスから接続できない: %v", err)
	}
	if !slices.Contains(allowed.Addresses, lanURL) {
		t.Errorf("addresses = %v, %q が無い", allowed.Addresses, lanURL)
	}
	for _, a := range allowed.Addresses {
		if strings.Contains(a, "127.0.0.1") {
			t.Errorf("addresses にループバックが入った: %v", allowed.Addresses)
		}
	}
	if saved, err := settingsStore.LANAccess(context.Background()); err != nil || !saved {
		t.Errorf("保存値 = %v, %v", saved, err)
	}

	if _, err := settings.SetLANAccess(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if got := listener.Addr(); !strings.HasPrefix(got, "127.0.0.1:") {
		t.Errorf("許可をやめたあとの待ち受け = %q", got)
	}
	if err := getOK(lanURL); err == nil {
		t.Error("許可をやめたのに LAN のアドレスから接続できた")
	}
	if err := getOK("http://" + net.JoinHostPort("127.0.0.1", port) + "/"); err != nil {
		t.Errorf("ループバックから接続できない: %v", err)
	}
}
