package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"slices"
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
	// beforeAdd, when set, runs as AddPeer starts, before the record is stored.
	beforeAdd func(incoming string)
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
	if f.beforeAdd != nil {
		f.beforeAdd(incoming)
	}
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

func (f *fakePeerStore) RevokeIncomingSecret(_ context.Context, nodeID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record != nil && f.record.NodeID == nodeID {
		f.record.IncomingRevoked, f.incomingHash = true, [32]byte{}
	}
	return nil
}

func (f *fakePeerStore) PinPeerCertificate(_ context.Context, nodeID, spki string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record == nil || f.record.NodeID != nodeID {
		return domain.ErrNotFound
	}
	f.record.PinnedSPKI = spki
	return nil
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
	// handshakes lists the address of each handshake, the only call besides RequestPairing that carries a secret.
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
	l.requests++
	node, err := l.node(target)
	if err != nil {
		return domain.PairingAnswer{}, err
	}
	return node.Accept(ctx, req)
}

// Certificate presents the self-signed certificate of address, or one the roots verify.
func (l *directLink) Certificate(_ context.Context, address string) (domain.PresentedCertificate, error) {
	if _, err := l.node(domain.PeerTarget{Address: address, PinnedSPKI: l.certificates[address]}); err != nil {
		return domain.PresentedCertificate{}, err
	}
	spki, selfSigned := l.certificates[address]
	return domain.PresentedCertificate{Fingerprint: spki, Verified: !selfSigned}, nil
}

func (l *directLink) Handshake(ctx context.Context, target domain.PeerTarget, secret string) (domain.PeerNode, error) {
	l.handshakes = append(l.handshakes, target.Address)
	if l.failHandshake {
		return domain.PeerNode{}, &domain.PeerUnreachableError{Message: "callback refused"}
	}
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
		if state, err := node.State(ctx); err != nil || state.State != domain.PeerConnected || state.PeerVersion != "1.2.0" {
			t.Errorf("State = %+v, %v, want connected with PeerVersion 1.2.0", state, err)
		}
	}
	if err := n.pairFromB("ANYCODE2"); !errors.Is(err, domain.ErrAlreadyPaired) {
		t.Errorf("ペアリング済みの B の Pair err = %v, want ErrAlreadyPaired", err)
	}
}

func TestPairingRefusalsStoreNothing(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(t *testing.T, n *pairingNodes) string
		want    error
	}{
		{name: "誤ったコード", want: domain.ErrInvalidPairingCode,
			prepare: func(t *testing.T, n *pairingNodes) string {
				t.Helper()
				n.issueCodeOnA(t)
				return "WRONGCOD"
			}},
		{name: "期限切れのコード", want: domain.ErrInvalidPairingCode,
			prepare: func(t *testing.T, n *pairingNodes) string {
				t.Helper()
				code := n.issueCodeOnA(t)
				n.now = n.now.Add(domain.PairingCodeLifetime)
				return code
			}},
		{name: "コールバックの失敗", want: domain.ErrPeerUnreachable,
			prepare: func(t *testing.T, n *pairingNodes) string {
				t.Helper()
				n.link.failHandshake = true
				return n.issueCodeOnA(t)
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := newPairingNodes(t, "1.2.0", "1.2.0")
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

// The version is a build label, so builds of the same source can differ in it; it is shown, never compared.
func TestPairingIgnoresTheVersionLabels(t *testing.T) {
	ctx := context.Background()
	const sha = "f1672930f88eac63ac8f92b96985c8ef76e32036"
	n := newPairingNodes(t, sha, domain.DefaultVersion)
	if err := n.pairFromB(n.issueCodeOnA(t)); err != nil {
		t.Fatalf("Pair: %v", err)
	}
	for _, check := range []struct {
		label string
		node  *Pairing
		peer  string
	}{{"A", n.a, domain.DefaultVersion}, {"B", n.b, sha}} {
		if state, err := check.node.State(ctx); err != nil || state.State != domain.PeerConnected || state.PeerVersion != check.peer {
			t.Errorf("%s の State = %+v, %v, want connected with PeerVersion %s", check.label, state, err, check.peer)
		}
	}
}

func TestHandshakeAcceptsOnlyKnownSecrets(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	if _, err := n.a.Handshake(ctx, "vvp_unknown"); !errors.Is(err, domain.ErrPeerUnknown) {
		t.Errorf("記録の無い Handshake err = %v, want ErrPeerUnknown", err)
	}
	// A's connection loop may hand the sent secret back before B stored its record.
	var beforeStored error
	n.storeB.beforeAdd = func(incoming string) { _, beforeStored = n.b.Handshake(ctx, incoming) }
	if err := n.pairFromB(n.issueCodeOnA(t)); err != nil {
		t.Fatal(err)
	}
	if beforeStored != nil {
		t.Errorf("B の保存前の Handshake err = %v, want nil", beforeStored)
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

// A callback that fails must leave A's code usable for the retry; only the pairing that succeeds uses it up.
func TestFailedCallbackKeepsTheCode(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	code := n.issueCodeOnA(t)
	n.link.failHandshake = true
	if err := n.pairFromB(code); !errors.Is(err, domain.ErrPeerUnreachable) {
		t.Fatalf("失敗するコールバックの Pair err = %v, want ErrPeerUnreachable", err)
	}

	n.link.failHandshake = false
	if err := n.pairFromB(code); err != nil {
		t.Fatalf("同じコードでの再試行の Pair err = %v", err)
	}
	node, _ := n.b.Node(ctx)
	secret, _ := domain.NewPeerSecret(rand.Reader)
	if _, err := n.a.Accept(ctx, domain.PairingRequest{Code: code, Node: node, Address: addressB, Secret: secret}); !errors.Is(err, domain.ErrInvalidPairingCode) {
		t.Errorf("ペアリング後の同じコードの Accept err = %v, want ErrInvalidPairingCode", err)
	}
}

func testFingerprint(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return base64.StdEncoding.EncodeToString(sum[:])
}

const httpsA, httpsB = "https://a.local", "https://b.local"

// newSelfSignedNodes serves both nodes at https addresses whose self-signed certificates have spkiA and spkiB.
func newSelfSignedNodes(t *testing.T) (n *pairingNodes, spkiA, spkiB string) {
	t.Helper()
	n = newPairingNodes(t, "1.2.0", "1.2.0")
	spkiA, spkiB = testFingerprint("a"), testFingerprint("b")
	n.link.nodes = map[string]*Pairing{httpsA: n.a, httpsB: n.b}
	n.link.certificates = map[string]string{httpsA: spkiA, httpsB: spkiB}
	return n, spkiA, spkiB
}

// Both nodes behind self-signed HTTPS: B's owner confirms B's own certificate and A's, and A pins the one B's
// owner confirmed for the callback and the pairing. B never calls its own address with a secret.
func TestPairingPinsBothSelfSignedCertificates(t *testing.T) {
	ctx := context.Background()
	n, spkiA, spkiB := newSelfSignedNodes(t)
	code := n.issueCodeOnA(t)

	_, err := n.b.Pair(ctx, domain.PairInput{Address: httpsA, Code: code, OwnAddress: httpsB, OwnFingerprint: spkiB})
	var untrusted *domain.CertificateUntrustedError
	if !errors.As(err, &untrusted) || untrusted.Fingerprint != spkiA {
		t.Fatalf("A の確認前の Pair err = %v, want A の指紋 %s の CertificateUntrustedError", err, spkiA)
	}
	confirmed := domain.PairInput{Address: httpsA, Code: code, OwnAddress: httpsB, Fingerprint: spkiA, OwnFingerprint: spkiB}
	if _, err := n.b.Pair(ctx, confirmed); err != nil {
		t.Fatalf("両方の証明書を確認した Pair err = %v", err)
	}
	if onA, onB := n.storeA.stored(), n.storeB.stored(); onA == nil || onB == nil ||
		onA.PinnedSPKI != spkiB || onB.PinnedSPKI != spkiA {
		t.Errorf("固定した指紋: A %+v, B %+v, want A に %s, B に %s", onA, onB, spkiB, spkiA)
	}
	// Only A's callback handshakes B's address; a probe carrying the pending secret would add calls.
	if want := []string{httpsB}; !slices.Equal(n.link.handshakes, want) {
		t.Errorf("handshake の宛先 = %q, want A のコールバックだけの %q", n.link.handshakes, want)
	}
}

// An own certificate the owner did not confirm, or confirmed as another one, stops the pairing before A is
// called, naming the certificate the address presents now.
func TestPairRefusesAnUnconfirmedOwnCertificateBeforeCallingThePeer(t *testing.T) {
	for _, confirmed := range []string{"", testFingerprint("other")} {
		n, spkiA, spkiB := newSelfSignedNodes(t)
		code := n.issueCodeOnA(t)
		_, err := n.b.Pair(context.Background(), domain.PairInput{
			Address: httpsA, Code: code, OwnAddress: httpsB, Fingerprint: spkiA, OwnFingerprint: confirmed,
		})
		var unverified *domain.OwnCertificateUnverifiedError
		if !errors.As(err, &unverified) || unverified.Fingerprint != spkiB {
			t.Errorf("自分の指紋 %q の Pair err = %v, want 指紋 %s の OwnCertificateUnverifiedError", confirmed, err, spkiB)
		}
		if n.link.requests != 0 || len(n.link.handshakes) != 0 {
			t.Errorf("自分の指紋 %q: ペアリング要求 %d 回、handshake %q, want どちらも無し", confirmed, n.link.requests, n.link.handshakes)
		}
		n.assertNothingStored(t)
	}
}

func TestAcceptRefusesAFingerprintWithoutHTTPSOrForm(t *testing.T) {
	ctx := context.Background()
	n := newPairingNodes(t, "1.2.0", "1.2.0")
	code := n.issueCodeOnA(t)
	node, _ := n.b.Node(ctx)
	secret, _ := domain.NewPeerSecret(rand.Reader)
	for _, tc := range []struct{ address, fingerprint string }{
		{addressB, testFingerprint("b")},
		{"https://b.local", "not-a-fingerprint"},
	} {
		_, err := n.a.Accept(ctx, domain.PairingRequest{
			Code: code, Node: node, Address: tc.address, Secret: secret, Fingerprint: tc.fingerprint,
		})
		if !errors.Is(err, domain.ErrInvalidPairingInput) {
			t.Errorf("%s と %q の Accept err = %v, want ErrInvalidPairingInput", tc.address, tc.fingerprint, err)
		}
	}
	n.assertNothingStored(t)
}
