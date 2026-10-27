package peer

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestHandshakeVerifiesTheCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"node":{"id":"n","name":"peer","version":"1.2.0"}}`))
	}))
	t.Cleanup(server.Close)
	ctx := context.Background()
	fingerprint := Fingerprint(server.Certificate())

	// The test certificate is not in the system roots.
	_, err := New(Options{}).Handshake(ctx, domain.PeerTarget{Address: server.URL}, "vvp_secret")
	var untrusted *domain.CertificateUntrustedError
	if !errors.As(err, &untrusted) || untrusted.Fingerprint != fingerprint {
		t.Fatalf("システムのルートでの err = %v, want 指紋 %s の CertificateUntrustedError", err, fingerprint)
	}

	if node, err := New(Options{}).Handshake(ctx, domain.PeerTarget{Address: server.URL, PinnedSPKI: fingerprint}, "vvp_secret"); err != nil || node.Version != "1.2.0" {
		t.Errorf("固定した指紋で Handshake = %+v, %v", node, err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	if node, err := New(Options{RootCAs: roots}).Handshake(ctx, domain.PeerTarget{Address: server.URL}, "vvp_secret"); err != nil || node.Version != "1.2.0" {
		t.Errorf("信頼したルートで Handshake = %+v, %v", node, err)
	}
	// The roots trust the certificate, but it does not name localhost.
	byName := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	_, err = New(Options{RootCAs: roots}).Handshake(ctx, domain.PeerTarget{Address: byName}, "vvp_secret")
	if !errors.As(err, &untrusted) || untrusted.Fingerprint != fingerprint {
		t.Errorf("証明書にない名前での err = %v, want 指紋 %s の CertificateUntrustedError", err, fingerprint)
	}
	// A pin the certificate no longer matches refuses it even though the roots verify it.
	_, err = New(Options{RootCAs: roots}).Handshake(ctx, domain.PeerTarget{Address: server.URL, PinnedSPKI: "changed"}, "vvp_secret")
	if !errors.As(err, &untrusted) || untrusted.Fingerprint != fingerprint {
		t.Errorf("一致しない固定と信頼したルートでの err = %v, want 指紋 %s の CertificateUntrustedError", err, fingerprint)
	}
}

func TestRefusalMapsThePeerReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"conflict","reason":"unsupported_version","message":"x","localVersion":"1.2.0","peerVersion":"1.3.0"}`))
	}))
	t.Cleanup(server.Close)

	_, err := New(Options{}).RequestPairing(context.Background(), domain.PeerTarget{Address: server.URL}, domain.PairingRequest{})
	// A build that still compared versions refuses this one; the owner is told to update it.
	var unreachable *domain.PeerUnreachableError
	if !errors.As(err, &unreachable) || !strings.Contains(unreachable.Message, "older build") {
		t.Errorf("err = %v, want 古い版を更新するよう伝える PeerUnreachableError", err)
	}

	server.Close()
	if _, err := New(Options{}).Handshake(context.Background(), domain.PeerTarget{Address: server.URL}, "vvp_secret"); !errors.Is(err, domain.ErrPeerUnreachable) {
		t.Errorf("止まった相手の err = %v, want ErrPeerUnreachable", err)
	}
}

// The sender's own fingerprint travels in node.fingerprint, and the receiver refusing this node's certificate
// comes back with the fingerprint it saw, not as the receiver's own untrusted certificate.
func TestRequestPairingSendsTheOwnFingerprint(t *testing.T) {
	var sent gen.PairingRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"conflict","reason":"certificate_untrusted","message":"x","fingerprint":"seen"}`))
	}))
	t.Cleanup(server.Close)

	_, err := New(Options{}).RequestPairing(context.Background(), domain.PeerTarget{Address: server.URL},
		domain.PairingRequest{Address: "https://b.local", Fingerprint: "own"})
	if sent.Node.Fingerprint == nil || *sent.Node.Fingerprint != "own" {
		t.Errorf("送った node.fingerprint = %v, want own", sent.Node.Fingerprint)
	}
	var own *domain.OwnCertificateUntrustedError
	var untrusted *domain.CertificateUntrustedError
	if !errors.As(err, &own) || own.Fingerprint != "seen" || errors.As(err, &untrusted) || !errors.Is(err, domain.ErrPeerUnreachable) {
		t.Errorf("err = %v, want 指紋 seen の OwnCertificateUntrustedError", err)
	}
}

// Reading the certificate of this node's own address completes only the TLS handshake: whoever terminates it
// receives no request, so no secret, and the roots decide whether the owner must confirm the fingerprint.
func TestCertificateSendsNoRequest(t *testing.T) {
	var requested atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requested.Store(true) }))
	t.Cleanup(server.Close)
	ctx := context.Background()
	want := Fingerprint(server.Certificate())

	presented, err := New(Options{}).Certificate(ctx, server.URL)
	if err != nil || presented != (domain.PresentedCertificate{Fingerprint: want}) {
		t.Errorf("システムのルートで Certificate = %+v, %v, want 検証されない指紋 %s", presented, err, want)
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	if presented, err := New(Options{RootCAs: roots}).Certificate(ctx, server.URL); err != nil || !presented.Verified {
		t.Errorf("信頼したルートで Certificate = %+v, %v, want 検証済み", presented, err)
	}
	server.Close()
	if requested.Load() {
		t.Error("Certificate が HTTP の要求を送った")
	}
	if _, err := New(Options{}).Certificate(ctx, server.URL); !errors.Is(err, domain.ErrPeerUnreachable) {
		t.Errorf("止まったアドレスの err = %v, want ErrPeerUnreachable", err)
	}
}
