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

// 認証の定数（specs/016-single-account-auth/plan.md Structural Decisions 6、
// contracts/auth-api.md §3・§7）。
const (
	// sessionTokenBytes はセッション ID の乱数のバイト数である。
	sessionTokenBytes = 32
	// maxConcurrentVerifies は Argon2id の照合を同時に行う数の上限である。
	maxConcurrentVerifies = 2
	// defaultVerifyWait は照合の空きを待つ時間の既定である。
	defaultVerifyWait = 5 * time.Second
)

// dummyPasswordHash は、アカウントが未設定のときにログインで照合するハッシュである。
// 設定済みのときと同じだけ時間を掛け、応答時間から未設定かどうかを分からなくする。
// internal/password が新しく作るハッシュと同じパラメータ（m=19456,t=2,p=1）で作った
// 固定の値で、どのパスワードとも一致させない前提である。internal/password の既定の
// パラメータを変えたら、この値も同じパラメータで作り直す。
const dummyPasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$/Q7m10dyrvPbZgh4jySOzQ$zmfvSfxjbt65JfCJEu7mai1a/zN+8AcpRYQh4f7V8NQ"

// AuthStore はアカウントとログインセッションの保存先である。internal/store の
// AuthStore がこれを満たす。セッション ID はそのまま渡し、保存する形（ハッシュ）への
// 変換は保存先が行う。
type AuthStore interface {
	// Account は唯一のアカウントを返す。未設定なら domain.ErrAccountNotConfigured を返す。
	Account(ctx context.Context) (domain.Account, error)
	// Setup はアカウントと最初のセッションを足す。設定済みなら
	// domain.ErrAccountAlreadyConfigured を返す。
	Setup(ctx context.Context, username, passwordHash, sessionToken string, now time.Time) error
	// AddSession は照合に使った版でセッションを足し、replaceToken が空でなければその行を消す。
	AddSession(ctx context.Context, sessionToken string, accountVersion int64, replaceToken string, now time.Time) error
	// Session はセッションが有効かを確かめ、有効なら期限を返す。
	Session(ctx context.Context, sessionToken string, now time.Time) (time.Time, bool, error)
	// DeleteSession はセッションを消す。無ければ何もしない。
	DeleteSession(ctx context.Context, sessionToken string) error
}

// PasswordHasher はパスワードのハッシュ化と照合である。internal/password の
// Hash と Verify がこれを満たす（cmd/mdm で包んで渡す）。
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encoded string) (bool, error)
}

// AuthOptions は認証の操作に必要な依存である。
type AuthOptions struct {
	Store  AuthStore
	Hasher PasswordHasher
	// Now は今の時刻を返す。nil なら time.Now を使う。
	Now func() time.Time
	// Random はセッション ID の乱数の元である。nil なら crypto/rand.Reader を使う。
	Random io.Reader
	// VerifyWait は照合の空きを待つ時間である。0 なら 5 秒。
	VerifyWait time.Duration
	// MaxThrottleSources は試行制限で覚える送信元の数の上限である。0 なら既定値。
	MaxThrottleSources int
}

// Auth は初回設定・ログイン・ログアウト・セッションの確認を受け持つ
// （specs/016-single-account-auth/contracts/auth-api.md §2〜§4）。
// HTTP の形（Cookie・状態コード）は知らず、送信元と Cookie の値は呼び出し側が渡す。
type Auth struct {
	store      AuthStore
	hasher     PasswordHasher
	now        func() time.Time
	random     io.Reader
	verifyWait time.Duration
	// verifySlots は Argon2id の計算を同時に maxConcurrentVerifies までにする。
	verifySlots chan struct{}
	throttle    *loginThrottle
}

// NewAuth は認証の操作を返す。
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

// Session は発行したセッションである。Token は Cookie に入れる値（32 バイトの乱数の
// base64url、パディングなし）で、ExpiresAt はその期限である。
type Session struct {
	Token     string
	ExpiresAt time.Time
}

// AuthState は見る人の状態である（contracts/auth-api.md §4）。
type AuthState string

// 見る人の状態。
const (
	AuthStateOwner         AuthState = "owner"
	AuthStateGuest         AuthState = "guest"
	AuthStateSetupRequired AuthState = "setupRequired"
)

// ThrottledError はログインの試行が制限されたことを表す。errors.Is で
// domain.ErrLoginThrottled と一致する。RetryAfter は再び試せるまでの目安で、
// `Retry-After` に秒で入れる。
type ThrottledError struct {
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string { return domain.ErrLoginThrottled.Error() }

// Unwrap は domain.ErrLoginThrottled を返す。
func (e *ThrottledError) Unwrap() error { return domain.ErrLoginThrottled }

// RetryAfterSeconds は RetryAfter を切り上げた秒数（1 以上）である。
func (e *ThrottledError) RetryAfterSeconds() int {
	seconds := int((e.RetryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

// Setup は初回設定である。設定済みなら domain.ErrAccountAlreadyConfigured を、
// 値が規則を外れれば domain.ErrInvalidUsername か domain.ErrInvalidPassword を返す。
// 成立したら、そのままログインした状態のセッションを返す。
//
// 設定済みかどうかを先に確かめ、設定済みなら Argon2id の計算をしない。確かめた後に
// 別の初回設定が先に成立した場合は、保存先の主キーの衝突で
// domain.ErrAccountAlreadyConfigured になる。
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
		return Session{}, fmt.Errorf("パスワードをハッシュ化できません: %w", err)
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

// LoginRequest はログインの要求である。
type LoginRequest struct {
	Username string
	Password string
	// Source は送信元の IP アドレスである。試行制限の単位を決める。
	Source netip.Addr
	// CurrentToken は要求に付いていたセッション Cookie の値である（無ければ空）。
	// 成功したら、そのセッションを消す。
	CurrentToken string
}

// Login はユーザー名とパスワードを照合し、成功すれば新しいセッションを返す。
//
// ユーザー名かパスワードの誤り、空や上限超え、未設定は、どれも
// domain.ErrInvalidCredentials を返し、どれも Argon2id の照合を1回だけ行う。
// 送信元の試行が制限されているとき、または照合の空きを待ち切れなかったときは
// *ThrottledError を返し、照合しない（失敗には数えない）。
func (a *Auth) Login(ctx context.Context, req LoginRequest) (Session, error) {
	source := throttleKey(req.Source)
	if retryAfter, ok := a.throttle.reserve(source, a.now()); !ok {
		return Session{}, &ThrottledError{RetryAfter: retryAfter}
	}
	// ここから先の return は、予約を失敗・成功・取り消しのどれかにする。

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
	// 空欄や上限超えでも、そのまま照合する。原因によって照合を省くと、応答時間で
	// 原因が分かる。本文の大きさは HTTP 側が 8 KiB に抑える。
	matched, err := a.hasher.Verify(req.Password, account.PasswordHash)
	a.releaseVerifySlot()
	if err != nil {
		a.throttle.cancel(source)
		return Session{}, fmt.Errorf("パスワードを照合できません: %w", err)
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
	// 古いセッションは、有効かどうかを確かめずに消してよい。無効な行は元から使えない。
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

// CheckSession は token のセッションが有効かを確かめ、有効なら期限を返す。
// 形式の違う値は保存先に問い合わせずに無効とする。問い合わせが失敗したら、
// 無効とせず誤りを返す（data-model.md §4）。
func (a *Auth) CheckSession(ctx context.Context, token string) (time.Time, bool, error) {
	if !validTokenFormat(token) {
		return time.Time{}, false, nil
	}
	return a.store.Session(ctx, token, a.now())
}

// Logout は token のセッションを消す。形式の違う値と、該当の無い値では何もしない。
func (a *Auth) Logout(ctx context.Context, token string) error {
	if !validTokenFormat(token) {
		return nil
	}
	return a.store.DeleteSession(ctx, token)
}

// State は見る人の状態を返す。未設定なら token によらず AuthStateSetupRequired である。
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

// acquireVerifySlot は Argon2id の計算の枠を1つ取る。wait が正なら、その時間を
// 待っても取れなければ *ThrottledError を返す。0 なら ctx が終わるまで待つ。
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

// newSessionToken は 32 バイトの暗号学的乱数から、Cookie に入れるセッション ID を作る。
func (a *Auth) newSessionToken() (string, error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := io.ReadFull(a.random, raw); err != nil {
		return "", fmt.Errorf("セッション ID を作れません: %w", err)
	}
	return sessionTokenEncoding.EncodeToString(raw), nil
}

var sessionTokenEncoding = base64.RawURLEncoding

// validTokenFormat は値がセッション ID の形（32 バイトの base64url）かを返す。
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

// equalUsername は送られたユーザー名と保存値を、正規化も大文字小文字の畳み込みも
// せずにバイト列で比べる。両方の SHA-256 を定数時間で比べるので、長さや一致した
// 位置が時間に現れない。
func equalUsername(given, stored string) bool {
	g := sha256.Sum256([]byte(given))
	s := sha256.Sum256([]byte(stored))
	return subtle.ConstantTimeCompare(g[:], s[:]) == 1
}
