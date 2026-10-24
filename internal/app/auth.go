package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	sessionTokenBytes     = 32
	maxConcurrentVerifies = 2
	defaultVerifyWait     = 5 * time.Second
)

// Use a dummy hash to conceal whether setup is complete through timing; regenerate it whenever
// password package defaults change to keep Argon2id parameters identical.
const dummyPasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$/Q7m10dyrvPbZgh4jySOzQ$zmfvSfxjbt65JfCJEu7mai1a/zN+8AcpRYQh4f7V8NQ"

// Pass plaintext session tokens to storage, which owns hashing.
type AuthStore interface {
	// Account returns ErrAccountNotConfigured before setup.
	Account(ctx context.Context) (domain.Account, error)
	// Setup atomically adds the account and first session, rejecting already configured accounts.
	Setup(ctx context.Context, username, passwordHash, sessionToken string, now time.Time) error
	// Add a session for the verified account version and optionally replace the prior token.
	AddSession(ctx context.Context, sessionToken string, accountVersion int64, replaceToken string, now time.Time) error
	Session(ctx context.Context, sessionToken string, now time.Time) (time.Time, bool, error)
	// Deleting an absent session is a no-op.
	DeleteSession(ctx context.Context, sessionToken string) error
	// ReplaceCredentials returns ErrSessionNotValid unless sessionToken is still a current session;
	// it signs out every session, keeps API tokens and adds newSessionToken at the new account version.
	ReplaceCredentials(ctx context.Context, sessionToken, username, passwordHash, newSessionToken string, now time.Time) error

	// Add tokens only while the authorizing session remains valid, using its account version;
	// otherwise return ErrSessionNotValid.
	AddAPIToken(ctx context.Context, sessionToken, name, token string, now time.Time) (domain.APIToken, error)
	ListAPITokens(ctx context.Context) ([]domain.APIToken, error)
	// Deleting an absent token is a no-op.
	DeleteAPIToken(ctx context.Context, id int64) error
	APIToken(ctx context.Context, token string) (domain.APIToken, bool, error)
	// Write last-use time only when absent or at least sixty seconds old.
	TouchAPIToken(ctx context.Context, id int64, now time.Time) error
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encoded string) (bool, error)
}

type AuthOptions struct {
	Store  AuthStore
	Hasher PasswordHasher
	// Nil uses time.Now.
	Now func() time.Time
	// Nil uses crypto/rand.Reader for session and API token entropy.
	Random io.Reader
	// Zero uses a five-second slot wait.
	VerifyWait time.Duration
	// Zero uses the default throttle-source limit.
	MaxThrottleSources int
}

// Auth owns authentication and API-token use cases without HTTP cookies or status codes; callers
// supply source and token values.
type Auth struct {
	store      AuthStore
	hasher     PasswordHasher
	now        func() time.Time
	random     io.Reader
	verifyWait time.Duration
	// Bound concurrent Argon2id computations.
	verifySlots chan struct{}
	throttle    *loginThrottle
}

func NewAuth(opts AuthOptions) *Auth {
	a := &Auth{
		store:       opts.Store,
		hasher:      opts.Hasher,
		now:         opts.Now,
		random:      opts.Random,
		verifyWait:  opts.VerifyWait,
		verifySlots: make(chan struct{}, maxConcurrentVerifies),
		throttle:    newLoginThrottle(opts.MaxThrottleSources),
	}
	if a.now == nil {
		a.now = time.Now
	}
	if a.random == nil {
		a.random = rand.Reader
	}
	if a.verifyWait <= 0 {
		a.verifyWait = defaultVerifyWait
	}
	return a
}

// Session.Token is thirty-two random bytes in unpadded base64url for the cookie.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

type AuthState string

const (
	AuthStateOwner         AuthState = "owner"
	AuthStateGuest         AuthState = "guest"
	AuthStateSetupRequired AuthState = "setupRequired"
)

// ThrottledError wraps ErrLoginThrottled; RetryAfter estimates when to retry and maps to HTTP
// Retry-After seconds.
type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string { return domain.ErrLoginThrottled.Error() }

func (e *ThrottledError) Unwrap() error { return domain.ErrLoginThrottled }

// Round RetryAfter up to at least one second.
func (e *ThrottledError) RetryAfterSeconds() int {
	seconds := int((e.RetryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

// Setup returns a logged-in session after validation; check existing configuration before Argon2id
// work, while storage rejects concurrent setup races atomically.
func (a *Auth) Setup(ctx context.Context, username, password string) (Session, error) {
	if _, err := a.store.Account(ctx); err == nil {
		return Session{}, domain.ErrAccountAlreadyConfigured
	} else if !errors.Is(err, domain.ErrAccountNotConfigured) {
		return Session{}, err
	}
	if err := domain.ValidateUsername(username); err != nil {
		return Session{}, err
	}
	if err := domain.ValidatePassword(password); err != nil {
		return Session{}, err
	}

	if err := a.acquireVerifySlot(ctx, 0); err != nil {
		return Session{}, err
	}
	hash, err := a.hasher.Hash(password)
	a.releaseVerifySlot()
	if err != nil {
		return Session{}, fmt.Errorf("cannot hash the password: %w", err)
	}

	token, err := a.newSessionToken()
	if err != nil {
		return Session{}, err
	}
	now := a.now()
	if err := a.store.Setup(ctx, username, hash, token, now); err != nil {
		return Session{}, err
	}
	return Session{Token: token, ExpiresAt: sessionExpiry(now)}, nil
}

type LoginRequest struct {
	Username string
	Password string
	// Source IP determines the throttle bucket.
	Source netip.Addr
	// Successful login removes this existing session token.
	CurrentToken string
}

// Login performs one Argon2id verification for every invalid-credential cause, including
// unconfigured accounts; throttled sources or exhausted verification slots skip hashing and do not
// count as failures.
func (a *Auth) Login(ctx context.Context, req LoginRequest) (Session, error) {
	source := throttleKey(req.Source)
	if retryAfter, ok := a.throttle.reserve(source, a.now()); !ok {
		return Session{}, &ThrottledError{RetryAfter: retryAfter}
	}
	// Every later return must resolve the reservation as success, failure, or cancellation.

	account, err := a.store.Account(ctx)
	configured := true
	if errors.Is(err, domain.ErrAccountNotConfigured) {
		configured = false
		account = domain.Account{PasswordHash: dummyPasswordHash}
	} else if err != nil {
		a.throttle.cancel(source)
		return Session{}, err
	}

	if err := a.acquireVerifySlot(ctx, a.verifyWait); err != nil {
		a.throttle.cancel(source)
		return Session{}, err
	}
	// Verify even blank or oversized credentials to conceal the failure cause through timing; HTTP
	// caps request bodies at 8 KiB.
	matched, err := a.hasher.Verify(req.Password, account.PasswordHash)
	a.releaseVerifySlot()
	if err != nil {
		a.throttle.cancel(source)
		return Session{}, fmt.Errorf("cannot verify the password: %w", err)
	}

	usernameMatched := equalUsername(req.Username, account.Username)
	if !configured || !usernameMatched || !matched {
		a.throttle.fail(source, a.now())
		return Session{}, domain.ErrInvalidCredentials
	}

	token, err := a.newSessionToken()
	if err != nil {
		a.throttle.cancel(source)
		return Session{}, err
	}
	now := a.now()
	// Deleting an invalid prior session is harmless because it was already unusable.
	replace := ""
	if validTokenFormat(req.CurrentToken) {
		replace = req.CurrentToken
	}
	if err := a.store.AddSession(ctx, token, account.Version, replace, now); err != nil {
		a.throttle.cancel(source)
		return Session{}, err
	}
	a.throttle.succeed(source)
	return Session{Token: token, ExpiresAt: sessionExpiry(now)}, nil
}

// Username returns ErrAccountNotConfigured before setup.
func (a *Auth) Username(ctx context.Context) (string, error) {
	account, err := a.store.Account(ctx)
	if err != nil {
		return "", err
	}
	return account.Username, nil
}

type CredentialChange struct {
	// SessionToken is the signed-in session; the change replaces it with the returned one.
	SessionToken    string
	CurrentPassword string
	Username        string
	// Empty keeps the current password.
	NewPassword string
	// Source IP shares the login throttle bucket.
	Source netip.Addr
}

// ChangeCredentials re-verifies the current password under the login throttle, then replaces both
// credentials and returns the only remaining session; invalid new values fail before any hashing.
func (a *Auth) ChangeCredentials(ctx context.Context, req CredentialChange) (Session, error) {
	if err := domain.ValidateUsername(req.Username); err != nil {
		return Session{}, err
	}
	if req.NewPassword != "" {
		if err := domain.ValidatePassword(req.NewPassword); err != nil {
			return Session{}, err
		}
	}
	source := throttleKey(req.Source)
	if retryAfter, ok := a.throttle.reserve(source, a.now()); !ok {
		return Session{}, &ThrottledError{RetryAfter: retryAfter}
	}
	session, counted, err := a.replaceCredentials(ctx, req)
	switch {
	case err == nil:
		a.throttle.succeed(source)
	case counted:
		a.throttle.fail(source, a.now())
	default:
		a.throttle.cancel(source)
	}
	return session, err
}

// replaceCredentials reports counted when the error is a wrong current password.
func (a *Auth) replaceCredentials(ctx context.Context, req CredentialChange) (Session, bool, error) {
	account, err := a.store.Account(ctx)
	if err != nil {
		return Session{}, false, err
	}
	if err := a.acquireVerifySlot(ctx, a.verifyWait); err != nil {
		return Session{}, false, err
	}
	hash, err := a.verifyAndHash(req, account.PasswordHash)
	a.releaseVerifySlot()
	if errors.Is(err, domain.ErrInvalidCredentials) {
		return Session{}, true, err
	}
	if err != nil {
		return Session{}, false, err
	}

	token, err := a.newSessionToken()
	if err != nil {
		return Session{}, false, err
	}
	now := a.now()
	if err := a.store.ReplaceCredentials(ctx, req.SessionToken, req.Username, hash, token, now); err != nil {
		return Session{}, false, err
	}
	return Session{Token: token, ExpiresAt: sessionExpiry(now)}, false, nil
}

// verifyAndHash returns the hash to store: the current one unless a new password is given.
func (a *Auth) verifyAndHash(req CredentialChange, currentHash string) (string, error) {
	matched, err := a.hasher.Verify(req.CurrentPassword, currentHash)
	if err != nil {
		return "", fmt.Errorf("cannot verify the password: %w", err)
	}
	if !matched {
		return "", domain.ErrInvalidCredentials
	}
	if req.NewPassword == "" {
		return currentHash, nil
	}
	hash, err := a.hasher.Hash(req.NewPassword)
	if err != nil {
		return "", fmt.Errorf("cannot hash the password: %w", err)
	}
	return hash, nil
}

// CheckSession rejects malformed tokens without storage access; storage errors must not masquerade
// as invalid sessions.
func (a *Auth) CheckSession(ctx context.Context, token string) (time.Time, bool, error) {
	if !validTokenFormat(token) {
		return time.Time{}, false, nil
	}
	return a.store.Session(ctx, token, a.now())
}

// Logout ignores malformed or absent tokens.
func (a *Auth) Logout(ctx context.Context, token string) error {
	if !validTokenFormat(token) {
		return nil
	}
	return a.store.DeleteSession(ctx, token)
}

// Before setup, State returns AuthStateSetupRequired regardless of token.
func (a *Auth) State(ctx context.Context, token string) (AuthState, error) {
	if _, err := a.store.Account(ctx); errors.Is(err, domain.ErrAccountNotConfigured) {
		return AuthStateSetupRequired, nil
	} else if err != nil {
		return "", err
	}
	_, valid, err := a.CheckSession(ctx, token)
	if err != nil {
		return "", err
	}
	if valid {
		return AuthStateOwner, nil
	}
	return AuthStateGuest, nil
}

// A positive wait returns ThrottledError on expiry; zero waits until context cancellation.
func (a *Auth) acquireVerifySlot(ctx context.Context, wait time.Duration) error {
	select {
	case a.verifySlots <- struct{}{}:
		return nil
	default:
	}
	var timeout <-chan time.Time
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case a.verifySlots <- struct{}{}:
		return nil
	case <-timeout:
		return &ThrottledError{RetryAfter: wait}
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Auth) releaseVerifySlot() { <-a.verifySlots }

func (a *Auth) newSessionToken() (string, error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := io.ReadFull(a.random, raw); err != nil {
		return "", fmt.Errorf("cannot create a session ID: %w", err)
	}
	return sessionTokenEncoding.EncodeToString(raw), nil
}

var sessionTokenEncoding = base64.RawURLEncoding

func validTokenFormat(token string) bool {
	if len(token) != sessionTokenEncoding.EncodedLen(sessionTokenBytes) {
		return false
	}
	raw, err := sessionTokenEncoding.Strict().DecodeString(token)
	return err == nil && len(raw) == sessionTokenBytes
}

func sessionExpiry(now time.Time) time.Time {
	return time.Unix(now.Add(domain.SessionLifetime).Unix(), 0)
}

// Compare SHA-256 digests in constant time without normalization or case folding to avoid revealing
// username length or matching prefixes.
func equalUsername(given, stored string) bool {
	g := sha256.Sum256([]byte(given))
	s := sha256.Sum256([]byte(stored))
	return subtle.ConstantTimeCompare(g[:], s[:]) == 1
}
