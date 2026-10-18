package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

type NetworkSettingsStore interface {
	SaveLANAccess(ctx context.Context, allowed bool) error
}

type NetworkListener interface {
	// Reopen rolls back on failure without disconnecting established connections.
	Reopen(addr string) error
	Addr() string
}

type NetworkSettingsOptions struct {
	Store    NetworkSettingsStore
	Listener NetworkListener
	// The listener must already match this saved startup value.
	LANAccess bool
	// Called only while LAN access is enabled; return active non-loopback IPv4 addresses.
	Addresses func() ([]netip.Addr, error)
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// Reopen before saving so the persisted LAN setting and actual listener stay consistent.
type NetworkSettings struct {
	store     NetworkSettingsStore
	listener  NetworkListener
	addresses func() ([]netip.Addr, error)
	logger    *slog.Logger

	// Read permission and listener state under the same lock used for changes.
	mu        sync.Mutex
	lanAccess bool
}

func NewNetworkSettings(opts NetworkSettingsOptions) *NetworkSettings {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &NetworkSettings{
		store:     opts.Store,
		listener:  opts.Listener,
		addresses: opts.Addresses,
		logger:    logger,
		lanAccess: opts.LANAccess,
	}
}

func (n *NetworkSettings) Current() domain.NetworkSettings {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.currentLocked()
}

// SetLANAccess leaves storage unchanged on bind failure; if saving fails after reopening, restore
// the prior listener and return the save error.
func (n *NetworkSettings) SetLANAccess(ctx context.Context, allowed bool) (domain.NetworkSettings, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if allowed == n.lanAccess {
		return n.currentLocked(), nil
	}

	previous := n.listener.Addr()
	port, err := portOf(previous)
	if err != nil {
		return domain.NetworkSettings{}, err
	}
	next := domain.LANListenAddr(allowed, port)
	if err := n.listener.Reopen(next); err != nil {
		n.logger.Warn("could not listen on the new address; kept the previous one",
			slog.String("addr", next), slog.String("previous", previous), slog.Any("error", err))
		return domain.NetworkSettings{}, fmt.Errorf("%w: %w", domain.ErrListenFailed, err)
	}

	// Finish persistence even after request cancellation once the listener changes, preventing
	// divergent state.
	if err := n.store.SaveLANAccess(context.WithoutCancel(ctx), allowed); err != nil {
		if backErr := n.listener.Reopen(previous); backErr != nil {
			n.logger.Error("could not listen on the previous address after the save failed",
				slog.String("addr", previous), slog.Any("error", backErr))
		}
		return domain.NetworkSettings{}, fmt.Errorf("cannot save the LAN access setting: %w", err)
	}
	n.lanAccess = allowed
	n.logger.Info("switched LAN access", slog.Bool("lanAccess", allowed), slog.String("addr", next))
	return n.currentLocked(), nil
}

func (n *NetworkSettings) currentLocked() domain.NetworkSettings {
	port, err := portOf(n.listener.Addr())
	if err != nil {
		n.logger.Warn("cannot read the listening port", slog.Any("error", err))
	}
	settings := domain.NetworkSettings{LANAccess: n.lanAccess, Port: port, Addresses: []string{}}
	if !n.lanAccess || n.addresses == nil {
		return settings
	}
	addrs, err := n.addresses()
	if err != nil {
		// LAN permission is effective even when address discovery fails; omit only the addresses.
		n.logger.Warn("cannot list the network addresses", slog.Any("error", err))
		return settings
	}
	for _, addr := range addrs {
		if !addr.Is4() || addr.IsLoopback() || addr.IsUnspecified() {
			continue
		}
		settings.Addresses = append(settings.Addresses,
			"http://"+net.JoinHostPort(addr.String(), strconv.Itoa(port))+"/")
	}
	return settings
}

func portOf(addr string) (int, error) {
	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("cannot read the port of %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return 0, fmt.Errorf("cannot read the port of %q: %w", addr, err)
	}
	return port, nil
}
