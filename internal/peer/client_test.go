package peer

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestVersionVerifiesTheCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","version":"1.2.0"}`))
	}))
	t.Cleanup(server.Close)
	ctx := context.Background()
	fingerprint := Fingerprint(server.Certificate())

	// The test certificate is not in the system roots.
	_, err := New(Options{}).Version(ctx, domain.PeerTarget{Address: server.URL})
	var untrusted *domain.CertificateUntrustedError
	if !errors.As(err, &untrusted) || untrusted.Fingerprint != fingerprint {
		t.Fatalf("システムのルートでの err = %v, want 指紋 %s の CertificateUntrustedError", err, fingerprint)
	}

	if version, err := New(Options{}).Version(ctx, domain.PeerTarget{Address: server.URL, PinnedSPKI: fingerprint}); err != nil || version != "1.2.0" {
		t.Errorf("固定した指紋で Version = %q, %v", version, err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	if version, err := New(Options{RootCAs: roots}).Version(ctx, domain.PeerTarget{Address: server.URL}); err != nil || version != "1.2.0" {
		t.Errorf("信頼したルートで Version = %q, %v", version, err)
	}
	// A pin the certificate no longer matches refuses it even though the roots verify it.
	_, err = New(Options{RootCAs: roots}).Version(ctx, domain.PeerTarget{Address: server.URL, PinnedSPKI: "changed"})
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
	var version *domain.UnsupportedVersionError
	// The peer names its own version local; seen from the sender they swap.
	if !errors.As(err, &version) || version.Local != "1.3.0" || version.Peer != "1.2.0" {
		t.Errorf("err = %v, want Local 1.3.0, Peer 1.2.0", err)
	}

	server.Close()
	if _, err := New(Options{}).Version(context.Background(), domain.PeerTarget{Address: server.URL}); !errors.Is(err, domain.ErrPeerUnreachable) {
		t.Errorf("止まった相手の err = %v, want ErrPeerUnreachable", err)
	}
}
