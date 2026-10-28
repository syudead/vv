package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// PeerStore holds the outgoing link, the incoming grant and this node's id; it hashes the grant's secret itself.
type PeerStore interface {
	// EnsureNodeID saves candidate when no node id exists and returns the stored one.
	EnsureNodeID(ctx context.Context, candidate string) (string, error)
	Link(ctx context.Context) (domain.PeerLink, bool, error)
	// AddLink answers domain.ErrAlreadyPaired when a link exists.
	AddLink(ctx context.Context, link domain.PeerLink) error
	// UpdateLinkAddress answers domain.ErrNotFound without a link.
	UpdateLinkAddress(ctx context.Context, address string) (domain.PeerLink, error)
	// PinLinkCertificate stores the SPKI fingerprint the owner confirmed.
	PinLinkCertificate(ctx context.Context, nodeID, spki string) error
	// MarkLinkRevoked records that the peer refused the link's secret.
	MarkLinkRevoked(ctx context.Context, nodeID string) error
	DeleteLink(ctx context.Context, nodeID string) (bool, error)
	Grant(ctx context.Context) (domain.PeerGrant, bool, error)
	// AddGrant answers domain.ErrAlreadyPaired when a grant exists.
	AddGrant(ctx context.Context, grant domain.PeerGrant, secret string) error
	GrantBySecret(ctx context.Context, secret string) (domain.PeerGrant, bool, error)
	DeleteGrant(ctx context.Context, nodeID string) (bool, error)
}

// PeerClient calls the node this one reads. A failed call answers *domain.PeerUnreachableError or
// *domain.CertificateUntrustedError; a refusal answers the domain error the other node named.
type PeerClient interface {
	RequestPairing(ctx context.Context, target domain.PeerTarget, req domain.PairingRequest) (domain.PairingAnswer, error)
	// Handshake presents secret and returns the identity the node at target answers with.
	Handshake(ctx context.Context, target domain.PeerTarget, secret string) (domain.PeerNode, error)
	// Watch holds the peer's event stream open with secret: it calls opened once the stream is open and
	// changed for each forwarded notification, and returns a non-nil error when the stream ends.
	Watch(ctx context.Context, target domain.PeerTarget, secret string, opened func(), changed func(domain.PeerChanged)) error
	// Forget tells the peer that the node nodeID stopped reading it.
	Forget(ctx context.Context, target domain.PeerTarget, secret, nodeID string) error
}

type PairingOptions struct {
	Store  PeerStore
	Client PeerClient
	// Version is this node's release version, which the peer shows for reference.
	Version string
	// NodeName is this node's host name, shown on the peer's screens.
	NodeName string
	// Nil uses time.Now.
	Now func() time.Time
	// Nil uses crypto/rand.Reader for node ids, codes and secrets.
	Random io.Reader
	// Publisher receives domain.PeerStateChanged and the peer's forwarded domain.PeerChanged; nil drops them.
	Publisher Publisher
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// Pairing owns the pairing code, the outgoing link this node reads with its connection state, and the
// incoming grant of the node that reads this one. The two directions are independent: pairing creates one
// link here and one grant on the other node, and nothing else.
type Pairing struct {
	store     PeerStore
	client    PeerClient
	version   string
	nodeName  string
	now       func() time.Time
	random    io.Reader
	publisher Publisher
	logger    *slog.Logger

	// pairMu serializes pairings this node starts, which call the other node before storing the link;
	// acceptMu serializes the grants it gives. They are separate so a node cannot block itself.
	pairMu   sync.Mutex
	acceptMu sync.Mutex
	// revokeMu makes marking the link revoked and interrupting the loop exclusive, so an interrupted
	// attempt cannot revoke the link its interrupter acts on.
	revokeMu sync.Mutex

	mu     sync.Mutex
	nodeID string
	code   domain.PairingCodeSlot
	loaded bool
	status peerStatus
	// generation counts interruptions of the connection loop, so an attempt that was interrupted
	// cannot overwrite the state its interrupter set.
	generation uint64
	// stopAttempt ends the running connection attempt; nil while none runs.
	stopAttempt context.CancelFunc
	// wake ends the loop's wait after pairing, an address change or a certificate confirmation.
	wake chan struct{}
}

func NewPairing(opts PairingOptions) *Pairing {
	p := &Pairing{
		store: opts.Store, client: opts.Client, version: opts.Version, nodeName: opts.NodeName,
		now: opts.Now, random: opts.Random, publisher: opts.Publisher, logger: opts.Logger,
		wake: make(chan struct{}, 1),
	}
	if p.logger == nil {
		p.logger = slog.Default()
	}
	if p.now == nil {
		p.now = time.Now
	}
	if p.random == nil {
		p.random = rand.Reader
	}
	return p
}

// Node returns this node's identity, creating its id on first use.
func (p *Pairing) Node(ctx context.Context) (domain.PeerNode, error) {
	p.mu.Lock()
	id := p.nodeID
	p.mu.Unlock()
	if id == "" {
		candidate, err := domain.NewNodeID(p.random)
		if err != nil {
			return domain.PeerNode{}, err
		}
		if id, err = p.store.EnsureNodeID(ctx, candidate); err != nil {
			return domain.PeerNode{}, err
		}
		p.mu.Lock()
		p.nodeID = id
		p.mu.Unlock()
	}
	return domain.PeerNode{ID: id, Name: p.nodeName, Version: p.version}, nil
}

// State returns the link with its connection state, the grant and this node's identity.
func (p *Pairing) State(ctx context.Context) (domain.PeerState, error) {
	node, err := p.Node(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	link, err := p.link(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	grant, found, err := p.store.Grant(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	state := domain.PeerState{Link: link, Node: node}
	if found {
		state.Grant = &grant
	}
	status := p.loadStatus(link)
	state.State, state.Reason, state.PeerVersion, state.Fingerprint =
		status.state, status.reason, status.peerVersion, status.fingerprint
	return state, nil
}

// link reads the stored link as a pointer, nil without one.
func (p *Pairing) link(ctx context.Context) (*domain.PeerLink, error) {
	link, found, err := p.store.Link(ctx)
	if err != nil || !found {
		return nil, err
	}
	return &link, nil
}

// IssueCode replaces an unused code; a node another VVMDM already reads answers domain.ErrAlreadyPaired.
func (p *Pairing) IssueCode(ctx context.Context) (domain.PairingCode, error) {
	if _, found, err := p.store.Grant(ctx); err != nil {
		return domain.PairingCode{}, err
	} else if found {
		return domain.PairingCode{}, domain.ErrAlreadyPaired
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.code.Issue(p.random, p.now())
}

// Pair makes this node read the node at input.Address by sending that node's code; the link is stored once
// that node answered with the secret to present. The other node never connects back.
func (p *Pairing) Pair(ctx context.Context, input domain.PairInput) (domain.PeerState, error) {
	address, err := domain.NormalizePeerAddress(input.Address)
	if err != nil {
		return domain.PeerState{}, &domain.InvalidPeerAddressError{Field: "address"}
	}
	p.pairMu.Lock()
	defer p.pairMu.Unlock()
	if _, found, err := p.store.Link(ctx); err != nil {
		return domain.PeerState{}, err
	} else if found {
		return domain.PeerState{}, domain.ErrAlreadyPaired
	}
	node, err := p.Node(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	target := domain.PeerTarget{Address: address, PinnedSPKI: input.Fingerprint}
	answer, err := p.client.RequestPairing(ctx, target, domain.PairingRequest{Code: input.Code, Node: node})
	if err != nil {
		return domain.PeerState{}, err
	}
	if err := checkPairingAnswer(answer, node.ID); err != nil {
		return domain.PeerState{}, err
	}
	link := domain.PeerLink{
		NodeID: answer.Node.ID, Name: answer.Node.Name, Address: address, Secret: answer.Secret,
		PinnedSPKI: input.Fingerprint, PairedAt: p.now(),
	}
	if err := p.store.AddLink(ctx, link); err != nil {
		return domain.PeerState{}, err
	}
	generation := p.interrupt(true)
	p.setStatus(generation, peerStatus{state: domain.PeerConnected, peerVersion: answer.Node.Version})
	return p.State(ctx)
}

func checkPairingAnswer(answer domain.PairingAnswer, ownID string) error {
	if !domain.ValidNodeID(answer.Node.ID) || answer.Node.ID == ownID || !domain.ValidPeerName(answer.Node.Name) ||
		!domain.ValidPeerSecret(answer.Secret) {
		return &domain.PeerUnreachableError{Message: "The other VVMDM answered the pairing with values this one cannot use."}
	}
	return nil
}

// Accept answers another node's PairingRequest: it checks the code, stores the grant that lets that node
// read this one, and answers with the secret it will present. Only a pairing that succeeded clears the code.
func (p *Pairing) Accept(ctx context.Context, req domain.PairingRequest) (domain.PairingAnswer, error) {
	if !domain.ValidNodeID(req.Node.ID) || !domain.ValidPeerName(req.Node.Name) {
		return domain.PairingAnswer{}, domain.ErrInvalidPairingInput
	}
	p.acceptMu.Lock()
	defer p.acceptMu.Unlock()
	p.mu.Lock()
	accepted := p.code.Matches(req.Code, p.now())
	p.mu.Unlock()
	if !accepted {
		return domain.PairingAnswer{}, domain.ErrInvalidPairingCode
	}
	if _, found, err := p.store.Grant(ctx); err != nil {
		return domain.PairingAnswer{}, err
	} else if found {
		return domain.PairingAnswer{}, domain.ErrAlreadyPaired
	}
	node, err := p.Node(ctx)
	if err != nil {
		return domain.PairingAnswer{}, err
	}
	if req.Node.ID == node.ID {
		return domain.PairingAnswer{}, domain.ErrInvalidPairingInput
	}
	secret, err := domain.NewPeerSecret(p.random)
	if err != nil {
		return domain.PairingAnswer{}, err
	}
	grant := domain.PeerGrant{NodeID: req.Node.ID, Name: req.Node.Name, PairedAt: p.now()}
	if err := p.store.AddGrant(ctx, grant, secret); err != nil {
		return domain.PairingAnswer{}, err
	}
	p.mu.Lock()
	p.code.Clear()
	p.mu.Unlock()
	p.publish(domain.PeerStateChanged{})
	return domain.PairingAnswer{Node: node, Secret: secret}, nil
}

// Handshake answers this node's identity to a caller presenting the grant's secret; anything else answers
// domain.ErrPeerUnknown.
func (p *Pairing) Handshake(ctx context.Context, secret string) (domain.PeerNode, error) {
	valid, err := p.Authenticate(ctx, secret)
	if err != nil {
		return domain.PeerNode{}, err
	}
	if !valid {
		return domain.PeerNode{}, domain.ErrPeerUnknown
	}
	return p.Node(ctx)
}

// Authenticate reports whether secret is the credential of the node that reads this one; malformed values
// skip the store.
func (p *Pairing) Authenticate(ctx context.Context, secret string) (bool, error) {
	if !domain.ValidPeerSecret(secret) {
		return false, nil
	}
	_, found, err := p.store.GrantBySecret(ctx, secret)
	return found, err
}

// ChangeAddress replaces the link's address; without a link it answers domain.ErrNotFound.
func (p *Pairing) ChangeAddress(ctx context.Context, raw string) (domain.PeerState, error) {
	address, err := domain.NormalizePeerAddress(raw)
	if err != nil {
		return domain.PeerState{}, &domain.InvalidPeerAddressError{Field: "address"}
	}
	if _, err := p.store.UpdateLinkAddress(ctx, address); err != nil {
		return domain.PeerState{}, err
	}
	p.interrupt(true)
	return p.State(ctx)
}

// Unpair tells the read node, best effort, and deletes the link, which also clears a revoked state; without
// a link it does nothing. A node that was not told learns it from the 401 its reader's next request gets.
func (p *Pairing) Unpair(ctx context.Context) error {
	link, found, err := p.store.Link(ctx)
	if err != nil || !found {
		return err
	}
	// Stop the stream first, so the peer dropping it while it forgets this node is not a lost connection.
	p.interrupt(false)
	if !link.Revoked {
		p.tellPeerUnpaired(ctx, link)
	}
	deleted, err := p.store.DeleteLink(ctx, link.NodeID)
	if err != nil {
		return fmt.Errorf("cannot unpair: %w", err)
	}
	if deleted {
		generation := p.interrupt(true)
		p.setStatus(generation, peerStatus{state: domain.PeerUnpaired})
	}
	return nil
}

func (p *Pairing) tellPeerUnpaired(ctx context.Context, link domain.PeerLink) {
	node, err := p.Node(ctx)
	if err == nil {
		err = p.client.Forget(ctx, peerTarget(link), link.Secret, node.ID)
	}
	if err != nil {
		p.logger.Info("could not tell the read VVMDM that this one unpaired", slog.Any("error", err))
	}
}

// RevokeGrant withdraws the reading node's access; without a grant it does nothing. That node learns it from
// the 401 its next request gets and shows the link as revoked.
func (p *Pairing) RevokeGrant(ctx context.Context) error {
	grant, found, err := p.store.Grant(ctx)
	if err != nil || !found {
		return err
	}
	return p.Forget(ctx, grant.NodeID)
}

// Forget deletes the grant of nodeID, as the reading node asks after it unpaired.
func (p *Pairing) Forget(ctx context.Context, nodeID string) error {
	deleted, err := p.store.DeleteGrant(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("cannot revoke the peer grant: %w", err)
	}
	if deleted {
		p.publish(domain.PeerStateChanged{})
	}
	return nil
}
