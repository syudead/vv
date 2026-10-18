package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Token management is session-only; bearerBoundary uses APITokens separately, while app/store own token generation, naming rules, and persistence.

type APITokens interface {
	// Lookup failures return errors rather than treating tokens as invalid.
	CheckAPIToken(ctx context.Context, secret string) (domain.APIToken, bool, error)
	// RecordAPITokenUse writes at most once per sixty seconds.
	RecordAPITokenUse(ctx context.Context, id int64) error
	ListAPITokens(ctx context.Context) ([]domain.APIToken, error)
	// CreateAPIToken revalidates the session when persisting and returns plaintext only with the created row.
	CreateAPIToken(ctx context.Context, sessionToken, name string) (domain.APIToken, string, error)
	// Revoking an absent ID is a no-op.
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
	// Recheck the boundary's exact session in the write transaction so revoked credentials cannot issue a new-version token.
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
	// The response contains plaintext and must never be cached.
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
	// Immediately cancel external API/MCP requests using the revoked token.
	s.sessions.revoke(apiTokenLedgerKey(id))
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

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

// Unused tokens expose lastUsedAt as null.
func toAPIToken(token domain.APIToken) gen.APIToken {
	out := gen.APIToken{Id: token.ID, Name: token.Name, CreatedAt: token.CreatedAt.UTC()}
	if !token.LastUsedAt.IsZero() {
		lastUsed := token.LastUsedAt.UTC()
		out.LastUsedAt = &lastUsed
	}
	return out
}
