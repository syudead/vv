package wasmplugin

import (
	"context"
	"io"
	"net/http"

	"github.com/syudead/vv/internal/domain"
)

// Fetch covers once with the execution HTTP client, outside the request-count limit; unavailable,
// oversized, or non-2xx responses omit the cover. Blocked addresses still fail execution; see
// docs/design-docs/work-info-plugins.md.
func (s *httpSession) coverImage(ctx context.Context, rawURL string) []byte {
	if s.client == nil || rawURL == "" {
		return nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, domain.WorkInfoPluginHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp")
	resp, err := s.client.Do(req)
	if err != nil {
		s.failure(req, err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, domain.WorkInfoCoverMaxBytes+1))
	if err != nil {
		s.failure(resp.Request, err)
		return nil
	}
	if len(body) > domain.WorkInfoCoverMaxBytes {
		return nil
	}
	return body
}
