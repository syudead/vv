package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakePeerStore keeps the record and the hash of the incoming secret like the SQLite role.
type fakePeerStore struct {
	mu           sync.Mutex
	nodeID       string
	record       *domain.PeerRecord
	incomingHash [32]byte
}

func (f *fakePeerStore) EnsureNodeID(_ context.Context, candidate string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.nodeID == "" {
		f.nodeID = candidate
	}
	return f.nodeID, nil
}

func (f *fakePeerStore) Peer(context.Context) (domain.PeerRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record == nil {
		return domain.PeerRecord{}, false, nil
	}
	return *f.record, true, nil
}

func (f *fakePeerStore) AddPeer(_ context.Context, record domain.PeerRecord, incoming string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record != nil {
		return domain.ErrAlreadyPaired
	}
	f.record, f.incomingHash = &record, sha256.Sum256([]byte(incoming))
	return nil
}

func (f *fakePeerStore) PeerBySecret(_ context.Context, secret string) (domain.PeerRecord, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record == nil || f.incomingHash != sha256.Sum256([]byte(secret)) {
		return domain.PeerRecord{}, false, nil
	}
	return *f.record, true, nil
}

func (f *fakePeerStore) UpdatePeerAddress(_ context.Context, address string) (domain.PeerRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record == nil {
		return domain.PeerRecord{}, domain.ErrNotFound
	}
	f.record.Address = address
	return *f.record, nil
}

func (f *fakePeerStore) DeletePeer(_ context.Context, nodeID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record == nil || f.record.NodeID != nodeID {
		return false, nil
	}
	f.record = nil
	return true, nil
}

func (f *fakePeerStore) stored() *domain.PeerRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.record
}

// directLink routes calls to the Pairing registered at the address, as two nodes on one LAN.
type directLink struct {
	nodes map[string]*Pairing
	// failHandshake makes every callback fail, as an unreachable return address.
	failHandshake bool
	requests      int
}

func (l *directLink) node(target domain.PeerTarget) (*Pairing, error) {
	node, ok := l.nodes[target.Address]
	if !ok {
		return nil, &domain.PeerUnreachableError{Message: "no node at " + target.Address}
	}
	return node, nil
}

func (l *directLink) Version(_ context.Context, target domain.PeerTarget) (string, error) {
	node, err := l.node(target)
	if err != nil {
		return "", err
	}
	return node.version, nil
}

func (l *directLink) RequestPairing(
	ctx context.Context, target domain.PeerTarget, req domain.PairingRequest,
) (domain.PairingAnswer, error) {
	l.requests++
	node, err := l.node(target)
	if err != nil {
		return domain.PairingAnswer{}, err
	}
	return node.Accept(ctx, req)
}

func (l *directLink) Handshake(ctx context.Context, target domain.PeerTarget, secret string) (domain.PeerNode, error) {
	if l.failHandshake {
		return domain.PeerNode{}, &domain.PeerUnreachableError{Message: "callback refused"}
	}
	node, err := l.node(target)
	if err != nil {
		return domain.PeerNode{}, err
	}
	return node.Handshake(ctx, secret)
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
	n.a = NewPairing(PairingOptions{Store: n.storeA, Link: n.link, Version: versionA, NodeName: "a", Now: clock})
	n.b = NewPairing(PairingOptions{Store: n.storeB, Link: n.link, Version: versionB, NodeName: "b", Now: clock})
	n.link.nodes[addressA], n.link.nodes[addressB] = n.a, n.b
	return n
}

func (n *pairingNodes) issueCodeOnA(t *testing.T) string {
	t.Helper()
	code, err := n.a.IssueCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return code.Value
}

func (n *pairingNodes) pairFromB(code string) error {
	_, err := n.b.Pair(context.Background(), domain.PairInput{Address: addressA, Code: code, OwnAddress: addressB})
	return err
}

func (n *pairingNodes) assertNothingStored(t *testing.T) {
	t.Helper()
	if n.storeA.stored() != nil || n.storeB.stored() != nil {
		t.Errorf("記録が保存された: A %+v, B %+v", n.storeA.stored(), n.storeB.stored())
	}
}

func TestPairingStoresBothSecrets(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	if err := n.pairFromB(n.issueCodeOnA(t)); err != nil {
		t.Fatalf("Pair: %v", err)
	}

	onA, onB := n.storeA.stored(), n.storeB.stored()
	if onA == nil || onB == nil {
		t.Fatalf("両方に記録が無い: A %+v, B %+v", onA, onB)
	}
	nodeA, _ := n.a.Node(ctx)
	nodeB, _ := n.b.Node(ctx)
	if onA.NodeID != nodeB.ID || onA.Address != addressB || onA.Name != "b" {
		t.Errorf("A の記録 = %+v, want B (%s)", onA, nodeB.ID)
	}
	if onB.NodeID != nodeA.ID || onB.Address != addressA || onB.Name != "a" {
		t.Errorf("B の記録 = %+v, want A (%s)", onB, nodeA.ID)
	}
	// Each node's outgoing secret is the other's incoming credential.
	for _, check := range []struct {
		label    string
		receiver *Pairing
		secret   string
	}{{"B が A に示す secret", n.a, onB.OutgoingSecret}, {"A が B に示す secret", n.b, onA.OutgoingSecret}} {
		if valid, err := check.receiver.Authenticate(ctx, check.secret); err != nil || !valid {
			t.Errorf("%s が通らない: %v, %v", check.label, valid, err)
		}
	}
	if onA.OutgoingSecret == onB.OutgoingSecret {
		t.Error("両方向の secret が同じ")
	}
	for _, node := range []*Pairing{n.a, n.b} {
		if state, err := node.State(ctx); err != nil || state.State != domain.PeerConnected {
			t.Errorf("State = %+v, %v, want connected", state, err)
		}
	}
	if err := n.pairFromB("ANYCODE2"); !errors.Is(err, domain.ErrAlreadyPaired) {
		t.Errorf("ペアリング済みの B の Pair err = %v, want ErrAlreadyPaired", err)
	}
}

func TestPairingRefusalsStoreNothing(t *testing.T) {
	cases := []struct {
		name     string
		versionB string
		prepare  func(t *testing.T, n *pairingNodes) string
		want     error
	}{
		{name: "誤ったコード", versionB: "1.2.0", want: domain.ErrInvalidPairingCode,
			prepare: func(t *testing.T, n *pairingNodes) string {
				t.Helper()
				n.issueCodeOnA(t)
				return "WRONGCOD"
			}},
		{name: "期限切れのコード", versionB: "1.2.0", want: domain.ErrInvalidPairingCode,
			prepare: func(t *testing.T, n *pairingNodes) string {
				t.Helper()
				code := n.issueCodeOnA(t)
				n.now = n.now.Add(domain.PairingCodeLifetime)
				return code
			}},
		{name: "バージョンの不一致", versionB: "1.3.0", want: domain.ErrUnsupportedVersion,
			prepare: func(t *testing.T, n *pairingNodes) string {
				t.Helper()
				return n.issueCodeOnA(t)
			}},
		{name: "コールバックの失敗", versionB: "1.2.0", want: domain.ErrPeerUnreachable,
			prepare: func(t *testing.T, n *pairingNodes) string {
				t.Helper()
				n.link.failHandshake = true
				return n.issueCodeOnA(t)
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := newPairingNodes(t, "1.2.0", tc.versionB)
			err := n.pairFromB(tc.prepare(t, n))
			if !errors.Is(err, tc.want) {
				t.Fatalf("Pair err = %v, want %v", err, tc.want)
			}
			n.assertNothingStored(t)
		})
	}
}

func TestAcceptRefusesAPairedNode(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	code := n.issueCodeOnA(t)
	existing := domain.PeerRecord{NodeID: "7c2f4c3a-1d2e-4f50-8a6b-0c1d2e3f4a5b", Name: "c", Address: "http://c.local"}
	if err := n.storeA.AddPeer(ctx, existing, "vvp_c"); err != nil {
		t.Fatal(err)
	}

	if err := n.pairFromB(code); !errors.Is(err, domain.ErrAlreadyPaired) {
		t.Fatalf("Pair err = %v, want ErrAlreadyPaired", err)
	}
	if got := n.storeA.stored(); got.NodeID != existing.NodeID {
		t.Errorf("A の記録が置き換わった: %+v", got)
	}
	if n.storeB.stored() != nil {
		t.Errorf("B に記録が保存された: %+v", n.storeB.stored())
	}
	if _, err := n.a.IssueCode(ctx); !errors.Is(err, domain.ErrAlreadyPaired) {
		t.Errorf("ペアリング済みの IssueCode err = %v, want ErrAlreadyPaired", err)
	}
}

func TestPairVersionMismatchCallsNoPairing(t *testing.T) {
	n := newPairingNodes(t, "1.2.0", "dev")
	var version *domain.UnsupportedVersionError
	if err := n.pairFromB(n.issueCodeOnA(t)); !errors.As(err, &version) || version.Local != "dev" || version.Peer != "1.2.0" {
		t.Fatalf("Pair err = %v, want 両方の版を持つ UnsupportedVersionError", err)
	}
	if n.link.requests != 0 {
		t.Errorf("版の確認の後に A へ %d 回ペアリングを送った", n.link.requests)
	}
}

func TestHandshakeAcceptsOnlyKnownSecrets(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	if _, err := n.a.Handshake(ctx, "vvp_unknown"); !errors.Is(err, domain.ErrPeerUnknown) {
		t.Errorf("記録の無い Handshake err = %v, want ErrPeerUnknown", err)
	}
	if err := n.pairFromB(n.issueCodeOnA(t)); err != nil {
		t.Fatal(err)
	}
	// The secret B sent was accepted only while its pairing ran; afterwards only the stored credential is.
	onB := n.storeB.stored()
	if node, err := n.a.Handshake(ctx, onB.OutgoingSecret); err != nil || node.Name != "a" {
		t.Errorf("Handshake = %+v, %v", node, err)
	}
}

func TestUnpairClearsTheRecordAndState(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	if err := n.pairFromB(n.issueCodeOnA(t)); err != nil {
		t.Fatal(err)
	}
	if err := n.b.Unpair(ctx); err != nil {
		t.Fatal(err)
	}
	if state, err := n.b.State(ctx); err != nil || state.State != domain.PeerUnpaired || state.Peer != nil {
		t.Errorf("解除後の State = %+v, %v", state, err)
	}
	if _, err := n.b.ChangeAddress(ctx, addressA); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("解除後の ChangeAddress err = %v, want ErrNotFound", err)
	}
}
