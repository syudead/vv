package wasmplugin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/syudead/vv/internal/domain"
)

// Unknown response handles return hostInvalid from http_meta/http_body.
const (
	hostInvalid      int32 = -1
	hostRefused      int32 = -2
	hostUnreachable  int32 = -3
	hostTimedOut     int32 = -4
	hostTooMany      int32 = -5
	hostBodyTooLarge int32 = -6
)

// Default to a desktop-browser User-Agent because plugins fetch user-selected pages rather than
// crawl; callers may override it.
const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

// Drop Host and RFC 9110 section 7.6.1 hop-by-hop headers.
var droppedHeaders = map[string]bool{
	"Host": true, "Connection": true, "Keep-Alive": true, "Proxy-Connection": true,
	"Proxy-Authenticate": true, "Proxy-Authorization": true, "Te": true, "Trailer": true,
	"Transfer-Encoding": true, "Upgrade": true,
}

// Host functions use the execution's context-bound httpSession.
func hostModule(rt wazero.Runtime) wazero.HostModuleBuilder {
	i32 := api.ValueTypeI32
	b := rt.NewHostModuleBuilder(vvModule)
	b.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			stack[0] = api.EncodeI32(sessionFrom(ctx).send(ctx, mod, api.DecodeU32(stack[0]), api.DecodeU32(stack[1])))
		}), []api.ValueType{i32, i32}, []api.ValueType{i32}).
		WithParameterNames("req_ptr", "req_len").
		Export("http_send")
	read := map[string]func(*response) []byte{
		"http_meta": func(r *response) []byte { return r.meta },
		"http_body": func(r *response) []byte { return r.body },
	}
	for name, part := range read {
		b.NewFunctionBuilder().
			WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
				stack[0] = api.EncodeI32(sessionFrom(ctx).read(mod, api.DecodeI32(stack[0]), api.DecodeU32(stack[1]), api.DecodeU32(stack[2]), part))
			}), []api.ValueType{i32, i32, i32}, []api.ValueType{i32}).
			WithParameterNames("handle", "buf_ptr", "buf_cap").
			Export(name)
	}
	return b
}

type sessionKey struct{}

// Calls outside execution get an inert session that sends no requests.
func sessionFrom(ctx context.Context) *httpSession {
	if s, ok := ctx.Value(sessionKey{}).(*httpSession); ok {
		return s
	}
	return &httpSession{}
}

// Response handles index this session's responses; track blocked and first unreachable hosts.
type httpSession struct {
	client *http.Client
	// Empty when no proxy is configured.
	proxyHost string

	mu          sync.Mutex
	sent        int
	responses   []*response
	refused     string
	unreachable string
}

type response struct {
	meta []byte
	body []byte
	// status, url and header are what meta encodes, for the bundled plugins that read them in Go.
	status int
	url    *url.URL
	header http.Header
}

type sendRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    *string           `json:"body"`
}

type metaJSON struct {
	Status  int                 `json:"status"`
	URL     string              `json:"url"`
	Headers map[string][]string `json:"headers"`
}

// send returns a response handle or negative host error.
func (s *httpSession) send(ctx context.Context, mod api.Module, ptr, length uint32) int32 {
	raw, ok := mod.Memory().Read(ptr, length)
	if !ok || s.client == nil {
		return hostInvalid
	}
	req, ok := buildRequest(ctx, raw)
	if !ok {
		return hostInvalid
	}
	if !s.take() {
		return hostTooMany
	}
	reqCtx, cancel := context.WithTimeout(ctx, domain.WorkInfoPluginHTTPTimeout)
	defer cancel()
	resp, code := s.do(req.WithContext(reqCtx))
	if code < 0 {
		return code
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = append(s.responses, resp)
	return int32(len(s.responses) - 1)
}

func (s *httpSession) take() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sent >= domain.WorkInfoPluginHTTPMaxRequests {
		return false
	}
	s.sent++
	return true
}

// Only close per-execution proxy clients; direct clients are shared across executions.
func (s *httpSession) close() {
	if s.proxyHost != "" && s.client != nil {
		s.client.CloseIdleConnections()
	}
}

// Record blocked/unreachable hosts; proxy HTTP 407 counts as an unreachable request due to failed
// proxy authentication.
func (s *httpSession) do(req *http.Request) (*response, int32) {
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, s.failure(req, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if s.proxyHost != "" && resp.StatusCode == http.StatusProxyAuthRequired {
		s.recordUnreachable(s.proxyHost)
		return nil, hostUnreachable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, domain.WorkInfoPluginHTTPMaxBodyBytes+1))
	if err != nil {
		// After redirects, resp.Request identifies the host that actually supplied the body.
		return nil, s.failure(resp.Request, err)
	}
	if len(body) > domain.WorkInfoPluginHTTPMaxBodyBytes {
		return nil, hostBodyTooLarge
	}
	meta, err := json.Marshal(metaJSON{Status: resp.StatusCode, URL: resp.Request.URL.String(), Headers: resp.Header})
	if err != nil {
		return nil, hostUnreachable
	}
	return &response{meta: meta, body: body, status: resp.StatusCode, url: resp.Request.URL, header: resp.Header}, 0
}

func (s *httpSession) failure(req *http.Request, err error) int32 {
	var refused *refusedError
	switch {
	case errors.As(err, &refused):
		s.mu.Lock()
		if s.refused == "" {
			s.refused = refused.host
		}
		s.mu.Unlock()
		return hostRefused
	case errors.Is(req.Context().Err(), context.DeadlineExceeded):
		return hostTimedOut
	case s.proxyHost != "" && isProxyFailure(err):
		s.recordUnreachable(s.proxyHost)
	default:
		s.recordUnreachable(failedHost(req, err))
	}
	return hostUnreachable
}

// Keep only the first unreachable host.
func (s *httpSession) recordUnreachable(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unreachable == "" {
		s.unreachable = host
	}
}

// Report the redirect destination when failure occurred there.
func failedHost(req *http.Request, err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if u, perr := url.Parse(urlErr.URL); perr == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	return req.URL.Hostname()
}

// read always returns required length, writes only when capacity suffices, and returns -1 for
// invalid handles or memory.
func (s *httpSession) read(mod api.Module, handle int32, ptr, capacity uint32, part func(*response) []byte) int32 {
	s.mu.Lock()
	if handle < 0 || int(handle) >= len(s.responses) {
		s.mu.Unlock()
		return hostInvalid
	}
	data := part(s.responses[handle])
	s.mu.Unlock()
	if uint64(len(data)) <= uint64(capacity) && !mod.Memory().Write(ptr, data) {
		return hostInvalid
	}
	return int32(len(data))
}

func buildRequest(ctx context.Context, raw []byte) (*http.Request, bool) {
	var in sendRequest
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, false
	}
	if in.Method != http.MethodGet && in.Method != http.MethodPost {
		return nil, false
	}
	u, err := url.Parse(in.URL)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return nil, false
	}
	var body io.Reader
	if in.Body != nil {
		decoded, err := base64.StdEncoding.DecodeString(*in.Body)
		if err != nil {
			return nil, false
		}
		body = bytes.NewReader(decoded)
	}
	req, err := http.NewRequestWithContext(ctx, in.Method, u.String(), body)
	if err != nil {
		return nil, false
	}
	for name, value := range in.Headers {
		key := textproto.CanonicalMIMEHeaderKey(name)
		if droppedHeaders[key] || !validHeader(key, value) {
			continue
		}
		req.Header.Set(key, value)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}
	return req, true
}

// Drop malformed headers instead of letting net/http misclassify them as connection failures;
// values may contain HTAB but no other control characters.
func validHeader(name, value string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isTokenByte(name[i]) {
			return false
		}
	}
	for i := 0; i < len(value); i++ {
		if b := value[i]; b < ' ' && b != '\t' || b == 0x7f {
			return false
		}
	}
	return true
}

func isTokenByte(b byte) bool {
	switch {
	case 'a' <= b && b <= 'z', 'A' <= b && b <= 'Z', '0' <= b && b <= '9':
		return true
	}
	return strings.IndexByte("!#$%&'*+-.^_`|~", b) >= 0
}

// get sends a bundled plugin's GET under the request count, time and size limits and the address rule of http_send;
// nil stands for any of its error codes.
func (s *httpSession) get(ctx context.Context, rawURL, accept string) *response {
	if s.client == nil || !s.take() {
		return nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, domain.WorkInfoPluginHTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", accept)
	resp, code := s.do(req)
	if code < 0 {
		return nil
	}
	return resp
}
