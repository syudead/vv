package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
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
		// Normalization preserves a single leading slash and therefore the same origin.
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

func TestValidateUsernameLengthProblem(t *testing.T) {
	for _, tooLong := range []string{"", strings.Repeat("a", MaxUsernameLength+1)} {
		if err := ValidateUsername(tooLong); !errors.Is(err, ErrUsernameLength) {
			t.Errorf("ValidateUsername(len %d) = %v, want ErrUsernameLength", len(tooLong), err)
		}
	}
	for _, other := range []string{" bob", "bob ", "a\x00b", "\xff"} {
		if err := ValidateUsername(other); errors.Is(err, ErrUsernameLength) {
			t.Errorf("ValidateUsername(%q) = %v, want not ErrUsernameLength", other, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	for _, ok := range []string{"x", " spaced ", strings.Repeat("a", MaxPasswordBytes)} {
		if err := ValidatePassword(ok); err != nil {
			t.Errorf("ValidatePassword(len %d) = %v, want nil", len(ok), err)
		}
	}
	// Each code point is three bytes, so 342 code points exceed the 1,024-byte limit.
	for _, bad := range []string{"", strings.Repeat("a", MaxPasswordBytes+1), strings.Repeat("あ", 342)} {
		if err := ValidatePassword(bad); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("ValidatePassword(len %d) = %v, want ErrInvalidPassword", len(bad), err)
		}
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
	if _, explicit := a.ExcludedFolders(); explicit {
		t.Fatalf("Audience のゼロ値が明示の除外リストを持つ")
	}
}

func TestAudienceExcludedFoldersReplaceTheSwitches(t *testing.T) {
	scoped := AudienceOwner.WithExcludedFolders([]int64{5, 3, 5, 0})
	ids, explicit := scoped.ExcludedFolders()
	if !explicit || !reflect.DeepEqual(ids, []int64{3, 5}) || !scoped.IsOwner() {
		t.Fatalf("WithExcludedFolders = %v (explicit %v, owner %v)、[3 5] で所有者のまま", ids, explicit, scoped.IsOwner())
	}
	if scoped != AudienceOwner.WithExcludedFolders([]int64{3, 5}) || scoped == AudienceOwner {
		t.Fatalf("同じ除外リストの Audience が等しくない、またはスイッチの Audience と等しい")
	}
	if ids, explicit := AudienceGuest.WithExcludedFolders(nil).ExcludedFolders(); !explicit || len(ids) != 0 {
		t.Fatalf("空の除外リスト = %v (explicit %v)、スイッチを使わない空のリストであるべき", ids, explicit)
	}
}

func TestAudienceListsFolderFollowsItsScope(t *testing.T) {
	on, off := MediaFolder{ID: 3, Enabled: true}, MediaFolder{ID: 5, Enabled: false}
	if !AudienceOwner.ListsFolder(on) || AudienceGuest.ListsFolder(off) {
		t.Errorf("スイッチの Audience: オン %v, オフ %v, want オンだけを並べる",
			AudienceOwner.ListsFolder(on), AudienceGuest.ListsFolder(off))
	}
	scoped := AudienceOwner.WithExcludedFolders([]int64{3})
	if scoped.ListsFolder(on) || !scoped.ListsFolder(off) {
		t.Errorf("除外リスト [3] の Audience: 3 %v, 5 %v, want スイッチではなくリストで 3 だけを外す",
			scoped.ListsFolder(on), scoped.ListsFolder(off))
	}
}

func TestAudienceListsAsSwitches(t *testing.T) {
	folders := []MediaFolder{{ID: 3, Enabled: true}, {ID: 5, Enabled: false}}
	for _, c := range []struct {
		name     string
		audience Audience
		want     bool
	}{
		{"switches", AudienceOwner, true},
		{"the switched-off folder", AudienceOwner.WithExcludedFolders([]int64{5}), true},
		{"plus a folder this node lacks", AudienceGuest.WithExcludedFolders([]int64{5, 9}), true},
		{"a switched-on folder too", AudienceOwner.WithExcludedFolders([]int64{3, 5}), false},
		{"nothing", AudienceOwner.WithExcludedFolders(nil), false},
	} {
		if got := c.audience.ListsAsSwitches(folders); got != c.want {
			t.Errorf("%s: ListsAsSwitches = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCheckVideoQuery(t *testing.T) {
	defaults := VideoQuery{Sort: SortAddedDesc}
	allowed := []VideoQuery{
		{},
		defaults,
		{Sort: SortTitleAsc, Query: "cat"},
		{Sort: SortRandom, Seed: 5},
		{VideoConditions: VideoConditions{MinDurationMs: new(int64(1)), AddedFrom: "2025-01-01"}},
	}
	denied := []VideoQuery{
		{Sort: SortPlayedDesc},
		{Sort: SortPlayedAsc},
		{TagIDs: []int64{1}},
		{FavoriteOnly: true},
		{Sort: SortFavoritedAsc},
		{Sort: SortFavoritedDesc},
		{Person: 1},
		{Organization: 1},
		{TagKindID: 1},
		{VideoConditions: VideoConditions{WatchState: WatchStateUnwatched}},
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
