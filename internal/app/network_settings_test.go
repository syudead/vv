package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// callTrace は待ち受けと保存先の呼び出しを 1 本の順序で記録する。
type callTrace struct{ calls []string }

func (c *callTrace) record(call string) {
	if c != nil {
		c.calls = append(c.calls, call)
	}
}

// fakeNetworkSettingsStore は保存した値と、保存時の ctx が切れていたかを覚える。
// trace があれば、失敗した保存の試みも含めて記録する。
type fakeNetworkSettingsStore struct {
	saved      []bool
	saveErr    error
	ctxErrSeen error
	trace      *callTrace
}

func (f *fakeNetworkSettingsStore) SaveLANAccess(ctx context.Context, allowed bool) error {
	f.trace.record(fmt.Sprintf("save %v", allowed))
	f.ctxErrSeen = ctx.Err()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, allowed)
	return nil
}

// fakeNetworkListener は今のアドレスを持ち、Reopen の呼び出しを記録する。failOn に
// 含むアドレスでは開けずに元のアドレスのままにする。
type fakeNetworkListener struct {
	addr    string
	reopens []string
	failOn  map[string]error
	trace   *callTrace
}

func (f *fakeNetworkListener) Reopen(addr string) error {
	f.trace.record("reopen " + addr)
	f.reopens = append(f.reopens, addr)
	if err := f.failOn[addr]; err != nil {
		return err
	}
	f.addr = addr
	return nil
}

func (f *fakeNetworkListener) Addr() string { return f.addr }

func staticAddresses(addrs ...string) func() ([]netip.Addr, error) {
	return func() ([]netip.Addr, error) {
		parsed := make([]netip.Addr, 0, len(addrs))
		for _, addr := range addrs {
			parsed = append(parsed, netip.MustParseAddr(addr))
		}
		return parsed, nil
	}
}

func TestNetworkSettingsCurrentHidesAddressesWhileLANAccessIsOff(t *testing.T) {
	called := false
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store:    &fakeNetworkSettingsStore{},
		Listener: &fakeNetworkListener{addr: "127.0.0.1:8080"},
		Addresses: func() ([]netip.Addr, error) {
			called = true
			return nil, nil
		},
		Logger: discardLogger(),
	})
	got := settings.Current()
	want := domain.NetworkSettings{LANAccess: false, Port: 8080, Addresses: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Current() = %+v, want %+v", got, want)
	}
	if called {
		t.Fatal("許可していないのにアドレスを列挙した")
	}
}

func TestNetworkSettingsCurrentListsOnlyReachableIPv4Addresses(t *testing.T) {
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store:     &fakeNetworkSettingsStore{},
		Listener:  &fakeNetworkListener{addr: "0.0.0.0:8080"},
		LANAccess: true,
		// ループバック・未指定・IPv6 は LAN から開けるアドレスとして示さない。
		Addresses: staticAddresses("192.168.1.10", "127.0.0.1", "0.0.0.0", "fe80::1", "10.0.0.5"),
		Logger:    discardLogger(),
	})
	got := settings.Current()
	want := domain.NetworkSettings{
		LANAccess: true,
		Port:      8080,
		Addresses: []string{"http://192.168.1.10:8080/", "http://10.0.0.5:8080/"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Current() = %+v, want %+v", got, want)
	}
}

func TestNetworkSettingsCurrentKeepsLANAccessWhenAddressesFail(t *testing.T) {
	var buf bytes.Buffer
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store:     &fakeNetworkSettingsStore{},
		Listener:  &fakeNetworkListener{addr: "0.0.0.0:8080"},
		LANAccess: true,
		Addresses: func() ([]netip.Addr, error) { return nil, errors.New("interfaces unavailable") },
		Logger:    slog.New(slog.NewTextHandler(&buf, nil)),
	})
	got := settings.Current()
	want := domain.NetworkSettings{LANAccess: true, Port: 8080, Addresses: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Current() = %+v, want %+v", got, want)
	}
	if !strings.Contains(buf.String(), "interfaces unavailable") {
		t.Fatalf("アドレスを列挙できなかった理由がログに無い: %s", buf.String())
	}
}

func TestNetworkSettingsCurrentWithoutAddressesFunc(t *testing.T) {
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store:     &fakeNetworkSettingsStore{},
		Listener:  &fakeNetworkListener{addr: "0.0.0.0:8080"},
		LANAccess: true,
		Logger:    discardLogger(),
	})
	got := settings.Current()
	want := domain.NetworkSettings{LANAccess: true, Port: 8080, Addresses: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Current() = %+v, want %+v", got, want)
	}
}

func TestNetworkSettingsCurrentReportsZeroPortForUnreadableAddress(t *testing.T) {
	var buf bytes.Buffer
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store:    &fakeNetworkSettingsStore{},
		Listener: &fakeNetworkListener{addr: "not-an-address"},
		Logger:   slog.New(slog.NewTextHandler(&buf, nil)),
	})
	if got := settings.Current(); got.Port != 0 {
		t.Fatalf("Port = %d, want 0", got.Port)
	}
	if !strings.Contains(buf.String(), "cannot read the listening port") {
		t.Fatalf("ポートを読めなかったことがログに無い: %s", buf.String())
	}
}

func TestNetworkSettingsUsesDefaultLoggerWhenNil(t *testing.T) {
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store:    &fakeNetworkSettingsStore{},
		Listener: &fakeNetworkListener{addr: "127.0.0.1:8080"},
	})
	if settings.logger == nil {
		t.Fatal("Logger が nil のとき slog の既定を使っていない")
	}
}

func TestNetworkSettingsSetLANAccessSwitchesListenerThenSaves(t *testing.T) {
	for name, tc := range map[string]struct {
		initial   bool
		allowed   bool
		startAddr string
		wantAddr  string
		wantURLs  []string
	}{
		"許可する": {
			initial: false, allowed: true,
			startAddr: "127.0.0.1:8080", wantAddr: "0.0.0.0:8080",
			wantURLs: []string{"http://192.168.1.10:8080/"},
		},
		"許可をやめる": {
			initial: true, allowed: false,
			startAddr: "0.0.0.0:8080", wantAddr: "127.0.0.1:8080",
			wantURLs: []string{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			trace := &callTrace{}
			store := &fakeNetworkSettingsStore{trace: trace}
			listener := &fakeNetworkListener{addr: tc.startAddr, trace: trace}
			settings := NewNetworkSettings(NetworkSettingsOptions{
				Store: store, Listener: listener, LANAccess: tc.initial,
				Addresses: staticAddresses("192.168.1.10"), Logger: discardLogger(),
			})
			got, err := settings.SetLANAccess(context.Background(), tc.allowed)
			if err != nil {
				t.Fatal(err)
			}
			want := domain.NetworkSettings{LANAccess: tc.allowed, Port: 8080, Addresses: tc.wantURLs}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("SetLANAccess() = %+v, want %+v", got, want)
			}
			// 待ち受けを開き直してから保存する。逆順だと開けなかったときに保存値と食い違う。
			wantCalls := []string{"reopen " + tc.wantAddr, fmt.Sprintf("save %v", tc.allowed)}
			if !reflect.DeepEqual(trace.calls, wantCalls) {
				t.Fatalf("calls = %v, want %v", trace.calls, wantCalls)
			}
			if !reflect.DeepEqual(store.saved, []bool{tc.allowed}) {
				t.Fatalf("saved = %v, want [%v]", store.saved, tc.allowed)
			}
			if current := settings.Current(); !reflect.DeepEqual(current, want) {
				t.Fatalf("Current() = %+v, want %+v", current, want)
			}
		})
	}
}

func TestNetworkSettingsSetLANAccessSameValueDoesNothing(t *testing.T) {
	store := &fakeNetworkSettingsStore{}
	listener := &fakeNetworkListener{addr: "0.0.0.0:8080"}
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store: store, Listener: listener, LANAccess: true,
		Addresses: staticAddresses("192.168.1.10"), Logger: discardLogger(),
	})
	got, err := settings.SetLANAccess(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.NetworkSettings{LANAccess: true, Port: 8080, Addresses: []string{"http://192.168.1.10:8080/"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SetLANAccess() = %+v, want %+v", got, want)
	}
	if len(listener.reopens) != 0 || len(store.saved) != 0 {
		t.Fatalf("同じ値なのに開き直しか保存をした: reopens=%v saved=%v", listener.reopens, store.saved)
	}
}

func TestNetworkSettingsSetLANAccessKeepsSettingWhenListenFails(t *testing.T) {
	listenErr := errors.New("address in use")
	trace := &callTrace{}
	store := &fakeNetworkSettingsStore{trace: trace}
	listener := &fakeNetworkListener{
		addr:   "127.0.0.1:8080",
		failOn: map[string]error{"0.0.0.0:8080": listenErr},
		trace:  trace,
	}
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store: store, Listener: listener, Addresses: staticAddresses("192.168.1.10"), Logger: discardLogger(),
	})
	_, err := settings.SetLANAccess(context.Background(), true)
	if !errors.Is(err, domain.ErrListenFailed) || !errors.Is(err, listenErr) {
		t.Fatalf("error = %v, want domain.ErrListenFailed wrapping %v", err, listenErr)
	}
	// 開けなかったら保存を試みない。
	if want := []string{"reopen 0.0.0.0:8080"}; !reflect.DeepEqual(trace.calls, want) {
		t.Fatalf("calls = %v, want %v", trace.calls, want)
	}
	want := domain.NetworkSettings{LANAccess: false, Port: 8080, Addresses: []string{}}
	if current := settings.Current(); !reflect.DeepEqual(current, want) {
		t.Fatalf("Current() = %+v, want %+v", current, want)
	}
}

func TestNetworkSettingsSetLANAccessRestoresListenerWhenSaveFails(t *testing.T) {
	saveErr := errors.New("disk full")
	trace := &callTrace{}
	store := &fakeNetworkSettingsStore{saveErr: saveErr, trace: trace}
	listener := &fakeNetworkListener{addr: "127.0.0.1:8080", trace: trace}
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store: store, Listener: listener, Addresses: staticAddresses("192.168.1.10"), Logger: discardLogger(),
	})
	_, err := settings.SetLANAccess(context.Background(), true)
	if !errors.Is(err, saveErr) {
		t.Fatalf("error = %v, want %v", err, saveErr)
	}
	if errors.Is(err, domain.ErrListenFailed) {
		t.Fatalf("保存の失敗を待ち受けの失敗として返した: %v", err)
	}
	// 新しいアドレスで開き直し、保存に失敗してから元のアドレスへ戻す。
	wantCalls := []string{"reopen 0.0.0.0:8080", "save true", "reopen 127.0.0.1:8080"}
	if !reflect.DeepEqual(trace.calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", trace.calls, wantCalls)
	}
	if listener.addr != "127.0.0.1:8080" {
		t.Fatalf("待ち受けが元のアドレスへ戻っていない: %s", listener.addr)
	}
	want := domain.NetworkSettings{LANAccess: false, Port: 8080, Addresses: []string{}}
	if current := settings.Current(); !reflect.DeepEqual(current, want) {
		t.Fatalf("Current() = %+v, want %+v", current, want)
	}
}

func TestNetworkSettingsSetLANAccessLogsWhenRestoreAlsoFails(t *testing.T) {
	var buf bytes.Buffer
	saveErr := errors.New("disk full")
	listener := &fakeNetworkListener{
		addr:   "127.0.0.1:8080",
		failOn: map[string]error{"127.0.0.1:8080": errors.New("previous address taken")},
	}
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store: &fakeNetworkSettingsStore{saveErr: saveErr}, Listener: listener,
		Logger: slog.New(slog.NewTextHandler(&buf, nil)),
	})
	_, err := settings.SetLANAccess(context.Background(), true)
	if !errors.Is(err, saveErr) {
		t.Fatalf("error = %v, want %v", err, saveErr)
	}
	if !strings.Contains(buf.String(), "previous address taken") {
		t.Fatalf("元のアドレスへ戻せなかったことがログに無い: %s", buf.String())
	}
}

func TestNetworkSettingsSetLANAccessSavesEvenWhenRequestIsCanceled(t *testing.T) {
	store := &fakeNetworkSettingsStore{}
	listener := &fakeNetworkListener{addr: "127.0.0.1:8080"}
	settings := NewNetworkSettings(NetworkSettingsOptions{
		Store: store, Listener: listener, Logger: discardLogger(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := settings.SetLANAccess(ctx, true); err != nil {
		t.Fatal(err)
	}
	if store.ctxErrSeen != nil {
		t.Fatalf("保存に切れた ctx を渡した: %v", store.ctxErrSeen)
	}
	if !reflect.DeepEqual(store.saved, []bool{true}) {
		t.Fatalf("saved = %v, want [true]", store.saved)
	}
}

func TestNetworkSettingsSetLANAccessRejectsUnreadableListenerAddress(t *testing.T) {
	for name, addr := range map[string]string{
		"ポートが無い":   "127.0.0.1",
		"ポートが数でない": "127.0.0.1:http",
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeNetworkSettingsStore{}
			listener := &fakeNetworkListener{addr: addr}
			settings := NewNetworkSettings(NetworkSettingsOptions{
				Store: store, Listener: listener, Logger: discardLogger(),
			})
			_, err := settings.SetLANAccess(context.Background(), true)
			if err == nil || !strings.Contains(err.Error(), "cannot read the port") {
				t.Fatalf("error = %v, want a port read error", err)
			}
			if len(listener.reopens) != 0 || len(store.saved) != 0 {
				t.Fatalf("ポートを読めないのに開き直しか保存をした: reopens=%v saved=%v", listener.reopens, store.saved)
			}
		})
	}
}
