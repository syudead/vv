package wasmplugin

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// プロキシのテストは loopback に立てたプロキシを使う。プロキシは要求の接続先を記録し、どの
// 接続先への要求もテストのサイトに届ける。CI はインターネットに出ないので、接続先の名前は
// testNames で解決し、サーバー自身のアドレスは無いものとする。

// testProxy は loopback のプロキシである。answer が 0 でなければ、どの要求にもその状態で答える。
type testProxy struct {
	*httptest.Server
	answer int

	mu   sync.Mutex
	seen []string
}

func newTestProxy(t *testing.T, site *testSite, answer int) *testProxy {
	t.Helper()
	p := &testProxy{answer: answer}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.seen = append(p.seen, r.Host)
		p.mu.Unlock()
		switch {
		case p.answer != 0:
			w.WriteHeader(p.answer)
		case r.Method == http.MethodConnect:
			tunnel(w, site)
		default:
			forward(w, r, site)
		}
	}))
	t.Cleanup(p.Close)
	return p
}

// hosts はプロキシが要求を受けた接続先のホストを返す。
func (p *testProxy) hosts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.seen)
}

func (p *testProxy) network(t *testing.T) domain.WorkInfoRunNetwork {
	t.Helper()
	return domain.WorkInfoRunNetwork{Proxy: mustURL(t, p.URL)}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := domain.ParseWorkInfoPluginProxyURL(raw)
	if err != nil {
		t.Fatalf("ParseWorkInfoPluginProxyURL(%q): %v", raw, err)
	}
	return u
}

// forward は絶対 URI の要求を、Host をそのままにサイトへ送り、応答を返す。
func forward(w http.ResponseWriter, r *http.Request, site *testSite) {
	out, err := http.NewRequestWithContext(r.Context(), r.Method, site.URL+r.URL.RequestURI(), r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	out.Host = r.Host
	out.Header = r.Header.Clone()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	resp, err := transport.RoundTrip(out)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// tunnel は CONNECT の接続をサイトにつなぐ。
func tunnel(w http.ResponseWriter, site *testSite) {
	conn, _, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	upstream, err := net.Dial("tcp", site.Listener.Addr().String())
	if err != nil {
		return
	}
	defer func() { _ = upstream.Close() }()
	_, _ = conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	go func() { _, _ = io.Copy(upstream, conn) }()
	_, _ = io.Copy(conn, upstream)
}

// newProxyTestRuntime は、testNames で名前を解決し、サーバー自身のアドレスを持たない Runtime を返す。
// プロキシのない要求は実際の dialer を使うので、プロキシを通らない要求は届かない。
func newProxyTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	rt := newTestRuntime(t)
	nc := rt.run.net
	nc.rule.local = func() []netip.Addr { return nil }
	nc.lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		if a, ok := testNames[host]; ok {
			return []netip.Addr{netip.MustParseAddr(a)}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return rt
}

func TestProxyCarriesThePluginRequestToTheSite(t *testing.T) {
	site := newTestSite(t)
	proxy := newTestProxy(t, site, 0)
	rt := newProxyTestRuntime(t)

	info, err := runHTTPVia(t, rt, proxy.network(t), get("http://site.test/page"), 1, false)
	wantCodes(t, info, err, "0")
	if info.Title != "hello" {
		t.Fatalf("title = %q, want the page body", info.Title)
	}
	if got := proxy.hosts(); !slices.Equal(got, []string{"site.test"}) {
		t.Fatalf("proxy saw %q, want site.test", got)
	}
	site.mu.Lock()
	defer site.mu.Unlock()
	if !slices.Equal(site.hosts, []string{"site.test"}) {
		t.Fatalf("site saw %q, want site.test", site.hosts)
	}
}

func TestProxyRefusesADestinationThatResolvesToTheLANBeforeTheProxySeesIt(t *testing.T) {
	cases := []struct {
		name, url, host string
		proxySaw        []string
	}{
		{"name resolving to the LAN", "http://lan.test/page", "lan.test", nil},
		{"literal LAN address", "http://192.168.1.20/page", "192.168.1.20", nil},
		{"literal loopback address", "http://[::1]/page", "::1", nil},
		{"redirect to a name resolving to the LAN", "http://site.test/to-lan-name", "lan.test", []string{"site.test"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			site := newTestSite(t)
			proxy := newTestProxy(t, site, 0)
			rt := newProxyTestRuntime(t)

			_, err := runHTTPVia(t, rt, proxy.network(t), get(c.url), 1, false)
			wantFailure(t, err, domain.WorkInfoRunBlockedAddress)
			if host := runErrorOf(err).Host; host != c.host {
				t.Fatalf("host = %q, want %q", host, c.host)
			}
			if got := proxy.hosts(); !slices.Equal(got, c.proxySaw) {
				t.Fatalf("proxy saw %q, want %q", got, c.proxySaw)
			}
		})
	}
}

func TestProxyThatFailsReportsTheProxyHost(t *testing.T) {
	site := newTestSite(t)
	closed := newTestProxy(t, site, 0)
	closedURL := closed.URL
	closed.Close()

	cases := []struct {
		name, proxy, url string
	}{
		{"proxy not listening", closedURL, "http://site.test/page"},
		{"proxy answering 407", newTestProxy(t, site, http.StatusProxyAuthRequired).URL, "http://site.test/page"},
		{"proxy answering CONNECT with 407", newTestProxy(t, site, http.StatusProxyAuthRequired).URL, "https://site.test/page"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := newProxyTestRuntime(t)
			network := domain.WorkInfoRunNetwork{Proxy: mustURL(t, c.proxy)}

			info, err := runHTTPVia(t, rt, network, get(c.url), 1, false)
			wantCodes(t, info, err, "-3")

			_, err = runHTTPVia(t, rt, network, get(c.url), 1, true)
			wantFailure(t, err, domain.WorkInfoRunNetworkError)
			if host := runErrorOf(err).Host; host != "127.0.0.1" {
				t.Fatalf("host = %q, want the proxy's host 127.0.0.1", host)
			}
		})
	}
}

func TestRunWithoutAProxyStillChecksAtDialTime(t *testing.T) {
	site := newTestSite(t)
	proxy := newTestProxy(t, site, 0)
	rt := newNetTestRuntime(t, site)

	info, err := runHTTP(t, rt, get("http://site.test/page"), 1, false)
	wantCodes(t, info, err, "0")
	_, err = runHTTP(t, rt, get("http://lan.test/page"), 1, false)
	wantFailure(t, err, domain.WorkInfoRunBlockedAddress)
	if got := proxy.hosts(); len(got) != 0 {
		t.Fatalf("proxy saw %q, want no request without a proxy", got)
	}
}
