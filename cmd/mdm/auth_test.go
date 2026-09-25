package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/store"
)

// ownerAuth はどの要求も所有者として通す。認証の境界は internal/httpapi の
// auth_test.go が確かめるので、他の経路の組み立てのテストはこれで境界を越える。
type ownerAuth struct{}

func (ownerAuth) Setup(context.Context, string, string) (httpapi.IssuedSession, error) {
	return httpapi.IssuedSession{}, domain.ErrAccountAlreadyConfigured
}

func (ownerAuth) Login(context.Context, httpapi.LoginAttempt) (httpapi.IssuedSession, error) {
	return httpapi.IssuedSession{}, domain.ErrInvalidCredentials
}

func (ownerAuth) CheckSession(context.Context, string) (time.Time, bool, error) {
	return time.Now().Add(time.Hour), true, nil
}

func (ownerAuth) Logout(context.Context, string) error { return nil }

func (ownerAuth) State(context.Context, string) (httpapi.AuthState, error) {
	return httpapi.AuthStateOwner, nil
}

// 組み立てた認証で初回設定した Cookie が所有者として通り、ホストのコマンドで
// パスワードを再設定すると通らなくなる。
func TestAssembledAuthSetupAndHostPasswordReset(t *testing.T) {
	dataDir := newMigratedDataDir(t)
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	handler := httpapi.NewRouter(httpapi.Options{
		Videos: db.Library(), Assets: fstest.MapFS{}, Auth: newHTTPAuth(db.Auth()),
	})
	serve := func(method, target, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := serve(http.MethodGet, "/api/videos", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("未設定の一覧: status = %d", rec.Code)
	}
	rec := serve(http.MethodPost, "/api/auth/setup", `{"username":"alice","password":"`+oldPassword+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("初回設定: status = %d: %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Cookie = %v", cookies)
	}
	cookie := &http.Cookie{Name: cookies[0].Name, Value: cookies[0].Value}
	if rec := serve(http.MethodGet, "/api/videos", "", cookie); rec.Code != http.StatusOK {
		t.Fatalf("初回設定の Cookie で一覧: status = %d: %s", rec.Code, rec.Body)
	}

	if run := runAccountCommand(t, dataDir, []string{"account", "set-password"}, newPassword+"\n", nil); run.code != exitAccountOK {
		t.Fatalf("set-password: code = %d: %s", run.code, run.stderr)
	}
	if rec := serve(http.MethodGet, "/api/videos", "", cookie); rec.Code != http.StatusUnauthorized {
		t.Fatalf("再設定後の Cookie: status = %d", rec.Code)
	}
	login := serve(http.MethodPost, "/api/auth/login", `{"username":"alice","password":"`+newPassword+`"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("新しいパスワードでログイン: status = %d: %s", login.Code, login.Body)
	}
}

func TestPrepareAuthWarnsWhenNotConfigured(t *testing.T) {
	dataDir := newMigratedDataDir(t)
	var logs bytes.Buffer
	withAuth(t, dataDir, func(auth *store.AuthStore) {
		prepareAuth(context.Background(), auth, time.Now(), slog.New(slog.NewTextHandler(&logs, nil)))
	})
	if !strings.Contains(logs.String(), "アカウントが未設定です") {
		t.Errorf("未設定の警告が無い:\n%s", logs.String())
	}
}

func TestPrepareAuthDeletesExpiredSessions(t *testing.T) {
	dataDir := newConfiguredDataDir(t)
	var logs bytes.Buffer
	withAuth(t, dataDir, func(auth *store.AuthStore) {
		prepareAuth(context.Background(), auth, accountTestNow.Add(domain.SessionLifetime+time.Hour),
			slog.New(slog.NewTextHandler(&logs, nil)))
	})
	if got := countRows(t, dataDir, "sessions"); got != 0 {
		t.Errorf("期限切れのセッションが %d 行残った", got)
	}
	if strings.Contains(logs.String(), "アカウントが未設定です") {
		t.Errorf("設定済みなのに未設定の警告が出た:\n%s", logs.String())
	}
}
