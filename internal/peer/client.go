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

// httpClient is built per target because the pin is part of the TLS check.
func (c *Client) httpClient(target domain.PeerTarget, host string) *http.Client {
	dialer := &net.Dialer{Timeout: connectTimeout}
	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: connectTimeout,
		// The peer is on the LAN; an environment proxy would not reach it.
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// VerifyConnection below replaces the default check so a failure can name the fingerprint.
			InsecureSkipVerify: true,
			VerifyConnection:   verifyPeer(host, target.PinnedSPKI, c.roots),
		},
		ResponseHeaderTimeout: responseTimeout,
		DisableKeepAlives:     true,
	}
	return &http.Client{Transport: transport, Timeout: responseTimeout}
}

func verifyPeer(host, pinned string, roots *x509.CertPool) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("the peer presented no certificate")
		}
		leaf := state.PeerCertificates[0]
		fingerprint := Fingerprint(leaf)
		// A confirmed pin replaces the roots: a changed certificate stops until the owner confirms it again.
		if pinned != "" {
			if fingerprint != pinned {
				return &domain.CertificateUntrustedError{Fingerprint: fingerprint}
			}
			return nil
		}
		intermediates := x509.NewCertPool()
		for _, cert := range state.PeerCertificates[1:] {
			intermediates.AddCert(cert)
		}
		if _, err := leaf.Verify(x509.VerifyOptions{
			Roots: roots, Intermediates: intermediates, DNSName: host,
		}); err != nil {
			return &domain.CertificateUntrustedError{Fingerprint: fingerprint}
		}
		return nil
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

// call sends one request and decodes a response whose status is in accepted into out.
func (c *Client) call(
	ctx context.Context, target domain.PeerTarget, method, path, secret string, in, out any, accepted ...int,
) error {
	base, err := url.Parse(target.Address)
	if err != nil || base.Host == "" {
		return &domain.PeerUnreachableError{Message: "The address of the other VVMDM is not valid.", Cause: err}
	}
	var payload io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("cannot encode the request to the peer: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, base.JoinPath(path).String(), payload)
	if err != nil {
		return &domain.PeerUnreachableError{Message: "The address of the other VVMDM is not valid.", Cause: err}
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if secret != "" {
		req.Header.Set("Authorization", "VV-Peer "+secret)
	}
	resp, err := c.httpClient(target, base.Hostname()).Do(req)
	if err != nil {
		var untrusted *domain.CertificateUntrustedError
		if errors.As(err, &untrusted) {
			return untrusted
		}
		return &domain.PeerUnreachableError{
			Message: "Could not reach the other VVMDM at " + target.Address + ".", Cause: err,
		}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return &domain.PeerUnreachableError{
			Message: "The other VVMDM at " + target.Address + " stopped answering.", Cause: err,
		}
	}
	for _, status := range accepted {
		if resp.StatusCode == status {
			if err := json.Unmarshal(body, out); err != nil {
				return &domain.PeerUnreachableError{
					Message: "The other VVMDM at " + target.Address + " answered in an unexpected form.", Cause: err,
				}
			}
			return nil
		}
	}
	return refusal(resp.StatusCode, body, target.Address)
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
		return &domain.PeerUnreachableError{
			Message: "The other VVMDM does not trust the certificate of this one (SPKI " + deref(answer.Fingerprint) + ").",
		}
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
