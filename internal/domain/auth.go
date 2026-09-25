package domain

import (
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// 認証の誤り（specs/016-single-account-auth/contracts/auth-api.md）。
var (
	// ErrAccountNotConfigured はアカウントがまだ設定されていないことを表す。
	ErrAccountNotConfigured = errors.New("アカウントが設定されていません")
	// ErrAccountAlreadyConfigured は初回設定の時点でアカウントが既にあることを表す。
	// 同時の初回設定で負けた場合も含む。
	ErrAccountAlreadyConfigured = errors.New("アカウントは既に設定されています")
	// ErrInvalidCredentials はユーザー名かパスワードが違うことを表す。
	ErrInvalidCredentials = errors.New("ユーザー名またはパスワードが違います")
	// ErrLoginThrottled はログインの試行が制限されていることを表す。
	ErrLoginThrottled = errors.New("ログインの試行が多すぎます")
	// ErrInvalidUsername はユーザー名が ValidateUsername の規則を外れることを表す。
	ErrInvalidUsername = errors.New("ユーザー名が正しくありません")
	// ErrInvalidPassword はパスワードが ValidatePassword の規則を外れることを表す。
	ErrInvalidPassword = errors.New("パスワードが正しくありません")
	// ErrGuestQueryNotAllowed は、ゲストが所有者のデータに依る一覧の条件を
	// 指定したことを表す（specs/016-single-account-auth/contracts/guest-api.md §3）。
	ErrGuestQueryNotAllowed = errors.New("ログインしていないと使えない条件です")
)

// ユーザー名とパスワードの長さの上限（specs/016-single-account-auth/data-model.md §6）。
const (
	// MaxUsernameLength はユーザー名の文字数（Unicode のコードポイント数）の上限。
	MaxUsernameLength = 128
	// MaxPasswordBytes はパスワードのバイト数の上限。
	MaxPasswordBytes = 1024
)

// SessionLifetime はログインセッションの寿命である。発行から数え、使っても
// 延長しない（親 Issue #135 要件 7）。
const SessionLifetime = 90 * 24 * time.Hour

// ValidateUsername はユーザー名が規則を満たすかを確かめる。1〜128 文字で、
// 制御文字を含まず、先頭と末尾に空白を置かない。正規化や大文字小文字の畳み込みは
// しないので、規則を満たす値はそのまま保存する。外れたら ErrInvalidUsername を返す。
func ValidateUsername(username string) error {
	if username == "" || !utf8.ValidString(username) {
		return ErrInvalidUsername
	}
	if utf8.RuneCountInString(username) > MaxUsernameLength {
		return ErrInvalidUsername
	}
	if strings.IndexFunc(username, unicode.IsControl) >= 0 {
		return ErrInvalidUsername
	}
	first, _ := utf8.DecodeRuneInString(username)
	last, _ := utf8.DecodeLastRuneInString(username)
	if unicode.IsSpace(first) || unicode.IsSpace(last) {
		return ErrInvalidUsername
	}
	return nil
}

// ValidatePassword はパスワードが 1〜1024 バイトであるかを確かめる。強度の規則は
// 置かない。外れたら ErrInvalidPassword を返す。
func ValidatePassword(password string) error {
	if password == "" || len(password) > MaxPasswordBytes {
		return ErrInvalidPassword
	}
	return nil
}

// DefaultRedirectTarget は、戻り先が省略されたときと安全でないときに使う戻り先である。
const DefaultRedirectTarget = "/"

// redirectBase は戻り先を解釈する固定の基底である。予約済みの .invalid を使うので、
// 実在のホストと一致しない。
var redirectBase = &url.URL{Scheme: "http", Host: "vv.invalid", Path: "/"}

// SafeRedirectTarget はログイン後の戻り先 next を確かめ、安全な戻り先を返す
// （specs/016-single-account-auth/contracts/auth-api.md §3）。次をすべて満たす
// ときだけ、解釈した結果のパスと問い合わせ文字列を組み立て直した値を返し、
// それ以外と空は DefaultRedirectTarget を返す。入力そのものは返さない。
//   - 制御文字・空白・`\` をどこにも含まない（ブラウザは URL からタブと改行を
//     取り除くので、`/\t/evil.example` は `//evil.example` になる）。
//   - 固定の基底に対して解釈した結果が、スキームもホストも持たず、`/` で始まる
//     パスになる。
//   - パスが `/login`・`/setup`・`/api/` 以下でない。
func SafeRedirectTarget(next string) string {
	if next == "" || !utf8.ValidString(next) {
		return DefaultRedirectTarget
	}
	if strings.IndexFunc(next, isUnsafeRedirectRune) >= 0 {
		return DefaultRedirectTarget
	}
	ref, err := url.Parse(next)
	if err != nil || ref.Scheme != "" || ref.Host != "" || ref.User != nil || ref.Opaque != "" {
		return DefaultRedirectTarget
	}
	if !strings.HasPrefix(ref.Path, "/") {
		return DefaultRedirectTarget
	}
	// 基底に対して解く。`.` と `..` の区切りはここで取り除かれる。
	resolved := redirectBase.ResolveReference(ref)
	if resolved.Scheme != redirectBase.Scheme || resolved.Host != redirectBase.Host {
		return DefaultRedirectTarget
	}
	if isAuthOnlyPath(resolved.Path) {
		return DefaultRedirectTarget
	}
	target := resolved.EscapedPath()
	// `/.//evil.example` のように、区切りを取り除いた結果が `//` で始まると、
	// ブラウザはホストとして解釈する。
	if !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		return DefaultRedirectTarget
	}
	// ブラウザは `%2e` を `.` とみなして区切りを取り除くので、`/videos/%2e%2e/login`
	// は `/login` になる。Go の解釈では取り除かれないので、ここで拒む。
	if hasEncodedDotSegment(target) {
		return DefaultRedirectTarget
	}
	if resolved.RawQuery != "" {
		target += "?" + resolved.RawQuery
	}
	return target
}

// isUnsafeRedirectRune は戻り先に含めてはならない文字（制御文字・空白・`\`）かを返す。
func isUnsafeRedirectRune(r rune) bool {
	return r == '\\' || unicode.IsControl(r) || unicode.IsSpace(r)
}

// hasEncodedDotSegment は、エスケープしたパスに `%2e` を含む `.` か `..` の区切りが
// あるかを返す。
func hasEncodedDotSegment(escapedPath string) bool {
	for _, segment := range strings.Split(escapedPath, "/") {
		switch strings.ReplaceAll(strings.ToLower(segment), "%2e", ".") {
		case ".", "..":
			return true
		}
	}
	return false
}

// isAuthOnlyPath は、ログイン後の戻り先にしてはならないパス（ログイン画面・
// 初回設定画面・API）かを返す。画面の経路（React Router）は大文字と小文字を
// 区別しないので、ここでも区別しない。
func isAuthOnlyPath(path string) bool {
	path = strings.ToLower(path)
	switch {
	case path == "/login", path == "/setup", path == "/api":
		return true
	case strings.HasPrefix(path, "/login/"), strings.HasPrefix(path, "/setup/"), strings.HasPrefix(path, "/api/"):
		return true
	}
	return false
}

// Audience は要求を処理する相手が所有者かゲストかを表す
// （specs/016-single-account-auth/data-model.md §3）。ゼロ値はゲストで、
// 付け忘れたときは狭い側に倒れる。
type Audience int

const (
	// AudienceGuest は有効なセッションを持たない見る人である（ゼロ値）。
	AudienceGuest Audience = iota
	// AudienceOwner は有効なセッションを持つ所有者である。
	AudienceOwner
)

// IsOwner は所有者かを返す。AudienceOwner 以外はすべてゲストとして扱う。
func (a Audience) IsOwner() bool { return a == AudienceOwner }

// String は `X-VV-Audience` などで使う名前（`owner` か `guest`）を返す。
func (a Audience) String() string {
	if a.IsOwner() {
		return "owner"
	}
	return "guest"
}

// CheckVideoQuery は、見る人がこの一覧の条件を使えるかを確かめる
// （specs/016-single-account-auth/contracts/guest-api.md §3）。所有者はすべて
// 使える。ゲストは所有者のデータ（再生位置・タグ）に依る条件、つまり `all` 以外の
// 視聴状態、再生日時の並べ替え、タグの絞り込みを使えず、ErrGuestQueryNotAllowed を返す。
func (a Audience) CheckVideoQuery(q VideoQuery) error {
	if a.IsOwner() {
		return nil
	}
	switch q.Watch {
	case "", WatchAll:
	default:
		return ErrGuestQueryNotAllowed
	}
	switch q.Sort {
	case SortPlayedAsc, SortPlayedDesc:
		return ErrGuestQueryNotAllowed
	}
	if len(q.TagIDs) > 0 {
		return ErrGuestQueryNotAllowed
	}
	return nil
}
