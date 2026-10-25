package peer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const (
	peerScheme         = "VV-Peer"
	peerAudienceHeader = "X-VV-Peer-Audience"
)

// Connection is app.Pairing: how to reach the peer while it is connected, and where a failed request goes
// so the state becomes unreachable at once.
type Connection interface {
	Connection(ctx context.Context) (target domain.PeerTarget, secret string, ok bool, err error)
	ReportFailure(ctx context.Context, err error)
}

// Forwarder sends the screens' requests for the peer's entities to the peer.
type Forwarder struct {
	client *Client
	conn   Connection
}

func NewForwarder(client *Client, conn Connection) *Forwarder {
	return &Forwarder{client: client, conn: conn}
}

// Forward sends req, whose URL holds the peer's path and query only, with the peer credential, lowered to
// the guest audience when guest is set; media has no response timeout. A peer that is not connected, gives
// no response or no longer knows this node answers *domain.PeerUnreachableError. Any other response is the
// caller's to close, and a failed read of its body is reported like a failed request.
func (f *Forwarder) Forward(req *http.Request, guest, media bool) (*http.Response, error) {
	ctx := req.Context()
	target, secret, ok, err := f.conn.Connection(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &domain.PeerUnreachableError{Message: "The other VVMDM is not connected."}
	}
	base, err := url.Parse(target.Address)
	if err != nil || base.Host == "" {
		return nil, &domain.PeerUnreachableError{Message: "The address of the other VVMDM is not valid.", Cause: err}
	}
	out := req.WithContext(ctx)
	out.URL = &url.URL{
		Scheme: base.Scheme, Host: base.Host, Path: req.URL.Path, RawPath: req.URL.RawPath, RawQuery: req.URL.RawQuery,
	}
	out.Host, out.RequestURI = "", ""
	out.Header = req.Header.Clone()
	if out.Header == nil {
		out.Header = http.Header{}
	}
	out.Header.Set("Authorization", peerScheme+" "+secret)
	out.Header.Del(peerAudienceHeader)
	if guest {
		out.Header.Set(peerAudienceHeader, domain.AudienceGuest.String())
	}
	bounds := callLimits
	if media {
		bounds = mediaLimits
	}
	resp, err := f.client.do(out, target, bounds)
	if err != nil {
		f.report(ctx, err)
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && peerUnknown(resp) {
		f.report(ctx, domain.ErrPeerUnknown)
		return nil, &domain.PeerUnreachableError{Message: "The other VVMDM no longer knows this one."}
	}
	resp.Body = &reportingBody{ReadCloser: resp.Body, ctx: ctx, report: f.report}
	return resp, nil
}

// peerUnknown reads and closes the body of a 401.
func peerUnknown(resp *http.Response) bool {
	defer func() { _ = resp.Body.Close() }()
	var answer gen.Error
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	_ = json.Unmarshal(body, &answer)
	return answer.Reason != nil && *answer.Reason == gen.ErrorReasonPeerUnknown
}

// report skips a request its caller abandoned, which says nothing about the peer.
func (f *Forwarder) report(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	f.conn.ReportFailure(context.WithoutCancel(ctx), err)
}

// reportingBody reports the first failed read that was not the caller leaving.
type reportingBody struct {
	io.ReadCloser
	ctx    context.Context
	report func(context.Context, error)
	once   sync.Once
}

func (b *reportingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		b.once.Do(func() {
			b.report(b.ctx, &domain.PeerUnreachableError{Message: "The other VVMDM stopped answering.", Cause: err})
		})
	}
	return n, err
}
