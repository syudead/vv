package peer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// fakeConnection holds one link in slot 2.
type fakeConnection struct {
	target   domain.PeerTarget
	ok       bool
	excluded []int64
	reported []error
}

const fakeSlot = 2

func (c *fakeConnection) Connection(_ context.Context, slot int64) (domain.PeerTarget, string, bool, error) {
	if slot != fakeSlot {
		return domain.PeerTarget{}, "", false, domain.ErrNotFound
	}
	return c.target, "vvp_secret", c.ok, nil
}

func (c *fakeConnection) ExcludedFolders(context.Context, int64) ([]int64, error) {
	return c.excluded, nil
}

func (c *fakeConnection) ReportFailure(_ context.Context, slot int64, err error) {
	if slot == fakeSlot {
		c.reported = append(c.reported, err)
	}
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
	resp, err := NewForwarder(New(Options{}), conn).Forward(req, fakeSlot, true, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got.URL.String() != "/api/videos/7?group=3" || got.Header.Get("Authorization") != "VV-Peer vvp_secret" ||
		got.Header.Get("X-VV-Peer-Audience") != "guest" {
		t.Errorf("相手の受けた要求 = %s %v", got.URL, got.Header)
	}
}

// The link's switched-off folders replace whatever the incoming request carried, and none sends no header.
func TestForwardSendsTheLinksSwitchedOffFolders(t *testing.T) {
	var got []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Values("X-VV-Peer-Excluded-Folders")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	conn := &fakeConnection{target: domain.PeerTarget{Address: server.URL}, ok: true}
	forwarder := NewForwarder(New(Options{}), conn)
	for _, tc := range []struct {
		excluded []int64
		want     []string
	}{
		{[]int64{3, 5}, []string{"3,5"}},
		{nil, nil},
	} {
		conn.excluded = tc.excluded
		req, _ := http.NewRequest(http.MethodGet, "/api/videos", nil)
		req.Header.Set("X-VV-Peer-Excluded-Folders", "9")
		resp, err := forwarder.Forward(req, fakeSlot, false, false)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if !slices.Equal(got, tc.want) {
			t.Errorf("オフ %v: 相手の受けた見出し = %q, want %q", tc.excluded, got, tc.want)
		}
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
	if _, err := forwarder.Forward(req, fakeSlot, false, false); !errors.Is(err, domain.ErrPeerUnreachable) {
		t.Errorf("err = %v, want ErrPeerUnreachable", err)
	}
	if len(conn.reported) != 1 || !errors.Is(conn.reported[0], domain.ErrPeerUnknown) {
		t.Errorf("報告 = %v, want ErrPeerUnknown を 1 回", conn.reported)
	}

	conn.ok = false
	if _, err := forwarder.Forward(req, fakeSlot, false, false); !errors.Is(err, domain.ErrPeerUnreachable) || len(conn.reported) != 1 {
		t.Errorf("接続していない間: err = %v, 報告 = %v, want 送らずに ErrPeerUnreachable", err, conn.reported)
	}
	if _, err := forwarder.Forward(req, fakeSlot+1, false, false); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("リンクの無いスロット: err = %v, want ErrNotFound", err)
	}
}
