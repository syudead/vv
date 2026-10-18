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

var errAddressRefused = errors.New("address refused for work-info plugins")

// Keep the original URL hostname alongside the refused resolved address.
type refusedError struct {
	host string
	addr string
}

func (e *refusedError) Error() string {
	return fmt.Sprintf("work-info plugin request to %s (%s): %v", e.host, e.addr, errAddressRefused)
}

func (e *refusedError) Unwrap() error { return errAddressRefused }

// Direct requests ignore environment proxies and check resolved addresses at every dial, including
// redirects; configured proxy sessions validate destinations per request instead.
type netClient struct {
	client *http.Client
	rule   addressRule
	lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	// Default dialing checks the rule through net.Dialer.Control; tests may replace it.
	dial func(ctx context.Context, network, address string) (net.Conn, error)
}

// Nil Resolve/Connect use the system resolver and dialer.
func newNetClient(opts Options) *netClient {
	c := &netClient{
		rule: addressRule{local: interfaceAddrs},
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
	}
	if opts.Resolve != nil {
		c.lookup = opts.Resolve
	}
	dialer := &net.Dialer{Control: c.control}
	c.dial = dialer.DialContext
	if connect := opts.Connect; connect != nil {
		c.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			if err := c.control(network, address, nil); err != nil {
				return nil, err
			}
			return connect(ctx, network, address)
		}
	}
	c.client = newPluginHTTPClient(c.transport(nil, c.dialContext))
	return c
}

// Only request context sets the timeout; separate dial/TLS/header deadlines could misclassify
// timeout -4 as connection failure -3. Nil proxy also ignores environment proxies.
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

func newPluginHTTPClient(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			// via includes the initial request; stop before the sixth redirect and return that response.
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

// Proxy connections belong to one execution and close with its session; all execution requests use
// that proxy.
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
	// Distinguish refused CONNECT responses from other connection failures.
	transport.OnProxyConnectResponse = func(_ context.Context, _ *url.URL, _ *http.Request, resp *http.Response) error {
		if resp.StatusCode != http.StatusOK {
			return &proxyRefusedError{status: resp.Status}
		}
		return nil
	}
	return &httpSession{client: newPluginHTTPClient(transport), proxyHost: proxy.Hostname()}
}

// Validate every resolved destination address for each proxied request and redirect; do not
// validate the operator-selected proxy itself, which may be on the LAN.
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

// Reject empty resolver results.
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

type proxyRefusedError struct{ status string }

func (e *proxyRefusedError) Error() string { return "proxy refused CONNECT: " + e.status }

// Recognize proxyconnect and socks-prefixed net.OpError operations, along with refused CONNECT
// responses.
func isProxyFailure(err error) bool {
	var refused *proxyRefusedError
	if errors.As(err, &refused) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr) && (opErr.Op == "proxyconnect" || strings.HasPrefix(opErr.Op, "socks "))
}

// Check the resolved address immediately before connecting.
func (c *netClient) control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil || !c.rule.allows(ap.Addr()) {
		return errAddressRefused
	}
	return nil
}

// Split remaining time across addresses so a blackholed first address cannot consume the whole
// deadline; any refused address immediately fails the host.
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

// Match net.Dialer's minimum per-address share, capped by the actual remaining time.
const minDialShare = 2 * time.Second

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
