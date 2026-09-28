package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

type fakeAPIToken struct {
	token   domain.APIToken
	version int64
}

func (f *fakeAuthStore) AddAPIToken(_ context.Context, name, token string, now time.Time) (domain.APIToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.account == nil {
		return domain.APIToken{}, domain.ErrAccountNotConfigured
	}
	added := domain.APIToken{ID: int64(len(f.apiTokens) + 1), Name: name, CreatedAt: now}
	f.apiTokens[token] = fakeAPIToken{token: added, version: f.account.Version}
	return added, nil
}

func (f *fakeAuthStore) ListAPITokens(context.Context) ([]domain.APIToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tokens := []domain.APIToken{}
	for _, t := range f.apiTokens {
		if f.account != nil && t.version == f.account.Version {
			tokens = append(tokens, t.token)
		}
	}
	slices.SortFunc(tokens, func(a, b domain.APIToken) int { return int(b.ID - a.ID) })
	return tokens, nil
}

func (f *fakeAuthStore) DeleteAPIToken(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for secret, t := range f.apiTokens {
		if t.token.ID == id {
			delete(f.apiTokens, secret)
		}
	}
	return nil
}

func (f *fakeAuthStore) APIToken(_ context.Context, token string) (domain.APIToken, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.apiTokenQueries++
	if f.failAPIToken {
		return domain.APIToken{}, false, errFakeDB
	}
	t, ok := f.apiTokens[token]
	if !ok || f.account == nil || t.version != f.account.Version {
		return domain.APIToken{}, false, nil
	}
	return t.token, true, nil
}

func (f *fakeAuthStore) TouchAPIToken(_ context.Context, id int64, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for secret, t := range f.apiTokens {
		if t.token.ID == id {
			t.token.LastUsedAt = now
			f.apiTokens[secret] = t
		}
	}
	return nil
}

func TestAuthCreateAPITokenReturnsPrefixedSecret(t *testing.T) {
	random := bytes.Repeat([]byte{0xfb}, sessionTokenBytes)
	f := newAuthFixture(t, nil).configured(t)
	f.auth.random = bytes.NewReader(random)
	ctx := context.Background()

	token, secret, err := f.auth.CreateAPIToken(ctx, "  Claude Code ")
	if err != nil {
		t.Fatal(err)
	}
	if want := "vvt_" + base64.RawURLEncoding.EncodeToString(random); secret != want {
		t.Errorf("secret = %q, want %q", secret, want)
	}
	if len(secret) != 47 {
		t.Errorf("len(secret) = %d, want 47", len(secret))
	}
	if token.Name != "Claude Code" || !token.CreatedAt.Equal(f.clock.Now()) {
		t.Errorf("token = %+v", token)
	}

	got, ok, err := f.auth.CheckAPIToken(ctx, secret)
	if err != nil || !ok || got.ID != token.ID {
		t.Fatalf("CheckAPIToken = %+v, %v, %v", got, ok, err)
	}
	list, err := f.auth.ListAPITokens(ctx)
	if err != nil || len(list) != 1 || list[0].ID != token.ID {
		t.Fatalf("ListAPITokens = %+v, %v", list, err)
	}

	if err := f.auth.RevokeAPIToken(ctx, token.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := f.auth.CheckAPIToken(ctx, secret); ok {
		t.Error("失効したトークンが有効")
	}
}

func TestAuthCreateAPITokenRejectsInvalidName(t *testing.T) {
	f := newAuthFixture(t, nil).configured(t)
	for _, name := range []string{"", " ", "a\nb", strings.Repeat("a", domain.APITokenNameMaxLength+1)} {
		if _, _, err := f.auth.CreateAPIToken(context.Background(), name); !errors.Is(err, domain.ErrInvalidAPITokenName) {
			t.Errorf("CreateAPIToken(%q) err = %v, want ErrInvalidAPITokenName", name, err)
		}
	}
	if len(f.store.apiTokens) != 0 {
		t.Error("規則を外れた名前で保存した")
	}
}

func TestAuthCreateAPITokenRequiresAccount(t *testing.T) {
	f := newAuthFixture(t, nil)
	if _, _, err := f.auth.CreateAPIToken(context.Background(), "n"); !errors.Is(err, domain.ErrAccountNotConfigured) {
		t.Fatalf("err = %v, want ErrAccountNotConfigured", err)
	}
}

func TestAuthCheckAPITokenRejectsMalformedWithoutQuery(t *testing.T) {
	f := newAuthFixture(t, nil).configured(t)
	ctx := context.Background()
	_, secret, err := f.auth.CreateAPIToken(ctx, "n")
	if err != nil {
		t.Fatal(err)
	}
	queries := f.store.apiTokenQueries
	for _, value := range []string{
		"",
		strings.TrimPrefix(secret, "vvt_"),
		"vvx_" + strings.TrimPrefix(secret, "vvt_"),
		secret + "A",
		secret[:len(secret)-1],
		"vvt_" + strings.Repeat("!", 43),
	} {
		if _, ok, err := f.auth.CheckAPIToken(ctx, value); ok || err != nil {
			t.Errorf("CheckAPIToken(%q) = %v, %v", value, ok, err)
		}
	}
	if f.store.apiTokenQueries != queries {
		t.Error("形式の違う値で保存先に問い合わせた")
	}
}

func TestAuthCheckAPITokenReportsQueryFailure(t *testing.T) {
	f := newAuthFixture(t, nil).configured(t)
	ctx := context.Background()
	_, secret, err := f.auth.CreateAPIToken(ctx, "n")
	if err != nil {
		t.Fatal(err)
	}
	f.store.failAPIToken = true
	if _, ok, err := f.auth.CheckAPIToken(ctx, secret); ok || !errors.Is(err, errFakeDB) {
		t.Fatalf("CheckAPIToken = %v, %v, want 誤り", ok, err)
	}
}

func TestAuthRecordAPITokenUseUsesClock(t *testing.T) {
	f := newAuthFixture(t, nil).configured(t)
	ctx := context.Background()
	token, secret, err := f.auth.CreateAPIToken(ctx, "n")
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Hour)
	if err := f.auth.RecordAPITokenUse(ctx, token.ID); err != nil {
		t.Fatal(err)
	}
	got, _, _ := f.auth.CheckAPIToken(ctx, secret)
	if !got.LastUsedAt.Equal(f.clock.Now()) {
		t.Errorf("LastUsedAt = %v, want %v", got.LastUsedAt, f.clock.Now())
	}
}
