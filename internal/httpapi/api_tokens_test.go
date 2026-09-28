package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 画面の API トークンの管理（specs/026-external-api/contracts/token-api.md）を、本物の
// Auth と保存先で確かめる。

func createAPITokenBody(name string) string {
	body, _ := json.Marshal(map[string]string{"name": name})
	return string(body)
}

func (e *authEnv) createAPIToken(cookie *http.Cookie, name string) gen.CreatedAPIToken {
	e.t.Helper()
	rec := e.serve(authRequest{
		method: http.MethodPost, target: "/api/api-tokens", body: createAPITokenBody(name),
		cookies: []*http.Cookie{cookie},
	})
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("発行: status = %d: %s", rec.Code, rec.Body)
	}
	var created gen.CreatedAPIToken
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		e.t.Fatal(err)
	}
	return created
}

func (e *authEnv) listAPITokens(cookie *http.Cookie) (gen.APITokenList, string) {
	e.t.Helper()
	rec := e.get("/api/api-tokens", cookie)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("一覧: status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		e.t.Errorf("一覧の Cache-Control = %q", got)
	}
	var list gen.APITokenList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		e.t.Fatal(err)
	}
	return list, rec.Body.String()
}

func TestAPITokenCreateReturnsSecretOnce(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{})
	cookie := env.setup()

	rec := env.serve(authRequest{
		method: http.MethodPost, target: "/api/api-tokens", body: createAPITokenBody("  Claude Code "),
		cookies: []*http.Cookie{cookie},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("発行: status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("発行の Cache-Control = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `"lastUsedAt":null`) {
		t.Errorf("未使用の lastUsedAt が null でない: %s", rec.Body)
	}
	var created gen.CreatedAPIToken
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Secret, "vvt_") || len(created.Secret) != 47 {
		t.Errorf("secret = %q", created.Secret)
	}
	if created.Token.Name != "Claude Code" || created.Token.Id < 1 || created.Token.LastUsedAt != nil ||
		!created.Token.CreatedAt.Equal(env.clock().Truncate(1e9)) {
		t.Errorf("token = %+v", created.Token)
	}

	list, raw := env.listAPITokens(cookie)
	if len(list.Items) != 1 || list.Items[0] != created.Token {
		t.Fatalf("一覧 = %+v", list)
	}
	sum := sha256.Sum256([]byte(created.Secret))
	for _, secret := range []string{created.Secret, strings.TrimPrefix(created.Secret, "vvt_"), hex.EncodeToString(sum[:])} {
		if strings.Contains(raw, secret) {
			t.Errorf("一覧に平文かハッシュがある: %s", raw)
		}
	}
	if strings.Contains(raw, "secret") {
		t.Errorf("一覧に secret の項目がある: %s", raw)
	}
}

func TestAPITokenListIsNewestFirstAndAllowsDuplicateNames(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{})
	cookie := env.setup()
	first := env.createAPIToken(cookie, "same")
	env.advance(1e9)
	second := env.createAPIToken(cookie, "same")

	list, _ := env.listAPITokens(cookie)
	if len(list.Items) != 2 || list.Items[0].Id != second.Token.Id || list.Items[1].Id != first.Token.Id {
		t.Fatalf("一覧 = %+v", list)
	}
}

func TestAPITokenRevokeIsIdempotent(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{})
	cookie := env.setup()
	kept := env.createAPIToken(cookie, "kept")
	revoked := env.createAPIToken(cookie, "revoked")

	target := "/api/api-tokens/" + strconv.FormatInt(revoked.Token.Id, 10)
	for range 2 {
		rec := env.serve(authRequest{method: http.MethodDelete, target: target, cookies: []*http.Cookie{cookie}})
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("失効: status = %d: %s", rec.Code, rec.Body)
		}
	}
	list, _ := env.listAPITokens(cookie)
	if len(list.Items) != 1 || list.Items[0].Id != kept.Token.Id {
		t.Fatalf("一覧 = %+v", list)
	}

	rec := env.serve(authRequest{method: http.MethodDelete, target: "/api/api-tokens/abc", cookies: []*http.Cookie{cookie}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("数でない id: status = %d: %s", rec.Code, rec.Body)
	}
}

func TestAPITokenCreateRejectsInvalidNames(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{})
	cookie := env.setup()
	cases := []struct {
		name string
		want wantError
	}{
		{"", wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonAPITokenNameEmpty}},
		{" 　", wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonAPITokenNameEmpty}},
		{"a\tb", wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonAPITokenNameControlCharacters}},
		{strings.Repeat("a", domain.APITokenNameMaxLength+1), wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
			reason: reasonAPITokenNameTooLong, limit: domain.APITokenNameMaxLength,
		}},
	}
	for _, tc := range cases {
		rec := env.serve(authRequest{
			method: http.MethodPost, target: "/api/api-tokens", body: createAPITokenBody(tc.name),
			cookies: []*http.Cookie{cookie},
		})
		assertErrorBody(t, strconv.Quote(tc.name), rec.Code, rec.Body.Bytes(), tc.want)
	}
	// 上限ちょうどは受け付ける。
	env.createAPIToken(cookie, strings.Repeat("あ", domain.APITokenNameMaxLength))

	// 本文は JSON が必須である。
	rec := env.serve(authRequest{
		method: http.MethodPost, target: "/api/api-tokens", body: createAPITokenBody("n"),
		cookies: []*http.Cookie{cookie}, header: map[string]string{"Content-Type": "text/plain"},
	})
	assertErrorBody(t, "text/plain", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})

	list, _ := env.listAPITokens(cookie)
	if len(list.Items) != 1 {
		t.Errorf("規則を外れた名前で発行した: %+v", list)
	}
}

// 画面の API は Bearer を読まない。有効なトークンでも Cookie が無ければ未認証である（受け入れ条件 8）。
func TestAPITokenOperationsIgnoreBearer(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{})
	cookie := env.setup()
	created := env.createAPIToken(cookie, "n")
	bearer := map[string]string{"Authorization": "Bearer " + created.Secret}

	for _, req := range []authRequest{
		{method: http.MethodGet, target: "/api/api-tokens", header: bearer},
		{method: http.MethodPost, target: "/api/api-tokens", body: createAPITokenBody("x"), header: bearer},
		{method: http.MethodDelete, target: "/api/api-tokens/" + strconv.FormatInt(created.Token.Id, 10), header: bearer},
	} {
		assertUnauthenticated(t, "Bearer の "+req.method+" "+req.target, env.serve(req))
	}
	list, _ := env.listAPITokens(cookie)
	if len(list.Items) != 1 {
		t.Errorf("Bearer の要求が一覧を変えた: %+v", list)
	}
}
