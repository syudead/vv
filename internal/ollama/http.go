package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// kindError keeps the adapter's English message while letting callers match the domain cause.
type kindError struct {
	kind, cause error
	message     string
}

func (e *kindError) Error() string   { return e.message }
func (e *kindError) Unwrap() []error { return []error{e.kind, e.cause} }

func kindErrorf(kind, cause error, format string, args ...any) error {
	return &kindError{kind: kind, cause: cause, message: fmt.Sprintf(format, args...)}
}

// transportError tells a timeout from an endpoint that cannot be reached; a canceled request is neither.
func transportError(endpoint string, err error) error {
	var netErr net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("the request to Ollama at %s was canceled: %w", endpoint, err)
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return kindErrorf(domain.ErrOllamaTimeout, err, "Ollama at %s did not answer in time", endpoint)
	default:
		return kindErrorf(domain.ErrOllamaUnreachable, err, "cannot reach Ollama at %s: %v", endpoint, err)
	}
}

// postJSON reads at most maxResponseBytes of the answer; the caller interprets the status.
func postJSON(ctx context.Context, client *http.Client, endpoint, path string, body any) (int, []byte, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("cannot build the request to Ollama: %w", err)
	}
	url := strings.TrimRight(endpoint, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return 0, nil, fmt.Errorf("cannot build the request to Ollama: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, transportError(endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return 0, nil, transportError(endpoint, err)
	}
	return resp.StatusCode, raw, nil
}

// statusMessage exposes only Ollama's structured error message, because a raw body could reveal
// content from an unrelated service.
func statusMessage(status int, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return fmt.Sprintf("ollama answered %d", status)
	}
	if len(message) > 300 {
		message = message[:300]
	}
	return fmt.Sprintf("ollama answered %d: %s", status, message)
}
