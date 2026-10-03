package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
)

// reopenableListener は HTTP サーバーの待ち受けを作り、アドレスを変えて開き直せる
// 部品である（specs/037-windows-app/research.md R-14 の「待ち受け」）。
//
// 開き直しでは今の待ち受けを閉じてから、同じ http.Server で新しい待ち受けを
// Serve する。http.Server は接続ごとに別の goroutine で応答するので、待ち受けを
// 閉じても確立済みの接続（SSE など）はそのまま続く。同じポートのループバックと
// ワイルドカードを同時には待ち受けないので、新しい待ち受けは古いものを閉じてから開く。
type reopenableListener struct {
	mu sync.Mutex
	// srv は Serve で渡されたサーバー。Serve の前は nil。
	srv *http.Server
	// current は今の待ち受け。開き直しの途中と閉じたあとは nil で、そのあいだに
	// 終わった Serve の失敗は知らせない。
	current net.Listener
	// addr は今の待ち受けの実際のアドレス（ポート 0 で開いたら決まったポート）である。
	// 開き直しに失敗したら、ここへ戻す。
	addr   string
	closed bool
	// failed は今の待ち受けの Serve が終わった理由を 1 度だけ送る。
	failed chan error
}

func newReopenableListener() *reopenableListener {
	return &reopenableListener{failed: make(chan error, 1)}
}

// Listen は addr で待ち受けを開く。Serve より前に 1 度だけ呼ぶ。
func (l *reopenableListener) Listen(addr string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.current != nil || l.closed {
		return errors.New("the listener is already open")
	}
	ln, err := listenTCP(addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	l.current, l.addr = ln, ln.Addr().String()
	return nil
}

// Serve は srv で今の待ち受けの接続を受け始め、すぐに戻る。今の待ち受けの
// Serve が終わると、その理由を Failed へ送る。
func (l *reopenableListener) Serve(srv *http.Server) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.srv = srv
	if l.current != nil {
		go l.serve(l.current)
	}
}

// Failed は今の待ち受けの Serve が終わった理由を受け取る。停止の Shutdown で
// 終わったときは http.ErrServerClosed が届く。
func (l *reopenableListener) Failed() <-chan error {
	return l.failed
}

// Addr は今の待ち受けの実際のアドレスを返す。
func (l *reopenableListener) Addr() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.addr
}

// Reopen は待ち受けを addr で開き直す。開けなければ元のアドレスで待ち受けを
// 開き直し、addr を開けなかった理由を返す。元のアドレスも開けなければ、その理由も
// 添えて返し、待ち受けは無くなる（Failed へ知らせる）。
func (l *reopenableListener) Reopen(addr string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv == nil || l.closed {
		return errors.New("the listener is not serving")
	}

	previous := l.addr
	if l.current != nil {
		old := l.current
		// 先に current を外すので、閉じた待ち受けの Serve の終わりは知らせない。
		l.current = nil
		_ = old.Close()
	}

	ln, err := listenTCP(addr)
	if err == nil {
		l.current, l.addr = ln, ln.Addr().String()
		go l.serve(ln)
		return nil
	}
	reopenErr := fmt.Errorf("cannot listen on %s: %w", addr, err)

	back, backErr := listenTCP(previous)
	if backErr != nil {
		lost := errors.Join(reopenErr, fmt.Errorf("cannot listen on %s again: %w", previous, backErr))
		l.notify(lost)
		return lost
	}
	l.current = back
	go l.serve(back)
	return reopenErr
}

// listenTCP は addr で TCP の待ち受けを開く。ホストが IPv4 のアドレスなら IPv4 だけで
// 待ち受ける。"tcp" のままだと、IPv6 の使える機械では 0.0.0.0 が IPv4 と IPv6 の両方を
// 受ける [::] の待ち受けになり、許可の切り替えが決める 0.0.0.0／127.0.0.1（R-14）から
// 外れる。
func listenTCP(addr string) (net.Listener, error) {
	return net.Listen(listenNetwork(addr), addr)
}

// listenNetwork は addr を待ち受ける net.Listen のネットワークを選ぶ。
func listenNetwork(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "tcp"
	}
	if ip, err := netip.ParseAddr(host); err == nil && ip.Is4() {
		return "tcp4"
	}
	return "tcp"
}

// Close は今の待ち受けを閉じ、以後の開き直しを断る。確立済みの接続は閉じない
// （それは http.Server の Shutdown が行う）。
func (l *reopenableListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.current == nil {
		return nil
	}
	ln := l.current
	l.current = nil
	return ln.Close()
}

// serve は ln の接続を受け、終わったのが今の待ち受けなら理由を知らせる。
func (l *reopenableListener) serve(ln net.Listener) {
	err := l.srv.Serve(ln)
	l.mu.Lock()
	defer l.mu.Unlock()
	if ln != l.current {
		return
	}
	l.notify(err)
}

// notify は Serve の終わりの理由を送る。受け取りは 1 度だけなので、先に送った
// 理由があれば捨てる。
func (l *reopenableListener) notify(err error) {
	select {
	case l.failed <- err:
	default:
	}
}
