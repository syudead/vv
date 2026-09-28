package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 画面の API トークンの管理（specs/026-external-api/contracts/token-api.md）。
//
// 3 つの操作はどれも所有者だけで、accessRoutes に載せない。Bearer はここでは読まない。
// 名前の規則・平文の作り・保存は internal/app と internal/store が持ち、httpapi は要求の
// 解釈と契約の形への変換だけを持つ。

// APITokens は API トークンの発行・一覧・失効である。internal/app の *Auth がこれを満たす。
type APITokens interface {
	// ListAPITokens は有効な API トークンを作成日時の降順で返す。
	ListAPITokens(ctx context.Context) ([]domain.APIToken, error)
	// CreateAPIToken はセッション sessionToken の求めで API トークンを発行し、保存した行と
	// 平文を返す。名前が規則を外れれば *domain.InvalidAPITokenNameError を、保存の時点で
	// セッションが有効でなければ domain.ErrSessionNotValid を返す。
	CreateAPIToken(ctx context.Context, sessionToken, name string) (domain.APIToken, string, error)
	// RevokeAPIToken は API トークンを失効させる。無い id では何もしない。
	RevokeAPIToken(ctx context.Context, id int64) error
}

func (s *server) ListApiTokens(w http.ResponseWriter, r *http.Request) {
	if s.apiTokens == nil {
		s.internalError(w, "API token storage is not configured.", nil)
		return
	}
	tokens, err := s.apiTokens.ListAPITokens(r.Context())
	if err != nil {
		s.internalError(w, "Could not load API tokens.", err)
		return
	}
	items := make([]gen.APIToken, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, toAPIToken(token))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.APITokenList{Items: items}, s.logger)
}

func (s *server) CreateApiToken(w http.ResponseWriter, r *http.Request) {
	var body gen.CreateAPITokenRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.apiTokens == nil {
		s.internalError(w, "API token storage is not configured.", nil)
		return
	}
	// 境界が確かめたのと同じセッションを渡し、保存と同じ取引でもう一度確かめさせる。
	// 境界の後に資格情報が変わっていれば、新しい版のトークンを作らない（research.md R-2）。
	token, secret, err := s.apiTokens.CreateAPIToken(r.Context(), s.sessionToken(r), body.Name)
	if errors.Is(err, domain.ErrInvalidAPITokenName) {
		s.invalidAPITokenName(w, err)
		return
	}
	if errors.Is(err, domain.ErrSessionNotValid) {
		s.unauthenticated(w)
		return
	}
	if err != nil {
		s.internalError(w, "Could not create the API token.", err)
		return
	}
	// 平文を含むので、どこにも残させない。
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, gen.CreatedAPIToken{Token: toAPIToken(token), Secret: secret}, s.logger)
}

func (s *server) DeleteApiToken(w http.ResponseWriter, r *http.Request, id gen.APITokenId) {
	if s.apiTokens == nil {
		s.internalError(w, "API token storage is not configured.", nil)
		return
	}
	if err := s.apiTokens.RevokeAPIToken(r.Context(), id); err != nil {
		s.internalError(w, "Could not revoke the API token.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// invalidAPITokenName は domain.NormalizeAPITokenName の失敗を、その理由に応じた reason の
// invalid_request へ写す（contracts/token-api.md）。
func (s *server) invalidAPITokenName(w http.ResponseWriter, err error) {
	var invalid *domain.InvalidAPITokenNameError
	if !errors.As(err, &invalid) {
		s.invalidRequest(w, "The API token name cannot be used.")
		return
	}
	switch invalid.Problem {
	case domain.APITokenNameEmpty:
		s.invalidRequestReason(w, reasonAPITokenNameEmpty, "Enter a name for the API token.")
	case domain.APITokenNameControlCharacters:
		s.invalidRequestReason(w, reasonAPITokenNameControlCharacters, "API token names cannot contain control characters.")
	case domain.APITokenNameTooLong:
		s.invalidRequestLimit(w, reasonAPITokenNameTooLong, domain.APITokenNameMaxLength,
			"API token names must be at most "+strconv.Itoa(domain.APITokenNameMaxLength)+" characters.")
	default:
		s.invalidRequest(w, "The API token name cannot be used.")
	}
}

// toAPIToken は API トークンを契約の形にする。未使用なら lastUsedAt は null である。
func toAPIToken(token domain.APIToken) gen.APIToken {
	out := gen.APIToken{Id: token.ID, Name: token.Name, CreatedAt: token.CreatedAt.UTC()}
	if !token.LastUsedAt.IsZero() {
		lastUsed := token.LastUsedAt.UTC()
		out.LastUsedAt = &lastUsed
	}
	return out
}
