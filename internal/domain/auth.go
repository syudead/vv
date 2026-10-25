package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrAccountNotConfigured = errors.New("account is not configured")
	// ErrAccountAlreadyConfigured includes concurrent setup attempts that lose the race.
	ErrAccountAlreadyConfigured = errors.New("account is already configured")
	ErrInvalidCredentials       = errors.New("username or password is incorrect")
	ErrLoginThrottled           = errors.New("too many login attempts")
	// ErrSessionNotValid includes sessions revoked after boundary validation by logout or credential changes.
	ErrSessionNotValid = errors.New("session is no longer valid")
	ErrInvalidUsername = errors.New("invalid username")
	// ErrUsernameLength wraps ErrInvalidUsername so the API can distinguish length violations from whitespace or control characters.
	ErrUsernameLength       = fmt.Errorf("%w: must be 1 to %d characters", ErrInvalidUsername, MaxUsernameLength)
	ErrInvalidPassword      = errors.New("invalid password")
	ErrGuestQueryNotAllowed = errors.New("this filter requires signing in")
)

const (
	// MaxUsernameLength counts Unicode code points.
	MaxUsernameLength = 128
	// MaxPasswordBytes counts bytes.
	MaxPasswordBytes = 1024
)

// SessionLifetime starts at issuance and never extends on use.
const SessionLifetime = 90 * 24 * time.Hour

type Account struct {
	Username string
	// PasswordHash stores an Argon2id PHC string, never plaintext.
	PasswordHash string
	// Version increments on credential changes; sessions issued against other versions are invalid.
	Version int64
}

// ValidateUsername accepts 1–128 code points without controls or surrounding whitespace, preserving case and normalization; length failures wrap ErrInvalidUsername in ErrUsernameLength.
func ValidateUsername(username string) error {
	if !utf8.ValidString(username) {
		return ErrInvalidUsername
	}
	if username == "" || utf8.RuneCountInString(username) > MaxUsernameLength {
		return ErrUsernameLength
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

// ValidatePassword accepts 1–1024 bytes without strength requirements.
func ValidatePassword(password string) error {
	if password == "" || len(password) > MaxPasswordBytes {
		return ErrInvalidPassword
	}
	return nil
}

const DefaultRedirectTarget = "/"

// The reserved .invalid base cannot match a real host.
var redirectBase = &url.URL{Scheme: "http", Host: "vv.invalid", Path: "/"}

// SafeRedirectTarget rebuilds a local path and query, rejecting controls, whitespace, backslashes, hosts, schemes, and auth/API destinations; empty or unsafe input returns DefaultRedirectTarget.
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
	// Resolving against the base removes dot segments.
	resolved := redirectBase.ResolveReference(ref)
	if resolved.Scheme != redirectBase.Scheme || resolved.Host != redirectBase.Host {
		return DefaultRedirectTarget
	}
	if isAuthOnlyPath(resolved.Path) {
		return DefaultRedirectTarget
	}
	target := resolved.EscapedPath()
	// A normalized path starting with // becomes a host reference in browsers.
	if !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		return DefaultRedirectTarget
	}
	// Browsers normalize encoded dot segments such as /videos/%2e%2e/login, unlike Go; reject them here.
	if hasEncodedDotSegment(target) {
		return DefaultRedirectTarget
	}
	if resolved.RawQuery != "" {
		target += "?" + resolved.RawQuery
	}
	return target
}

func isUnsafeRedirectRune(r rune) bool {
	return r == '\\' || unicode.IsControl(r) || unicode.IsSpace(r)
}

func hasEncodedDotSegment(escapedPath string) bool {
	for _, segment := range strings.Split(escapedPath, "/") {
		switch strings.ReplaceAll(strings.ToLower(segment), "%2e", ".") {
		case ".", "..":
			return true
		}
	}
	return false
}

// Auth route matching is case-insensitive to match React Router.
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

// Audience defaults to guest so omitted values fail closed.
type Audience int

const (
	AudienceGuest Audience = iota
	AudienceOwner
)

// IsOwner treats every value except AudienceOwner as guest.
func (a Audience) IsOwner() bool { return a == AudienceOwner }

// String returns the owner/guest names used by X-VV-Audience.
func (a Audience) String() string {
	if a.IsOwner() {
		return "owner"
	}
	return "guest"
}

// CheckVideoQuery rejects guest filters and sorting based on owner data with ErrGuestQueryNotAllowed.
func (a Audience) CheckVideoQuery(q VideoQuery) error {
	if a.IsOwner() {
		return nil
	}
	switch q.Sort {
	case SortPlayedAsc, SortPlayedDesc, SortFavoritedAsc, SortFavoritedDesc:
		return ErrGuestQueryNotAllowed
	}
	if len(q.TagIDs) > 0 || q.FavoriteOnly || q.Person != 0 || q.PersonRole != "" || q.Organization != 0 || q.TagKindID != 0 ||
		q.ByName() {
		return ErrGuestQueryNotAllowed
	}
	return nil
}
