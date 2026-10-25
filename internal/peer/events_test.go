package peer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// eventPeer answers /api/events with the given stream and holds it open until the request ends.
func eventPeer(stream string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events" || r.Header.Get("Authorization") != "VV-Peer vvp_secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"unauthenticated","reason":"peer_unknown","message":"x"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, stream)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}
}

// watchUntil runs Watch until want notifications arrived and returns them with Watch's error.
func watchUntil(t *testing.T, client *Client, target domain.PeerTarget, secret string, want int) ([]domain.PeerChanged, bool, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var (
		got    []domain.PeerChanged
		opened bool
	)
	err := client.Watch(ctx, target, secret, func() {
		opened = true
		if want == 0 {
			cancel()
		}
	}, func(changed domain.PeerChanged) {
		got = append(got, changed)
		if len(got) == want {
			cancel()
		}
	})
	return got, opened, err
}

func TestWatchForwardsEventsWithRemoteIDs(t *testing.T) {
	server := httptest.NewServer(eventPeer("retry: 3000\n\n" +
		"event: scan\ndata: {\"status\":\"idle\"}\n\n" +
		": keep-alive\n\n" +
		"event: video\ndata: {\"id\":7}\n\n" +
		// An id that is already remote on the peer names this node's own video and is not sent back.
		"event: group\ndata: {\"ids\":[3,1099511627779]}\n\n" +
		"event: smartFolder\ndata: {\"ids\":[4]}\n\n"))
	t.Cleanup(server.Close)

	got, opened, err := watchUntil(t, New(Options{}), domain.PeerTarget{Address: server.URL}, "vvp_secret", 3)
	if !opened || !errors.Is(err, context.Canceled) {
		t.Fatalf("opened = %v, err = %v", opened, err)
	}
	want := []domain.PeerChanged{
		{VideoIDs: []int64{domain.PeerIDOffset + 7}},
		{GroupIDs: []int64{domain.PeerIDOffset + 3}},
		{SmartFolderIDs: []int64{domain.PeerIDOffset + 4}},
	}
	if !slices.EqualFunc(got, want, func(a, b domain.PeerChanged) bool {
		return slices.Equal(a.VideoIDs, b.VideoIDs) && slices.Equal(a.GroupIDs, b.GroupIDs) &&
			slices.Equal(a.SmartFolderIDs, b.SmartFolderIDs)
	}) {
		t.Errorf("転送されたイベント = %+v, want %+v", got, want)
	}

	if _, opened, err := watchUntil(t, New(Options{}), domain.PeerTarget{Address: server.URL}, "vvp_other", 1); opened ||
		!errors.Is(err, domain.ErrPeerUnknown) {
		t.Errorf("知られていない資格情報: opened = %v, err = %v, want ErrPeerUnknown", opened, err)
	}
}

func TestWatchStopsOnAChangedPinnedCertificate(t *testing.T) {
	server := httptest.NewTLSServer(eventPeer(""))
	t.Cleanup(server.Close)
	fingerprint := Fingerprint(server.Certificate())

	_, opened, err := watchUntil(t, New(Options{}),
		domain.PeerTarget{Address: server.URL, PinnedSPKI: "previously-pinned"}, "vvp_secret", 0)
	var untrusted *domain.CertificateUntrustedError
	if opened || !errors.As(err, &untrusted) || untrusted.Fingerprint != fingerprint {
		t.Fatalf("変わった証明書: opened = %v, err = %v, want 指紋 %s の CertificateUntrustedError", opened, err, fingerprint)
	}

	// The fingerprint the owner confirmed is the new pin.
	_, opened, err = watchUntil(t, New(Options{}),
		domain.PeerTarget{Address: server.URL, PinnedSPKI: untrusted.Fingerprint}, "vvp_secret", 0)
	if !opened || !errors.Is(err, context.Canceled) {
		t.Errorf("確認した指紋: opened = %v, err = %v", opened, err)
	}
}
