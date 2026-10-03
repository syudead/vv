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

// NetworkSettingsStore は LAN からの接続の許可の保存先である。internal/store の
// *SettingsStore がこれを満たす。
type NetworkSettingsStore interface {
	SaveLANAccess(ctx context.Context, allowed bool) error
}

// NetworkListener はサーバーの待ち受けで、アドレスを変えて開き直せる。cmd/mdm の
// 待ち受けの部品がこれを満たす。
type NetworkListener interface {
	// Reopen は待ち受けを addr で開き直す。開けなければ元のアドレスへ戻して誤りを返す。
	// 確立済みの接続は切らない。
	Reopen(addr string) error
	// Addr は今の待ち受けの実際のアドレスを返す。
	Addr() string
}

// NetworkSettingsOptions は LAN からの接続の設定に必要な依存である。
type NetworkSettingsOptions struct {
	Store    NetworkSettingsStore
	Listener NetworkListener
	// LANAccess は起動時に読んだ保存値で、待ち受けはこれに合うアドレスで開いている。
	LANAccess bool
	// Addresses は上がっている非ループバックの IPv4 アドレスを返す。許可中の GET と PUT の
	// 応答で呼ぶ。
	Addresses func() ([]netip.Addr, error)
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// NetworkSettings は Windows デスクトップ版の LAN からの接続の許可を持ち、切り替えでは
// 待ち受けを開き直してから保存する（specs/037-windows-app/research.md R-14）。
// 待ち受けのアドレスと保存値を食い違わせない。
type NetworkSettings struct {
	store     NetworkSettingsStore
	listener  NetworkListener
	addresses func() ([]netip.Addr, error)
	logger    *slog.Logger

	// mu は切り替えを 1 つずつにし、許可の値と待ち受けを一緒に読む。
	mu        sync.Mutex
	lanAccess bool
}

// NewNetworkSettings は起動時の保存値から設定を作る。
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

// Current は今の許可・ポート・許可中に開けるアドレスを返す。
func (n *NetworkSettings) Current() domain.NetworkSettings {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.currentLocked()
}

// SetLANAccess は許可を切り替える。今と同じ値なら何もしない。新しいアドレスで待ち受けを
// 開けなければ、保存値を変えずに domain.ErrListenFailed を包んで返す。開き直したあとの
// 保存に失敗したら、待ち受けを元のアドレスへ戻して保存の誤りを返す
// （specs/037-windows-app/contracts/network-settings-api.md §3）。
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

	// 開き直したあとは、要求が切れても保存までを済ませ、待ち受けと保存値を食い違わせない。
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
		// 許可は効いているので、アドレスを示せないだけにする。
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

// portOf は "host:port" のポートを返す。
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
