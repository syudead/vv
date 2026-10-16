package wasmplugin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// HTTP のテストは probe の http の振る舞いを使う。probe は設定 request の要求を count 回送り、
// 誤りを無視して成功で終わる（fail を付けると誤りを報告して失敗する）。

// testNames はテストの resolver が返す名前とアドレスである。site.test は公開のアドレスに、
// lan.test は LAN のアドレスに解決される。
var testNames = map[string]string{
	"site.test":  "93.184.215.14",
	"other.test": "93.184.215.15",
	"lan.test":   "192.168.1.20",
}

// testSite はテストのサイトである。届いた要求のホストと User-Agent を記録する。
type testSite struct {
	*httptest.Server
	mu        sync.Mutex
	hosts     []string
	userAgent []string
}

func newTestSite(t *testing.T) *testSite {
	t.Helper()
	s := &testSite{}
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hosts = append(s.hosts, r.Host)
		s.userAgent = append(s.userAgent, r.UserAgent())
		s.mu.Unlock()
		w.Header().Set("X-Site", "yes")
		_, _ = w.Write([]byte("hello"))
	})
	mux.HandleFunc("/to-lan", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://192.168.1.20/page", http.StatusFound)
	})
	mux.HandleFunc("/to-lan-name", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://lan.test/page", http.StatusFound)
	})
	mux.HandleFunc("/to-broken", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://other.test/broken", http.StatusFound)
	})
	mux.HandleFunc("/broken", func(w http.ResponseWriter, _ *http.Request) {
		// 本文の途中で接続を切る。
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
		if conn, _, err := http.NewResponseController(w).Hijack(); err == nil {
			_ = conn.Close()
		}
	})
	mux.HandleFunc("/hop/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.PathValue("n"))
		http.Redirect(w, r, fmt.Sprintf("/hop/%d", n+1), http.StatusFound)
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 5<<20))
	})
	mux.HandleFunc("/big-headers", func(w http.ResponseWriter, _ *http.Request) {
		value := strings.Repeat("x", 64<<10)
		for i := range domain.WorkInfoPluginHTTPMaxHeaderBytes/len(value) + 1 {
			w.Header().Set("X-Big-"+strconv.Itoa(i), value)
		}
		_, _ = w.Write([]byte("hello"))
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(11 * time.Second):
		case <-r.Context().Done():
		}
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// newNetTestRuntime は、testNames で名前を解決し、規則に許されたアドレスへの接続を site に
// つなぐ Runtime を返す。許されるかは本物の dialer と同じ control で確かめる。
func newNetTestRuntime(t *testing.T, site *testSite) *Runtime {
	t.Helper()
	rt := newTestRuntime(t)
	nc := rt.run.net
	nc.lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		if a, ok := testNames[host]; ok {
			return []netip.Addr{netip.MustParseAddr(a)}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	nc.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if err := nc.control(network, address, nil); err != nil {
			return nil, err
		}
		var d net.Dialer
		return d.DialContext(ctx, network, site.Listener.Addr().String())
	}
	return rt
}

// runHTTP は probe に request を count 回、プロキシなしで送らせる。
func runHTTP(t *testing.T, rt *Runtime, request string, count int, fail bool) (domain.FetchedWorkInfo, error) {
	t.Helper()
	return runHTTPVia(t, rt, domain.WorkInfoRunNetwork{}, request, count, fail)
}

// runHTTPVia は probe に request を count 回、network のとおりに送らせる。
func runHTTPVia(
	t *testing.T, rt *Runtime, network domain.WorkInfoRunNetwork, request string, count int, fail bool,
) (domain.FetchedWorkInfo, error) {
	t.Helper()
	req := fetchMode("http")
	req.Settings = map[string]string{"request": request, "count": strconv.Itoa(count)}
	if fail {
		req.Settings["fail"] = "1"
	}
	got, _, err := rt.Run(context.Background(), probeDigest, probeModule(t), req, network, nil)
	return got.WorkInfo, err
}

func get(url string) string {
	return `{"method":"GET","url":"` + url + `"}`
}

// runErrorOf は wantFailure で確かめた err の *domain.WorkInfoRunError を返す。
func runErrorOf(err error) *domain.WorkInfoRunError {
	var runErr *domain.WorkInfoRunError
	errors.As(err, &runErr)
	return runErr
}

func wantCodes(t *testing.T, info domain.FetchedWorkInfo, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("Run(http): %v", err)
	}
	if len(info.Tags) == 0 || info.Tags[0] != "codes="+want {
		t.Fatalf("tags = %q, want codes=%s first", info.Tags, want)
	}
}

func TestHTTPPluginFetchesAPageThroughVV(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)

	request := `{"method":"GET","url":"http://site.test/page","headers":{"Host":"lan.test","Connection":"close","X-Plugin":"1"}}`
	info, err := runHTTP(t, rt, request, 1, false)
	wantCodes(t, info, err, "0")
	if info.Title != "hello" {
		t.Fatalf("title = %q, want the page body", info.Title)
	}
	meta := info.Tags[1]
	for _, want := range []string{`"status":200`, `"url":"http://site.test/page"`, `"X-Site":["yes"]`} {
		if !strings.Contains(meta, want) {
			t.Fatalf("meta = %s, want it to contain %s", meta, want)
		}
	}
	site.mu.Lock()
	defer site.mu.Unlock()
	if len(site.hosts) != 1 || site.hosts[0] != "site.test" || site.userAgent[0] != defaultUserAgent {
		t.Fatalf("site saw hosts %q with User-Agent %q, want site.test with the default User-Agent", site.hosts, site.userAgent)
	}
}

func TestHTTPRefusedAddressFailsTheRunWhateverThePluginReturns(t *testing.T) {
	site := newTestSite(t)
	cases := []struct {
		name, url, host string
		realDialer      bool
	}{
		{"loopback through the real dialer", site.URL + "/page", "127.0.0.1", true},
		{"LAN address", "http://192.168.1.20/page", "192.168.1.20", false},
		{"public name redirecting to the LAN", "http://site.test/to-lan", "192.168.1.20", false},
		{"public name resolving to the LAN", "http://lan.test/page", "lan.test", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := newTestRuntime(t)
			if !c.realDialer {
				rt = newNetTestRuntime(t, site)
			}
			_, err := runHTTP(t, rt, get(c.url), 1, false)
			wantFailure(t, err, domain.WorkInfoRunBlockedAddress)
			if host := runErrorOf(err).Host; host != c.host {
				t.Fatalf("host = %q, want %q", host, c.host)
			}
		})
	}
}

func TestHTTPAnswersEachLimitWithItsErrorCode(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)

	info, err := runHTTP(t, rt, get("http://site.test/page"), 17, false)
	wantCodes(t, info, err, "0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,-5")

	info, err = runHTTP(t, rt, get("http://site.test/big"), 1, false)
	wantCodes(t, info, err, "-6")

	info, err = runHTTP(t, rt, get("http://site.test/big-headers"), 1, false)
	wantCodes(t, info, err, "-3")

	for _, bad := range []string{
		`{"method":"PUT","url":"http://site.test/page"}`,
		get("ftp://site.test/page"),
		`{"method":"POST","url":"http://site.test/page","body":"not base64!"}`,
		`not json`,
	} {
		info, err = runHTTP(t, rt, bad, 1, false)
		wantCodes(t, info, err, "-1")
	}

	// 不正なヘッダーは要求を壊さずに落とす。
	info, err = runHTTP(t, rt, `{"method":"GET","url":"http://site.test/page","headers":{"X-\u00dc":"1","X-Ctl":"a\u0001b"}}`, 1, false)
	wantCodes(t, info, err, "0")

	info, err = runHTTP(t, rt, get("http://site.test/hop/0"), 1, false)
	wantCodes(t, info, err, "0")
	if !strings.Contains(info.Tags[1], `"status":302`) || !strings.Contains(info.Tags[1], `"url":"http://site.test/hop/5"`) {
		t.Fatalf("meta = %s, want the sixth redirect handed to the plugin", info.Tags[1])
	}
}

func TestHTTPAnswersASlowResponseWithTimedOut(t *testing.T) {
	t.Parallel()
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)

	start := time.Now()
	info, err := runHTTP(t, rt, get("http://site.test/slow"), 1, false)
	wantCodes(t, info, err, "-4")
	// 上は実行の時間の上限で抑える。実行は並べて動くテストのビルドとコンパイルの時間も含む。
	if d := time.Since(start); d < domain.WorkInfoPluginHTTPTimeout || d >= domain.WorkInfoPluginRunTimeout {
		t.Fatalf("slow request answered after %v, want after %v and before the run's limit", d, domain.WorkInfoPluginHTTPTimeout)
	}
}

func TestHTTPUnreachableHostIsReportedWhenThePluginFails(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)

	info, err := runHTTP(t, rt, get("http://missing.test/page"), 1, false)
	wantCodes(t, info, err, "-3")

	_, err = runHTTP(t, rt, get("http://missing.test/page"), 1, true)
	wantFailure(t, err, domain.WorkInfoRunNetworkError)
	if runErr := runErrorOf(err); runErr.Host != "missing.test" || runErr.PluginMessage != "gave up" {
		t.Fatalf("network error = %+v, want host missing.test and the plugin's message", runErr)
	}
}

func TestHTTPBodyFailureAfterARedirectReportsTheRedirectHost(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)

	_, err := runHTTP(t, rt, get("http://site.test/to-broken"), 1, true)
	wantFailure(t, err, domain.WorkInfoRunNetworkError)
	if host := runErrorOf(err).Host; host != "other.test" {
		t.Fatalf("host = %q, want other.test, the host that sent the broken body", host)
	}
}

func TestHTTPDialLeavesTimeForTheNextAddress(t *testing.T) {
	site := newTestSite(t)
	nc := newNetClient(Options{})
	nc.lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("2606:4700::1"), netip.MustParseAddr("93.184.215.14")}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), domain.WorkInfoPluginHTTPTimeout)
	defer cancel()
	overall, _ := ctx.Deadline()
	var firstDeadline time.Time
	nc.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if strings.HasPrefix(address, "[2606:4700::1]") {
			// パケットを捨てるアドレスとして、与えられた期限を記録して時間切れにする。
			firstDeadline, _ = ctx.Deadline()
			return nil, context.DeadlineExceeded
		}
		var d net.Dialer
		return d.DialContext(ctx, network, site.Listener.Addr().String())
	}
	conn, err := nc.dialContext(ctx, "tcp", "site.test:80")
	if err != nil {
		t.Fatalf("dialContext: %v, want the second address", err)
	}
	_ = conn.Close()
	if left := overall.Sub(firstDeadline); left < domain.WorkInfoPluginHTTPTimeout/3 {
		t.Fatalf("first address got until %v before the request's deadline, want about half of the request's time left", left)
	}
}
