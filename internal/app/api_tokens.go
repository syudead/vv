package app

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Tokens are vvt_ plus thirty-two random bytes in unpadded base64url; storage hashes the plaintext
// it receives.

// The prefix makes tokens recognizable in configuration and logs.
const apiTokenPrefix = "vvt_"

// CreateAPIToken returns plaintext only once; reject invalid names and sessions no longer valid at
// commit time.
func (a *Auth) CreateAPIToken(ctx context.Context, sessionToken, name string) (domain.APIToken, string, error) {
	normalized, err := domain.NormalizeAPITokenName(name)
	if err != nil {
		return domain.APIToken{}, "", err
	}
	raw := make([]byte, sessionTokenBytes)
	if _, err := io.ReadFull(a.random, raw); err != nil {
		return domain.APIToken{}, "", fmt.Errorf("cannot create an API token: %w", err)
	}
	secret := apiTokenPrefix + sessionTokenEncoding.EncodeToString(raw)
	token, err := a.store.AddAPIToken(ctx, sessionToken, normalized, secret, a.now())
	if err != nil {
		return domain.APIToken{}, "", err
	}
	return token, secret, nil
}

// ListAPITokens excludes secrets and hashes and sorts newest first.
func (a *Auth) ListAPITokens(ctx context.Context) ([]domain.APIToken, error) {
	return a.store.ListAPITokens(ctx)
}

// Revoking a missing ID is a no-op.
func (a *Auth) RevokeAPIToken(ctx context.Context, id int64) error {
	return a.store.DeleteAPIToken(ctx, id)
}

// CheckAPIToken rejects malformed values without storage access; storage failures remain errors
// rather than invalid credentials.
func (a *Auth) CheckAPIToken(ctx context.Context, secret string) (domain.APIToken, bool, error) {
	if !validAPITokenFormat(secret) {
		return domain.APIToken{}, false, nil
	}
	return a.store.APIToken(ctx, secret)
}

// Storage throttles last-use writes to once per sixty seconds.
func (a *Auth) RecordAPITokenUse(ctx context.Context, id int64) error {
	return a.store.TouchAPIToken(ctx, id, a.now())
}

func validAPITokenFormat(secret string) bool {
	rest, ok := strings.CutPrefix(secret, apiTokenPrefix)
	return ok && validTokenFormat(rest)
}
