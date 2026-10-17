package wasmplugin

import (
	"context"
	"io"
	"net/http"

	"github.com/syudead/vv/internal/domain"
)

// coverImage はプラグインが返したカバー画像の URL rawURL を、プラグインの要求と同じクライアントで 1 回 GET し、
// 2xx の本文を返す（docs/design-docs/work-info-plugins.md のカバー画像の節）。届かないか、2xx でないか、本文が
// domain.WorkInfoCoverMaxBytes を越えれば nil で、カバー画像は無いものとする。拒んだアドレスはプラグインの
// 要求と同じく記録し、実行は blocked_address で失敗する。要求の数には数えない。
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
