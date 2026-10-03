package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// fakeNetworkListener は待ち受けの代わりで、開き直したアドレスを覚える。fail に入った
// アドレスは開けない。開き直しが重なったら overlapped を立てる。
type fakeNetworkListener struct {
	mu         sync.Mutex
	addr       string
	fail       map[string]bool
	reopened   []string
	busy       bool
	overlapped bool
}

func (l *fakeNetworkListener) Reopen(addr string) error {
	l.mu.Lock()
	if l.busy {
		l.overlapped = true
	}
	l.busy = true
	l.reopened = append(l.reopened, addr)
	failed := l.fail[addr]
	if !failed {
		l.addr = addr
	}
	l.mu.Unlock()

	// 開き直しの途中で他の goroutine に譲り、重なりがあれば見つけられるようにする。
	runtime.Gosched()

	l.mu.Lock()
	l.busy = false
	l.mu.Unlock()
	if failed {
		return errors.New("bind: address already in use")
	}
	return nil
}

func (l *fakeNetworkListener) Addr() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.addr
}

func (l *fakeNetworkListener) reopens() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.reopened)
}

// memoryLANStore は保存値をメモリに持つ。
type memoryLANStore struct {
	mu      sync.Mutex
	value   bool
	saves   int
	saveErr error
}

func (s *memoryLANStore) SaveLANAccess(_ context.Context, allowed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	s.value = allowed
	s.saves++
	return nil
}

func (s *memoryLANStore) stored() (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value, s.saves
}

const testNetworkPort = "47880"

// testInterfaceAddrs は上がっているインターフェースのアドレスとして、ループバックと IPv6 を
// 混ぜて返す。応答には非ループバックの IPv4 だけが入る。
func testInterfaceAddrs() ([]netip.Addr, error) {
	return []netip.Addr{
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParseAddr("192.168.1.20"),
		netip.MustParseAddr("fe80::1"),
		netip.MustParseAddr("10.0.0.5"),
	}, nil
}

var testLANAddresses = []string{"http://192.168.1.20:47880/", "http://10.0.0.5:47880/"}

// newTestNetworkSettings は本物の app.NetworkSettings を、偽の待ち受けと保存先で作る。
func newTestNetworkSettings(store *memoryLANStore, listener *fakeNetworkListener) *app.NetworkSettings {
	return app.NewNetworkSettings(app.NetworkSettingsOptions{
		Store: store, Listener: listener, LANAccess: store.value, Addresses: testInterfaceAddrs, Logger: discardLogger(),
	})
}

func loopbackListener() *fakeNetworkListener {
	return &fakeNetworkListener{addr: "127.0.0.1:" + testNetworkPort}
}

func putNetwork(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/settings/network", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func assertNetworkResponse(t *testing.T, label string, rec *httptest.ResponseRecorder, want gen.NetworkSettings) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", label, rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("%s: Cache-Control = %q", label, got)
	}
	// addresses は偽のときも空の配列で、null にしない。
	if !strings.Contains(rec.Body.String(), `"addresses":[`) {
		t.Errorf("%s: addresses が配列でない: %s", label, rec.Body)
	}
	got := decode[gen.NetworkSettings](t, rec)
	if got.LanAccess != want.LanAccess || got.Port != want.Port || !slices.Equal(got.Addresses, want.Addresses) {
		t.Errorf("%s: 応答 = %+v, want %+v", label, got, want)
	}
}

// §2・§3: デスクトップ版でなければ 404 not_found（reason なし）。
func TestNetworkSettingsOutsideDesktopIsNotFound(t *testing.T) {
	handler := newTestServer(t, Options{Logger: discardLogger()})
	for _, tc := range []struct {
		label string
		rec   *httptest.ResponseRecorder
	}{
		{"GET", do(t, handler, http.MethodGet, "/api/settings/network")},
		{"PUT", putNetwork(t, handler, `{"lanAccess":true}`)},
		// 本文の確認より、デスクトップ版でないことの判定が先。
		{"PUT の不正な本文", putNetwork(t, handler, `{}`)},
	} {
		assertErrorBody(t, tc.label, tc.rec.Code, tc.rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
	}
}

// §2: 許可していなければ addresses は空、許可中は非ループバックの IPv4 だけが入る。
func TestGetNetworkSettings(t *testing.T) {
	denied := newTestServer(t, Options{NetworkSettings: newTestNetworkSettings(&memoryLANStore{}, loopbackListener())})
	assertNetworkResponse(t, "許可していない", do(t, denied, http.MethodGet, "/api/settings/network"), gen.NetworkSettings{
		LanAccess: false, Port: 47880, Addresses: []string{},
	})

	allowed := newTestServer(t, Options{NetworkSettings: newTestNetworkSettings(
		&memoryLANStore{value: true}, &fakeNetworkListener{addr: "0.0.0.0:" + testNetworkPort})})
	assertNetworkResponse(t, "許可中", do(t, allowed, http.MethodGet, "/api/settings/network"), gen.NetworkSettings{
		LanAccess: true, Port: 47880, Addresses: testLANAddresses,
	})
}

// §3: 切り替えでは待ち受けを開き直してから保存し、開き直したあとの値を返す。
func TestUpdateNetworkSettingsSwitchesListenerThenSaves(t *testing.T) {
	store := &memoryLANStore{}
	listener := loopbackListener()
	handler := newTestServer(t, Options{NetworkSettings: newTestNetworkSettings(store, listener)})

	assertNetworkResponse(t, "許可する", putNetwork(t, handler, `{"lanAccess":true}`), gen.NetworkSettings{
		LanAccess: true, Port: 47880, Addresses: testLANAddresses,
	})
	if got := listener.Addr(); got != "0.0.0.0:47880" {
		t.Errorf("許可したあとの待ち受け = %q", got)
	}
	if value, saves := store.stored(); !value || saves != 1 {
		t.Errorf("保存値 = %v（%d 回）", value, saves)
	}

	assertNetworkResponse(t, "許可をやめる", putNetwork(t, handler, `{"lanAccess":false}`), gen.NetworkSettings{
		LanAccess: false, Port: 47880, Addresses: []string{},
	})
	if got := listener.Addr(); got != "127.0.0.1:47880" {
		t.Errorf("許可をやめたあとの待ち受け = %q", got)
	}
	if value, saves := store.stored(); value || saves != 2 {
		t.Errorf("保存値 = %v（%d 回）", value, saves)
	}
}

// §3: 今と同じ値なら何もせず 200。
func TestUpdateNetworkSettingsSameValueDoesNothing(t *testing.T) {
	store := &memoryLANStore{}
	listener := loopbackListener()
	handler := newTestServer(t, Options{NetworkSettings: newTestNetworkSettings(store, listener)})

	assertNetworkResponse(t, "同じ値", putNetwork(t, handler, `{"lanAccess":false}`), gen.NetworkSettings{
		LanAccess: false, Port: 47880, Addresses: []string{},
	})
	if reopens := listener.reopens(); len(reopens) != 0 {
		t.Errorf("同じ値で開き直した: %v", reopens)
	}
	if _, saves := store.stored(); saves != 0 {
		t.Errorf("同じ値で保存した（%d 回）", saves)
	}
}

// §3: 本文が不正なら 400 で、待ち受けも保存値も変えない。
func TestUpdateNetworkSettingsRejectsInvalidBody(t *testing.T) {
	store := &memoryLANStore{}
	listener := loopbackListener()
	handler := newTestServer(t, Options{NetworkSettings: newTestNetworkSettings(store, listener)})

	for _, tc := range []struct{ label, body string }{
		{"値が無い", `{}`},
		{"真偽でない", `{"lanAccess":"true"}`},
		{"null", `{"lanAccess":null}`},
		{"知らない項目", `{"lanAccess":true,"port":80}`},
		{"JSON でない", `true`},
	} {
		rec := putNetwork(t, handler, tc.body)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}
	if reopens := listener.reopens(); len(reopens) != 0 {
		t.Errorf("不正な本文で開き直した: %v", reopens)
	}
	if _, saves := store.stored(); saves != 0 {
		t.Errorf("不正な本文で保存した（%d 回）", saves)
	}
}

// §3: 新しいアドレスで開けなければ 409 listen_failed。待ち受けは元のアドレスのまま、
// 保存値も変えない。
func TestUpdateNetworkSettingsListenFailure(t *testing.T) {
	store := &memoryLANStore{}
	listener := loopbackListener()
	listener.fail = map[string]bool{"0.0.0.0:47880": true}
	handler := newTestServer(t, Options{NetworkSettings: newTestNetworkSettings(store, listener), Logger: discardLogger()})

	rec := putNetwork(t, handler, `{"lanAccess":true}`)
	assertErrorBody(t, "開き直しの失敗", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonListenFailed,
	})
	if strings.Contains(rec.Body.String(), "already in use") {
		t.Errorf("OS の誤りが応答に出ている: %s", rec.Body)
	}
	if value, saves := store.stored(); value || saves != 0 {
		t.Errorf("保存値 = %v（%d 回）", value, saves)
	}
	if got := listener.Addr(); got != "127.0.0.1:47880" {
		t.Errorf("待ち受け = %q", got)
	}
	assertNetworkResponse(t, "失敗のあとの GET", do(t, handler, http.MethodGet, "/api/settings/network"), gen.NetworkSettings{
		LanAccess: false, Port: 47880, Addresses: []string{},
	})
}

// §3: 開き直せたが保存に失敗したら 500 で、待ち受けを元のアドレスへ開き直す。
func TestUpdateNetworkSettingsSaveFailureRestoresListener(t *testing.T) {
	store := &memoryLANStore{saveErr: errors.New("disk I/O error")}
	listener := loopbackListener()
	handler := newTestServer(t, Options{NetworkSettings: newTestNetworkSettings(store, listener), Logger: discardLogger()})

	rec := putNetwork(t, handler, `{"lanAccess":true}`)
	assertErrorBody(t, "保存の失敗", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusInternalServerError, code: gen.ErrorCodeInternal})
	if strings.Contains(rec.Body.String(), "disk I/O") {
		t.Errorf("保存の誤りが応答に出ている: %s", rec.Body)
	}
	if got := listener.reopens(); !slices.Equal(got, []string{"0.0.0.0:47880", "127.0.0.1:47880"}) {
		t.Errorf("開き直し = %v", got)
	}
	if got := listener.Addr(); got != "127.0.0.1:47880" {
		t.Errorf("待ち受け = %q（保存値は偽のまま）", got)
	}
	assertNetworkResponse(t, "失敗のあとの GET", do(t, handler, http.MethodGet, "/api/settings/network"), gen.NetworkSettings{
		LanAccess: false, Port: 47880, Addresses: []string{},
	})
}

// §3: 同時の PUT は 1 つずつ処理し、終わったあとの待ち受けは保存値と一致する。
func TestUpdateNetworkSettingsSerializesConcurrentRequests(t *testing.T) {
	store := &memoryLANStore{}
	listener := loopbackListener()
	handler := newTestServer(t, Options{NetworkSettings: newTestNetworkSettings(store, listener)})

	var wg sync.WaitGroup
	for i := range 40 {
		body := `{"lanAccess":true}`
		if i%2 == 1 {
			body = `{"lanAccess":false}`
		}
		wg.Go(func() {
			if rec := putNetwork(t, handler, body); rec.Code != http.StatusOK {
				t.Errorf("status = %d: %s", rec.Code, rec.Body)
			}
		})
	}
	wg.Wait()

	if listener.overlapped {
		t.Error("開き直しが重なった")
	}
	value, _ := store.stored()
	if got, want := listener.Addr(), domain.LANListenAddr(value, 47880); got != want {
		t.Errorf("待ち受け = %q, 保存値 %v に合うのは %q", got, value, want)
	}
}

// ゲストは見られず変えられず、別サイトからの PUT は 403（§2・§3、/api/settings/transcoding と同じ境界）。
func TestNetworkSettingsAreOwnerOnly(t *testing.T) {
	store := &memoryLANStore{}
	listener := loopbackListener()
	env := newAuthEnv(t, t.TempDir(), Options{NetworkSettings: newTestNetworkSettings(store, listener)})
	env.setup()

	assertUnauthenticated(t, "ゲストの GET", env.get("/api/settings/network"))
	assertUnauthenticated(t, "ゲストの PUT", env.serve(authRequest{
		method: http.MethodPut, target: "/api/settings/network", body: `{"lanAccess":true}`,
	}))
	crossSite := env.serve(authRequest{
		method: http.MethodPut, target: "/api/settings/network", body: `{"lanAccess":true}`,
		cookies: []*http.Cookie{env.login(false)}, header: map[string]string{"Sec-Fetch-Site": "cross-site"},
	})
	assertErrorBody(t, "別サイトからの PUT", crossSite.Code, crossSite.Body.Bytes(), wantError{
		status: http.StatusForbidden, code: gen.ErrorCodeForbidden, reason: gen.ErrorReasonCrossOrigin,
	})
	if _, saves := store.stored(); saves != 0 {
		t.Error("拒んだ要求で保存された")
	}
	if reopens := listener.reopens(); len(reopens) != 0 {
		t.Errorf("拒んだ要求で開き直した: %v", reopens)
	}
}
