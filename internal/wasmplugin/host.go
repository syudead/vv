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

// http_send の誤りの値である（contracts/plugin-contract.md の Host functions）。http_meta と
// http_body は知らないハンドルに hostInvalid を返す。
const (
	hostInvalid      int32 = -1
	hostRefused      int32 = -2
	hostUnreachable  int32 = -3
	hostTimedOut     int32 = -4
	hostTooMany      int32 = -5
	hostBodyTooLarge int32 = -6
	defaultUserAgent       = "vv-work-info-plugin/1"
)

// droppedHeaders はプラグインが付けても送らない要求のヘッダーである。Host と、ホップバイホップの
// ヘッダー（RFC 9110 の 7.6.1）を落とす。
var droppedHeaders = map[string]bool{
	"Host": true, "Connection": true, "Keep-Alive": true, "Proxy-Connection": true,
	"Proxy-Authenticate": true, "Proxy-Authorization": true, "Te": true, "Trailer": true,
	"Transfer-Encoding": true, "Upgrade": true,
}

// hostModule は vv のホストモジュール（http_send、http_meta、http_body）を組み立てる。
// どの関数も、実行の ctx が持つ httpSession を使う。
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

// sessionFrom は ctx の httpSession を返す。無ければ（実行の外からの呼び出し）要求を何も
// 送らない空の httpSession を返す。
func sessionFrom(ctx context.Context) *httpSession {
	if s, ok := ctx.Value(sessionKey{}).(*httpSession); ok {
		return s
	}
	return &httpSession{}
}

// httpSession は 1 回の実行の HTTP の状態である。送った要求の数、応答（ハンドルはその添字）、
// 拒んだホストと、最初に届かなかったホストを持つ。
type httpSession struct {
	net *netClient

	mu          sync.Mutex
	sent        int
	responses   []*response
	refused     string
	unreachable string
}

// response はプラグインに渡す応答で、meta は http_meta の JSON である。
type response struct {
	meta []byte
	body []byte
}

// sendRequest は http_send の要求の JSON である。
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

// send は http_send である。要求を組み立て、上限の中で送り、ハンドルか負の誤りを返す。
func (s *httpSession) send(ctx context.Context, mod api.Module, ptr, length uint32) int32 {
	raw, ok := mod.Memory().Read(ptr, length)
	if !ok || s.net == nil {
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

// take は要求の数を 1 つ数え、上限を超えていれば false を返す。
func (s *httpSession) take() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sent >= domain.WorkInfoPluginHTTPMaxRequests {
		return false
	}
	s.sent++
	return true
}

// do は要求を送って本文を読み、応答か負の誤りを返す。拒んだホストと、届かなかったホストを記録する。
func (s *httpSession) do(req *http.Request) (*response, int32) {
	resp, err := s.net.client.Do(req)
	if err != nil {
		return nil, s.failure(req, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, domain.WorkInfoPluginHTTPMaxBodyBytes+1))
	if err != nil {
		// resp.Request はリダイレクトの最後の要求で、本文を送ってきたホストを持つ。
		return nil, s.failure(resp.Request, err)
	}
	if len(body) > domain.WorkInfoPluginHTTPMaxBodyBytes {
		return nil, hostBodyTooLarge
	}
	meta, err := json.Marshal(metaJSON{Status: resp.StatusCode, URL: resp.Request.URL.String(), Headers: resp.Header})
	if err != nil {
		return nil, hostUnreachable
	}
	return &response{meta: meta, body: body}, 0
}

// failure は送るか読むかの誤りを http_send の誤りの値にする。
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
	}
	s.mu.Lock()
	if s.unreachable == "" {
		s.unreachable = failedHost(req, err)
	}
	s.mu.Unlock()
	return hostUnreachable
}

// failedHost は失敗した要求のホストを返す。リダイレクトの先で失敗したときは、その先のホストである。
func failedHost(req *http.Request, err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if u, perr := url.Parse(urlErr.URL); perr == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	return req.URL.Hostname()
}

// read は http_meta と http_body である。part の中身を、buf_cap に収まるときだけ書き、常に
// 全長を返す。知らないハンドルと、書けない場所には -1 を返す。
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

// buildRequest は http_send の要求の JSON を *http.Request にする。method、scheme、URL、
// base64 の本文のどれかが取り決めに合わなければ false を返す。
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

// validHeader はヘッダーの名前が RFC 9110 の token で、値に HTAB 以外の制御文字が無いかを
// 確かめる。net/http はこれに外れるヘッダーの要求を送らずに失敗させるので、そのヘッダーは
// 落とす（接続の失敗の -3 にしない）。
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

// isTokenByte は b が RFC 9110 の tchar かを返す。
func isTokenByte(b byte) bool {
	switch {
	case 'a' <= b && b <= 'z', 'A' <= b && b <= 'Z', '0' <= b && b <= '9':
		return true
	}
	return strings.IndexByte("!#$%&'*+-.^_`|~", b) >= 0
}
