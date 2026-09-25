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

// 認証の HTTP 境界（specs/016-single-account-auth/contracts/auth-api.md、
// plan.md Structural Decisions 1・5・8・14）。

const (
	// sessionCookieHTTPS は HTTPS の要求で使うセッション Cookie の名前である。
	sessionCookieHTTPS = "__Host-vv_session"
	// sessionCookieHTTP は HTTP の要求で使うセッション Cookie の名前である。
	sessionCookieHTTP = "vv_session"
	// audienceHeader は /api/* の応答に、処理した見る人を載せるヘッダーである。
	audienceHeader = "X-VV-Audience"
	// maxAuthBodyBytes は初回設定とログインの本文の上限である。
	maxAuthBodyBytes = 8 << 10
	// defaultSessionRecheck は、長く続く所有者の要求のセッションを確かめ直す間隔である。
	defaultSessionRecheck = 30 * time.Second
)

// AuthState は見る人の状態である（contracts/auth-api.md §4）。
type AuthState string

// 見る人の状態。
const (
	AuthStateOwner         AuthState = "owner"
	AuthStateGuest         AuthState = "guest"
	AuthStateSetupRequired AuthState = "setupRequired"
)

// IssuedSession は初回設定とログインで発行したセッションである。Token は Cookie に
// 入れる値で、ExpiresAt はその期限である。
type IssuedSession struct {
	Token     string
	ExpiresAt time.Time
}

// LoginAttempt はログインの要求である。
type LoginAttempt struct {
	Username string
	Password string
	// Source は送信元の IP アドレスである。解釈できなければゼロ値。
	Source netip.Addr
	// CurrentToken は要求に付いていたセッション Cookie の値である（無ければ空）。
	CurrentToken string
}

// Authenticator は初回設定・ログイン・ログアウト・セッションの確認を行う。
// internal/app の *Auth を cmd/mdm が包んで渡す。
//
// 誤りは domain の値で返す。初回設定は domain.ErrAccountAlreadyConfigured・
// ErrInvalidUsername・ErrInvalidPassword、ログインは domain.ErrInvalidCredentials と、
// 試行の制限（errors.Is で domain.ErrLoginThrottled と一致し、RetryAfterSeconds() int を
// 持つ誤り）である。
type Authenticator interface {
	Setup(ctx context.Context, username, password string) (IssuedSession, error)
	Login(ctx context.Context, attempt LoginAttempt) (IssuedSession, error)
	// CheckSession は token のセッションが有効かを確かめ、有効なら期限を返す。
	// 問い合わせが失敗したら、無効とせずに誤りを返す。
	CheckSession(ctx context.Context, token string) (time.Time, bool, error)
	Logout(ctx context.Context, token string) error
	State(ctx context.Context, token string) (AuthState, error)
}

// access は要求の扱いである（contracts/auth-api.md §1）。ゼロ値は「所有者だけ」で、
// 分類に挙がらない要求はここに倒れる。
type access int

const (
	accessOwner access = iota
	accessGuest
	accessPublic
)

func (a access) String() string {
	switch a {
	case accessPublic:
		return "public"
	case accessGuest:
		return "guest"
	default:
		return "owner"
	}
}

// accessRoutes は「誰でも」と「ゲストも」の API 経路である。ここに無い /api/* は
// すべて「所有者だけ」である。正本は api/openapi.yaml の各操作の security で、
// 一致は openapi_routes_test.go が確かめる。
//
// GET /api/videos/ids は GET /api/videos/{id} に取られないよう、「所有者だけ」として
// 明示する（ServeMux は字面の段を優先する）。
var accessRoutes = map[string]access{
	"GET /api/health":                     accessPublic,
	"GET /api/auth/session":               accessPublic,
	"POST /api/auth/setup":                accessPublic,
	"POST /api/auth/login":                accessPublic,
	"POST /api/auth/logout":               accessPublic,
	"GET /api/videos/ids":                 accessOwner,
	"GET /api/videos":                     accessGuest,
	"GET /api/videos/{id}":                accessGuest,
	"GET /api/videos/{id}/related":        accessGuest,
	"GET /api/videos/{id}/stream":         accessGuest,
	"GET /api/videos/{id}/preview":        accessGuest,
	"GET /api/videos/{id}/transcode.mp4":  accessGuest,
	"GET /api/videos/{id}/thumbnail":      accessGuest,
	"GET /api/videos/{id}/seek-thumbnail": accessGuest,
	"GET /api/folders":                    accessGuest,
	"GET /api/folders/{rootId}":           accessGuest,
	"GET /api/folders/{rootId}/videos":    accessGuest,
}

// accessMux は accessRoutes の模様を引き当てるためだけの ServeMux である。
// {id} の解釈と GET に HEAD を含める規則を、実際の経路と同じ ServeMux に任せる。
var accessMux = func() *http.ServeMux {
	mux := http.NewServeMux()
	for pattern := range accessRoutes {
		mux.Handle(pattern, http.NotFoundHandler())
	}
	return mux
}()

// classifyRequest は要求の扱いと、それが /api/ 以下かを返す。判定は path.Clean した
// 復号済みの経路で行うので、//api/…・/./api/…・/%61pi/…・/api/../api/… も /api/ 以下になる。
func classifyRequest(r *http.Request) (access, bool) {
	cleaned := path.Clean("/" + r.URL.Path)
	// Clean は末尾の / を落とすので、/api/ そのものは /api になる。
	if cleaned != "/api" && !strings.HasPrefix(cleaned, "/api/") {
		// SPA のビルド成果物は利用者データを含まないので、GET・HEAD だけ誰にでも配る。
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			return accessPublic, false
		}
		return accessOwner, false
	}
	probe := &http.Request{Method: r.Method, URL: &url.URL{Path: cleaned}, Host: r.Host}
	if _, pattern := accessMux.Handler(probe); pattern != "" {
		if class, ok := accessRoutes[pattern]; ok {
			return class, true
		}
	}
	return accessOwner, true
}

type audienceKey struct{}

// withAudience は見る人を要求の context に載せる。
func withAudience(ctx context.Context, audience domain.Audience) context.Context {
	return context.WithValue(ctx, audienceKey{}, audience)
}

// audienceFrom は境界が決めた見る人を返す。載っていなければゲスト（狭い側）である。
func audienceFrom(ctx context.Context) domain.Audience {
	audience, _ := ctx.Value(audienceKey{}).(domain.Audience)
	return audience
}

// sessionCookieName は認証で読む Cookie の名前である。HTTPS では __Host-vv_session
// だけを、HTTP では vv_session だけを読む（contracts/auth-api.md §7）。HTTPS かどうかは
// clientOrigin で決める。
func sessionCookieName(https bool) string {
	if https {
		return sessionCookieHTTPS
	}
	return sessionCookieHTTP
}

// sessionToken は要求の認証に使う Cookie の値である。無ければ空を返す。
func (s *server) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName(s.clientOrigin(r).https))
	if err != nil {
		return ""
	}
	return cookie.Value
}

// authBoundary は要求を3つの扱いに振り分け、見る人を決めて context に載せる
// （Structural Decisions 1）。/api/* の応答には処理した見る人を X-VV-Audience で付ける。
//
// 「ゲストも」の要求は、有効なセッションが無ければゲストとして処理する
// （contracts/guest-api.md）。アカウントが未設定の間は未認証の応答を返す。
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

		if s.auth == nil {
			setAudience(domain.AudienceGuest)
			if class == accessPublic {
				next.ServeHTTP(w, r.WithContext(withAudience(r.Context(), domain.AudienceGuest)))
				return
			}
			s.internalError(w, "認証を確かめられません", errors.New("認証がつながっていません"))
			return
		}

		token := s.sessionToken(r)
		expiresAt, valid, err := s.auth.CheckSession(r.Context(), token)
		if err != nil {
			setAudience(domain.AudienceGuest)
			if class == accessPublic {
				// 誰でもの要求は見る人に依らないので、確かめられなくてもゲストとして続ける。
				// 稼働確認がこの失敗で 500 にならないようにする。
				next.ServeHTTP(w, r.WithContext(withAudience(r.Context(), domain.AudienceGuest)))
				return
			}
			// 所有者ともゲストともみなさない（contracts/auth-api.md §5）。
			s.internalError(w, "ログインの状態を確かめられませんでした", err)
			return
		}

		audience := domain.AudienceGuest
		if valid {
			audience = domain.AudienceOwner
		}
		setAudience(audience)
		switch {
		case valid || class == accessPublic:
		case class == accessGuest:
			// 「ゲストも」の要求はゲストとして処理する。ただしアカウントが未設定の間は
			// 公開フラグも効かせず、未認証にする（contracts/auth-api.md §1、親 Issue 要件 2）。
			configured, err := s.accountConfigured(r.Context())
			if err != nil {
				s.internalError(w, "ログインの状態を確かめられませんでした", err)
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

		ctx := withAudience(r.Context(), audience)
		if valid && class != accessPublic {
			var release func()
			ctx, release = s.sessions.track(ctx, w, token, expiresAt)
			defer release()
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// accountConfigured はアカウントが設定済みかを返す。セッションを持たない状態を
// 問い合わせ、未設定かどうかだけを読む。
func (s *server) accountConfigured(ctx context.Context) (bool, error) {
	state, err := s.auth.State(ctx, "")
	if err != nil {
		return false, err
	}
	return state != AuthStateSetupRequired, nil
}

// unauthenticated は未認証の応答である。原因は区別せず、WWW-Authenticate は付けない。
func (s *server) unauthenticated(w http.ResponseWriter) {
	s.writeError(w, http.StatusUnauthorized, gen.ErrorCodeUnauthenticated, "ログインが必要です")
}

// SetupAccount は初回設定である（contracts/auth-api.md §2）。
func (s *server) SetupAccount(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.internalError(w, "初回設定を行えません", errors.New("認証がつながっていません"))
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
		s.writeError(w, http.StatusConflict, gen.ErrorCodeAccountAlreadyConfigured, "アカウントは既に設定されています")
		return
	case errors.Is(err, domain.ErrInvalidUsername):
		s.invalidRequest(w, "ユーザー名は1〜"+strconv.Itoa(domain.MaxUsernameLength)+
			"文字で、制御文字を含まず、先頭と末尾に空白を置かないでください")
		return
	case errors.Is(err, domain.ErrInvalidPassword):
		s.invalidRequest(w, "パスワードは1〜"+strconv.Itoa(domain.MaxPasswordBytes)+"バイトにしてください")
		return
	default:
		s.internalError(w, "初回設定を行えませんでした", err)
		return
	}
	s.logAuthEvent(r, "setup")
	s.setSessionCookie(w, r, session)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.AuthRedirect{RedirectTo: domain.DefaultRedirectTarget}, s.logger)
}

// Login はログインである（contracts/auth-api.md §3）。
func (s *server) Login(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.internalError(w, "ログインできません", errors.New("認証がつながっていません"))
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
	var throttled interface{ RetryAfterSeconds() int }
	switch {
	case err == nil:
	case errors.Is(err, domain.ErrInvalidCredentials):
		s.logAuthEvent(r, "login_failed")
		s.writeError(w, http.StatusUnauthorized, gen.ErrorCodeInvalidCredentials, "ユーザー名またはパスワードが違います")
		return
	case errors.Is(err, domain.ErrLoginThrottled):
		s.logAuthEvent(r, "login_throttled")
		retryAfter := 1
		if errors.As(err, &throttled) {
			retryAfter = max(throttled.RetryAfterSeconds(), 1)
		}
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		s.writeError(w, http.StatusTooManyRequests, gen.ErrorCodeLoginThrottled,
			"ログインの試行が多すぎます。しばらく待ってから試してください")
		return
	default:
		s.internalError(w, "ログインできませんでした", err)
		return
	}
	// 付いていた古いセッションは Login が消した。それで処理中の要求も打ち切る。
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

// GetAuthSession は見る人の状態を返す（contracts/auth-api.md §4）。
func (s *server) GetAuthSession(w http.ResponseWriter, r *http.Request, params gen.GetAuthSessionParams) {
	if s.auth == nil {
		s.internalError(w, "ログインの状態を確かめられません", errors.New("認証がつながっていません"))
		return
	}
	state, err := s.auth.State(r.Context(), s.sessionToken(r))
	if err != nil {
		s.internalError(w, "ログインの状態を確かめられませんでした", err)
		return
	}
	// 境界の確認のあとに別のタブでログアウトされると、ここでの確認は境界と食い違う。
	// X-VV-Audience は本文と同じこの確認から付け直し、応答の中で矛盾させない。
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

// Logout はログアウトである（contracts/auth-api.md §4）。認証の読み分けと違い、届いた
// 両方の名前の Cookie を見て、それぞれのセッションを消し、処理中の要求を打ち切る。
func (s *server) Logout(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.internalError(w, "ログアウトできません", errors.New("認証がつながっていません"))
		return
	}
	for _, cookie := range r.Cookies() {
		if cookie.Name != sessionCookieHTTPS && cookie.Name != sessionCookieHTTP {
			continue
		}
		if err := s.auth.Logout(r.Context(), cookie.Value); err != nil {
			s.internalError(w, "ログアウトできませんでした", err)
			return
		}
		s.sessions.revoke(cookie.Value)
	}
	s.logAuthEvent(r, "logout")
	// __Host- の Cookie は Secure でなければ消せない。HTTP の名前は Secure を付けずに消す。
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

// readAuthBody は初回設定とログインの本文を 8 KiB まで読む。
func (s *server) readAuthBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	return s.readJSONBody(w, r, target)
}

// setSessionCookie はセッションの Cookie を付ける（contracts/auth-api.md §7）。
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

// logAuthEvent は認証の出来事を1行記録する（contracts/auth-api.md §9）。属性は出来事の
// 種類と送信元だけで、送られたユーザー名・パスワード・セッション ID・Cookie は出さない。
func (s *server) logAuthEvent(r *http.Request, event string) {
	source := "unknown"
	if addr := s.clientOrigin(r).source; addr.IsValid() {
		source = addr.String()
	}
	s.logger.LogAttrs(r.Context(), slog.LevelInfo, "認証の出来事",
		slog.String("event", event), slog.String("source", source))
}

// sessionLedger は所有者として処理中の要求を、セッションごとに覚える台帳である
// （Structural Decisions 5）。ログアウトは同じセッションの要求を打ち切り、長く続く
// 要求は一定間隔でセッションを確かめ直して、別のプロセスによる再設定を見つける。
type sessionLedger struct {
	mu       sync.Mutex
	requests map[string]map[*trackedRequest]struct{}
	// recheck は確かめ直す間隔で、要求がこれより長く続いたときから確かめ始める。
	recheck time.Duration
	// check はセッションが今も有効かを返す。nil なら確かめ直さない。
	check func(ctx context.Context, token string) (bool, error)
	// logger は確かめ直しの失敗を記録する。
	logger *slog.Logger
}

type trackedRequest struct {
	cancel     context.CancelFunc
	controller *http.ResponseController

	mu sync.Mutex
	// finished はハンドラから戻ったことを表す。戻った後は接続が次の要求に使われうるので、
	// 書き込みの締め切りを触らない。
	finished bool
	aborted  bool
	timer    *time.Timer
}

// abort は要求を打ち切る。context を取り消し、書き込みの締め切りを今にする。
// http.ServeContent は context を見ずに書き続けるので、取り消しだけでは止まらない。
func (t *trackedRequest) abort() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished || t.aborted {
		return
	}
	t.aborted = true
	t.cancel()
	_ = t.controller.SetWriteDeadline(time.Now())
}

// finish はハンドラから戻ったことを記し、確かめ直しを止める。
func (t *trackedRequest) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.finished = true
	if t.timer != nil {
		t.timer.Stop()
	}
}

func newSessionLedger(recheck time.Duration, logger *slog.Logger) *sessionLedger {
	if recheck <= 0 {
		recheck = defaultSessionRecheck
	}
	return &sessionLedger{requests: map[string]map[*trackedRequest]struct{}{}, recheck: recheck, logger: logger}
}

// track は要求を台帳に載せ、セッションの期限を締め切りとする context を返す。
// release は要求の処理を終えたとき（ハンドラから戻る前）に呼ぶ。
//
// 要求が recheck より長く続いたら、そこから recheck ごとにセッションを確かめ直し、
// 無効になっていたら打ち切る。ほとんどの要求は数秒で終わるので、確かめ直すのは
// /api/events・ライブ変換・大きな Range 応答のような長く続くものだけになる。
func (l *sessionLedger) track(
	ctx context.Context, w http.ResponseWriter, token string, expiresAt time.Time,
) (context.Context, func()) {
	ctx, cancel := context.WithDeadline(ctx, expiresAt)
	req := &trackedRequest{cancel: cancel, controller: http.NewResponseController(w)}
	// 期限を迎えたときにも書き込みを止める。
	stopWatch := context.AfterFunc(ctx, req.abort)

	l.mu.Lock()
	if l.requests[token] == nil {
		l.requests[token] = map[*trackedRequest]struct{}{}
	}
	l.requests[token][req] = struct{}{}
	l.mu.Unlock()

	if l.check != nil {
		checkCtx := context.WithoutCancel(ctx)
		recheck := func() {
			valid, err := l.check(checkCtx, token)
			if err != nil {
				// 確かめられないときは打ち切らず、次の間隔で確かめ直す。
				l.logger.Warn("処理中の要求のセッションを確かめ直せませんでした", slog.Any("error", err))
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
		req.mu.Lock()
		req.timer = time.AfterFunc(l.recheck, recheck)
		req.mu.Unlock()
	}

	release := func() {
		req.finish()
		stopWatch()
		cancel()
		l.mu.Lock()
		delete(l.requests[token], req)
		if len(l.requests[token]) == 0 {
			delete(l.requests, token)
		}
		l.mu.Unlock()
	}
	return ctx, release
}

// revoke は token のセッションで処理中の要求をすべて打ち切る。
func (l *sessionLedger) revoke(token string) {
	l.mu.Lock()
	requests := make([]*trackedRequest, 0, len(l.requests[token]))
	for req := range l.requests[token] {
		requests = append(requests, req)
	}
	l.mu.Unlock()
	for _, req := range requests {
		req.abort()
	}
}
