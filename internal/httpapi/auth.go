package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const (
	sessionCookieHTTPS    = "__Host-vv_session"
	sessionCookieHTTP     = "vv_session"
	audienceHeader        = "X-VV-Audience"
	maxAuthBodyBytes      = 8 << 10
	defaultSessionRecheck = 30 * time.Second
)

type AuthState string

const (
	AuthStateOwner         AuthState = "owner"
	AuthStateGuest         AuthState = "guest"
	AuthStateSetupRequired AuthState = "setupRequired"
)

type IssuedSession struct {
	Token     string
	ExpiresAt time.Time
}

type LoginAttempt struct {
	Username string
	Password string
	// Source is zero when the remote IP cannot be parsed.
	Source       netip.Addr
	CurrentToken string
}

// Authenticator returns domain errors; throttling matches ErrLoginThrottled and exposes RetryAfterSeconds() int.
type Authenticator interface {
	Setup(ctx context.Context, username, password string) (IssuedSession, error)
	Login(ctx context.Context, attempt LoginAttempt) (IssuedSession, error)
	// CheckSession returns lookup errors separately from invalid sessions.
	CheckSession(ctx context.Context, token string) (time.Time, bool, error)
	Logout(ctx context.Context, token string) error
	State(ctx context.Context, token string) (AuthState, error)
	Username(ctx context.Context) (string, error)
	// ChangeCredentials matches ErrInvalidCredentials for a wrong current password and
	// ErrSessionNotValid when the session was revoked meanwhile.
	ChangeCredentials(ctx context.Context, change CredentialChange) (IssuedSession, error)
}

type CredentialChange struct {
	SessionToken    string
	CurrentPassword string
	Username        string
	// Empty keeps the current password.
	NewPassword string
	Source      netip.Addr
}

// Unknown routes default to owner-only; bearer routes ignore cookies, and all other routes ignore Authorization.
type access int

const (
	accessOwner access = iota
	accessGuest
	accessPublic
	accessBearer
)

func (a access) String() string {
	switch a {
	case accessPublic:
		return "public"
	case accessGuest:
		return "guest"
	case accessBearer:
		return "bearer"
	default:
		return "owner"
	}
}

// api/external-v1.yaml servers is authoritative; openapi_routes_test.go checks alignment.
const externalAPIBase = "/api/v1"

const mcpPath = "/mcp"

// Undefined /api/v1 routes still require Bearer authentication before returning JSON 404.
func bearerPath(cleaned string) bool {
	return cleaned == externalAPIBase || strings.HasPrefix(cleaned, externalAPIBase+"/") || cleaned == mcpPath
}

// Routes omitted from accessRoutes are owner-only; API security definitions are authoritative and checked by openapi_routes_test.go.
var accessRoutes = map[string]access{
	"GET /api/health":                             accessPublic,
	"GET /api/auth/session":                       accessPublic,
	"POST /api/auth/setup":                        accessPublic,
	"POST /api/auth/login":                        accessPublic,
	"POST /api/auth/logout":                       accessPublic,
	"GET /api/videos":                             accessGuest,
	"GET /api/videos/{id}":                        accessGuest,
	"GET /api/videos/{id}/related":                accessGuest,
	"GET /api/videos/{id}/group-members":          accessGuest,
	"GET /api/videos/{id}/versions":               accessGuest,
	"GET /api/videos/{id}/stream":                 accessGuest,
	"GET /api/videos/{id}/preview":                accessGuest,
	"GET /api/videos/{id}/transcode.mp4":          accessGuest,
	"GET /api/videos/{id}/transcode-start":        accessGuest,
	"GET /api/videos/{id}/subtitles":              accessGuest,
	"GET /api/videos/{id}/subtitles/{file}":       accessGuest,
	"GET /api/videos/{id}/thumbnail":              accessGuest,
	"GET /api/videos/{id}/seek-thumbnail":         accessGuest,
	"GET /api/videos/{id}/seek-thumbnail/{sheet}": accessGuest,
	"GET /api/folders":                            accessGuest,
	"GET /api/folders/{rootId}":                   accessGuest,
	"GET /api/folders/{rootId}/videos":            accessGuest,
	"GET /api/library":                            accessGuest,
	"GET /api/groups":                             accessGuest,
	"GET /api/groups/{id}":                        accessGuest,
	"GET /api/groups/{id}/items":                  accessGuest,
	"GET /api/smart-folders":                      accessGuest,
	"GET /api/smart-folders/cards":                accessGuest,
	"GET /api/smart-folders/{id}":                 accessGuest,
	"GET /api/smart-folders/{id}/items":           accessGuest,
	"GET /api/events":                             accessGuest,
	"GET /api/peer":                               accessGuest,
	"POST /api/peer/pairings":                     accessPublic,
	"GET /api/peer/handshake":                     accessPublic,
	"POST /api/peer/related":                      accessGuest,
	"GET /api/peer/videos/{id}/clues":             accessGuest,
}

// Use ServeMux for access matching so wildcard and GET/HEAD semantics match actual routing.
var accessMux = func() *http.ServeMux {
	mux := http.NewServeMux()
	for pattern := range accessRoutes {
		mux.Handle(pattern, http.NotFoundHandler())
	}
	return mux
}()

// classifyRequest fails closed when decoded path segments differ from ServeMux's escaped-segment interpretation; ambiguous API paths remain owner-only and never become Bearer routes.
func classifyRequest(r *http.Request) (access, bool) {
	cleaned := path.Clean("/" + r.URL.Path)
	segmentsAgree := escapedSegmentsMatch(r.URL, cleaned)
	if segmentsAgree && bearerPath(cleaned) {
		return accessBearer, cleaned != mcpPath
	}
	// Clean removes the trailing slash from /api/.
	if cleaned != "/api" && !strings.HasPrefix(cleaned, "/api/") {
		if !segmentsAgree && escapedUnderAPI(r.URL) {
			// ServeMux may route to an API operation even when the fully decoded path leaves /api.
			return accessOwner, true
		}
		// SPA assets contain no user data and are public only for GET/HEAD.
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			return accessPublic, false
		}
		return accessOwner, false
	}
	if !segmentsAgree {
		return accessOwner, true
	}
	probe := &http.Request{Method: r.Method, URL: &url.URL{Path: cleaned}, Host: r.Host}
	if _, pattern := accessMux.Handler(probe); pattern != "" {
		if class, ok := accessRoutes[pattern]; ok {
			return class, true
		}
	}
	return accessOwner, true
}

// Reject escaped segments that decode to separators, dot segments, or invalid escapes.
func escapedSegmentsMatch(u *url.URL, cleaned string) bool {
	escaped := path.Clean("/" + u.EscapedPath())
	segments := strings.Split(strings.TrimPrefix(escaped, "/"), "/")
	for i, segment := range segments {
		decoded, err := url.PathUnescape(segment)
		if err != nil || strings.Contains(decoded, "/") || decoded == "." || decoded == ".." {
			return false
		}
		segments[i] = decoded
	}
	return "/"+strings.Join(segments, "/") == cleaned
}

// Undecodable escaped prefixes fail closed as API routes.
func escapedUnderAPI(u *url.URL) bool {
	escaped := path.Clean("/" + u.EscapedPath())
	first, _, _ := strings.Cut(strings.TrimPrefix(escaped, "/"), "/")
	decoded, err := url.PathUnescape(first)
	return err != nil || decoded == "api"
}

type audienceKey struct{}

func withAudience(ctx context.Context, audience domain.Audience) context.Context {
	return context.WithValue(ctx, audienceKey{}, audience)
}

// Missing audience context defaults to guest.
func audienceFrom(ctx context.Context) domain.Audience {
	audience, _ := ctx.Value(audienceKey{}).(domain.Audience)
	return audience
}

// clientOrigin selects exactly one cookie name: __Host-vv_session for HTTPS, vv_session for HTTP.
func sessionCookieName(https bool) string {
	if https {
		return sessionCookieHTTPS
	}
	return sessionCookieHTTP
}

func (s *server) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName(s.clientOrigin(r).https))
	if err != nil {
		return ""
	}
	return cookie.Value
}

// authBoundary sets X-VV-Audience for API responses; guest-capable routes still reject unauthenticated requests before account setup.
func (s *server) authBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		class, api := classifyRequest(r)
		if !api && class == accessPublic {
			next.ServeHTTP(w, r)
			return
		}
		setAudience := func(audience domain.Audience) {
			if api {
				w.Header().Set(audienceHeader, audience.String())
			}
		}
		if class == accessBearer {
			s.bearerBoundary(w, r, next, setAudience)
			return
		}
		// Public routes ignore the peer credential; the handshake checks its secret itself.
		if class != accessPublic {
			if secret, present := peerSecret(r); present {
				s.peerBoundary(w, r, next, class, secret, setAudience)
				return
			}
		}

		if s.auth == nil {
			setAudience(domain.AudienceGuest)
			if class == accessPublic {
				next.ServeHTTP(w, r.WithContext(withAudience(r.Context(), domain.AudienceGuest)))
				return
			}
			s.internalError(w, "Cannot verify authentication.", errors.New("authentication is not configured"))
			return
		}

		token := s.sessionToken(r)
		// Register requests before session validation so logout cannot fall between validation and cancellation registration.
		ctx := r.Context()
		var tracked *trackedRequest
		if class != accessPublic {
			var release func()
			ctx, tracked, release = s.sessions.open(ctx, w, token)
			defer release()
		}
		expiresAt, valid, err := s.auth.CheckSession(ctx, token)
		if tracked != nil && tracked.revoked() {
			// Logout during validation overrides its result.
			valid, err = false, nil
		}
		if err != nil {
			setAudience(domain.AudienceGuest)
			if class == accessPublic {
				// Public routes continue as guests after auth lookup failure so health checks remain available.
				next.ServeHTTP(w, r.WithContext(withAudience(r.Context(), domain.AudienceGuest)))
				return
			}
			s.internalError(w, "Could not check the sign-in status.", err)
			return
		}

		if valid && tracked != nil {
			// Enable write cancellation only after validation; a preceding logout makes the request invalid.
			var served bool
			ctx, served = s.sessions.serve(ctx, tracked, expiresAt)
			if !served {
				valid = false
			}
		}

		audience := domain.AudienceGuest
		if valid {
			audience = domain.AudienceOwner
		} else {
			// Guest requests must not inherit the revoked session's canceled context or their public-data reads would fail.
			ctx = r.Context()
		}
		setAudience(audience)
		switch {
		case valid || class == accessPublic:
		case class == accessGuest:
			// Before account setup, public flags do not grant guest access.
			configured, err := s.accountConfigured(r.Context())
			if err != nil {
				s.internalError(w, "Could not check the sign-in status.", err)
				return
			}
			if !configured {
				s.unauthenticated(w)
				return
			}
		default:
			s.unauthenticated(w)
			return
		}

		next.ServeHTTP(w, r.WithContext(withAudience(ctx, audience)))
	})
}

func (s *server) accountConfigured(ctx context.Context) (bool, error) {
	state, err := s.auth.State(ctx, "")
	if err != nil {
		return false, err
	}
	return state != AuthStateSetupRequired, nil
}

// Bearer authentication ignores cookies and returns uniform 401 challenges; validate, register by token ID, then revalidate to close the revocation race.
func (s *server) bearerBoundary(
	w http.ResponseWriter, r *http.Request, next http.Handler, setAudience func(domain.Audience),
) {
	setAudience(domain.AudienceGuest)
	if s.apiTokens == nil {
		s.internalError(w, "Cannot verify the API token.", errors.New("API tokens are not configured"))
		return
	}
	secret, ok := bearerSecret(r)
	if !ok {
		s.bearerUnauthenticated(w)
		return
	}
	token, valid, err := s.apiTokens.CheckAPIToken(r.Context(), secret)
	if err != nil {
		s.internalError(w, "Could not check the API token.", err)
		return
	}
	if !valid {
		s.bearerUnauthenticated(w)
		return
	}

	check := func(ctx context.Context) (bool, error) {
		again, valid, err := s.apiTokens.CheckAPIToken(ctx, secret)
		return valid && again.ID == token.ID, err
	}
	ctx, tracked, release := s.sessions.openWith(r.Context(), w, apiTokenLedgerKey(token.ID), check)
	defer release()
	valid, err = check(ctx)
	if tracked.revoked() {
		valid, err = false, nil
	}
	if err != nil {
		s.internalError(w, "Could not check the API token.", err)
		return
	}
	if valid {
		// Tokens have no expiry; only revocation and revalidation cancel them.
		ctx, valid = s.sessions.serve(ctx, tracked, time.Time{})
	}
	if !valid {
		s.bearerUnauthenticated(w)
		return
	}

	// Last-use recording is throttled to sixty seconds and never blocks serving on failure.
	if err := s.apiTokens.RecordAPITokenUse(ctx, token.ID); err != nil {
		s.logger.Warn("could not record the API token use", slog.Any("error", err))
	}
	setAudience(domain.AudienceOwner)
	next.ServeHTTP(w, r.WithContext(withAudience(ctx, domain.AudienceOwner)))
}

// bearerSecret accepts one case-insensitive Bearer header with a nonempty whitespace-free token (RFC 9110 §11.1).
func bearerSecret(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	scheme, secret, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || secret == "" ||
		strings.ContainsAny(secret, " \t") {
		return "", false
	}
	return secret, true
}

// The colon namespace cannot collide with base64url session IDs.
func apiTokenLedgerKey(id int64) string {
	return "api-token:" + strconv.FormatInt(id, 10)
}

func (s *server) bearerUnauthenticated(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	s.writeError(w, http.StatusUnauthorized, gen.ErrorCodeUnauthenticated, "A valid API token is required.")
}

// Cookie authentication failures are uniform and omit WWW-Authenticate.
func (s *server) unauthenticated(w http.ResponseWriter) {
	s.writeError(w, http.StatusUnauthorized, gen.ErrorCodeUnauthenticated, "Sign-in required.")
}

var invalidUsernameMessage = "Usernames must be 1 to " + strconv.Itoa(domain.MaxUsernameLength) +
	" characters, with no control characters or leading or trailing spaces."

func (s *server) SetupAccount(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.internalError(w, "Cannot run the initial setup.", errors.New("authentication is not configured"))
		return
	}
	var body gen.SetupRequest
	if !s.readAuthBody(w, r, &body) {
		return
	}
	session, err := s.auth.Setup(r.Context(), body.Username, body.Password)
	switch {
	case err == nil:
	case errors.Is(err, domain.ErrAccountAlreadyConfigured):
		s.writeError(w, http.StatusConflict, gen.ErrorCodeAccountAlreadyConfigured, "The account is already set up.")
		return
	case s.invalidCredentialValue(w, err):
		return
	default:
		s.internalError(w, "Could not complete the initial setup.", err)
		return
	}
	s.logAuthEvent(r, "setup")
	s.setSessionCookie(w, r, session)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.AuthRedirect{RedirectTo: domain.DefaultRedirectTarget}, s.logger)
}

func (s *server) Login(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.internalError(w, "Cannot sign in.", errors.New("authentication is not configured"))
		return
	}
	var body gen.LoginRequest
	if !s.readAuthBody(w, r, &body) {
		return
	}
	current := s.sessionToken(r)
	session, err := s.auth.Login(r.Context(), LoginAttempt{
		Username:     body.Username,
		Password:     body.Password,
		Source:       s.clientOrigin(r).source,
		CurrentToken: current,
	})
	switch {
	case err == nil:
	case errors.Is(err, domain.ErrInvalidCredentials):
		s.logAuthEvent(r, "login_failed")
		s.writeError(w, http.StatusUnauthorized, gen.ErrorCodeInvalidCredentials, "Incorrect username or password.")
		return
	case errors.Is(err, domain.ErrLoginThrottled):
		s.loginThrottled(w, r, err, "login_throttled")
		return
	default:
		s.internalError(w, "Could not sign in.", err)
		return
	}
	// Login deleted the previous session; cancel its active requests too.
	if current != "" {
		s.sessions.revoke(current)
	}
	s.logAuthEvent(r, "login")
	s.setSessionCookie(w, r, session)
	next := ""
	if body.Next != nil {
		next = *body.Next
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.AuthRedirect{RedirectTo: domain.SafeRedirectTarget(next)}, s.logger)
}

// invalidCredentialValue writes the 400 for a rejected new username or password and reports whether it did.
func (s *server) invalidCredentialValue(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, domain.ErrUsernameLength):
		s.invalidRequestLimit(w, reasonUsernameLength, domain.MaxUsernameLength, invalidUsernameMessage)
	case errors.Is(err, domain.ErrInvalidUsername):
		// Only length violations receive username_length; controls/whitespace have no corresponding reason.
		s.invalidRequest(w, invalidUsernameMessage)
	case errors.Is(err, domain.ErrInvalidPassword):
		s.invalidRequestLimit(w, reasonPasswordLength, domain.MaxPasswordBytes,
			"Passwords must be 1 to "+strconv.Itoa(domain.MaxPasswordBytes)+" bytes.")
	default:
		return false
	}
	return true
}

func (s *server) loginThrottled(w http.ResponseWriter, r *http.Request, err error, event string) {
	s.logAuthEvent(r, event)
	retryAfter := 1
	var throttled interface{ RetryAfterSeconds() int }
	if errors.As(err, &throttled) {
		retryAfter = max(throttled.RetryAfterSeconds(), 1)
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	s.writeError(w, http.StatusTooManyRequests, gen.ErrorCodeLoginThrottled,
		"Too many sign-in attempts. Wait a moment and try again.")
}

func (s *server) GetAccount(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.internalError(w, "Cannot load the account.", errors.New("authentication is not configured"))
		return
	}
	username, err := s.auth.Username(r.Context())
	if err != nil {
		s.internalError(w, "Could not load the account.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.Account{Username: username}, s.logger)
}

// UpdateAccount keeps this request's tracked session alive; the store already deleted it, and other
// sessions' in-flight requests stop at their next recheck.
func (s *server) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.internalError(w, "Cannot change the account.", errors.New("authentication is not configured"))
		return
	}
	var body gen.UpdateAccountRequest
	if !s.readAuthBody(w, r, &body) {
		return
	}
	change := CredentialChange{
		SessionToken:    s.sessionToken(r),
		CurrentPassword: body.CurrentPassword,
		Username:        body.Username,
		Source:          s.clientOrigin(r).source,
	}
	if body.NewPassword != nil {
		change.NewPassword = *body.NewPassword
		if change.NewPassword == "" {
			// An explicit empty password is invalid rather than a request to keep the current one.
			s.invalidCredentialValue(w, domain.ErrInvalidPassword)
			return
		}
	}
	session, err := s.auth.ChangeCredentials(r.Context(), change)
	switch {
	case err == nil:
	case s.invalidCredentialValue(w, err):
		return
	case errors.Is(err, domain.ErrInvalidCredentials):
		s.logAuthEvent(r, "credentials_change_failed")
		s.writeError(w, http.StatusForbidden, gen.ErrorCodeInvalidCredentials, "The current password is incorrect.")
		return
	case errors.Is(err, domain.ErrLoginThrottled):
		s.loginThrottled(w, r, err, "credentials_change_throttled")
		return
	case errors.Is(err, domain.ErrSessionNotValid):
		s.unauthenticated(w)
		return
	default:
		s.internalError(w, "Could not change the account.", err)
		return
	}
	s.logAuthEvent(r, "credentials_changed")
	s.setSessionCookie(w, r, session)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.Account{Username: body.Username}, s.logger)
}

func (s *server) GetAuthSession(w http.ResponseWriter, r *http.Request, params gen.GetAuthSessionParams) {
	if s.auth == nil {
		s.internalError(w, "Cannot check the sign-in status.", errors.New("authentication is not configured"))
		return
	}
	state, err := s.auth.State(r.Context(), s.sessionToken(r))
	if err != nil {
		s.internalError(w, "Could not check the sign-in status.", err)
		return
	}
	// Recompute X-VV-Audience from the same state as the body to handle logout between boundary and handler checks.
	audience := domain.AudienceGuest
	if state == AuthStateOwner {
		audience = domain.AudienceOwner
	}
	w.Header().Set(audienceHeader, audience.String())
	body := gen.AuthSession{State: gen.AuthSessionState(state)}
	if state == AuthStateOwner && params.Next != nil {
		redirect := domain.SafeRedirectTarget(*params.Next)
		body.RedirectTo = &redirect
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, s.logger)
}

// Logout revokes both cookie names and their active requests, unlike authentication's scheme-specific lookup.
func (s *server) Logout(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.internalError(w, "Cannot sign out.", errors.New("authentication is not configured"))
		return
	}
	for _, cookie := range r.Cookies() {
		if cookie.Name != sessionCookieHTTPS && cookie.Name != sessionCookieHTTP {
			continue
		}
		if err := s.auth.Logout(r.Context(), cookie.Value); err != nil {
			s.internalError(w, "Could not sign out.", err)
			return
		}
		s.sessions.revoke(cookie.Value)
	}
	s.logAuthEvent(r, "logout")
	// Deleting __Host- cookies requires Secure; delete HTTP cookies without it.
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieHTTPS, Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieHTTP, Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) readAuthBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	return s.readJSONBody(w, r, target)
}

func (s *server) setSessionCookie(w http.ResponseWriter, r *http.Request, session IssuedSession) {
	https := s.clientOrigin(r).https
	maxAge := int(session.ExpiresAt.Sub(s.now()) / time.Second)
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName(https),
		Value:    session.Token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   https,
		SameSite: http.SameSiteStrictMode,
	})
}

// Log only event type and source, never usernames, passwords, session IDs, or cookies.
func (s *server) logAuthEvent(r *http.Request, event string) {
	source := "unknown"
	if addr := s.clientOrigin(r).source; addr.IsValid() {
		source = addr.String()
	}
	s.logger.LogAttrs(r.Context(), slog.LevelInfo, "auth event",
		slog.String("event", event), slog.String("source", source))
}

// sessionLedger cancels requests on logout and periodically revalidates long requests to detect credential changes in other processes.
type sessionLedger struct {
	mu       sync.Mutex
	requests map[string]map[*trackedRequest]struct{}
	recheck  time.Duration
	// Nil check disables periodic revalidation.
	check  func(ctx context.Context, token string) (bool, error)
	logger *slog.Logger
}

type trackedRequest struct {
	token string
	// Nil check uses the ledger's session validator.
	check      func(ctx context.Context) (bool, error)
	cancel     context.CancelFunc
	controller *http.ResponseController

	mu sync.Mutex
	// Before serving, revocation cancels context only, preserving the boundary's ability to write an authentication error.
	serving bool
	// Never alter deadlines after handler return because the connection may serve another request.
	finished  bool
	aborted   bool
	timer     *time.Timer
	stopWatch func() bool
}

// abort also expires the write deadline because http.ServeContent does not observe context cancellation.
func (t *trackedRequest) abort() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished || t.aborted {
		return
	}
	t.aborted = true
	t.cancel()
	if t.serving {
		_ = t.controller.SetWriteDeadline(time.Now())
	}
}

func (t *trackedRequest) revoked() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.aborted
}

func (t *trackedRequest) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finished = true
	if t.timer != nil {
		t.timer.Stop()
	}
	if t.stopWatch != nil {
		t.stopWatch()
	}
	t.cancel()
}

func newSessionLedger(recheck time.Duration, logger *slog.Logger) *sessionLedger {
	if recheck <= 0 {
		recheck = defaultSessionRecheck
	}
	return &sessionLedger{requests: map[string]map[*trackedRequest]struct{}{}, recheck: recheck, logger: logger}
}

// open registers before validation to avoid missed revocation; call release before the handler returns.
func (l *sessionLedger) open(
	ctx context.Context, w http.ResponseWriter, token string,
) (context.Context, *trackedRequest, func()) {
	return l.openWith(ctx, w, token, nil)
}

// openWith permits token-ID keys and token validation; nil check falls back to session validation.
func (l *sessionLedger) openWith(
	ctx context.Context, w http.ResponseWriter, key string, check func(ctx context.Context) (bool, error),
) (context.Context, *trackedRequest, func()) {
	token := key
	ctx, cancel := context.WithCancel(ctx)
	req := &trackedRequest{token: token, check: check, cancel: cancel, controller: http.NewResponseController(w)}

	l.mu.Lock()
	if l.requests[token] == nil {
		l.requests[token] = map[*trackedRequest]struct{}{}
	}
	l.requests[token][req] = struct{}{}
	l.mu.Unlock()

	release := func() {
		req.finish()
		l.mu.Lock()
		delete(l.requests[token], req)
		if len(l.requests[token]) == 0 {
			delete(l.requests, token)
		}
		l.mu.Unlock()
	}
	return ctx, req, release
}

// serve rejects already-revoked requests, enforces expiry on context and writes, and revalidates only requests longer than the interval; zero expiry is unbounded for API tokens.
func (l *sessionLedger) serve(
	ctx context.Context, req *trackedRequest, expiresAt time.Time,
) (context.Context, bool) {
	req.mu.Lock()
	defer req.mu.Unlock()
	if req.aborted {
		return ctx, false
	}
	req.serving = true
	var cancel context.CancelFunc
	if expiresAt.IsZero() {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithDeadline(ctx, expiresAt)
	}
	parentCancel := req.cancel
	req.cancel = func() { cancel(); parentCancel() }
	req.stopWatch = context.AfterFunc(ctx, req.abort)

	check := req.check
	if check == nil && l.check != nil {
		token := req.token
		check = func(ctx context.Context) (bool, error) { return l.check(ctx, token) }
	}
	if check != nil {
		checkCtx := context.WithoutCancel(ctx)
		recheck := func() {
			valid, err := check(checkCtx)
			if err != nil {
				// Transient validation failures defer to the next interval without canceling the request.
				l.logger.Warn("could not recheck the session of an in-flight request", slog.Any("error", err))
			} else if !valid {
				req.abort()
				return
			}
			req.mu.Lock()
			defer req.mu.Unlock()
			if !req.finished && !req.aborted {
				req.timer.Reset(l.recheck)
			}
		}
		req.timer = time.AfterFunc(l.recheck, recheck)
	}
	return ctx, true
}

func (l *sessionLedger) revoke(token string) {
	l.revokeExcept(token, nil)
}

// revokeExcept leaves keep running, for a request that revokes its own credential and must still answer.
func (l *sessionLedger) revokeExcept(token string, keep *trackedRequest) {
	l.mu.Lock()
	requests := make([]*trackedRequest, 0, len(l.requests[token]))
	for req := range l.requests[token] {
		if req != keep {
			requests = append(requests, req)
		}
	}
	l.mu.Unlock()
	for _, req := range requests {
		req.abort()
	}
}
