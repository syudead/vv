package app

import (
	"context"
	"encoding/base64"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeAuthStore は AuthStore の偽物である。セッションは渡された ID のまま覚える
// （ハッシュにして保存するのは internal/store の役目で、そのテストが確かめる）。
type fakeAuthStore struct {
	mu       sync.Mutex
	account  *domain.Account
	sessions map[string]fakeSession
	// failSession が真なら Session が誤りを返す。
	failSession bool
	// failAccount が真なら Account が誤りを返す。
	failAccount bool
}

type fakeSession struct {
	version int64
	expires time.Time
}

var errFakeDB = errors.New("DB の失敗")

func newFakeAuthStore() *fakeAuthStore {
	return &fakeAuthStore{sessions: map[string]fakeSession{}}
}

func (f *fakeAuthStore) Account(context.Context) (domain.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAccount {
		return domain.Account{}, errFakeDB
	}
	if f.account == nil {
		return domain.Account{}, domain.ErrAccountNotConfigured
	}
	return *f.account, nil
}

func (f *fakeAuthStore) Setup(_ context.Context, username, passwordHash, token string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.account != nil {
		return domain.ErrAccountAlreadyConfigured
	}
	f.account = &domain.Account{Username: username, PasswordHash: passwordHash, Version: 1}
	f.sessions[token] = fakeSession{version: 1, expires: now.Add(domain.SessionLifetime)}
	return nil
}

func (f *fakeAuthStore) AddSession(_ context.Context, token string, version int64, replace string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if replace != "" {
		delete(f.sessions, replace)
	}
	f.sessions[token] = fakeSession{version: version, expires: now.Add(domain.SessionLifetime)}
	return nil
}

func (f *fakeAuthStore) Session(_ context.Context, token string, now time.Time) (time.Time, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSession {
		return time.Time{}, false, errFakeDB
	}
	s, ok := f.sessions[token]
	if !ok || f.account == nil || s.version != f.account.Version || !s.expires.After(now) {
		return time.Time{}, false, nil
	}
	return s.expires, true, nil
}

func (f *fakeAuthStore) DeleteSession(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, token)
	return nil
}

func (f *fakeAuthStore) hasSession(token string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.sessions[token]
	return ok
}

// fakeHasher は Argon2id の代わりに "hash:" を前に付けるだけのハッシュである。
// Verify の呼び出しを数え、gate があればそこから値を受け取るまで照合を止める。
type fakeHasher struct {
	verifies atomic.Int64
	gate     chan struct{}
	entered  chan struct{}
}

func (h *fakeHasher) Hash(password string) (string, error) { return "hash:" + password, nil }

func (h *fakeHasher) Verify(password, encoded string) (bool, error) {
	h.verifies.Add(1)
	if h.entered != nil {
		h.entered <- struct{}{}
	}
	if h.gate != nil {
		<-h.gate
	}
	return encoded == "hash:"+password, nil
}

// fakeClock はテストが進める時計である。
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type authFixture struct {
	auth   *Auth
	store  *fakeAuthStore
	hasher *fakeHasher
	clock  *fakeClock
}

func newAuthFixture(t *testing.T, configure func(*AuthOptions)) authFixture {
	t.Helper()
	f := authFixture{
		store:  newFakeAuthStore(),
		hasher: &fakeHasher{},
		clock:  &fakeClock{now: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)},
	}
	opts := AuthOptions{Store: f.store, Hasher: f.hasher, Now: f.clock.Now}
	if configure != nil {
		configure(&opts)
	}
	f.auth = NewAuth(opts)
	return f
}

// configured はアカウント owner / correct-password を設定済みにする。
func (f authFixture) configured(t *testing.T) authFixture {
	t.Helper()
	if _, err := f.auth.Setup(context.Background(), "owner", "correct-password"); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	return f
}

var (
	sourceA = netip.MustParseAddr("192.0.2.1")
	sourceB = netip.MustParseAddr("192.0.2.2")
)

func login(f authFixture, username, password string, source netip.Addr) (Session, error) {
	return f.auth.Login(context.Background(), LoginRequest{Username: username, Password: password, Source: source})
}

func TestAuthSetupReturnsSessionOnlyOnce(t *testing.T) {
	f := newAuthFixture(t, nil)
	ctx := context.Background()

	session, err := f.auth.Setup(ctx, "owner", "correct-password")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !f.store.hasSession(session.Token) {
		t.Fatal("初回設定のセッションが保存されていない")
	}
	if want := f.clock.Now().Add(domain.SessionLifetime); !session.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", session.ExpiresAt, want)
	}
	account, _ := f.store.Account(ctx)
	if account.Username != "owner" || account.PasswordHash != "hash:correct-password" {
		t.Errorf("account = %+v", account)
	}

	if _, err := f.auth.Setup(ctx, "other", "other-password"); !errors.Is(err, domain.ErrAccountAlreadyConfigured) {
		t.Errorf("2回目の Setup = %v, want ErrAccountAlreadyConfigured", err)
	}
	// 設定済みなら値の規則より先に「設定済み」を返す。
	if _, err := f.auth.Setup(ctx, "", ""); !errors.Is(err, domain.ErrAccountAlreadyConfigured) {
		t.Errorf("設定済みで空の Setup = %v, want ErrAccountAlreadyConfigured", err)
	}
}

func TestAuthSetupRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name, username, password string
		want                     error
	}{
		{"空のユーザー名", "", "password", domain.ErrInvalidUsername},
		{"先頭の空白", " owner", "password", domain.ErrInvalidUsername},
		{"空のパスワード", "owner", "", domain.ErrInvalidPassword},
		{"長すぎるパスワード", "owner", strings.Repeat("a", domain.MaxPasswordBytes+1), domain.ErrInvalidPassword},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthFixture(t, nil)
			if _, err := f.auth.Setup(context.Background(), tc.username, tc.password); !errors.Is(err, tc.want) {
				t.Fatalf("Setup = %v, want %v", err, tc.want)
			}
			if _, err := f.store.Account(context.Background()); !errors.Is(err, domain.ErrAccountNotConfigured) {
				t.Errorf("規則を外れた初回設定でアカウントができた: %v", err)
			}
		})
	}
}

func TestAuthLoginFailuresAreIndistinguishableAndVerifyOnce(t *testing.T) {
	cases := []struct {
		name, username, password string
		unconfigured             bool
	}{
		{name: "ユーザー名の誤り", username: "someone", password: "correct-password"},
		{name: "パスワードの誤り", username: "owner", password: "wrong-password"},
		{name: "両方の誤り", username: "someone", password: "wrong-password"},
		{name: "空のユーザー名", username: "", password: "correct-password"},
		{name: "空のパスワード", username: "owner", password: ""},
		{name: "上限を超えるパスワード", username: "owner", password: strings.Repeat("a", domain.MaxPasswordBytes+1)},
		{name: "上限を超えるユーザー名", username: strings.Repeat("a", domain.MaxUsernameLength+1), password: "correct-password"},
		{name: "大文字小文字だけ違うユーザー名", username: "Owner", password: "correct-password"},
		{name: "未設定", username: "owner", password: "correct-password", unconfigured: true},
		{name: "未設定で空欄", unconfigured: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthFixture(t, nil)
			if !tc.unconfigured {
				f.configured(t)
			}
			_, err := login(f, tc.username, tc.password, sourceA)
			if !errors.Is(err, domain.ErrInvalidCredentials) || err != domain.ErrInvalidCredentials { //nolint:errorlint // 原因を区別しない同じ値であることを確かめる。
				t.Fatalf("Login = %v, want ErrInvalidCredentials そのもの", err)
			}
			if got := f.hasher.verifies.Load(); got != 1 {
				t.Errorf("照合の回数 = %d, want 1", got)
			}
		})
	}
}

func TestAuthLoginIssuesRandomTokenWithAccountVersion(t *testing.T) {
	f := newAuthFixture(t, nil).configured(t)
	f.store.mu.Lock()
	f.store.account.Version = 7
	f.store.mu.Unlock()

	session, err := login(f, "owner", "correct-password", sourceA)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(session.Token)
	if err != nil || len(raw) != 32 {
		t.Fatalf("セッション ID %q は 32 バイトの base64url でない（%d バイト, %v）", session.Token, len(raw), err)
	}
	f.store.mu.Lock()
	stored := f.store.sessions[session.Token]
	f.store.mu.Unlock()
	if stored.version != 7 {
		t.Errorf("セッションの版 = %d, want 7（照合に使った版）", stored.version)
	}

	other, err := login(f, "owner", "correct-password", sourceA)
	if err != nil {
		t.Fatalf("2回目の Login: %v", err)
	}
	if other.Token == session.Token {
		t.Error("ログインのたびに違う ID を発行していない")
	}
}

func TestAuthLoginReplacesCurrentSession(t *testing.T) {
	f := newAuthFixture(t, nil)
	ctx := context.Background()
	first, err := f.auth.Setup(ctx, "owner", "correct-password")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	second, err := f.auth.Login(ctx, LoginRequest{
		Username: "owner", Password: "correct-password", Source: sourceA, CurrentToken: first.Token,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if f.store.hasSession(first.Token) {
		t.Error("有効なセッションの Cookie を持ったログインで、古いセッションが残った")
	}
	if !f.store.hasSession(second.Token) {
		t.Error("新しいセッションが保存されていない")
	}
}

func TestAuthLoginThrottlesSourceAfterFiveFailures(t *testing.T) {
	f := newAuthFixture(t, nil).configured(t)
	for i := range 5 {
		// ユーザー名を変えても同じ送信元として数える。
		if _, err := login(f, "user"+string(rune('a'+i)), "wrong", sourceA); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("%d 回目の Login = %v", i+1, err)
		}
		f.clock.Advance(time.Second)
	}
	before := f.hasher.verifies.Load()

	_, err := login(f, "owner", "correct-password", sourceA)
	var throttled *ThrottledError
	if !errors.As(err, &throttled) || !errors.Is(err, domain.ErrLoginThrottled) {
		t.Fatalf("6回目の Login = %v, want *ThrottledError", err)
	}
	// 最初の失敗から 5 秒経っているので、残りは 4 分 55 秒。
	if got := throttled.RetryAfterSeconds(); got != 295 {
		t.Errorf("RetryAfterSeconds = %d, want 295", got)
	}
	if got := f.hasher.verifies.Load(); got != before {
		t.Errorf("制限中に照合した（%d → %d）", before, got)
	}

	// 制限の誤りは数えないので、何度送っても期限は延びない。
	for range 3 {
		_, _ = login(f, "owner", "wrong", sourceA)
	}
	// 別の送信元には掛からない。
	if _, err := login(f, "owner", "correct-password", sourceB); err != nil {
		t.Errorf("別の送信元の Login = %v", err)
	}

	f.clock.Advance(loginFailureWindow - 5*time.Second)
	if _, err := login(f, "owner", "correct-password", sourceA); err != nil {
		t.Fatalf("5 分後の Login = %v", err)
	}
	// 成功で記録が消えるので、また 5 回まで照合する。
	for i := range 5 {
		if _, err := login(f, "owner", "wrong", sourceA); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("成功の後の %d 回目の Login = %v", i+1, err)
		}
	}
}

func TestAuthLoginConcurrentRequestsVerifyAtMostFiveTimes(t *testing.T) {
	f := newAuthFixture(t, nil).configured(t)
	f.hasher.verifies.Store(0)

	var (
		wg        sync.WaitGroup
		start     = make(chan struct{})
		throttled atomic.Int64
		invalid   atomic.Int64
	)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := login(f, "owner", "wrong", sourceA)
			switch {
			case errors.Is(err, domain.ErrLoginThrottled):
				throttled.Add(1)
			case errors.Is(err, domain.ErrInvalidCredentials):
				invalid.Add(1)
			default:
				t.Errorf("Login = %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := f.hasher.verifies.Load(); got > loginFailureLimit {
		t.Errorf("照合の回数 = %d, want %d 以下", got, loginFailureLimit)
	}
	if invalid.Load() != 5 || throttled.Load() != 15 {
		t.Errorf("資格情報の誤り %d・制限 %d, want 5・15", invalid.Load(), throttled.Load())
	}
}

func TestAuthLoginVerifyWaitTimeoutIsNotCounted(t *testing.T) {
	f := newAuthFixture(t, func(o *AuthOptions) { o.VerifyWait = 20 * time.Millisecond }).configured(t)
	f.hasher.gate = make(chan struct{})
	f.hasher.entered = make(chan struct{}, maxConcurrentVerifies)

	var wg sync.WaitGroup
	for range maxConcurrentVerifies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = login(f, "owner", "wrong", sourceA)
		}()
	}
	for range maxConcurrentVerifies {
		<-f.hasher.entered
	}

	_, err := login(f, "owner", "wrong", sourceA)
	var throttled *ThrottledError
	if !errors.As(err, &throttled) {
		t.Fatalf("空きを待ち切れない Login = %v, want *ThrottledError", err)
	}
	if throttled.RetryAfterSeconds() < 1 {
		t.Errorf("RetryAfterSeconds = %d", throttled.RetryAfterSeconds())
	}

	close(f.hasher.gate)
	wg.Wait()
	f.hasher.gate = nil
	f.hasher.entered = nil

	// 待ち切れなかった要求は数えていないので、失敗は2回。あと3回照合できる。
	for i := range 3 {
		if _, err := login(f, "owner", "wrong", sourceA); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("%d 回目の追加の Login = %v", i+1, err)
		}
	}
	if _, err := login(f, "owner", "wrong", sourceA); !errors.Is(err, domain.ErrLoginThrottled) {
		t.Errorf("6回目の Login = %v, want 制限", err)
	}
}

func TestAuthLoginThrottlesIPv6By64Prefix(t *testing.T) {
	f := newAuthFixture(t, nil).configured(t)
	for i := range 5 {
		addr := netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 15: byte(i + 1)})
		if _, err := login(f, "owner", "wrong", addr); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("%d 回目の Login = %v", i+1, err)
		}
	}
	if _, err := login(f, "owner", "correct-password", netip.MustParseAddr("2001:db8::ffff:1234")); !errors.Is(err, domain.ErrLoginThrottled) {
		t.Errorf("同じ /64 の別のアドレスの Login = %v, want 制限", err)
	}
	if _, err := login(f, "owner", "correct-password", netip.MustParseAddr("2001:db8:0:1::1")); err != nil {
		t.Errorf("別の /64 の Login = %v", err)
	}
}

func TestThrottleKey(t *testing.T) {
	cases := []struct{ addr, want string }{
		{"192.0.2.1", "192.0.2.1"},
		{"::ffff:192.0.2.1", "192.0.2.1"},
		{"2001:db8::1", "2001:db8::/64"},
		{"2001:db8::ffff:1", "2001:db8::/64"},
		{"fe80::1%eth0", "fe80::/64"},
	}
	for _, tc := range cases {
		if got := throttleKey(netip.MustParseAddr(tc.addr)); got != tc.want {
			t.Errorf("throttleKey(%s) = %q, want %q", tc.addr, got, tc.want)
		}
	}
	if got := throttleKey(netip.Addr{}); got != "invalid" {
		t.Errorf("throttleKey(zero) = %q", got)
	}
}

func TestLoginThrottleBoundsSources(t *testing.T) {
	throttle := newLoginThrottle(3)
	now := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	for _, key := range []string{"a", "b", "c", "d"} {
		if _, ok := throttle.reserve(key, now); !ok {
			t.Fatalf("reserve(%s) が拒まれた", key)
		}
		throttle.fail(key, now)
	}
	if len(throttle.sources) != 3 {
		t.Fatalf("送信元の数 = %d, want 3", len(throttle.sources))
	}
	if _, ok := throttle.sources["a"]; ok {
		t.Error("最も古い送信元が捨てられていない")
	}

	// 送信元ごとの失敗は上限の数だけ覚え、古いものから捨てる。
	for i := range 8 {
		throttle.fail("d", now.Add(time.Duration(i)*time.Second))
	}
	d := throttle.sources["d"].Value.(*throttleSource)
	if len(d.failures) != loginFailureLimit {
		t.Errorf("失敗の記録 = %d 件, want %d", len(d.failures), loginFailureLimit)
	}
}

func TestAuthSessionCheckAndLogout(t *testing.T) {
	f := newAuthFixture(t, nil)
	ctx := context.Background()
	session, err := f.auth.Setup(ctx, "owner", "correct-password")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	expires, valid, err := f.auth.CheckSession(ctx, session.Token)
	if err != nil || !valid || !expires.Equal(session.ExpiresAt) {
		t.Fatalf("CheckSession = %v, %v, %v", expires, valid, err)
	}
	for _, token := range []string{"", "not-a-token", session.Token + "A", strings.Repeat("=", 43)} {
		if _, valid, err := f.auth.CheckSession(ctx, token); err != nil || valid {
			t.Errorf("CheckSession(%q) = %v, %v", token, valid, err)
		}
	}

	f.clock.Advance(domain.SessionLifetime)
	if _, valid, _ := f.auth.CheckSession(ctx, session.Token); valid {
		t.Error("90 日を過ぎたセッションが有効")
	}
	f.clock.Advance(-domain.SessionLifetime)

	if err := f.auth.Logout(ctx, session.Token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, valid, _ := f.auth.CheckSession(ctx, session.Token); valid {
		t.Error("ログアウトしたセッションが有効")
	}
	if err := f.auth.Logout(ctx, "not-a-token"); err != nil {
		t.Errorf("形式の違う値の Logout = %v", err)
	}
}

func TestAuthSessionCheckDBFailureIsError(t *testing.T) {
	f := newAuthFixture(t, nil)
	ctx := context.Background()
	session, err := f.auth.Setup(ctx, "owner", "correct-password")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	f.store.failSession = true
	if _, valid, err := f.auth.CheckSession(ctx, session.Token); !errors.Is(err, errFakeDB) || valid {
		t.Errorf("CheckSession = %v, %v, want DB の誤り", valid, err)
	}
	if _, err := f.auth.State(ctx, session.Token); !errors.Is(err, errFakeDB) {
		t.Errorf("State = %v, want DB の誤り", err)
	}
}

func TestAuthState(t *testing.T) {
	f := newAuthFixture(t, nil)
	ctx := context.Background()
	unrelated := base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	if state, err := f.auth.State(ctx, unrelated); err != nil || state != AuthStateSetupRequired {
		t.Errorf("未設定の State = %q, %v, want setupRequired", state, err)
	}
	session, err := f.auth.Setup(ctx, "owner", "correct-password")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if state, err := f.auth.State(ctx, session.Token); err != nil || state != AuthStateOwner {
		t.Errorf("ログイン済みの State = %q, %v, want owner", state, err)
	}
	for _, token := range []string{"", unrelated} {
		if state, err := f.auth.State(ctx, token); err != nil || state != AuthStateGuest {
			t.Errorf("State(%q) = %q, %v, want guest", token, state, err)
		}
	}

	// 資格情報が書き換えられると（版が進むと）、同じ Cookie はゲストになる。
	f.store.mu.Lock()
	f.store.account.Version++
	f.store.mu.Unlock()
	if state, _ := f.auth.State(ctx, session.Token); state != AuthStateGuest {
		t.Errorf("版が進んだ後の State = %q, want guest", state)
	}

	f.store.failAccount = true
	if _, err := f.auth.State(ctx, session.Token); !errors.Is(err, errFakeDB) {
		t.Errorf("Account が失敗した State = %v, want DB の誤り", err)
	}
}
