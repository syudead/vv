package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
)

// lanAccessStore は LAN からの接続の許可の保存先である。internal/store の
// *SettingsStore がこれを満たす。
type lanAccessStore interface {
	LANAccess(ctx context.Context) (bool, error)
	app.NetworkSettingsStore
}

// startNetworkSettings はデスクトップ版の LAN からの接続の許可を、DB を開いたあと待ち受けの
// 前に読み、待ち受けるアドレスと切り替えの設定を返す。許可していれば 0.0.0.0、
// していないか保存値が無ければ 127.0.0.1 で、addr のポートで待ち受ける
// （specs/037-windows-app/research.md R-14）。切り替えは listener を開き直してから保存する。
func startNetworkSettings(
	ctx context.Context,
	store lanAccessStore,
	listener app.NetworkListener,
	addr string,
	logger *slog.Logger,
) (string, *app.NetworkSettings, error) {
	allowed, err := store.LANAccess(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("cannot read the LAN access setting: %w", err)
	}
	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return "", nil, fmt.Errorf("cannot read the port of %q: %w", addr, err)
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		return "", nil, fmt.Errorf("cannot read the port of %q: %w", addr, err)
	}
	logger.Info("LAN access loaded", slog.Bool("lanAccess", allowed))
	settings := app.NewNetworkSettings(app.NetworkSettingsOptions{
		Store:     store,
		Listener:  listener,
		LANAccess: allowed,
		Addresses: lanAddresses,
		Logger:    logger,
	})
	return domain.LANListenAddr(allowed, port), settings, nil
}

// lanAddresses は上がっているネットワークインターフェースの、ループバックでない IPv4
// アドレスを返す。許可中に LAN の端末から開く URL を示すのに使う。
func lanAddresses() ([]netip.Addr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("cannot list the network interfaces: %w", err)
	}
	var out []netip.Addr
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("cannot list the addresses of %s: %w", iface.Name, err)
		}
		out = append(out, lanIPv4(addrs)...)
	}
	return out, nil
}

// lanIPv4 はインターフェースのアドレスから、ループバックでない IPv4 だけを取り出す。
func lanIPv4(addrs []net.Addr) []netip.Addr {
	var out []netip.Addr
	for _, addr := range addrs {
		var ip net.IP
		switch a := addr.(type) {
		case *net.IPNet:
			ip = a.IP
		case *net.IPAddr:
			ip = a.IP
		default:
			continue
		}
		parsed, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		parsed = parsed.Unmap()
		if !parsed.Is4() || parsed.IsLoopback() || parsed.IsUnspecified() {
			continue
		}
		out = append(out, parsed)
	}
	return out
}
