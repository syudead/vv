package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/password"
	"github.com/syudead/vv/internal/store"
)

// passwordHasher は internal/password を app.PasswordHasher にする。
type passwordHasher struct{}

func (passwordHasher) Hash(p string) (string, error)          { return password.Hash(p) }
func (passwordHasher) Verify(p, encoded string) (bool, error) { return password.Verify(p, encoded) }

// httpAuth は *app.Auth を httpapi.Authenticator にする。値の形を移すだけで、判断はしない。
type httpAuth struct{ auth *app.Auth }

func (a httpAuth) Setup(ctx context.Context, username, pass string) (httpapi.IssuedSession, error) {
	session, err := a.auth.Setup(ctx, username, pass)
	return httpapi.IssuedSession(session), err
}

func (a httpAuth) Login(ctx context.Context, attempt httpapi.LoginAttempt) (httpapi.IssuedSession, error) {
	session, err := a.auth.Login(ctx, app.LoginRequest(attempt))
	return httpapi.IssuedSession(session), err
}

func (a httpAuth) CheckSession(ctx context.Context, token string) (time.Time, bool, error) {
	return a.auth.CheckSession(ctx, token)
}

func (a httpAuth) Logout(ctx context.Context, token string) error { return a.auth.Logout(ctx, token) }

func (a httpAuth) State(ctx context.Context, token string) (httpapi.AuthState, error) {
	state, err := a.auth.State(ctx, token)
	return httpapi.AuthState(state), err
}

// newHTTPAuth は認証を組み立て、HTTP の境界に渡す形にする。
func newHTTPAuth(authStore *store.AuthStore) httpapi.Authenticator {
	return httpAuth{auth: app.NewAuth(app.AuthOptions{Store: authStore, Hasher: passwordHasher{}})}
}

// prepareAuth は起動時に期限切れのセッションを消し、アカウントが未設定なら初回設定を
// 促す警告を1行記録する（contracts/auth-api.md §9）。どちらの失敗も起動は止めない。
// セッションは要求ごとに期限を確かめるので、消し残しても使えない。
func prepareAuth(ctx context.Context, authStore *store.AuthStore, now time.Time, logger *slog.Logger) {
	if deleted, err := authStore.DeleteExpiredSessions(ctx, now); err != nil {
		logger.Warn("期限切れのセッションを消せませんでした", slog.Any("error", err))
	} else {
		logger.Info("期限切れのセッションを消しました", slog.Int64("sessions", deleted))
	}
	_, err := authStore.Account(ctx)
	switch {
	case errors.Is(err, domain.ErrAccountNotConfigured):
		logger.Warn("アカウントが未設定です。ブラウザで vv を開き、画面の初回設定でユーザー名とパスワードを決めてください。" +
			"設定するまでは、最初に開いた人がアカウントを作れます")
	case err != nil:
		logger.Warn("アカウントを確かめられませんでした", slog.Any("error", err))
	}
}
