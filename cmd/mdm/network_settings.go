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

type lanAccessStore interface {
	LANAccess(ctx context.Context) (bool, error)
	app.NetworkSettingsStore
}

// startNetworkSettings binds the saved LAN host, defaulting to loopback, with addr's port; updates
// reopen the listener before saving.
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

// Return active non-loopback IPv4 addresses for LAN access URLs.
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
