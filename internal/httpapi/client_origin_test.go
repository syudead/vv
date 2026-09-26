package httpapi

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
)

// 送信元と HTTPS の判定（plan.md Structural Decisions 7、#302）。

const (
	// proxyRemote は信頼するプロキシの接続元である。
	proxyRemote = "10.0.0.5:40000"
	// directRemote は信頼しない、直接つないだ接続元である。
	directRemote = "203.0.113.7:40000"
)

var testTrustedProxies = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/24"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("::1/128"),
}

func TestClientOrigin(t *testing.T) {
	srv := &server{trustedProxies: testTrustedProxies}
	for _, tc := range []struct {
		name       string
		remote     string
		tls        bool
		forwarded  []string
		proto      []string
		wantSource string
		wantHTTPS  bool
	}{
		{name: "転送ヘッダーなし", remote: directRemote, wantSource: "203.0.113.7"},
		{name: "TLS で直接受けた", remote: directRemote, tls: true, wantSource: "203.0.113.7", wantHTTPS: true},
		{
			name: "信頼しない接続元の転送ヘッダーは読まない", remote: directRemote,
			forwarded: []string{"198.51.100.1"}, proto: []string{"https"}, wantSource: "203.0.113.7",
		},
		{
			name: "信頼しない接続元が TLS で届き http を名乗っても HTTPS", remote: directRemote, tls: true,
			proto: []string{"http"}, wantSource: "203.0.113.7", wantHTTPS: true,
		},
		{
			name: "信頼するプロキシの転送元と https", remote: proxyRemote,
			forwarded: []string{"198.51.100.1"}, proto: []string{"https"}, wantSource: "198.51.100.1", wantHTTPS: true,
		},
		{
			name: "右から辿り、偽った左の値は使わない", remote: proxyRemote,
			forwarded: []string{"127.0.0.1, 198.51.100.1", "10.0.0.9"}, wantSource: "198.51.100.1",
		},
		{
			name: "どれも信頼するならいちばん左", remote: proxyRemote,
			forwarded: []string{"127.0.0.1, 10.0.0.9"}, wantSource: "127.0.0.1",
		},
		{
			name: "解釈できない値の手前で止める", remote: proxyRemote,
			forwarded: []string{"198.51.100.1, unknown, 10.0.0.9"}, wantSource: "10.0.0.9",
		},
		{
			name: "ポートと角括弧付きの値", remote: proxyRemote,
			forwarded: []string{"[2001:db8::1]:443"}, wantSource: "2001:db8::1",
		},
		{name: "転送元が無ければプロキシ", remote: proxyRemote, wantSource: "10.0.0.5"},
		{
			name: "X-Forwarded-Proto は最後の値", remote: proxyRemote,
			proto: []string{"https", "http"}, wantSource: "10.0.0.5",
		},
		{
			name: "X-Forwarded-Proto の最後の値が https", remote: proxyRemote,
			proto: []string{"http, HTTPS"}, wantSource: "10.0.0.5", wantHTTPS: true,
		},
		{
			name: "IPv4 射影の接続元も IPv4 として比べる", remote: "[::ffff:10.0.0.5]:40000",
			forwarded: []string{"198.51.100.1"}, wantSource: "198.51.100.1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			r.RemoteAddr = tc.remote
			if tc.tls {
				r.TLS = &tls.ConnectionState{}
			}
			for _, value := range tc.forwarded {
				r.Header.Add("X-Forwarded-For", value)
			}
			for _, value := range tc.proto {
				r.Header.Add("X-Forwarded-Proto", value)
			}
			// Forwarded（RFC 7239）は、どの場合も読まない。
			r.Header.Set("Forwarded", "for=192.0.2.60;proto=https")
			got := srv.clientOrigin(r)
			if got.source.String() != tc.wantSource || got.https != tc.wantHTTPS {
				t.Errorf("clientOrigin = (%s, %t), want (%s, %t)", got.source, got.https, tc.wantSource, tc.wantHTTPS)
			}
		})
	}
}

// 信頼するプロキシが無いとき（MDM_TRUSTED_PROXIES=none）は、ループバックの接続元でも
// 転送ヘッダーを読まない。
func TestClientOriginIgnoresHeadersWithoutTrustedProxies(t *testing.T) {
	srv := &server{}
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	r.RemoteAddr = "127.0.0.1:40000"
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := srv.clientOrigin(r); got.source.String() != "127.0.0.1" || got.https {
		t.Errorf("clientOrigin = (%s, %t), want (127.0.0.1, false)", got.source, got.https)
	}
}

// forgedHeaders は直接つないだ利用者が偽る転送ヘッダーである。
func forgedHeaders(forwardedFor string) map[string]string {
	return map[string]string{"X-Forwarded-For": forwardedFor, "X-Forwarded-Proto": "https"}
}

// 信頼しない接続元が付けた X-Forwarded-For と X-Forwarded-Proto: https は無視される。
// 試行制限は接続元のアドレスで掛かり、Cookie は vv_session で Secure が付かない。
func TestUntrustedForwardedHeadersAreIgnored(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{TrustedProxies: testTrustedProxies})
	env.setup()

	rec := env.serve(authRequest{
		method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(testUsername, testPassword),
		remote: directRemote, header: forgedHeaders("198.51.100.1"),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("ログイン: status = %d: %s", rec.Code, rec.Body)
	}
	setCookie := rec.Header().Get("Set-Cookie")
	if !strings.HasPrefix(setCookie, sessionCookieHTTP+"=") || strings.Contains(setCookie, "Secure") {
		t.Errorf("Set-Cookie = %q, want Secure の無い vv_session", setCookie)
	}

	// 転送元を毎回変えて偽っても、5回の失敗で接続元が制限される。
	for i := range 5 {
		rec := env.serve(authRequest{
			method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(testUsername, "wrong"),
			remote: directRemote, header: forgedHeaders("198.51.100." + strconv.Itoa(10+i)),
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("失敗 %d: status = %d: %s", i, rec.Code, rec.Body)
		}
	}
	rec = env.serve(authRequest{
		method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(testUsername, testPassword),
		remote: directRemote, header: forgedHeaders("198.51.100.99"),
	})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6回目: status = %d, want 429: %s", rec.Code, rec.Body)
	}
	if logs := env.logs.String(); !strings.Contains(logs, "source=203.0.113.7") || strings.Contains(logs, "198.51.100.") {
		t.Errorf("記録の送信元が接続元でない:\n%s", logs)
	}

	// Origin: https://<Host> は、偽った X-Forwarded-Proto では同一オリジンにならない。
	rec = env.serve(authRequest{
		method: http.MethodPost, target: "/api/auth/logout", remote: directRemote,
		header: map[string]string{"X-Forwarded-Proto": "https", "Origin": "https://example.com"},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("偽った https の Origin: status = %d, want 403: %s", rec.Code, rec.Body)
	}
}

// 信頼するプロキシからの X-Forwarded-Proto: https では __Host-vv_session に Secure が付き、
// Origin: https://<Host> の POST が通る。試行制限と記録は転送元のアドレスで分かれる。
func TestTrustedProxyForwardedHeadersAreUsed(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{TrustedProxies: testTrustedProxies, Videos: sampleLibrary()})
	env.setup()

	viaProxy := func(forwardedFor string, extra map[string]string) map[string]string {
		header := map[string]string{"X-Forwarded-For": forwardedFor, "X-Forwarded-Proto": "https"}
		for name, value := range extra {
			header[name] = value
		}
		return header
	}

	rec := env.serve(authRequest{
		method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(testUsername, testPassword),
		remote: proxyRemote, header: viaProxy("198.51.100.1", map[string]string{"Origin": "https://example.com"}),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("Origin: https://<Host> のログイン: status = %d: %s", rec.Code, rec.Body)
	}
	setCookie := rec.Header().Get("Set-Cookie")
	if !strings.HasPrefix(setCookie, sessionCookieHTTPS+"=") || !strings.Contains(setCookie, "Secure") {
		t.Errorf("Set-Cookie = %q, want Secure の __Host-vv_session", setCookie)
	}
	cookie := responseCookie(t, rec, sessionCookieHTTPS)

	if rec := env.serve(authRequest{
		method: http.MethodGet, target: "/api/videos", remote: proxyRemote,
		header: viaProxy("198.51.100.1", nil), cookies: []*http.Cookie{cookie},
	}); rec.Code != http.StatusOK {
		t.Errorf("プロキシ越しの HTTPS の Cookie: status = %d: %s", rec.Code, rec.Body)
	}

	// Origin が http:// なら、プロキシ越しの HTTPS とは別オリジンである。
	if rec := env.serve(authRequest{
		method: http.MethodPost, target: "/api/auth/logout", remote: proxyRemote,
		header: viaProxy("198.51.100.1", map[string]string{"Origin": "http://example.com"}),
	}); rec.Code != http.StatusForbidden {
		t.Errorf("Origin: http://<Host>: status = %d, want 403: %s", rec.Code, rec.Body)
	}

	// 別の転送元の失敗は、この転送元の制限を使い切らない。
	for i := range 5 {
		rec := env.serve(authRequest{
			method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(testUsername, "wrong"),
			remote: proxyRemote, header: viaProxy("198.51.100.2", nil),
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("失敗 %d: status = %d: %s", i, rec.Code, rec.Body)
		}
	}
	if rec := env.serve(authRequest{
		method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(testUsername, testPassword),
		remote: proxyRemote, header: viaProxy("198.51.100.2", nil),
	}); rec.Code != http.StatusTooManyRequests {
		t.Errorf("制限した転送元: status = %d, want 429", rec.Code)
	}
	if rec := env.serve(authRequest{
		method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(testUsername, testPassword),
		remote: proxyRemote, header: viaProxy("198.51.100.1", nil),
	}); rec.Code != http.StatusOK {
		t.Errorf("別の転送元: status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if logs := env.logs.String(); !strings.Contains(logs, "source=198.51.100.1") || strings.Contains(logs, "source=10.0.0.5") {
		t.Errorf("記録の送信元が転送元でない:\n%s", logs)
	}
}

// HTTP の要求は、認証に __Host-vv_session を読まない。偽った X-Forwarded-Proto でも同じである。
// 「ゲストも」の経路はゲストとして返るので、所有者だけの経路で確かめる。
func TestHTTPRequestDoesNotReadHostCookie(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{TrustedProxies: testTrustedProxies, Videos: sampleLibrary()})
	env.setup()
	cookie := env.login(true)

	assertUnauthenticated(t, "HTTP の要求の __Host-vv_session", env.get(ownerOnlyTarget, cookie))
	assertUnauthenticated(t, "信頼するプロキシの http の要求の __Host-vv_session", env.serve(authRequest{
		method: http.MethodGet, target: ownerOnlyTarget, remote: proxyRemote, cookies: []*http.Cookie{cookie},
		header: map[string]string{"X-Forwarded-For": "198.51.100.1", "X-Forwarded-Proto": "http"},
	}))
	if rec := env.serve(authRequest{
		method: http.MethodGet, target: ownerOnlyTarget, remote: directRemote, cookies: []*http.Cookie{cookie},
		header: forgedHeaders("198.51.100.1"),
	}); rec.Code != http.StatusUnauthorized {
		t.Errorf("偽った https の __Host-vv_session: status = %d, want 401", rec.Code)
	}
	if rec := env.serve(authRequest{method: http.MethodGet, target: "/api/videos", https: true, cookies: []*http.Cookie{cookie}}); rec.Code != http.StatusOK {
		t.Errorf("HTTPS の __Host-vv_session: status = %d, want 200", rec.Code)
	}
}

// 信頼するプロキシが同じ PC にあっても、転送元が外部なら既定アプリで開く操作は 403 になる。
func TestOpenVideoFileBehindLoopbackProxy(t *testing.T) {
	video, library := openFixture(t)
	opener := &fakeOpener{available: true}
	handler := newTestServer(t, Options{Videos: library, Opener: opener, TrustedProxies: testTrustedProxies})

	external := openRequest("/api/videos/1/open", "127.0.0.1:50000", "localhost:8080")
	external.Header.Set("X-Forwarded-For", "198.51.100.1")
	assertErrorCode(t, serve(handler, external), http.StatusForbidden, codeForbidden)

	// 外部の転送元がループバックを名乗っても、右端の信頼しない値が送信元になる。
	forged := openRequest("/api/videos/1/open", "127.0.0.1:50000", "localhost:8080")
	forged.Header.Set("X-Forwarded-For", "127.0.0.1, 198.51.100.1")
	assertErrorCode(t, serve(handler, forged), http.StatusForbidden, codeForbidden)
	if len(opener.opened) != 0 {
		t.Fatalf("opened = %q, want none", opener.opened)
	}

	// 同じ PC のブラウザがプロキシを通したときは開ける。
	local := openRequest("/api/videos/1/open", "127.0.0.1:50000", "localhost:8080")
	local.Header.Set("X-Forwarded-For", "::1")
	if rec := serve(handler, local); rec.Code != http.StatusNoContent {
		t.Fatalf("同じ PC の転送元: status = %d, want 204: %s", rec.Code, rec.Body)
	}
	if len(opener.opened) != 1 || opener.opened[0] != video.Path {
		t.Fatalf("opened = %q", opener.opened)
	}

	// LAN のプロキシを信頼していても、LAN の機器がループバックを名乗るだけでは開けない。
	lanHandler := newTestServer(t, Options{
		Videos: library, Opener: opener,
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16")},
	})
	lan := openRequest("/api/videos/1/open", "192.168.1.20:50000", "localhost:8080")
	lan.Header.Set("X-Forwarded-For", "127.0.0.1")
	assertErrorCode(t, serve(lanHandler, lan), http.StatusForbidden, codeForbidden)
	if len(opener.opened) != 1 {
		t.Fatalf("LAN の機器の偽装で開いた: opened = %q", opener.opened)
	}

	// 動画の応答の openable も同じ判定による。
	detail := httptest.NewRequest(http.MethodGet, "/api/videos/1", nil)
	detail.RemoteAddr = "127.0.0.1:50000"
	detail.Host = "localhost:8080"
	detail.Header.Set("X-Forwarded-For", "198.51.100.1")
	rec := serve(handler, detail)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"openable":false`) {
		t.Errorf("外部の転送元の openable: status = %d: %s", rec.Code, rec.Body)
	}
}
