package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/password"
	"github.com/syudead/vv/internal/store"
)

// 認証の境界は、本物の Auth（internal/app）・保存先（internal/store）・Argon2id
// （internal/password）で確かめる（specs/016-single-account-auth の #299）。

const (
	testUsername = "owner-name"
	testPassword = "correct horse battery staple"
)

// appAuthenticator は *app.Auth を Authenticator にする。cmd/mdm の組み立てと同じ写しである。
type appAuthenticator struct{ auth *app.Auth }

func (a appAuthenticator) Setup(ctx context.Context, username, pass string) (IssuedSession, error) {
	session, err := a.auth.Setup(ctx, username, pass)
	return IssuedSession(session), err
}

func (a appAuthenticator) Login(ctx context.Context, attempt LoginAttempt) (IssuedSession, error) {
	session, err := a.auth.Login(ctx, app.LoginRequest(attempt))
	return IssuedSession(session), err
}

func (a appAuthenticator) CheckSession(ctx context.Context, token string) (time.Time, bool, error) {
	return a.auth.CheckSession(ctx, token)
}

func (a appAuthenticator) Logout(ctx context.Context, token string) error {
	return a.auth.Logout(ctx, token)
}

func (a appAuthenticator) State(ctx context.Context, token string) (AuthState, error) {
	state, err := a.auth.State(ctx, token)
	return AuthState(state), err
}

type passwordHasher struct{}

func (passwordHasher) Hash(p string) (string, error)          { return password.Hash(p) }
func (passwordHasher) Verify(p, encoded string) (bool, error) { return password.Verify(p, encoded) }

// syncBuffer は並行に書かれる記録を集める。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// authEnv は本物の認証でつないだ経路である。
type authEnv struct {
	t       *testing.T
	db      *store.DB
	handler http.Handler
	logs    *syncBuffer

	mu  sync.Mutex
	now time.Time
}

func (e *authEnv) clock() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.now
}

func (e *authEnv) advance(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.now = e.now.Add(d)
}

// newAuthEnv は dataDir のデータベースで経路を組み立てる。opts の Auth・Now・Logger は
// ここで決める。
func newAuthEnv(t *testing.T, dataDir string, opts Options) *authEnv {
	t.Helper()
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	env := &authEnv{t: t, db: db, logs: &syncBuffer{}, now: time.Now()}
	auth := app.NewAuth(app.AuthOptions{Store: db.Auth(), Hasher: passwordHasher{}, Now: env.clock})
	opts.Auth = appAuthenticator{auth: auth}
	opts.Now = env.clock
	opts.Logger = slog.New(slog.NewTextHandler(env.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	if opts.Pinger == nil {
		opts.Pinger = db
	}
	env.handler = newTestServer(t, opts)
	return env
}

type authRequest struct {
	method  string
	target  string
	body    string
	https   bool
	cookies []*http.Cookie
	header  map[string]string
}

func (e *authEnv) serve(req authRequest) *httptest.ResponseRecorder {
	e.t.Helper()
	var body io.Reader
	if req.body != "" {
		body = strings.NewReader(req.body)
	}
	r := httptest.NewRequest(req.method, req.target, body)
	if req.body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if req.https {
		r.TLS = &tls.ConnectionState{}
	}
	for _, cookie := range req.cookies {
		r.AddCookie(cookie)
	}
	for name, value := range req.header {
		r.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, r)
	return rec
}

func (e *authEnv) get(target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.serve(authRequest{method: http.MethodGet, target: target, cookies: cookies})
}

func credentialsBody(username, pass string) string {
	body, _ := json.Marshal(map[string]string{"username": username, "password": pass})
	return string(body)
}

// setup は初回設定を行い、返った Cookie を返す。
func (e *authEnv) setup() *http.Cookie {
	e.t.Helper()
	rec := e.serve(authRequest{method: http.MethodPost, target: "/api/auth/setup", body: credentialsBody(testUsername, testPassword)})
	if rec.Code != http.StatusOK {
		e.t.Fatalf("初回設定: status = %d: %s", rec.Code, rec.Body)
	}
	return responseCookie(e.t, rec, sessionCookieHTTP)
}

// login はログインし、返った Cookie を返す。
func (e *authEnv) login(https bool, cookies ...*http.Cookie) *http.Cookie {
	e.t.Helper()
	rec := e.serve(authRequest{
		method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(testUsername, testPassword),
		https: https, cookies: cookies,
	})
	if rec.Code != http.StatusOK {
		e.t.Fatalf("ログイン: status = %d: %s", rec.Code, rec.Body)
	}
	name := sessionCookieHTTP
	if https {
		name = sessionCookieHTTPS
	}
	return responseCookie(e.t, rec, name)
}

func responseCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return &http.Cookie{Name: cookie.Name, Value: cookie.Value}
		}
	}
	t.Fatalf("応答に Cookie %s が無い: %v", name, rec.Header().Values("Set-Cookie"))
	return nil
}

// assertUnauthenticated は未認証の応答（contracts/auth-api.md §5）であることを確かめる。
func assertUnauthenticated(t *testing.T, label string, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("%s: status = %d, want 401: %s", label, rec.Code, rec.Body)
		return
	}
	var body struct{ Code, Message string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != "unauthenticated" || body.Message != "ログインが必要です" {
		t.Errorf("%s: 本文 = %s", label, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != contentTypeJSON {
		t.Errorf("%s: Content-Type = %q", label, got)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("%s: Cache-Control = %q", label, got)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Errorf("%s: WWW-Authenticate = %q, want なし", label, got)
	}
}

func assertAudience(t *testing.T, label string, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if got := rec.Header().Get(audienceHeader); got != want {
		t.Errorf("%s: %s = %q, want %q", label, audienceHeader, got, want)
	}
}

func authState(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("状態の確認: status = %d: %s", rec.Code, rec.Body)
	}
	var body struct{ State string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.State
}

// protectedOperations は openapi.yaml の「誰でも」以外の操作を、具体的な要求にして返す。
func protectedOperations(t *testing.T) []operation {
	t.Helper()
	var ops []operation
	for op, class := range openAPISecurity(t) {
		if class != accessPublic {
			ops = append(ops, op)
		}
	}
	if len(ops) == 0 {
		t.Fatal("保護対象の操作を読み取れていない")
	}
	return ops
}

func operationTarget(op operation) string {
	return operationRequest(op).URL.Path
}

func operationBody(op operation) string {
	switch op.method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return "{}"
	}
	return ""
}

func sampleLibrary() *fakeLibrary {
	video := sampleVideo(1, "海辺の散歩")
	return &fakeLibrary{
		videos: map[int64]domain.Video{1: video},
		page:   domain.VideoPage{Items: []domain.Video{video}, Total: 1},
	}
}

func TestAuthUnconfiguredRequiresSetup(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{Videos: sampleLibrary()})

	if got := authState(t, env.get("/api/auth/session")); got != "setupRequired" {
		t.Fatalf("state = %q, want setupRequired", got)
	}
	for _, op := range protectedOperations(t) {
		rec := env.serve(authRequest{method: op.method, target: operationTarget(op), body: operationBody(op)})
		assertUnauthenticated(t, "未設定の "+op.method+" "+op.path, rec)
	}

	setupRec := env.serve(authRequest{method: http.MethodPost, target: "/api/auth/setup", body: credentialsBody(testUsername, testPassword)})
	if setupRec.Code != http.StatusOK {
		t.Fatalf("初回設定: status = %d: %s", setupRec.Code, setupRec.Body)
	}
	if got := strings.TrimSpace(setupRec.Body.String()); got != `{"redirectTo":"/"}` {
		t.Errorf("初回設定の本文 = %s", got)
	}
	cookie := responseCookie(t, setupRec, sessionCookieHTTP)

	rec := env.get("/api/videos", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("初回設定の Cookie で一覧: status = %d: %s", rec.Code, rec.Body)
	}
	assertAudience(t, "初回設定の Cookie", rec, "owner")
	if got := authState(t, env.get("/api/auth/session", cookie)); got != "owner" {
		t.Errorf("初回設定の Cookie の state = %q, want owner", got)
	}

	again := env.serve(authRequest{method: http.MethodPost, target: "/api/auth/setup", body: credentialsBody("other", "another password")})
	if again.Code != http.StatusConflict || !strings.Contains(again.Body.String(), `"account_already_configured"`) {
		t.Fatalf("2回目の初回設定: status = %d: %s", again.Code, again.Body)
	}
	if again.Header().Get("Set-Cookie") != "" {
		t.Error("2回目の初回設定が Cookie を返した")
	}
	if got := authState(t, env.get("/api/auth/session")); got != "guest" {
		t.Errorf("設定後の Cookie なしの state = %q, want guest", got)
	}
}

func TestAuthSetupRejectsInvalidValues(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{})
	for _, body := range []string{credentialsBody(" padded", testPassword), credentialsBody(testUsername, "")} {
		rec := env.serve(authRequest{method: http.MethodPost, target: "/api/auth/setup", body: body})
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_request"`) {
			t.Errorf("%s: status = %d: %s", body, rec.Code, rec.Body)
		}
	}
	if got := authState(t, env.get("/api/auth/session")); got != "setupRequired" {
		t.Errorf("規則を外れた初回設定の後の state = %q, want setupRequired", got)
	}
}

func TestAuthWithoutCookieIsUnauthenticated(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{Videos: sampleLibrary()})
	env.setup()

	for _, op := range protectedOperations(t) {
		rec := env.serve(authRequest{method: op.method, target: operationTarget(op), body: operationBody(op)})
		assertUnauthenticated(t, op.method+" "+op.path, rec)
		assertAudience(t, op.method+" "+op.path, rec, "guest")
	}
	for _, target := range []string{"//api/videos", "/./api/scans", "/%61pi/scans", "/api/../api/scans", "/api/x", "/api/", "/api"} {
		rec := env.get(target)
		assertUnauthenticated(t, target, rec)
		assertAudience(t, target, rec, "guest")
	}
	// 不正な Cookie・該当の無い Cookie も同じ応答になる。
	for _, cookie := range []*http.Cookie{
		{Name: sessionCookieHTTP, Value: "not-a-token"},
		{Name: sessionCookieHTTP, Value: strings.Repeat("A", 43)},
	} {
		rec := env.get("/api/videos", cookie)
		assertUnauthenticated(t, "Cookie "+cookie.Value, rec)
	}
	// HTTP の要求は __Host- の Cookie を読まない。
	valid := env.login(true)
	rec := env.get("/api/videos", &http.Cookie{Name: sessionCookieHTTPS, Value: valid.Value})
	assertUnauthenticated(t, "HTTP の要求に __Host-vv_session", rec)

	// SPA のビルド成果物は誰にでも配り、X-VV-Audience は付けない。
	spa := env.get("/videos/1")
	if spa.Code == http.StatusUnauthorized || spa.Header().Get(audienceHeader) != "" {
		t.Errorf("SPA の経路: status = %d, %s = %q", spa.Code, audienceHeader, spa.Header().Get(audienceHeader))
	}
	// /api/ で始まらない GET・HEAD 以外は「所有者だけ」に倒れる。
	assertUnauthenticated(t, "POST /videos/1", env.serve(authRequest{method: http.MethodPost, target: "/videos/1", body: "{}"}))
}

func TestAuthOwnerCookieReachesEveryOperation(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{Videos: sampleLibrary()})
	cookie := env.setup()

	for _, op := range protectedOperations(t) {
		rec := env.serve(authRequest{method: op.method, target: operationTarget(op), body: operationBody(op), cookies: []*http.Cookie{cookie}})
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s: 所有者の Cookie で 401: %s", op.method, op.path, rec.Body)
		}
		assertAudience(t, op.method+" "+op.path, rec, "owner")
	}
	rec := env.get("/api/videos", cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "海辺の散歩") {
		t.Errorf("一覧: status = %d: %s", rec.Code, rec.Body)
	}
	// 誰でもの要求にも、処理した見る人を付ける。
	assertAudience(t, "稼働確認", env.get("/api/health", cookie), "owner")
	assertAudience(t, "稼働確認（Cookie なし）", env.get("/api/health"), "guest")
}

func TestAuthSessionReturnsRedirectOnlyForOwner(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{})
	cookie := env.setup()

	sessionBody := func(rec *httptest.ResponseRecorder) map[string]string {
		t.Helper()
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	body := sessionBody(env.get("/api/auth/session?next=%2Fvideos%2F12%3Ft%3D30", cookie))
	if len(body) != 2 || body["state"] != "owner" || body["redirectTo"] != "/videos/12?t=30" {
		t.Errorf("所有者: %v", body)
	}
	body = sessionBody(env.get("/api/auth/session?next=%2F%2Fevil.example", cookie))
	if len(body) != 2 || body["state"] != "owner" || body["redirectTo"] != "/" {
		t.Errorf("安全でない next: %v", body)
	}
	rec := env.get("/api/auth/session?next=%2Fvideos%2F12")
	if body := sessionBody(rec); len(body) != 1 || body["state"] != "guest" {
		t.Errorf("ゲスト: %v", body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
}

func TestAuthLoginSetsCookieAndRedirect(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{Videos: sampleLibrary()})
	env.setup()

	body, _ := json.Marshal(map[string]string{"username": testUsername, "password": testPassword, "next": "/videos/12?t=30"})
	rec := env.serve(authRequest{method: http.MethodPost, target: "/api/auth/login", body: string(body)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"redirectTo":"/videos/12?t=30"}` {
		t.Errorf("本文 = %s", got)
	}
	setCookie := rec.Header().Get("Set-Cookie")
	for _, want := range []string{"vv_session=", "HttpOnly", "SameSite=Strict", "Path=/", "Max-Age="} {
		if !strings.Contains(setCookie, want) {
			t.Errorf("Set-Cookie = %q に %q が無い", setCookie, want)
		}
	}
	// Max-Age は期限（90 日後）までの秒である。
	if cookies := rec.Result().Cookies(); len(cookies) != 1 ||
		cookies[0].MaxAge > int(domain.SessionLifetime/time.Second) ||
		cookies[0].MaxAge < int(domain.SessionLifetime/time.Second)-5 {
		t.Errorf("Max-Age が期限までの秒でない: %q", setCookie)
	}
	if strings.Contains(setCookie, "Secure") || strings.HasPrefix(setCookie, "__Host-") {
		t.Errorf("HTTP の Set-Cookie = %q", setCookie)
	}

	body, _ = json.Marshal(map[string]string{"username": testUsername, "password": testPassword, "next": "//evil.example"})
	rec = env.serve(authRequest{method: http.MethodPost, target: "/api/auth/login", body: string(body), https: true})
	if got := strings.TrimSpace(rec.Body.String()); got != `{"redirectTo":"/"}` {
		t.Errorf("安全でない next の本文 = %s", got)
	}
	setCookie = rec.Header().Get("Set-Cookie")
	for _, want := range []string{"__Host-vv_session=", "HttpOnly", "Secure", "SameSite=Strict", "Path=/", "Max-Age="} {
		if !strings.Contains(setCookie, want) {
			t.Errorf("HTTPS の Set-Cookie = %q に %q が無い", setCookie, want)
		}
	}
}

func TestAuthLoginFailuresAreIndistinguishableAndThrottled(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{})
	env.setup()

	type result struct {
		status int
		body   string
		header http.Header
	}
	attempt := func(username, pass string) result {
		rec := env.serve(authRequest{method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(username, pass)})
		header := rec.Header().Clone()
		return result{status: rec.Code, body: rec.Body.String(), header: header}
	}
	failures := []result{
		attempt("someone-else", testPassword),
		attempt(testUsername, "wrong password"),
		attempt("someone-else", "wrong password"),
	}
	for i, got := range failures {
		if got.status != http.StatusUnauthorized || !strings.Contains(got.body, `"invalid_credentials"`) {
			t.Fatalf("失敗 %d: status = %d: %s", i, got.status, got.body)
		}
		if got.body != failures[0].body || !equalHeaders(got.header, failures[0].header) {
			t.Errorf("失敗 %d が他と違う: %s %v / %s %v", i, got.body, got.header, failures[0].body, failures[0].header)
		}
	}
	attempt(strings.ToUpper(testUsername), testPassword)
	attempt("", "")

	throttled := env.serve(authRequest{method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(testUsername, testPassword)})
	if throttled.Code != http.StatusTooManyRequests || !strings.Contains(throttled.Body.String(), `"login_throttled"`) {
		t.Fatalf("6回目: status = %d: %s", throttled.Code, throttled.Body)
	}
	retryAfter, err := strconv.Atoi(throttled.Header().Get("Retry-After"))
	if err != nil || retryAfter < 1 {
		t.Errorf("Retry-After = %q", throttled.Header().Get("Retry-After"))
	}
	if throttled.Header().Get("Set-Cookie") != "" {
		t.Error("制限した応答が Cookie を返した")
	}
}

func equalHeaders(a, b http.Header) bool {
	if len(a) != len(b) {
		return false
	}
	for name, values := range a {
		if strings.Join(values, "\n") != strings.Join(b.Values(name), "\n") {
			return false
		}
	}
	return true
}

func TestAuthRequestsKeepMutationBoundary(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{})
	for _, target := range []string{"/api/auth/setup", "/api/auth/login"} {
		rec := env.serve(authRequest{method: http.MethodPost, target: target, body: "username=a", header: map[string]string{"Content-Type": "text/plain"}})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s を JSON でない本文で: status = %d: %s", target, rec.Code, rec.Body)
		}
		big := `{"username":"a","password":"` + strings.Repeat("x", maxAuthBodyBytes) + `"}`
		rec = env.serve(authRequest{method: http.MethodPost, target: target, body: big})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s を 8 KiB を超える本文で: status = %d: %s", target, rec.Code, rec.Body)
		}
	}
	for _, target := range []string{"/api/auth/setup", "/api/auth/login", "/api/auth/logout"} {
		rec := env.serve(authRequest{
			method: http.MethodPost, target: target, body: credentialsBody(testUsername, testPassword),
			header: map[string]string{"Origin": "http://evil.example"},
		})
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s を別オリジンから: status = %d: %s", target, rec.Code, rec.Body)
		}
	}
	if got := authState(t, env.get("/api/auth/session")); got != "setupRequired" {
		t.Errorf("拒んだ初回設定の後の state = %q", got)
	}
}

func TestAuthHealthDoesNotRevealSetupState(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{Build: domain.BuildInfo{Version: "test", Commit: "abc"}})
	before := env.get("/api/health")
	env.setup()
	after := env.get("/api/health")
	if before.Code != http.StatusOK || after.Code != http.StatusOK {
		t.Fatalf("status = %d / %d", before.Code, after.Code)
	}
	if before.Body.String() != after.Body.String() {
		t.Errorf("未設定と設定済みで本文が違う: %s / %s", before.Body, after.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(after.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for key := range body {
		switch key {
		case "status", "version", "commit", "builtAt":
		default:
			t.Errorf("稼働確認に %q が載った", key)
		}
	}
}

func TestAuthLogoutEndsBothSessionsOverHTTPS(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{Videos: sampleLibrary()})
	httpCookie := env.setup()
	httpsCookie := env.login(true)

	if rec := env.serve(authRequest{method: http.MethodGet, target: "/api/videos", https: true, cookies: []*http.Cookie{httpsCookie}}); rec.Code != http.StatusOK {
		t.Fatalf("HTTPS の Cookie: status = %d", rec.Code)
	}
	if rec := env.get("/api/videos", httpCookie); rec.Code != http.StatusOK {
		t.Fatalf("HTTP の Cookie: status = %d", rec.Code)
	}

	rec := env.serve(authRequest{method: http.MethodPost, target: "/api/auth/logout", https: true, cookies: []*http.Cookie{httpsCookie, httpCookie}})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("ログアウト: status = %d: %s", rec.Code, rec.Body)
	}
	cleared := map[string]bool{}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.MaxAge < 0 {
			cleared[cookie.Name] = true
		}
	}
	if !cleared[sessionCookieHTTPS] || !cleared[sessionCookieHTTP] {
		t.Errorf("両方の Cookie を消していない: %v", rec.Header().Values("Set-Cookie"))
	}
	for _, header := range rec.Header().Values("Set-Cookie") {
		if !strings.Contains(header, "Max-Age=0") {
			t.Errorf("Set-Cookie = %q に Max-Age=0 が無い", header)
		}
	}

	assertUnauthenticated(t, "ログアウト後の HTTP の Cookie", env.get("/api/videos", httpCookie))
	assertUnauthenticated(t, "ログアウト後の HTTPS の Cookie",
		env.serve(authRequest{method: http.MethodGet, target: "/api/videos", https: true, cookies: []*http.Cookie{httpsCookie}}))
}

func TestAuthSessionSurvivesRebuildAndExpires(t *testing.T) {
	dataDir := t.TempDir()
	first := newAuthEnv(t, dataDir, Options{Videos: sampleLibrary()})
	cookie := first.setup()
	if err := first.db.Close(); err != nil {
		t.Fatal(err)
	}

	second := newAuthEnv(t, dataDir, Options{Videos: sampleLibrary()})
	rec := second.get("/api/videos", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("組み立て直した経路: status = %d: %s", rec.Code, rec.Body)
	}
	assertAudience(t, "組み立て直した経路", rec, "owner")

	second.advance(domain.SessionLifetime - time.Minute)
	if rec := second.get("/api/videos", cookie); rec.Code != http.StatusOK {
		t.Fatalf("期限の直前: status = %d", rec.Code)
	}
	second.advance(2 * time.Minute)
	assertUnauthenticated(t, "90 日を過ぎた Cookie", second.get("/api/videos", cookie))
}

func TestAuthSessionCheckFailureIsInternalError(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{Videos: sampleLibrary()})
	cookie := env.setup()
	if err := env.db.Close(); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{"/api/videos", "/api/scans/current", "/api/videos/1"} {
		rec := env.get(target, cookie)
		if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"internal"`) {
			t.Errorf("%s: status = %d: %s", target, rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "海辺の散歩") {
			t.Errorf("%s: 保護対象を返した: %s", target, rec.Body)
		}
		assertAudience(t, target, rec, "guest")
	}
}

func TestAuthLogsOmitSecrets(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{})
	cookie := env.setup()
	env.serve(authRequest{method: http.MethodPost, target: "/api/auth/login", body: credentialsBody(testUsername, "wrong password")})
	second := env.login(false, cookie)
	env.serve(authRequest{method: http.MethodPost, target: "/api/auth/logout", cookies: []*http.Cookie{second}})

	logs := env.logs.String()
	for _, event := range []string{"event=setup", "event=login_failed", "event=login ", "event=logout", "source=192.0.2.1"} {
		if !strings.Contains(logs, event) {
			t.Errorf("記録に %q が無い:\n%s", event, logs)
		}
	}
	for _, secret := range []string{testPassword, "wrong password", testUsername, cookie.Value, second.Value} {
		if strings.Contains(logs, secret) {
			t.Errorf("記録に %q が出た:\n%s", secret, logs)
		}
	}
}

// 資格を失った要求の、処理中の /api/events と Range 応答が終わること
// （plan.md Structural Decisions 5）。
func TestAuthRevocationEndsInFlightResponses(t *testing.T) {
	cases := []struct {
		name   string
		revoke func(t *testing.T, env *authEnv, server *httptest.Server, cookie *http.Cookie)
	}{
		{"ログアウト", func(t *testing.T, _ *authEnv, server *httptest.Server, cookie *http.Cookie) {
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/auth/logout", nil)
			req.AddCookie(cookie)
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("ログアウト: status = %d", resp.StatusCode)
			}
		}},
		{"パスワードの再設定", func(t *testing.T, env *authEnv, _ *httptest.Server, _ *http.Cookie) {
			hash, err := password.Hash("a new password")
			if err != nil {
				t.Fatal(err)
			}
			if err := env.db.Auth().ChangePassword(context.Background(), hash, env.clock()); err != nil {
				t.Fatal(err)
			}
		}},
		{"ユーザー名の変更", func(t *testing.T, env *authEnv, _ *httptest.Server, _ *http.Cookie) {
			if err := env.db.Auth().ChangeUsername(context.Background(), "renamed", env.clock()); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mediaDir := t.TempDir()
			const size = 512 << 20
			moviePath := filepath.Join(mediaDir, "long.mp4")
			file, err := os.Create(moviePath)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Truncate(size); err != nil {
				t.Fatal(err)
			}
			_ = file.Close()
			video := sampleVideo(1, "long")
			video.Path = moviePath
			video.SizeBytes = size

			env := newAuthEnv(t, t.TempDir(), Options{
				Videos:         &fakeLibrary{videos: map[int64]domain.Video{1: video}, roots: []string{mediaDir}},
				Events:         NewEvents(),
				Processing:     &fakeProcessing{},
				SessionRecheck: 20 * time.Millisecond,
			})
			cookie := env.setup()
			server := httptest.NewServer(env.handler)
			t.Cleanup(server.Close)

			open := func(target string, header map[string]string) *http.Response {
				req, _ := http.NewRequest(http.MethodGet, server.URL+target, nil)
				req.AddCookie(cookie)
				for name, value := range header {
					req.Header.Set(name, value)
				}
				resp, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = resp.Body.Close() })
				return resp
			}
			events := open("/api/events", nil)
			if events.StatusCode != http.StatusOK {
				t.Fatalf("/api/events: status = %d", events.StatusCode)
			}
			if _, err := bufio.NewReader(events.Body).ReadString('\n'); err != nil {
				t.Fatal(err)
			}
			stream := open("/api/videos/1/stream", map[string]string{"Range": "bytes=0-"})
			if stream.StatusCode != http.StatusPartialContent {
				t.Fatalf("Range: status = %d", stream.StatusCode)
			}
			if _, err := io.ReadFull(stream.Body, make([]byte, 1024)); err != nil {
				t.Fatal(err)
			}

			tc.revoke(t, env, server, cookie)

			ended := func(label string, body io.Reader, check func(n int64, err error)) {
				done := make(chan struct{})
				go func() {
					defer close(done)
					n, err := io.Copy(io.Discard, body)
					check(n, err)
				}()
				select {
				case <-done:
				case <-time.After(10 * time.Second):
					t.Fatalf("%s が終わらない", label)
				}
			}
			ended("/api/events", events.Body, func(int64, error) {})
			ended("Range 応答", stream.Body, func(n int64, err error) {
				if err == nil && n+1024 >= size {
					t.Errorf("Range 応答が最後まで届いた (%d バイト)", n+1024)
				}
			})

			assertUnauthenticated(t, "打ち切り後の Cookie", env.get("/api/videos", cookie))
		})
	}
}

// 所有者として処理する要求の context は、セッションの期限を締め切りに持つ。
func TestSessionLedgerUsesSessionExpiryAsDeadline(t *testing.T) {
	ledger := newSessionLedger(time.Hour, slog.Default())
	ctx, release := ledger.track(context.Background(), httptest.NewRecorder(), "token", time.Now().Add(50*time.Millisecond))
	defer release()
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > time.Second {
		t.Fatalf("締め切り = %v, %v", deadline, ok)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("期限を過ぎても context が終わらない")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Errorf("ctx.Err() = %v", ctx.Err())
	}
}
