package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakePeerStore keeps one link and one grant, hashing the grant's secret like the SQLite role.
type fakePeerStore struct {
	mu        sync.Mutex
	nodeID    string
	link      *domain.PeerLink
	grant     *domain.PeerGrant
	grantHash [32]byte
}

func (f *fakePeerStore) EnsureNodeID(_ context.Context, candidate string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.nodeID == "" {
		f.nodeID = candidate
	}
	return f.nodeID, nil
}

func (f *fakePeerStore) Link(context.Context) (domain.PeerLink, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.link == nil {
		return domain.PeerLink{}, false, nil
	}
	return *f.link, true, nil
}

func (f *fakePeerStore) AddLink(_ context.Context, link domain.PeerLink) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.link != nil {
		return domain.ErrAlreadyPaired
	}
	f.link = &link
	return nil
}

func (f *fakePeerStore) UpdateLinkAddress(_ context.Context, address string) (domain.PeerLink, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.link == nil {
		return domain.PeerLink{}, domain.ErrNotFound
	}
	f.link.Address = address
	return *f.link, nil
}

func (f *fakePeerStore) PinLinkCertificate(_ context.Context, nodeID, spki string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.link == nil || f.link.NodeID != nodeID {
		return domain.ErrNotFound
	}
	f.link.PinnedSPKI = spki
	return nil
}

func (f *fakePeerStore) MarkLinkRevoked(_ context.Context, nodeID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.link != nil && f.link.NodeID == nodeID {
		f.link.Revoked = true
	}
	return nil
}

func (f *fakePeerStore) DeleteLink(_ context.Context, nodeID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.link == nil || f.link.NodeID != nodeID {
		return false, nil
	}
	f.link = nil
	return true, nil
}

func (f *fakePeerStore) Grant(context.Context) (domain.PeerGrant, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.grant == nil {
		return domain.PeerGrant{}, false, nil
	}
	return *f.grant, true, nil
}

func (f *fakePeerStore) AddGrant(_ context.Context, grant domain.PeerGrant, secret string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.grant != nil {
		return domain.ErrAlreadyPaired
	}
	f.grant, f.grantHash = &grant, sha256.Sum256([]byte(secret))
	return nil
}

func (f *fakePeerStore) GrantBySecret(_ context.Context, secret string) (domain.PeerGrant, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.grant == nil || f.grantHash != sha256.Sum256([]byte(secret)) {
		return domain.PeerGrant{}, false, nil
	}
	return *f.grant, true, nil
}

func (f *fakePeerStore) DeleteGrant(_ context.Context, nodeID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.grant == nil || f.grant.NodeID != nodeID {
		return false, nil
	}
	f.grant = nil
	return true, nil
}

func (f *fakePeerStore) storedLink() *domain.PeerLink {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.link
}

func (f *fakePeerStore) storedGrant() *domain.PeerGrant {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.grant
}

// directLink routes calls to the Pairing registered at the address, as two nodes on one LAN.
type directLink struct {
	nodes map[string]*Pairing
	// handshakes lists the address of each handshake; a pairing makes none, since the accepting node
	// never calls the sender.
	handshakes []string
	// certificates holds the SPKI of each address that presents a self-signed certificate, which a call
	// reaches only when pinned to it.
	certificates map[string]string
}

func (l *directLink) node(target domain.PeerTarget) (*Pairing, error) {
	node, ok := l.nodes[target.Address]
	if !ok {
		return nil, &domain.PeerUnreachableError{Message: "no node at " + target.Address}
	}
	if spki, selfSigned := l.certificates[target.Address]; selfSigned && target.PinnedSPKI != spki {
		return nil, &domain.CertificateUntrustedError{Fingerprint: spki}
	}
	return node, nil
}

func (l *directLink) RequestPairing(
	ctx context.Context, target domain.PeerTarget, req domain.PairingRequest,
) (domain.PairingAnswer, error) {
	node, err := l.node(target)
	if err != nil {
		return domain.PairingAnswer{}, err
	}
	return node.Accept(ctx, req)
}

func (l *directLink) Handshake(ctx context.Context, target domain.PeerTarget, secret string) (domain.PeerNode, error) {
	l.handshakes = append(l.handshakes, target.Address)
	node, err := l.node(target)
	if err != nil {
		return domain.PeerNode{}, err
	}
	return node.Handshake(ctx, secret)
}

// The pairing tests do not run the connection loop, so the stream never opens.
func (l *directLink) Watch(ctx context.Context, _ domain.PeerTarget, _ string, _ func(), _ func(domain.PeerChanged)) error {
	<-ctx.Done()
	return ctx.Err()
}

func (l *directLink) Forget(ctx context.Context, target domain.PeerTarget, _, nodeID string) error {
	node, err := l.node(target)
	if err != nil {
		return err
	}
	return node.Forget(ctx, nodeID)
}

const (
	addressA = "http://a.local:8080"
	addressB = "http://b.local:8080"
)

type pairingNodes struct {
	a, b           *Pairing
	storeA, storeB *fakePeerStore
	link           *directLink
	now            time.Time
}

func newPairingNodes(t *testing.T, versionA, versionB string) *pairingNodes {
	t.Helper()
	n := &pairingNodes{
		storeA: &fakePeerStore{}, storeB: &fakePeerStore{}, now: time.Unix(1_800_000_000, 0),
		link: &directLink{nodes: map[string]*Pairing{}},
	}
	clock := func() time.Time { return n.now }
	n.a = NewPairing(PairingOptions{Store: n.storeA, Client: n.link, Version: versionA, NodeName: "a", Now: clock})
	n.b = NewPairing(PairingOptions{Store: n.storeB, Client: n.link, Version: versionB, NodeName: "b", Now: clock})
	n.link.nodes[addressA], n.link.nodes[addressB] = n.a, n.b
	return n
}

func (n *pairingNodes) issueCode(t *testing.T, node *Pairing) string {
	t.Helper()
	code, err := node.IssueCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return code.Value
}

// pairFromB makes B read A with A's code.
func (n *pairingNodes) pairFromB(code string) error {
	_, err := n.b.Pair(context.Background(), domain.PairInput{Address: addressA, Code: code})
	return err
}

func (n *pairingNodes) assertNothingStored(t *testing.T) {
	t.Helper()
	for label, store := range map[string]*fakePeerStore{"A": n.storeA, "B": n.storeB} {
		if store.storedLink() != nil || store.storedGrant() != nil {
			t.Errorf("%s に記録が保存された: link %+v, grant %+v", label, store.storedLink(), store.storedGrant())
		}
	}
}

func assertState(t *testing.T, label string, node *Pairing, want domain.PeerConnection, peerVersion string) {
	t.Helper()
	state, err := node.State(context.Background())
	if err != nil || state.State != want || state.PeerVersion != peerVersion {
		t.Errorf("%s の State = %+v, %v, want %s と PeerVersion %q", label, state, err, want, peerVersion)
	}
}

// Entering A's code on B grants B read access to A and nothing else: B holds the link with the secret A
// authenticates, A holds the grant, and A never calls B.
func TestPairingGrantsOneDirection(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	code := n.issueCode(t, n.a)
	if err := n.pairFromB(code); err != nil {
		t.Fatalf("Pair: %v", err)
	}

	nodeA, _ := n.a.Node(ctx)
	nodeB, _ := n.b.Node(ctx)
	link, grant := n.storeB.storedLink(), n.storeA.storedGrant()
	if link == nil || link.NodeID != nodeA.ID || link.Address != addressA || link.Name != "a" || link.Revoked {
		t.Errorf("B の link = %+v, want A (%s)", link, nodeA.ID)
	}
	if grant == nil || grant.NodeID != nodeB.ID || grant.Name != "b" || !grant.PairedAt.Equal(n.now) {
		t.Errorf("A の grant = %+v, want B (%s)", grant, nodeB.ID)
	}
	if n.storeA.storedLink() != nil || n.storeB.storedGrant() != nil {
		t.Errorf("逆方向が保存された: A の link %+v, B の grant %+v", n.storeA.storedLink(), n.storeB.storedGrant())
	}
	if valid, err := n.a.Authenticate(ctx, link.Secret); err != nil || !valid {
		t.Errorf("B の secret が A で通らない: %v, %v", valid, err)
	}
	if len(n.link.handshakes) != 0 {
		t.Errorf("ペアリング中に handshake が呼ばれた: %v", n.link.handshakes)
	}
	assertState(t, "B", n.b, domain.PeerConnected, "1.2.0")
	assertState(t, "A", n.a, domain.PeerUnpaired, "")
	if state, _ := n.a.State(ctx); state.Grant == nil || state.Grant.Name != "b" || state.Link != nil {
		t.Errorf("A の State = %+v, want grant だけ", state)
	}

	// The code is used up, each direction holds one row, and A's code is refused while B reads it.
	secondB := NewPairing(PairingOptions{Store: &fakePeerStore{}, Client: n.link, Version: "1.2.0", NodeName: "b2"})
	if _, err := secondB.Pair(ctx, domain.PairInput{Address: addressA, Code: code}); !errors.Is(err, domain.ErrInvalidPairingCode) {
		t.Errorf("使ったコードでの Pair err = %v, want ErrInvalidPairingCode", err)
	}
	if err := n.pairFromB("ANYCODE2"); !errors.Is(err, domain.ErrAlreadyPaired) {
		t.Errorf("読む相手がある B の Pair err = %v, want ErrAlreadyPaired", err)
	}
	if _, err := n.a.IssueCode(ctx); !errors.Is(err, domain.ErrAlreadyPaired) {
		t.Errorf("読まれている A の IssueCode err = %v, want ErrAlreadyPaired", err)
	}
}

func TestPairingRefusalsStoreNothing(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(t *testing.T, n *pairingNodes) domain.PairInput
		want    error
	}{
		{name: "誤ったコード", want: domain.ErrInvalidPairingCode,
			prepare: func(t *testing.T, n *pairingNodes) domain.PairInput {
				t.Helper()
				n.issueCode(t, n.a)
				return domain.PairInput{Address: addressA, Code: "WRONGCOD"}
			}},
		{name: "期限切れのコード", want: domain.ErrInvalidPairingCode,
			prepare: func(t *testing.T, n *pairingNodes) domain.PairInput {
				t.Helper()
				code := n.issueCode(t, n.a)
				n.now = n.now.Add(domain.PairingCodeLifetime)
				return domain.PairInput{Address: addressA, Code: code}
			}},
		{name: "届かないアドレス", want: domain.ErrPeerUnreachable,
			prepare: func(t *testing.T, n *pairingNodes) domain.PairInput {
				t.Helper()
				return domain.PairInput{Address: "http://c.local", Code: n.issueCode(t, n.a)}
			}},
		{name: "不正なアドレス", want: domain.ErrInvalidPeerAddress,
			prepare: func(*testing.T, *pairingNodes) domain.PairInput {
				return domain.PairInput{Address: "a.local", Code: "ABCDEFGH"}
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := newPairingNodes(t, "1.2.0", "1.2.0")
			input := tc.prepare(t, n)
			if _, err := n.b.Pair(context.Background(), input); !errors.Is(err, tc.want) {
				t.Fatalf("Pair err = %v, want %v", err, tc.want)
			}
			n.assertNothingStored(t)
		})
	}
}

// A mutual pairing is two pairings, and each direction is removed on its own.
func TestMutualPairingIsTwoIndependentPairings(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	if err := n.pairFromB(n.issueCode(t, n.a)); err != nil {
		t.Fatalf("B → A: %v", err)
	}
	if _, err := n.a.Pair(ctx, domain.PairInput{Address: addressB, Code: n.issueCode(t, n.b)}); err != nil {
		t.Fatalf("A → B: %v", err)
	}
	for label, store := range map[string]*fakePeerStore{"A": n.storeA, "B": n.storeB} {
		if store.storedLink() == nil || store.storedGrant() == nil {
			t.Errorf("%s の link %+v, grant %+v, want 両方", label, store.storedLink(), store.storedGrant())
		}
	}
	assertState(t, "A", n.a, domain.PeerConnected, "1.2.0")
	assertState(t, "B", n.b, domain.PeerConnected, "1.2.0")

	// A withdraws B's access: B's secret stops working on A, while A still reads B.
	if err := n.a.RevokeGrant(ctx); err != nil {
		t.Fatal(err)
	}
	if valid, _ := n.a.Authenticate(ctx, n.storeB.storedLink().Secret); valid {
		t.Error("取り消した後も B の secret が A で通る")
	}
	if n.storeA.storedGrant() != nil || n.storeA.storedLink() == nil || n.storeB.storedGrant() == nil {
		t.Errorf("取り消しの後: A の grant %+v, A の link %+v, B の grant %+v, want A の grant だけ消える",
			n.storeA.storedGrant(), n.storeA.storedLink(), n.storeB.storedGrant())
	}
	assertState(t, "取り消し後の A", n.a, domain.PeerConnected, "1.2.0")
}

func TestAcceptRefusesInvalidSendersAndKeepsTheCode(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	code := n.issueCode(t, n.a)
	nodeA, _ := n.a.Node(ctx)
	nodeB, _ := n.b.Node(ctx)
	for _, tc := range []struct {
		label string
		node  domain.PeerNode
		want  error
	}{
		{"自分自身", nodeA, domain.ErrInvalidPairingInput},
		{"不正な node id", domain.PeerNode{ID: "not-a-uuid", Name: "b"}, domain.ErrInvalidPairingInput},
		{"空の名前", domain.PeerNode{ID: nodeB.ID}, domain.ErrInvalidPairingInput},
	} {
		if _, err := n.a.Accept(ctx, domain.PairingRequest{Code: code, Node: tc.node}); !errors.Is(err, tc.want) {
			t.Errorf("%s の Accept err = %v, want %v", tc.label, err, tc.want)
		}
	}
	n.assertNothingStored(t)

	// A node that is already read refuses after the code check, and the code stays usable once access is withdrawn.
	if err := n.storeA.AddGrant(ctx, domain.PeerGrant{NodeID: "7c2f4c3a-1d2e-4f50-8a6b-0c1d2e3f4a5b", Name: "c"}, "vvp_c"); err != nil {
		t.Fatal(err)
	}
	if err := n.pairFromB(code); !errors.Is(err, domain.ErrAlreadyPaired) {
		t.Fatalf("読まれている A への Pair err = %v, want ErrAlreadyPaired", err)
	}
	if got := n.storeA.storedGrant(); got.Name != "c" {
		t.Errorf("A の grant が置き換わった: %+v", got)
	}
	if err := n.a.RevokeGrant(ctx); err != nil {
		t.Fatal(err)
	}
	if err := n.pairFromB(code); err != nil {
		t.Errorf("取り消し後の同じコードでの Pair err = %v", err)
	}
}

// The version is a build label, so builds of the same source can differ in it; it is shown, never compared.
func TestPairingIgnoresTheVersionLabels(t *testing.T) {
	const sha = "f1672930f88eac63ac8f92b96985c8ef76e32036"
	n := newPairingNodes(t, sha, domain.DefaultVersion)
	if err := n.pairFromB(n.issueCode(t, n.a)); err != nil {
		t.Fatalf("Pair: %v", err)
	}
	assertState(t, "B", n.b, domain.PeerConnected, sha)
}

func TestHandshakeAcceptsOnlyTheGrantSecret(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	if _, err := n.a.Handshake(ctx, "vvp_unknown"); !errors.Is(err, domain.ErrPeerUnknown) {
		t.Errorf("記録の無い Handshake err = %v, want ErrPeerUnknown", err)
	}
	if err := n.pairFromB(n.issueCode(t, n.a)); err != nil {
		t.Fatal(err)
	}
	if node, err := n.a.Handshake(ctx, n.storeB.storedLink().Secret); err != nil || node.Name != "a" {
		t.Errorf("Handshake = %+v, %v", node, err)
	}
	if _, err := n.b.Handshake(ctx, n.storeB.storedLink().Secret); !errors.Is(err, domain.ErrPeerUnknown) {
		t.Errorf("読む側での自分の secret の Handshake err = %v, want ErrPeerUnknown", err)
	}
}

// Unpairing on the reader removes its link and asks the read node to drop the grant.
func TestUnpairClearsTheLinkAndTheGrant(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	if err := n.pairFromB(n.issueCode(t, n.a)); err != nil {
		t.Fatal(err)
	}
	if err := n.b.Unpair(ctx); err != nil {
		t.Fatal(err)
	}
	if state, err := n.b.State(ctx); err != nil || state.State != domain.PeerUnpaired || state.Link != nil {
		t.Errorf("解除後の B の State = %+v, %v", state, err)
	}
	if n.storeA.storedGrant() != nil {
		t.Errorf("解除後も A に grant が残る: %+v", n.storeA.storedGrant())
	}
	if _, err := n.b.ChangeAddress(ctx, addressA); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("解除後の ChangeAddress err = %v, want ErrNotFound", err)
	}
}

func testFingerprint(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// A behind self-signed HTTPS: B's owner confirms A's certificate, which B pins. B's own address and
// certificate play no part, since A never calls B.
func TestPairingPinsTheReadNodesCertificate(t *testing.T) {
	ctx := context.Background()
	const httpsA = "https://a.local"
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	spkiA := testFingerprint("a")
	n.link.nodes = map[string]*Pairing{httpsA: n.a}
	n.link.certificates = map[string]string{httpsA: spkiA}
	code := n.issueCode(t, n.a)

	_, err := n.b.Pair(ctx, domain.PairInput{Address: httpsA, Code: code})
	var untrusted *domain.CertificateUntrustedError
	if !errors.As(err, &untrusted) || untrusted.Fingerprint != spkiA {
		t.Fatalf("確認前の Pair err = %v, want 指紋 %s の CertificateUntrustedError", err, spkiA)
	}
	n.assertNothingStored(t)
	if _, err := n.b.Pair(ctx, domain.PairInput{Address: httpsA, Code: code, Fingerprint: spkiA}); err != nil {
		t.Fatalf("確認後の Pair err = %v", err)
	}
	if link := n.storeB.storedLink(); link == nil || link.PinnedSPKI != spkiA {
		t.Errorf("B の link = %+v, want 指紋 %s を固定", link, spkiA)
	}
}
