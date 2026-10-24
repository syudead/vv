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

type passwordHasher struct{}

func (passwordHasher) Hash(p string) (string, error)          { return password.Hash(p) }
func (passwordHasher) Verify(p, encoded string) (bool, error) { return password.Verify(p, encoded) }

// httpAuth only adapts values; authentication decisions belong to app.Auth.
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

func (a httpAuth) Username(ctx context.Context) (string, error) { return a.auth.Username(ctx) }

func (a httpAuth) ChangeCredentials(
	ctx context.Context, change httpapi.CredentialChange,
) (httpapi.IssuedSession, error) {
	session, err := a.auth.ChangeCredentials(ctx, app.CredentialChange(change))
	return httpapi.IssuedSession(session), err
}

// Reuse app.Auth for API tokens because it already implements httpapi.APITokens.
func newHTTPAuth(authStore *store.AuthStore) (httpapi.Authenticator, httpapi.APITokens) {
	auth := app.NewAuth(app.AuthOptions{Store: authStore, Hasher: passwordHasher{}})
	return httpAuth{auth: auth}, auth
}

// Cleanup failures do not block startup; each request still rejects expired sessions.
func prepareAuth(ctx context.Context, authStore *store.AuthStore, now time.Time, logger *slog.Logger) {
	if deleted, err := authStore.DeleteExpiredSessions(ctx, now); err != nil {
		logger.Warn("could not delete expired sessions", slog.Any("error", err))
	} else {
		logger.Info("deleted expired sessions", slog.Int64("sessions", deleted))
	}
	_, err := authStore.Account(ctx)
	switch {
	case errors.Is(err, domain.ErrAccountNotConfigured):
		logger.Warn("the account is not set up; open VVMDM in a browser and choose a username and password in the initial setup. " +
			"Until then, the first person to open VVMDM can create the account")
	case err != nil:
		logger.Warn("could not check the account", slog.Any("error", err))
	}
}
