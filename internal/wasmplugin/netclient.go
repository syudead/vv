package wasmplugin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
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

// netClient はプラグインの要求を送る vv の HTTP クライアントである（049 research.md R-5）。
// プロキシのない実行では、環境変数のプロキシを使わず、名前を解決してから、接続のたびに dialer の
// control hook で解決済みのアドレスを確かめる。リダイレクトの先も同じ dialer で接続するので、
// 同じ確認を通る。運用者がプロキシを選んだ実行では、接続はプロキシへのものなので、要求と
// リダイレクトのたびに接続先の名前を解決して確かめる（056 research.md R-5、session）。
type netClient struct {
	// client はプロキシのない実行のクライアントである。
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
	dialer := &net.Dialer{Control: c.control}
	c.dial = dialer.DialContext
	c.client = newPluginHTTPClient(c.transport(nil, c.dialContext))
	return c
}

// transport はプラグインの要求の Transport を作る。proxy が nil なら環境変数のプロキシも使わない。
// 要求の時間の上限は要求の ctx だけが決める。接続、TLS、応答のヘッダーに別の上限を置くと、
// 10 秒の要求が -4 でなく -3 で終わることがある。
func (c *netClient) transport(
	proxy func(*http.Request) (*url.URL, error), dial func(context.Context, string, string) (net.Conn, error),
) *http.Transport {
	return &http.Transport{
		Proxy:                  proxy,
		DialContext:            dial,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           8,
		IdleConnTimeout:        30 * time.Second,
		MaxResponseHeaderBytes: domain.WorkInfoPluginHTTPMaxHeaderBytes,
	}
}

// newPluginHTTPClient は transport でプラグインの要求を送り、リダイレクトを上限までたどる
// クライアントを作る。
func newPluginHTTPClient(transport *http.Transport) *http.Client {
	return &http.Client{
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
}

func (c *netClient) close() {
	c.client.CloseIdleConnections()
}

// session は 1 回の実行の httpSession を作る。network にプロキシがあれば、その実行の要求を
// すべてプロキシに送るクライアントを使う（research.md R-5）。プロキシの接続はその実行の
// あいだだけ持ち、httpSession.close で閉じる。
func (c *netClient) session(network domain.WorkInfoRunNetwork) *httpSession {
	if network.Proxy == nil {
		return &httpSession{client: c.client}
	}
	proxy := network.Proxy
	var dialer net.Dialer
	transport := c.transport(func(req *http.Request) (*url.URL, error) {
		if err := c.checkDestination(req.Context(), req.URL.Hostname()); err != nil {
			return nil, err
		}
		return proxy, nil
	}, dialer.DialContext)
	// CONNECT を断ったプロキシを、ほかの接続の失敗と見分けられる誤りにする。
	transport.OnProxyConnectResponse = func(_ context.Context, _ *url.URL, _ *http.Request, resp *http.Response) error {
		if resp.StatusCode != http.StatusOK {
			return &proxyRefusedError{status: resp.Status}
		}
		return nil
	}
	return &httpSession{client: newPluginHTTPClient(transport), proxyHost: proxy.Hostname()}
}

// checkDestination はプロキシに送る要求の接続先 host を解決し（リテラルのアドレスはそのまま
// 解析し）、解決したアドレスのどれかを rule が拒めば、host を付けた *refusedError を返す。
// 要求とたどるリダイレクトのたびに Transport の Proxy から呼ばれる。プロキシ自体のアドレスは
// 確かめない。運用者が選んだもので、多くは LAN にある。
func (c *netClient) checkDestination(ctx context.Context, host string) error {
	addrs, err := c.resolve(ctx, host)
	if err != nil {
		return err
	}
	for _, a := range addrs {
		if !c.rule.allows(a) {
			return &refusedError{host: host, addr: a.Unmap().String()}
		}
	}
	return nil
}

// resolve は host がリテラルのアドレスならそれを、名前なら lookup の答えを返す。答えが空なら誤りを返す。
func (c *netClient) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip}, nil
	}
	addrs, err := c.lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("resolve %s: no addresses", host)
	}
	return addrs, nil
}

// proxyRefusedError はプロキシが CONNECT に 200 以外で答えたことを表す。
type proxyRefusedError struct{ status string }

func (e *proxyRefusedError) Error() string { return "proxy refused CONNECT: " + e.status }

// isProxyFailure は、要求の誤り err がプロキシに接続できない、プロキシとの TLS か SOCKS の
// やり取りに失敗した、またはプロキシが CONNECT を断ったことによるかを返す。net/http は
// プロキシへの接続の失敗を Op が "proxyconnect" の *net.OpError で、SOCKS の失敗を Op が
// "socks " で始まる *net.OpError で返す。
func isProxyFailure(err error) bool {
	var refused *proxyRefusedError
	if errors.As(err, &refused) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr) && (opErr.Op == "proxyconnect" || strings.HasPrefix(opErr.Op, "socks "))
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
	addrs, err := c.resolve(ctx, host)
	if err != nil {
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
