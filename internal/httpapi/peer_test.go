package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/peer"
	"github.com/syudead/vv/internal/store"
)

const testPeerVersion = "1.2.0"

// audienceLibrary records the audience the boundary handed to the store.
type audienceLibrary struct {
	*fakeLibrary
	mu   sync.Mutex
	seen []domain.Audience
	// held, when set, keeps the next listing open until its request is canceled and reports why it ended.
	held *heldListing
}

type heldListing struct {
	entered chan struct{}
	ended   chan error
}

func (l *audienceLibrary) ListVideos(ctx context.Context, audience domain.Audience, _ domain.VideoQuery) (domain.VideoPage, error) {
	l.mu.Lock()
	l.seen = append(l.seen, audience)
	held := l.held
	l.held = nil
	l.mu.Unlock()
	if held != nil {
		close(held.entered)
		<-ctx.Done()
		held.ended <- ctx.Err()
		return domain.VideoPage{}, ctx.Err()
	}
	return domain.VideoPage{Items: []domain.Video{}}, nil
}

func (l *audienceLibrary) hold() *heldListing {
	held := &heldListing{entered: make(chan struct{}), ended: make(chan error, 1)}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.held = held
	return held
}

func (l *audienceLibrary) audiences() []domain.Audience {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]domain.Audience(nil), l.seen...)
}

// peerNode is one VVMDM listening on loopback with a real store, pairing use case and peer client.
type peerNode struct {
	env     *authEnv
	url     string
	cookie  *http.Cookie
	library *audienceLibrary
}

func newPeerNode(t *testing.T, name string) *peerNode {
	t.Helper()
	library := &audienceLibrary{fakeLibrary: sampleLibrary()}
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		return Options{
			Build:  domain.BuildInfo{Version: testPeerVersion},
			Videos: library,
			Pairing: app.NewPairing(app.PairingOptions{
				Store: db.Peers(), Link: peer.New(peer.Options{}), Version: testPeerVersion, NodeName: name,
			}),
		}
	})
	server := httptest.NewServer(env.handler)
	t.Cleanup(server.Close)
	return &peerNode{env: env, url: server.URL, cookie: env.setup(), library: library}
}

func (n *peerNode) owner(method, target, body string) *httptest.ResponseRecorder {
	n.env.t.Helper()
	return n.env.serve(authRequest{method: method, target: target, body: body, cookies: []*http.Cookie{n.cookie}})
}

// outgoingSecret is the credential this node presents to its peer.
func (n *peerNode) outgoingSecret(t *testing.T) string {
	t.Helper()
	record, found, err := n.env.db.Peers().Peer(context.Background())
	if err != nil || !found {
		t.Fatalf("記録が無い: %v, %v", found, err)
	}
	return record.OutgoingSecret
}

func pairTestNodes(t *testing.T) (a, b *peerNode) {
	t.Helper()
	a, b = newPeerNode(t, "node-a"), newPeerNode(t, "node-b")
	rec := a.owner(http.MethodPost, "/api/peer/code", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("コードの発行: status = %d: %s", rec.Code, rec.Body)
	}
	var code gen.PairingCode
	if err := json.Unmarshal(rec.Body.Bytes(), &code); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(gen.PairPeerRequest{Address: a.url, Code: code.Code, OwnAddress: b.url})
	if rec := b.owner(http.MethodPost, "/api/peer/pair", string(body)); rec.Code != http.StatusCreated {
		t.Fatalf("ペアリング: status = %d: %s", rec.Code, rec.Body)
	}
	return a, b
}

func decodePeerState(t *testing.T, rec *httptest.ResponseRecorder) (gen.PeerState, map[string]json.RawMessage) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/peer: status = %d: %s", rec.Code, rec.Body)
	}
	var state gen.PeerState
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	return state, fields
}

func TestPairingConnectsBothNodes(t *testing.T) {
	a, b := pairTestNodes(t)

	for _, check := range []struct {
		node     *peerNode
		peerName string
		address  string
	}{{a, "node-b", b.url}, {b, "node-a", a.url}} {
		state, _ := decodePeerState(t, check.node.owner(http.MethodGet, "/api/peer", ""))
		if state.State != gen.PeerStateStateConnected || state.Peer == nil ||
			state.Peer.Name != check.peerName || state.Peer.Address != check.address {
			t.Errorf("所有者の状態 = %+v, want %s と connected", state, check.peerName)
		}
	}

	guest := a.env.get("/api/peer")
	assertAudience(t, "ゲストの GET /api/peer", guest, "guest")
	state, fields := decodePeerState(t, guest)
	if state.State != gen.PeerStateStateConnected || len(fields) != 1 {
		t.Errorf("ゲストの状態 = %s, want state だけ", guest.Body)
	}

	// The record also lives on A, so A's code is refused as already paired.
	if rec := a.owner(http.MethodPost, "/api/peer/code", ""); rec.Code != http.StatusConflict {
		t.Errorf("ペアリング済みのコード発行: status = %d: %s", rec.Code, rec.Body)
	}
}

func TestPeerCredentialClassification(t *testing.T) {
	a, b := pairTestNodes(t)
	secret := b.outgoingSecret(t)
	peerRequest := func(method, target string, header map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		all := map[string]string{"Authorization": "VV-Peer " + secret}
		for name, value := range header {
			all[name] = value
		}
		return a.env.serve(authRequest{method: method, target: target, header: all})
	}

	owner := peerRequest(http.MethodGet, "/api/videos", nil)
	if owner.Code != http.StatusOK || owner.Header().Get("Set-Cookie") != "" {
		t.Errorf("資格情報の一覧: status = %d, Set-Cookie = %q", owner.Code, owner.Header().Get("Set-Cookie"))
	}
	assertAudience(t, "資格情報", owner, "owner")
	guest := peerRequest(http.MethodGet, "/api/videos", map[string]string{peerAudienceHeader: "guest"})
	assertAudience(t, "guest の見出し", guest, "guest")
	if got := a.library.audiences(); len(got) != 2 || got[0] != domain.AudienceOwner || got[1] != domain.AudienceGuest {
		t.Errorf("ストアに渡った視聴者 = %v, want [owner guest]", got)
	}
	rec := peerRequest(http.MethodGet, "/api/videos", map[string]string{peerAudienceHeader: "owner"})
	assertErrorBody(t, "見出しの別の値", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})

	for _, credential := range []string{"VV-Peer " + a.outgoingSecret(t), "VV-Peer vvp_unknown", "VV-Peer"} {
		rec := a.env.serve(authRequest{method: http.MethodGet, target: "/api/videos",
			header: map[string]string{"Authorization": credential}})
		assertErrorBody(t, credential, rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusUnauthorized, code: gen.ErrorCodeUnauthenticated, reason: gen.ErrorReasonPeerUnknown,
		})
	}

	for _, op := range []operation{
		{http.MethodGet, "/api/media-folders"}, {http.MethodPut, "/api/settings/transcoding"},
		{http.MethodGet, "/api/api-tokens"}, {http.MethodGet, "/api/peer"}, {http.MethodPost, "/api/peer/code"},
	} {
		rec := peerRequest(op.method, op.path, nil)
		assertErrorBody(t, op.method+" "+op.path, rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusForbidden, code: gen.ErrorCodeForbidden, reason: gen.ErrorReasonPeerNotAllowed,
		})
	}

	// The peer is not a browser: a cross-site mutation is not refused as cross_origin.
	rec = peerRequest(http.MethodDelete, "/api/peer/pairings/7c2f4c3a-1d2e-4f50-8a6b-0c1d2e3f4a5b",
		map[string]string{"Origin": "http://elsewhere.example", "Sec-Fetch-Site": "cross-site"})
	if rec.Code != http.StatusNoContent {
		t.Errorf("別の node id の削除: status = %d: %s", rec.Code, rec.Body)
	}
	if _, found, _ := a.env.db.Peers().Peer(context.Background()); !found {
		t.Error("別の node id の削除で記録が消えた")
	}

	if rec := peerRequest(http.MethodGet, "/api/v1/videos", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("/api/v1 への資格情報: status = %d, want 401", rec.Code)
	}
}

func TestPeerHandshakeAndForget(t *testing.T) {
	a, b := pairTestNodes(t)
	secret := b.outgoingSecret(t)

	rec := a.env.serve(authRequest{method: http.MethodGet, target: "/api/peer/handshake",
		header: map[string]string{"Authorization": "VV-Peer " + secret}})
	var handshake gen.PeerHandshake
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &handshake) != nil ||
		handshake.Node.Name != "node-a" || handshake.Node.Version != testPeerVersion {
		t.Errorf("ハンドシェイク: status = %d: %s", rec.Code, rec.Body)
	}
	rec = a.env.get("/api/peer/handshake")
	assertErrorBody(t, "資格情報なしのハンドシェイク", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusUnauthorized, code: gen.ErrorCodeUnauthenticated, reason: gen.ErrorReasonPeerUnknown,
	})

	// Over a real connection: deleting the record revokes the peer's requests but must still answer this one.
	record, _, _ := a.env.db.Peers().Peer(context.Background())
	req, err := http.NewRequest(http.MethodDelete, a.url+"/api/peer/pairings/"+record.NodeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "VV-Peer "+secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("削除: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("削除: status = %d", res.StatusCode)
	}
	state, _ := decodePeerState(t, a.owner(http.MethodGet, "/api/peer", ""))
	if state.State != gen.PeerStateStateUnpaired {
		t.Errorf("削除後の A の状態 = %s", state.State)
	}
	rec = a.env.serve(authRequest{method: http.MethodGet, target: "/api/videos",
		header: map[string]string{"Authorization": "VV-Peer " + secret}})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("削除後の資格情報: status = %d, want 401", rec.Code)
	}
}

func TestUnpairCancelsInFlightPeerRequests(t *testing.T) {
	a, b := pairTestNodes(t)
	held := a.library.hold()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- a.env.serve(authRequest{method: http.MethodGet, target: "/api/videos",
			header: map[string]string{"Authorization": "VV-Peer " + b.outgoingSecret(t)}})
	}()
	<-held.entered

	if rec := a.owner(http.MethodDelete, "/api/peer", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("解除: status = %d: %s", rec.Code, rec.Body)
	}
	select {
	case err := <-held.ended:
		if err == nil {
			t.Fatal("解除で進行中の要求が打ち切られなかった")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("解除で進行中の要求が打ち切られなかった")
	}
	<-done
}

func TestPairRejectsInvalidAddress(t *testing.T) {
	b := newPeerNode(t, "node-b")
	body, _ := json.Marshal(gen.PairPeerRequest{Address: "http://a.local/vv", Code: "ABCDEFGH", OwnAddress: b.url})
	rec := b.owner(http.MethodPost, "/api/peer/pair", string(body))
	assertErrorBody(t, "パス付きのアドレス", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonInvalidAddress,
	})
	if !strings.Contains(rec.Body.String(), `"field":"address"`) {
		t.Errorf("field が address でない: %s", rec.Body)
	}
}

func TestPeerOperationsAreOpenAPIOperations(t *testing.T) {
	declared := openAPISecurity(t)
	for _, pattern := range peerOperations {
		method, route, _ := strings.Cut(pattern, " ")
		if _, ok := declared[operation{method: method, path: route}]; !ok {
			t.Errorf("peerOperations の %q は openapi.yaml に無い", pattern)
		}
	}
}
