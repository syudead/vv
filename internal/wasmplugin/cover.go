package wasmplugin

import (
	"context"
	"io"
	"net/http"

	"github.com/syudead/vv/internal/domain"
)

// Fetch the cover and people's images with the execution HTTP client, outside the request-count limit;
// unavailable, oversized, or non-2xx responses omit the image. Blocked addresses still fail execution; see
// docs/design-docs/work-info-plugins.md.
func (s *httpSession) image(ctx context.Context, rawURL string) []byte {
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

// images fetches urls one after another within ctx, up to limit or PersonPluginImagesMaxCount when limit is zero, and
// returns nil without URLs.
func (s *httpSession) images(ctx context.Context, urls []string, limit int) [][]byte {
	if limit <= 0 || limit > domain.PersonPluginImagesMaxCount {
		limit = domain.PersonPluginImagesMaxCount
	}
	urls = urls[:min(len(urls), limit)]
	if len(urls) == 0 {
		return nil
	}
	out := make([][]byte, len(urls))
	for i, url := range urls {
		out[i] = s.image(ctx, url)
	}
	return out
}

// personImages fetches the first WorkInfoPersonImagesMaxCount image URLs in result order, one after another within
// ctx, and returns nil when no person has one.
func (s *httpSession) personImages(ctx context.Context, people []domain.FetchedPerson) [][]byte {
	var out [][]byte
	fetched := 0
	for i, person := range people {
		if person.ImageURL == "" {
			continue
		}
		if fetched == domain.WorkInfoPersonImagesMaxCount {
			break
		}
		fetched++
		if out == nil {
			out = make([][]byte, len(people))
		}
		out[i] = s.image(ctx, person.ImageURL)
	}
	return out
}
