package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSafeRedirectTarget(t *testing.T) {
	for _, tc := range []struct {
		next string
		want string
	}{
		{"/videos/1?t=2", "/videos/1?t=2"},
		{"/", "/"},
		{"/folders/3/a%20b?sort=titleAsc&q=x", "/folders/3/a%20b?sort=titleAsc&q=x"},
		{"/videos/1#frag", "/videos/1"},
		{"/videos/./1", "/videos/1"},
		{"/videos/../settings", "/settings"},
		{"/loginx", "/loginx"},

		{"", "/"},
		{"//evil.example", "/"},
		{"///evil.example", "/"},
		{`/\evil`, "/"},
		{"/\\t/evil.example", "/"},
		{"/\t/evil.example", "/"},
		{"/\\n/evil.example", "/"},
		{"/\n/evil.example", "/"},
		{"/\r/evil.example", "/"},
		{`/a\b`, "/"},
		{`/a\\b`, "/"},
		{"/a b", "/"},
		{"/a b", "/"},
		{"/a\u0085b", "/"},
		{"/a\x7fb", "/"},
		{"/\xff", "/"},
		{"https://evil.example/", "/"},
		{"http:/evil.example", "/"},
		{"javascript:alert(1)", "/"},
		{"//user@evil.example/", "/"},
		{"/.//evil.example", "/"},
		// 区切りを取り除いても `/` 1つで始まるので、同じオリジンのパスのままである。
		{"/a/..//evil.example", "/evil.example"},
		{"videos/1", "/"},
		{"?t=2", "/"},
		{"#x", "/"},
		{"/login", "/"},
		{"/login?next=/", "/"},
		{"/setup", "/"},
		{"/api/videos", "/"},
		{"/api", "/"},
		{"/Login", "/"},
		{"/SETUP", "/"},
		{"/Api/videos", "/"},
		{"/%61pi/videos", "/"},
		{"/videos/../api/scans", "/"},
		{"/videos/%2e%2e/login", "/"},
		{"/videos/%2E./x", "/"},
		{"/%2e/x", "/"},
		{"/a%2eb/c", "/a%2eb/c"},
		{"/%zz", "/"},
	} {
		if got := SafeRedirectTarget(tc.next); got != tc.want {
			t.Errorf("SafeRedirectTarget(%q) = %q, want %q", tc.next, got, tc.want)
		}
	}
}

func TestValidateUsername(t *testing.T) {
	for _, ok := range []string{
		"a",
		"Owner",
		"山田 太郎",
		strings.Repeat("あ", MaxUsernameLength),
	} {
		if err := ValidateUsername(ok); err != nil {
			t.Errorf("ValidateUsername(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{
		"",
		"a\x00b",
		"a\tb",
		"a\nb",
		"a\x7fb",
		"a\u0085b",
		" owner",
		"owner ",
		"　owner",
		"owner ",
		strings.Repeat("a", MaxUsernameLength+1),
		strings.Repeat("あ", MaxUsernameLength+1),
		"\xff",
	} {
		if err := ValidateUsername(bad); !errors.Is(err, ErrInvalidUsername) {
			t.Errorf("ValidateUsername(%q) = %v, want ErrInvalidUsername", bad, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	for _, ok := range []string{"x", " spaced ", strings.Repeat("a", MaxPasswordBytes)} {
		if err := ValidatePassword(ok); err != nil {
			t.Errorf("ValidatePassword(len %d) = %v, want nil", len(ok), err)
		}
	}
	// 「あ」は3バイトなので、342 文字で 1026 バイトになる。
	for _, bad := range []string{"", strings.Repeat("a", MaxPasswordBytes+1), strings.Repeat("あ", 342)} {
		if err := ValidatePassword(bad); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("ValidatePassword(len %d) = %v, want ErrInvalidPassword", len(bad), err)
		}
	}
}

func TestSessionLifetime(t *testing.T) {
	if SessionLifetime != 90*24*time.Hour {
		t.Fatalf("SessionLifetime = %v, want 90 days", SessionLifetime)
	}
}

func TestAudienceZeroValueIsGuest(t *testing.T) {
	var a Audience
	if a != AudienceGuest || a.IsOwner() || a.String() != "guest" {
		t.Fatalf("Audience のゼロ値 = %v (IsOwner %v)、ゲストであるべき", a, a.IsOwner())
	}
	if !AudienceOwner.IsOwner() || AudienceOwner.String() != "owner" {
		t.Fatalf("AudienceOwner が所有者として扱われない")
	}
	if Audience(99).IsOwner() {
		t.Fatalf("未知の Audience が所有者として扱われた")
	}
}

func TestCheckVideoQuery(t *testing.T) {
	defaults := VideoQuery{Watch: WatchAll, Sort: SortAddedDesc}
	allowed := []VideoQuery{
		{},
		defaults,
		{Watch: WatchAll, Sort: SortTitleAsc, Query: "cat", PlayableOnly: true},
		{Sort: SortRandom, Seed: 5},
	}
	denied := []VideoQuery{
		{Watch: WatchUnwatched},
		{Watch: WatchInProgress},
		{Watch: WatchWatched},
		{Sort: SortPlayedDesc},
		{Sort: SortPlayedAsc},
		{TagIDs: []int64{1}},
	}
	for _, q := range allowed {
		if err := AudienceGuest.CheckVideoQuery(q); err != nil {
			t.Errorf("ゲスト: CheckVideoQuery(%+v) = %v, want nil", q, err)
		}
	}
	for _, q := range denied {
		if err := AudienceGuest.CheckVideoQuery(q); !errors.Is(err, ErrGuestQueryNotAllowed) {
			t.Errorf("ゲスト: CheckVideoQuery(%+v) = %v, want ErrGuestQueryNotAllowed", q, err)
		}
		if err := AudienceOwner.CheckVideoQuery(q); err != nil {
			t.Errorf("所有者: CheckVideoQuery(%+v) = %v, want nil", q, err)
		}
	}
}
