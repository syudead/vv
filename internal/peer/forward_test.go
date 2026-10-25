package peer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeConnection struct {
	target   domain.PeerTarget
	ok       bool
	reported []error
}

func (c *fakeConnection) Connection(context.Context) (domain.PeerTarget, string, bool, error) {
	return c.target, "vvp_secret", c.ok, nil
}

func (c *fakeConnection) ReportFailure(_ context.Context, err error) {
	c.reported = append(c.reported, err)
}

func TestForwardSendsTheCredentialAndAudience(t *testing.T) {
	var got *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	conn := &fakeConnection{target: domain.PeerTarget{Address: server.URL}, ok: true}

	req, _ := http.NewRequest(http.MethodGet, "/api/videos/7?group=3", nil)
	resp, err := NewForwarder(New(Options{}), conn).Forward(req, true, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got.URL.String() != "/api/videos/7?group=3" || got.Header.Get("Authorization") != "VV-Peer vvp_secret" ||
		got.Header.Get("X-VV-Peer-Audience") != "guest" {
		t.Errorf("相手の受けた要求 = %s %v", got.URL, got.Header)
	}
}

func TestForwardReportsAPeerThatNoLongerKnowsThisNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthenticated","reason":"peer_unknown","message":"x"}`))
	}))
	t.Cleanup(server.Close)
	conn := &fakeConnection{target: domain.PeerTarget{Address: server.URL}, ok: true}
	forwarder := NewForwarder(New(Options{}), conn)

	req, _ := http.NewRequest(http.MethodGet, "/api/videos/7", nil)
	if _, err := forwarder.Forward(req, false, false); !errors.Is(err, domain.ErrPeerUnreachable) {
		t.Errorf("err = %v, want ErrPeerUnreachable", err)
	}
	if len(conn.reported) != 1 || !errors.Is(conn.reported[0], domain.ErrPeerUnknown) {
		t.Errorf("報告 = %v, want ErrPeerUnknown を 1 回", conn.reported)
	}

	conn.ok = false
	if _, err := forwarder.Forward(req, false, false); !errors.Is(err, domain.ErrPeerUnreachable) || len(conn.reported) != 1 {
		t.Errorf("接続していない間: err = %v, 報告 = %v, want 送らずに ErrPeerUnreachable", err, conn.reported)
	}
}
