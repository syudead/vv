package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClassifyRequestFailsClosedOnEscapedSeparators は、エスケープ済みの経路の段が
// 復号済みの経路の段と食い違う要求を「所有者だけ」に倒すことを確かめる。ServeMux は
// エスケープ済みの経路を段に分けてから復号するので %2F を区切りとして扱わず、段の中の
// .. も畳まない。復号した経路だけで分類すると、GET /api/videos/{id} に届く要求が
// /api の外の「誰でも」に分類されるなど、分類と実際の処理がずれる。
func TestClassifyRequestFailsClosedOnEscapedSeparators(t *testing.T) {
	cases := []struct {
		method, target string
		want           access
		api            bool
	}{
		// 復号すると /index.html だが、ServeMux は GET /api/videos/{id} に振り分ける。
		{http.MethodGet, "/api/videos/x%2F..%2F..%2F..%2Findex.html", accessOwner, true},
		// 復号すると /a/stream だが、ServeMux は GET /api/videos/{id}/stream に振り分ける。
		{http.MethodGet, "/api/videos/1%2F..%2F..%2F..%2Fa/stream", accessOwner, true},
		// ServeMux は GET /api/videos/{id} に振り分ける。復号した経路の /api/videos/ids ではない。
		{http.MethodGet, "/api/videos%2Fids", accessOwner, true},
		// 所有者だけの経路に届く要求は、復号すると誰でもの経路になっても所有者だけに倒れる。
		{http.MethodPost, "/api/tags/1%2F..%2F..%2Fauth%2Flogin/merge", accessOwner, true},
		{http.MethodDelete, "/api/tags/1%2F..%2F..%2F..%2Fx", accessOwner, true},
		// 段が食い違わない書き方は、これまでどおり復号済みの経路で分類する。
		{http.MethodGet, "/%61pi/videos", accessGuest, true},
		{http.MethodGet, "/api/../api/health", accessPublic, true},
		// 段が食い違う /api 以下の経路は、ServeMux が SPA に振り分けても狭い側に倒す。
		{http.MethodGet, "/api%2Fvideos", accessOwner, true},
		{http.MethodGet, "/%2E%2E/api/videos", accessOwner, true},
		// /api の外の SPA の経路は GET・HEAD だけ誰にでも配る。ServeMux が SPA に
		// 振り分ける経路は、段が食い違っても SPA のままである。
		{http.MethodGet, "/videos/1", accessPublic, false},
		{http.MethodGet, "/videos/a%2Fb", accessPublic, false},
		{http.MethodGet, "/api%2F..%2Findex.html", accessPublic, false},
	}
	for _, c := range cases {
		got, api := classifyRequest(httptest.NewRequest(c.method, c.target, nil))
		if got != c.want || api != c.api {
			t.Errorf("%s %s: 扱い = %s, api = %t; want %s, %t", c.method, c.target, got, api, c.want, c.api)
		}
	}
}

// TestAuthEscapedSeparatorsDoNotBypassBoundary は、%2F を含む経路が ServeMux で API の
// 操作に届くとき、未ログインの要求が境界を素通りしないことを確かめる。
func TestAuthEscapedSeparatorsDoNotBypassBoundary(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{Videos: sampleLibrary()})
	env.setup()

	for _, target := range []string{
		"/api/videos/x%2F..%2F..%2F..%2Findex.html",
		"/api/videos/1%2F..%2F..%2F..%2Fa/stream",
		"/api/folders/1%2F..%2F..%2F..%2Fa/videos",
		"/api/videos%2Fids",
	} {
		rec := env.get(target)
		assertUnauthenticated(t, target, rec)
		assertAudience(t, target, rec, "guest")
	}
}
