package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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

const (
	testPeerVersion = "1.2.0"
	otherPeerNodeID = "7c2f4c3a-1d2e-4f50-8a6b-0c1d2e3f4a5b"
	// The peer event a guest receives while the one link is connected, and once no link is left.
	guestConnected = `{"links":[{"slot":1,"state":"connected"}]}`
	guestNoLinks   = `{"links":[]}`
)

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
	pairing *app.Pairing
}

func newPeerNode(t *testing.T, name string) *peerNode {
	t.Helper()
	return newPeerNodeServed(t, name, httptest.NewServer)
}

// newPeerNodeServed is newPeerNode served by serve, such as httptest.NewTLSServer for a self-signed certificate.
func newPeerNodeServed(t *testing.T, name string, serve func(http.Handler) *httptest.Server) *peerNode {
	t.Helper()
	library := &audienceLibrary{fakeLibrary: sampleLibrary()}
	var pairing *app.Pairing
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		events := NewEvents()
		opts := Options{Videos: library, Events: events}
		pairing = withTestPairing(db, name, storePublisher{events: events}, &opts)
		return opts
	})
	server := serve(env.handler)
	t.Cleanup(server.Close)
	return &peerNode{env: env, url: server.URL, cookie: env.setup(), library: library, pairing: pairing}
}

// withTestPairing gives opts the pairing, its forwarder and the version both test nodes share.
func withTestPairing(db *store.DB, name string, publisher app.Publisher, opts *Options) *app.Pairing {
	client := peer.New(peer.Options{})
	pairing := app.NewPairing(app.PairingOptions{
		Store: db.Peers(), Client: client, Version: testPeerVersion, NodeName: name, Publisher: publisher,
	})
	opts.Build = domain.BuildInfo{Version: testPeerVersion}
	opts.Pairing = pairing
	opts.PeerLink = peer.NewForwarder(client, pairing)
	return pairing
}

// runConnection starts the node's connection loop until the test ends.
func (n *peerNode) runConnection(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		n.pairing.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

// nextPeerEvent skips other events and returns the next peer event's state as JSON.
func nextPeerEvent(t *testing.T, stream <-chan sseEvent) string {
	t.Helper()
	for {
		if event := nextEvent(t, stream); event.name == "peer" {
			return event.data
		}
	}
}

func (n *peerNode) owner(method, target, body string) *httptest.ResponseRecorder {
	n.env.t.Helper()
	return n.env.serve(authRequest{method: method, target: target, body: body, cookies: []*http.Cookie{n.cookie}})
}

// onlyLink is the one link this node reads.
func (n *peerNode) onlyLink(t *testing.T) domain.PeerLink {
	t.Helper()
	links, err := n.env.db.Peers().Links(context.Background())
	if err != nil || len(links) != 1 {
		t.Fatalf("リンク = %+v, %v, want 1 本", links, err)
	}
	return links[0]
}

// linkSecret is the credential this node presents to the one node it reads.
func (n *peerNode) linkSecret(t *testing.T) string {
	t.Helper()
	return n.onlyLink(t).Secret
}

// onlyLinkState is the state of the one link GET /api/peer answers to the owner of n.
func onlyLinkState(t *testing.T, n *peerNode) gen.PeerLinkState {
	t.Helper()
	state, _ := decodePeerState(t, n.owner(http.MethodGet, "/api/peer", ""))
	if len(state.Links) != 1 {
		t.Fatalf("リンク = %+v, want 1 本", state.Links)
	}
	return state.Links[0]
}

func linkNames(links []gen.PeerLinkState) []string {
	names := make([]string, len(links))
	for i, link := range links {
		names[i] = fmt.Sprintf("%d:%s:%s", link.Slot, valueOrEmpty(link.Name), link.State)
	}
	return names
}

func (n *peerNode) grant(t *testing.T) (domain.PeerGrant, bool) {
	t.Helper()
	grant, found, err := n.env.db.Peers().Grant(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return grant, found
}

// pairTestNodes makes b read a.
func pairTestNodes(t *testing.T) (a, b *peerNode) {
	t.Helper()
	a, b = newPeerNode(t, "node-a"), newPeerNode(t, "node-b")
	pairNodes(t, a, b)
	return a, b
}

func issueCode(t *testing.T, a *peerNode) string {
	t.Helper()
	rec := a.owner(http.MethodPost, "/api/peer/code", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("コードの発行: status = %d: %s", rec.Code, rec.Body)
	}
	var code gen.PairingCode
	if err := json.Unmarshal(rec.Body.Bytes(), &code); err != nil {
		t.Fatal(err)
	}
	return code.Code
}

// pairNodes makes b read a from b's Pairing page, with a's code.
func pairNodes(t *testing.T, a, b *peerNode) {
	t.Helper()
	body, _ := json.Marshal(gen.PairPeerRequest{Address: a.url, Code: issueCode(t, a)})
	if rec := b.owner(http.MethodPost, "/api/peer/pair", string(body)); rec.Code != http.StatusCreated {
		t.Fatalf("ペアリング: status = %d: %s", rec.Code, rec.Body)
	}
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

// Pairing from B grants B alone: B shows A as its connected link, A shows B as its grant and reads nothing.
func TestPairingGrantsTheReaderOnly(t *testing.T) {
	a, b := pairTestNodes(t)

	onB, _ := decodePeerState(t, b.owner(http.MethodGet, "/api/peer", ""))
	nodeA, _ := a.pairing.Node(context.Background())
	if len(onB.Links) != 1 || onB.Grant != nil {
		t.Fatalf("B の状態 = %+v, want node-a へのリンクだけ", onB)
	}
	if link := onB.Links[0]; link.Slot != 1 || link.State != gen.PeerLinkStateStateConnected ||
		valueOrEmpty(link.NodeId) != nodeA.ID || valueOrEmpty(link.Name) != "node-a" || valueOrEmpty(link.Address) != a.url {
		t.Errorf("B のリンク = %+v, want スロット 1 で node-a に connected", link)
	}
	onA, _ := decodePeerState(t, a.owner(http.MethodGet, "/api/peer", ""))
	if len(onA.Links) != 0 || onA.Grant == nil || onA.Grant.Name != "node-b" {
		t.Errorf("A の状態 = %+v, want リンク無しと node-b の grant", onA)
	}

	guest := b.env.get("/api/peer")
	assertAudience(t, "ゲストの GET /api/peer", guest, "guest")
	if got := guest.Body.String(); got != `{"links":[{"slot":1,"state":"connected"}]}`+"\n" {
		t.Errorf("ゲストの状態 = %s, want スロットと state だけ", got)
	}

	// A is read by B, so its code is refused.
	if rec := a.owner(http.MethodPost, "/api/peer/code", ""); rec.Code != http.StatusConflict {
		t.Errorf("読まれている A のコード発行: status = %d: %s", rec.Code, rec.Body)
	}
}

// A ninth link is refused with peer_limit before the other node is asked.
func TestPairingRefusesTheNinthLink(t *testing.T) {
	a, b := newPeerNode(t, "node-a"), newPeerNode(t, "node-b")
	for i := range domain.MaxPeerLinks {
		link := domain.PeerLink{
			NodeID: fmt.Sprintf("00000000-0000-4000-8000-%012d", i), Name: "n", Address: "http://n.local", Secret: "vvp_s",
		}
		if _, err := b.env.db.Peers().AddLink(context.Background(), link); err != nil {
			t.Fatal(err)
		}
	}
	body, _ := json.Marshal(gen.PairPeerRequest{Address: a.url, Code: issueCode(t, a)})
	rec := b.owner(http.MethodPost, "/api/peer/pair", string(body))
	assertErrorBody(t, "9 本目のペアリング", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonPeerLimit})
	if _, found := a.grant(t); found {
		t.Error("断ったペアリングで A に grant ができた")
	}
}

// One node reads two: GET /api/peer lists both in slot order, and unpairing one by its node id leaves the other.
func TestTwoLinksAreListedInSlotOrderAndUnpairedApart(t *testing.T) {
	a, b, c := newPeerNode(t, "node-a"), newPeerNode(t, "node-b"), newPeerNode(t, "node-c")
	pairNodes(t, a, b)
	pairNodes(t, c, b)

	state, _ := decodePeerState(t, b.owner(http.MethodGet, "/api/peer", ""))
	if got, want := linkNames(state.Links), []string{"1:node-a:connected", "2:node-c:connected"}; !slices.Equal(got, want) {
		t.Fatalf("B のリンク = %v, want %v", got, want)
	}

	nodeA, _ := a.pairing.Node(context.Background())
	if rec := b.owner(http.MethodDelete, "/api/peer/links/"+nodeA.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("node-a の解除: status = %d: %s", rec.Code, rec.Body)
	}
	state, _ = decodePeerState(t, b.owner(http.MethodGet, "/api/peer", ""))
	if got, want := linkNames(state.Links), []string{"2:node-c:connected"}; !slices.Equal(got, want) {
		t.Errorf("解除後の B のリンク = %v, want %v", got, want)
	}
	if _, found := a.grant(t); found {
		t.Error("解除で A の grant が消えない")
	}
	if _, found := c.grant(t); !found {
		t.Error("node-a の解除で C の grant が消えた")
	}
}

// A behind self-signed HTTPS pairs after B's owner confirms A's certificate; B serves plain http and
// needs no certificate of its own. The TLS server is the boundary under test.
func TestPairingOverSelfSignedHTTPSPinsTheReadNode(t *testing.T) {
	a, b := newPeerNodeServed(t, "node-a", httptest.NewTLSServer), newPeerNode(t, "node-b")
	code := issueCode(t, a)
	pair := func(fingerprint *string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(gen.PairPeerRequest{Address: a.url, Code: code, Fingerprint: fingerprint})
		return b.owner(http.MethodPost, "/api/peer/pair", string(body))
	}

	rec := pair(nil)
	refusal := decode[gen.Error](t, rec)
	if rec.Code != http.StatusConflict || refusal.Reason == nil || *refusal.Reason != gen.ErrorReasonCertificateUntrusted ||
		refusal.Fingerprint == nil {
		t.Fatalf("status = %d: %s, want 409 certificate_untrusted と指紋", rec.Code, rec.Body)
	}
	if rec := pair(refusal.Fingerprint); rec.Code != http.StatusCreated {
		t.Fatalf("確認後のペアリング: status = %d: %s", rec.Code, rec.Body)
	}
	if link := onlyLinkState(t, b); link.State != gen.PeerLinkStateStateConnected || link.Pinned == nil || !*link.Pinned {
		t.Errorf("B のリンク = %+v, want connected で証明書を固定", link)
	}
}

func TestPeerCredentialClassification(t *testing.T) {
	a, b := pairTestNodes(t)
	secret := b.linkSecret(t)
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

	// The credential is valid on A alone: B, which holds no grant, refuses it like any unknown secret.
	unknown := b.env.serve(authRequest{method: http.MethodGet, target: "/api/videos",
		header: map[string]string{"Authorization": "VV-Peer " + secret}})
	assertErrorBody(t, "読む側への資格情報", unknown.Code, unknown.Body.Bytes(), wantError{
		status: http.StatusUnauthorized, code: gen.ErrorCodeUnauthenticated, reason: gen.ErrorReasonPeerUnknown,
	})
	for _, credential := range []string{"VV-Peer vvp_unknown", "VV-Peer"} {
		rec := a.env.serve(authRequest{method: http.MethodGet, target: "/api/videos",
			header: map[string]string{"Authorization": credential}})
		assertErrorBody(t, credential, rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusUnauthorized, code: gen.ErrorCodeUnauthenticated, reason: gen.ErrorReasonPeerUnknown,
		})
	}

	for _, op := range []operation{
		{http.MethodGet, "/api/media-folders"}, {http.MethodPut, "/api/settings/transcoding"},
		{http.MethodGet, "/api/api-tokens"}, {http.MethodGet, "/api/peer"}, {http.MethodPost, "/api/peer/code"},
		{http.MethodDelete, "/api/peer/grant"}, {http.MethodDelete, "/api/peer/links/" + otherPeerNodeID},
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
	if _, found := a.grant(t); !found {
		t.Error("別の node id の削除で grant が消えた")
	}

	if rec := peerRequest(http.MethodGet, "/api/v1/videos", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("/api/v1 への資格情報: status = %d, want 401", rec.Code)
	}
}

func TestPeerHandshakeAndForget(t *testing.T) {
	a, b := pairTestNodes(t)
	secret := b.linkSecret(t)

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

	// Over a real connection: deleting the grant revokes the reader's requests but must still answer this one.
	grant, _ := a.grant(t)
	req, err := http.NewRequest(http.MethodDelete, a.url+"/api/peer/pairings/"+grant.NodeID, nil)
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
	if state, _ := decodePeerState(t, a.owner(http.MethodGet, "/api/peer", "")); state.Grant != nil {
		t.Errorf("削除後の A の状態 = %+v, want grant なし", state)
	}
	rec = a.env.serve(authRequest{method: http.MethodGet, target: "/api/videos",
		header: map[string]string{"Authorization": "VV-Peer " + secret}})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("削除後の資格情報: status = %d, want 401", rec.Code)
	}
}

func TestRevokingTheGrantCancelsInFlightPeerRequests(t *testing.T) {
	a, b := pairTestNodes(t)
	held := a.library.hold()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- a.env.serve(authRequest{method: http.MethodGet, target: "/api/videos",
			header: map[string]string{"Authorization": "VV-Peer " + b.linkSecret(t)}})
	}()
	<-held.entered

	if rec := a.owner(http.MethodDelete, "/api/peer/grant", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("取り消し: status = %d: %s", rec.Code, rec.Body)
	}
	select {
	case err := <-held.ended:
		if err == nil {
			t.Fatal("取り消しで進行中の要求が打ち切られなかった")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("取り消しで進行中の要求が打ち切られなかった")
	}
	<-done
}

func TestPairRejectsInvalidAddress(t *testing.T) {
	b := newPeerNode(t, "node-b")
	body, _ := json.Marshal(gen.PairPeerRequest{Address: "http://a.local/vv", Code: "ABCDEFGH"})
	rec := b.owner(http.MethodPost, "/api/peer/pair", string(body))
	assertErrorBody(t, "パス付きのアドレス", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonInvalidAddress,
	})
	if !strings.Contains(rec.Body.String(), `"field":"address"`) {
		t.Errorf("field が address でない: %s", rec.Body)
	}
}

// Unpairing on the reader tells the read node, which drops the grant; the reader's guests see the link go.
func TestUnpairTellsTheReadNodeAndReachesGuests(t *testing.T) {
	a, b := pairTestNodes(t)
	guest, _ := openEvents(t, b.env.handler)
	if got := nextPeerEvent(t, guest); got != guestConnected {
		t.Fatalf("接続時のゲストへの peer = %s, want スロットと connected だけ", got)
	}

	if rec := b.owner(http.MethodDelete, "/api/peer/links/"+b.onlyLink(t).NodeID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("B の解除: status = %d: %s", rec.Code, rec.Body)
	}
	if got := nextPeerEvent(t, guest); got != guestNoLinks {
		t.Errorf("B の解除の後のゲストへの peer = %s, want リンク無し", got)
	}
	if _, found := a.grant(t); found {
		t.Error("B の解除で A の grant が消えない")
	}
}

// The read node withdrawing access while the reader is away: the reader's next request is refused, and the
// reader shows the link as revoked.
func TestRevokedReaderLearnsItOnItsNextRequest(t *testing.T) {
	a, b := pairTestNodes(t)
	if rec := a.owner(http.MethodDelete, "/api/peer/grant", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("A の取り消し: status = %d: %s", rec.Code, rec.Body)
	}
	guest, _ := openEvents(t, b.env.handler)
	if got := nextPeerEvent(t, guest); got != guestConnected {
		t.Fatalf("B の状態 = %s, want connected のまま", got)
	}

	b.runConnection(t)
	if got := nextPeerEvent(t, guest); got != `{"links":[{"slot":1,"state":"revoked"}]}` {
		t.Fatalf("B の次の要求の後の peer = %s, want revoked", got)
	}
	link := b.onlyLink(t)
	if !link.Revoked {
		t.Error("revoked が link に記録されていない")
	}
	if rec := b.owner(http.MethodDelete, "/api/peer/links/"+link.NodeID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("revoked の link の削除: status = %d: %s", rec.Code, rec.Body)
	}
	if got := nextPeerEvent(t, guest); got != guestNoLinks {
		t.Errorf("削除後の peer = %s, want リンク無し", got)
	}
}
