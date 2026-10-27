// Package peer is the HTTP client to the paired VVMDM. It reaches the peer's screen API and its
// /api/peer operations, verifies HTTPS with the system roots or a pinned SPKI, and reports failures
// as the domain's pairing errors.
package peer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const (
	connectTimeout  = 5 * time.Second
	responseTimeout = 10 * time.Second
	maxResponseSize = 1 << 20
)

type Options struct {
	// Nil verifies with the system roots; tests pass their own.
	RootCAs *x509.CertPool
}

type Client struct {
	roots *x509.CertPool
}

func New(opts Options) *Client {
	return &Client{roots: opts.RootCAs}
}

// Fingerprint is the base64 SHA-256 of a certificate's SPKI, as the owner compares and pins it.
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// limits bound one request: total covers the whole response and header its headers; zero leaves either
// unbounded.
type limits struct {
	total, header time.Duration
}

var (
	// A call reads one small answer.
	callLimits = limits{total: responseTimeout, header: responseTimeout}
	// The event stream stays open, but the peer answers its headers at once.
	streamLimits = limits{header: responseTimeout}
	// Media has no response timeout: a live transcode answers its headers only after the first output.
	mediaLimits = limits{}
)

// httpClient is built per target because the pin is part of the TLS check.
func (c *Client) httpClient(target domain.PeerTarget, bounds limits) *http.Client {
	dialer := &net.Dialer{Timeout: connectTimeout}
	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: connectTimeout,
		// The peer is on the LAN; an environment proxy would not reach it.
		Proxy:                 nil,
		TLSClientConfig:       tlsConfig(target.PinnedSPKI, c.roots),
		ResponseHeaderTimeout: bounds.header,
		DisableKeepAlives:     true,
	}
	return &http.Client{Transport: transport, Timeout: bounds.total}
}

// tlsConfig verifies an unpinned peer with the roots and its host name. A confirmed pin replaces
// the roots, so only a pinned peer skips the chain check, and VerifyConnection compares its SPKI.
func tlsConfig(pinned string, roots *x509.CertPool) *tls.Config {
	pinnedOnly := pinned != ""
	config := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, InsecureSkipVerify: pinnedOnly}
	if pinnedOnly {
		config.VerifyConnection = verifyPin(pinned)
	}
	return config
}

func verifyPin(pinned string) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("the peer presented no certificate")
		}
		// A changed certificate stops until the owner confirms it again.
		if fingerprint := Fingerprint(state.PeerCertificates[0]); fingerprint != pinned {
			return &domain.CertificateUntrustedError{Fingerprint: fingerprint}
		}
		return nil
	}
}

// Certificate completes a TLS handshake with the https address and closes the connection without sending a
// request, so nothing reaches whoever terminates it; it reports whether the roots verify the certificate.
func (c *Client) Certificate(ctx context.Context, address string) (domain.PresentedCertificate, error) {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return domain.PresentedCertificate{}, &domain.PeerUnreachableError{Message: "Not an https address: " + address + "."}
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: connectTimeout},
		Config:    &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: c.roots, ServerName: parsed.Hostname()},
	}
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(parsed.Hostname(), port))
	if err == nil {
		defer func() { _ = conn.Close() }()
		if secure, ok := conn.(*tls.Conn); ok && len(secure.ConnectionState().PeerCertificates) > 0 {
			leaf := secure.ConnectionState().PeerCertificates[0]
			return domain.PresentedCertificate{Fingerprint: Fingerprint(leaf), Verified: true}, nil
		}
		return domain.PresentedCertificate{}, &domain.PeerUnreachableError{Message: address + " presented no certificate."}
	}
	// A failed verification still carries the certificate it rejected, so its fingerprint needs no skipped check.
	var unverified *tls.CertificateVerificationError
	if errors.As(err, &unverified) && len(unverified.UnverifiedCertificates) > 0 {
		return domain.PresentedCertificate{Fingerprint: Fingerprint(unverified.UnverifiedCertificates[0])}, nil
	}
	return domain.PresentedCertificate{}, &domain.PeerUnreachableError{
		Message: "Could not connect to " + address + ".", Cause: err,
	}
}

// Version reads the release version the peer's health reports, also while its storage is degraded.
func (c *Client) Version(ctx context.Context, target domain.PeerTarget) (string, error) {
	var health gen.Health
	if err := c.call(ctx, target, http.MethodGet, "/api/health", "", nil, &health,
		http.StatusOK, http.StatusServiceUnavailable); err != nil {
		return "", err
	}
	return health.Version, nil
}

func (c *Client) RequestPairing(
	ctx context.Context, target domain.PeerTarget, req domain.PairingRequest,
) (domain.PairingAnswer, error) {
	body := gen.PairingRequest{
		Code: req.Code,
		Node: gen.PairingRequestNode{
			Id: req.Node.ID, Name: req.Node.Name, Address: req.Address, Version: req.Node.Version,
		},
		Secret: req.Secret,
	}
	if req.Fingerprint != "" {
		body.Node.Fingerprint = &req.Fingerprint
	}
	var answer gen.PairingAnswer
	if err := c.call(ctx, target, http.MethodPost, "/api/peer/pairings", "", body, &answer,
		http.StatusCreated); err != nil {
		return domain.PairingAnswer{}, err
	}
	return domain.PairingAnswer{Node: nodeFrom(answer.Node), Secret: answer.Secret}, nil
}

func (c *Client) Handshake(ctx context.Context, target domain.PeerTarget, secret string) (domain.PeerNode, error) {
	var answer gen.PeerHandshake
	if err := c.call(ctx, target, http.MethodGet, "/api/peer/handshake", secret, nil, &answer,
		http.StatusOK); err != nil {
		return domain.PeerNode{}, err
	}
	return nodeFrom(answer.Node), nil
}

func nodeFrom(node gen.PeerNode) domain.PeerNode {
	return domain.PeerNode{ID: node.Id, Name: node.Name, Version: node.Version}
}

// Forget tells the peer that the node nodeID unpaired.
func (c *Client) Forget(ctx context.Context, target domain.PeerTarget, secret, nodeID string) error {
	return c.call(ctx, target, http.MethodDelete, "/api/peer/pairings/"+url.PathEscape(nodeID), secret, nil, nil,
		http.StatusNoContent)
}

// call sends one request and decodes a response whose status is in accepted into out, unless out is nil.
func (c *Client) call(
	ctx context.Context, target domain.PeerTarget, method, path, secret string, in, out any, accepted ...int,
) error {
	resp, err := c.send(ctx, target, method, path, secret, in, callLimits)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return &domain.PeerUnreachableError{
			Message: "The other VVMDM at " + target.Address + " stopped answering.", Cause: err,
		}
	}
	for _, status := range accepted {
		if resp.StatusCode != status {
			continue
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(body, out); err != nil {
			return &domain.PeerUnreachableError{
				Message: "The other VVMDM at " + target.Address + " answered in an unexpected form.", Cause: err,
			}
		}
		return nil
	}
	return refusal(resp.StatusCode, body, target.Address)
}

// send returns the peer's response with any status; a request that got no response answers the
// domain's pairing error.
func (c *Client) send(
	ctx context.Context, target domain.PeerTarget, method, path, secret string, in any, bounds limits,
) (*http.Response, error) {
	base, err := url.Parse(target.Address)
	if err != nil || base.Host == "" {
		return nil, &domain.PeerUnreachableError{Message: "The address of the other VVMDM is not valid.", Cause: err}
	}
	var payload io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return nil, fmt.Errorf("cannot encode the request to the peer: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, base.JoinPath(path).String(), payload)
	if err != nil {
		return nil, &domain.PeerUnreachableError{Message: "The address of the other VVMDM is not valid.", Cause: err}
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if secret != "" {
		req.Header.Set("Authorization", peerScheme+" "+secret)
	}
	return c.do(req, target, bounds)
}

// do answers the domain's pairing error for a request that got no response.
func (c *Client) do(req *http.Request, target domain.PeerTarget, bounds limits) (*http.Response, error) {
	resp, err := c.httpClient(target, bounds).Do(req)
	if err != nil {
		var untrusted *domain.CertificateUntrustedError
		if errors.As(err, &untrusted) {
			return nil, untrusted
		}
		// The owner compares the fingerprint of a certificate the roots or the host name did not verify.
		var unverified *tls.CertificateVerificationError
		if errors.As(err, &unverified) && len(unverified.UnverifiedCertificates) > 0 {
			return nil, &domain.CertificateUntrustedError{Fingerprint: Fingerprint(unverified.UnverifiedCertificates[0])}
		}
		return nil, &domain.PeerUnreachableError{
			Message: "Could not reach the other VVMDM at " + target.Address + ".", Cause: err,
		}
	}
	return resp, nil
}

// refusal maps the peer's error body to the domain error of the same meaning.
func refusal(status int, body []byte, address string) error {
	var answer gen.Error
	_ = json.Unmarshal(body, &answer)
	reason := gen.ErrorReason("")
	if answer.Reason != nil {
		reason = *answer.Reason
	}
	switch reason {
	case gen.ErrorReasonInvalidCode:
		return domain.ErrInvalidPairingCode
	case gen.ErrorReasonAlreadyPaired:
		return domain.ErrAlreadyPaired
	case gen.ErrorReasonPeerUnknown:
		return domain.ErrPeerUnknown
	case gen.ErrorReasonUnsupportedVersion:
		// The peer names itself local; seen from here the two swap.
		return &domain.UnsupportedVersionError{Local: deref(answer.PeerVersion), Peer: deref(answer.LocalVersion)}
	case gen.ErrorReasonPeerUnreachable:
		return &domain.PeerUnreachableError{Message: "The other VVMDM could not reach this one: " + answer.Message}
	case gen.ErrorReasonCertificateUntrusted:
		// The peer names the certificate it saw at this node's address, which only the owner of this node can judge.
		return &domain.OwnCertificateUntrustedError{Fingerprint: deref(answer.Fingerprint)}
	}
	return &domain.PeerUnreachableError{
		Message: "The other VVMDM at " + address + " answered with status " + strconv.Itoa(status) + ".",
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
