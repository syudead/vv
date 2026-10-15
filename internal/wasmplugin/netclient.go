package wasmplugin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// errAddressRefused は接続しようとしたアドレスを addressRule が拒んだことを表す。
var errAddressRefused = errors.New("address refused for work-info plugins")

// refusedError は拒んだ接続のホスト名とアドレスを持つ。ホスト名は名前解決の前の、URL の
// ホストである。
type refusedError struct {
	host string
	addr string
}

func (e *refusedError) Error() string {
	return fmt.Sprintf("work-info plugin request to %s (%s): %v", e.host, e.addr, errAddressRefused)
}

func (e *refusedError) Unwrap() error { return errAddressRefused }

// netClient はプラグインの要求を送る vv の HTTP クライアントである（research.md R-5）。
// 環境変数のプロキシを使わず、名前を解決してから、接続のたびに dialer の control hook で
// 解決済みのアドレスを確かめる。リダイレクトの先も同じ dialer で接続するので、同じ確認を通る。
type netClient struct {
	client *http.Client
	rule   addressRule
	// lookup はホスト名を解決する。テストでは差し替える。
	lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	// dial は解決済みのアドレス（ip:port）に接続する。既定は control hook で rule を確かめる
	// net.Dialer で、テストでは差し替える。
	dial func(ctx context.Context, network, address string) (net.Conn, error)
}

func newNetClient() *netClient {
	c := &netClient{
		rule: addressRule{local: interfaceAddrs},
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
	}
	// 要求の時間の上限は要求の ctx だけが決める。接続、TLS、応答のヘッダーに別の上限を置くと、
	// 10 秒の要求が -4 でなく -3 で終わることがある。
	dialer := &net.Dialer{Control: c.control}
	c.dial = dialer.DialContext
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            c.dialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           8,
		IdleConnTimeout:        30 * time.Second,
		MaxResponseHeaderBytes: domain.WorkInfoPluginHTTPMaxHeaderBytes,
	}
	c.client = &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			// via は元の要求を含むので、6 回目のリダイレクトで長さが 6 になる。そのときは
			// たどらずに、そのリダイレクトの応答をプラグインに渡す。
			if len(via) > domain.WorkInfoPluginHTTPMaxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	return c
}

func (c *netClient) close() {
	c.client.CloseIdleConnections()
}

// control は接続の直前に、接続するアドレスが rule に許されるかを確かめる。
func (c *netClient) control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil || !c.rule.allows(ap.Addr()) {
		return errAddressRefused
	}
	return nil
}

// dialContext は Transport の接続である。URL のホストを解決し、解決したアドレスに順に接続する。
// 1 つのアドレスは残りの時間を残りのアドレスで分けた分だけ試すので、パケットを捨てる先頭の
// アドレスが要求の時間を使い切らない。拒まれたアドレスがあれば、残りを試さずにホスト名を
// 付けた *refusedError を返す。
func (c *netClient) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	var addrs []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{ip}
	} else if addrs, err = c.lookup(ctx, host); err != nil {
		return nil, err
	}
	var errs []error
	for i, a := range addrs {
		target := net.JoinHostPort(a.Unmap().String(), port)
		conn, err := c.dialOne(ctx, network, target, len(addrs)-i)
		if errors.Is(err, errAddressRefused) {
			return nil, &refusedError{host: host, addr: target}
		}
		if err == nil {
			return conn, nil
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("resolve %s: no addresses", host)
	}
	return nil, errors.Join(errs...)
}

// minDialShare は 1 つのアドレスに与える接続の時間の下限である。残りの時間がこれより短いときは
// 残りの時間をすべて与える。net.Dialer が複数のアドレスを順に試すときの下限と同じ値である。
const minDialShare = 2 * time.Second

// dialOne は target に接続する。ctx に期限があれば、その残りを remaining 個のアドレスで分けた
// 時間（minDialShare 以上）をこの接続の期限にする。
func (c *netClient) dialOne(ctx context.Context, network, target string, remaining int) (net.Conn, error) {
	deadline, ok := ctx.Deadline()
	if !ok || remaining <= 1 {
		return c.dial(ctx, network, target)
	}
	left := time.Until(deadline)
	share := max(left/time.Duration(remaining), min(minDialShare, left))
	dialCtx, cancel := context.WithTimeout(ctx, share)
	defer cancel()
	return c.dial(dialCtx, network, target)
}
