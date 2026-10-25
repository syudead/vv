package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// PeerStore holds the peer record and this node's id; it hashes incoming secrets itself.
type PeerStore interface {
	// EnsureNodeID saves candidate when no node id exists and returns the stored one.
	EnsureNodeID(ctx context.Context, candidate string) (string, error)
	Peer(ctx context.Context) (domain.PeerRecord, bool, error)
	// AddPeer answers domain.ErrAlreadyPaired when a record exists.
	AddPeer(ctx context.Context, record domain.PeerRecord, incomingSecret string) error
	// PeerBySecret never matches a record whose incoming secret was revoked.
	PeerBySecret(ctx context.Context, secret string) (domain.PeerRecord, bool, error)
	// UpdatePeerAddress answers domain.ErrNotFound without a record.
	UpdatePeerAddress(ctx context.Context, address string) (domain.PeerRecord, error)
	DeletePeer(ctx context.Context, nodeID string) (bool, error)
}

// PeerLink calls the other node. A failed call answers *domain.PeerUnreachableError or
// *domain.CertificateUntrustedError; a refusal answers the domain error the other node named.
type PeerLink interface {
	// Version reads the release version the node at target reports in its health.
	Version(ctx context.Context, target domain.PeerTarget) (string, error)
	RequestPairing(ctx context.Context, target domain.PeerTarget, req domain.PairingRequest) (domain.PairingAnswer, error)
	// Handshake presents secret and returns the identity the node at target answers with.
	Handshake(ctx context.Context, target domain.PeerTarget, secret string) (domain.PeerNode, error)
}

type PairingOptions struct {
	Store PeerStore
	Link  PeerLink
	// Version is this node's release version, compared with the peer's.
	Version string
	// NodeName is this node's host name, shown on the peer's screens.
	NodeName string
	// Nil uses time.Now.
	Now func() time.Time
	// Nil uses crypto/rand.Reader for node ids, codes and secrets.
	Random io.Reader
}

// Pairing owns the pairing code, the pairing in both directions, unpairing and the connection state.
type Pairing struct {
	store    PeerStore
	link     PeerLink
	version  string
	nodeName string
	now      func() time.Time
	random   io.Reader

	// pairMu serializes pairing and acceptance, which each call the other node before storing.
	pairMu sync.Mutex

	mu      sync.Mutex
	nodeID  string
	code    domain.PairingCodeSlot
	pending map[string]struct{}
	loaded  bool
	state   domain.PeerConnection
	reason  domain.PeerUnreachableReason
}

func NewPairing(opts PairingOptions) *Pairing {
	p := &Pairing{
		store: opts.Store, link: opts.Link, version: opts.Version, nodeName: opts.NodeName,
		now: opts.Now, random: opts.Random, pending: map[string]struct{}{},
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

// State returns the connection state with the record and this node's identity.
func (p *Pairing) State(ctx context.Context) (domain.PeerState, error) {
	node, err := p.Node(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	record, found, err := p.store.Peer(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var peer *domain.PeerRecord
	if found {
		peer = &record
	}
	if !p.loaded || peer == nil {
		// Without a record the state is unpaired whatever was held; a first read derives it from the record.
		p.state, p.reason = domain.InitialPeerState(peer)
		p.loaded = true
	}
	return domain.PeerState{State: p.state, Reason: p.reason, Peer: peer, Node: node}, nil
}

// IssueCode replaces an unused code; a paired node answers domain.ErrAlreadyPaired.
func (p *Pairing) IssueCode(ctx context.Context) (domain.PairingCode, error) {
	if _, found, err := p.store.Peer(ctx); err != nil {
		return domain.PairingCode{}, err
	} else if found {
		return domain.PairingCode{}, domain.ErrAlreadyPaired
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.code.Issue(p.random, p.now())
}

// Pair pairs this node with the node at input.Address by sending that node's code; both nodes store
// their record only after the other node called this one back.
func (p *Pairing) Pair(ctx context.Context, input domain.PairInput) (domain.PeerState, error) {
	address, err := domain.NormalizePeerAddress(input.Address)
	if err != nil {
		return domain.PeerState{}, &domain.InvalidPeerAddressError{Field: "address"}
	}
	ownAddress, err := domain.NormalizePeerAddress(input.OwnAddress)
	if err != nil {
		return domain.PeerState{}, &domain.InvalidPeerAddressError{Field: "ownAddress"}
	}
	p.pairMu.Lock()
	defer p.pairMu.Unlock()
	if err := p.refuseWhenPaired(ctx); err != nil {
		return domain.PeerState{}, err
	}
	node, err := p.Node(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	target := domain.PeerTarget{Address: address, PinnedSPKI: input.Fingerprint}
	peerVersion, err := p.link.Version(ctx, target)
	if err != nil {
		return domain.PeerState{}, err
	}
	if err := domain.CheckPeerVersions(p.version, peerVersion); err != nil {
		return domain.PeerState{}, err
	}
	incoming, err := domain.NewPeerSecret(p.random)
	if err != nil {
		return domain.PeerState{}, err
	}
	// The other node calls this one back with incoming before it answers.
	release := p.expectHandshake(incoming)
	answer, err := p.link.RequestPairing(ctx, target, domain.PairingRequest{
		Code: input.Code, Node: node, Address: ownAddress, Secret: incoming,
	})
	release()
	if err != nil {
		return domain.PeerState{}, err
	}
	if err := checkPairingAnswer(answer, node.ID, p.version); err != nil {
		return domain.PeerState{}, err
	}
	record := domain.PeerRecord{
		NodeID: answer.Node.ID, Name: answer.Node.Name, Address: address,
		OutgoingSecret: answer.Secret, PinnedSPKI: input.Fingerprint, PairedAt: p.now(),
	}
	if err := p.store.AddPeer(ctx, record, incoming); err != nil {
		return domain.PeerState{}, err
	}
	p.paired()
	return p.State(ctx)
}

func checkPairingAnswer(answer domain.PairingAnswer, ownID, version string) error {
	if !domain.ValidNodeID(answer.Node.ID) || answer.Node.ID == ownID || !domain.ValidPeerName(answer.Node.Name) ||
		!domain.ValidPeerSecret(answer.Secret) {
		return &domain.PeerUnreachableError{Message: "The other VVMDM answered the pairing with values this one cannot use."}
	}
	return domain.CheckPeerVersions(version, answer.Node.Version)
}

// Accept answers another node's PairingRequest: it checks the code, calls the sender back at its
// address with the secret it sent, and stores the record only after that succeeds.
func (p *Pairing) Accept(ctx context.Context, req domain.PairingRequest) (domain.PairingAnswer, error) {
	address, err := domain.NormalizePeerAddress(req.Address)
	if err != nil {
		return domain.PairingAnswer{}, &domain.InvalidPeerAddressError{Field: "node.address"}
	}
	if !domain.ValidNodeID(req.Node.ID) || !domain.ValidPeerName(req.Node.Name) || !domain.ValidPeerSecret(req.Secret) {
		return domain.PairingAnswer{}, domain.ErrInvalidPairingInput
	}
	p.pairMu.Lock()
	defer p.pairMu.Unlock()
	p.mu.Lock()
	accepted := p.code.Consume(req.Code, p.now())
	p.mu.Unlock()
	if !accepted {
		return domain.PairingAnswer{}, domain.ErrInvalidPairingCode
	}
	if err := p.refuseWhenPaired(ctx); err != nil {
		return domain.PairingAnswer{}, err
	}
	node, err := p.Node(ctx)
	if err != nil {
		return domain.PairingAnswer{}, err
	}
	if req.Node.ID == node.ID {
		return domain.PairingAnswer{}, domain.ErrInvalidPairingInput
	}
	if err := domain.CheckPeerVersions(p.version, req.Node.Version); err != nil {
		return domain.PairingAnswer{}, err
	}
	if err := p.callBack(ctx, address, req); err != nil {
		return domain.PairingAnswer{}, err
	}
	incoming, err := domain.NewPeerSecret(p.random)
	if err != nil {
		return domain.PairingAnswer{}, err
	}
	record := domain.PeerRecord{
		NodeID: req.Node.ID, Name: req.Node.Name, Address: address, OutgoingSecret: req.Secret, PairedAt: p.now(),
	}
	if err := p.store.AddPeer(ctx, record, incoming); err != nil {
		return domain.PairingAnswer{}, err
	}
	p.paired()
	return domain.PairingAnswer{Node: node, Secret: incoming}, nil
}

// callBack proves the sender's address reaches the node that sent the request.
func (p *Pairing) callBack(ctx context.Context, address string, req domain.PairingRequest) error {
	answered, err := p.link.Handshake(ctx, domain.PeerTarget{Address: address}, req.Secret)
	if err != nil {
		var untrusted *domain.CertificateUntrustedError
		if errors.As(err, &untrusted) || errors.Is(err, domain.ErrPeerUnreachable) {
			return err
		}
		return &domain.PeerUnreachableError{
			Message: "This VVMDM could not call the other one back at " + address + ".", Cause: err,
		}
	}
	if answered.ID != req.Node.ID {
		return &domain.PeerUnreachableError{
			Message: "The address " + address + " answers as another VVMDM than the one pairing.",
		}
	}
	return domain.CheckPeerVersions(p.version, answered.Version)
}

func (p *Pairing) refuseWhenPaired(ctx context.Context) error {
	_, found, err := p.store.Peer(ctx)
	if err != nil {
		return err
	}
	if found {
		return domain.ErrAlreadyPaired
	}
	return nil
}

func (p *Pairing) expectHandshake(secret string) func() {
	p.mu.Lock()
	p.pending[secret] = struct{}{}
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		delete(p.pending, secret)
		p.mu.Unlock()
	}
}

func (p *Pairing) paired() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.code.Clear()
	p.loaded = true
	p.state, p.reason = domain.PeerConnected, ""
}

// Handshake answers this node's identity to a caller presenting the peer credential, or the secret
// a pairing in progress sent; anything else answers domain.ErrPeerUnknown.
func (p *Pairing) Handshake(ctx context.Context, secret string) (domain.PeerNode, error) {
	p.mu.Lock()
	_, pending := p.pending[secret]
	p.mu.Unlock()
	if !pending {
		valid, err := p.Authenticate(ctx, secret)
		if err != nil {
			return domain.PeerNode{}, err
		}
		if !valid {
			return domain.PeerNode{}, domain.ErrPeerUnknown
		}
	}
	return p.Node(ctx)
}

// Authenticate reports whether secret is the peer credential; malformed values skip the store.
func (p *Pairing) Authenticate(ctx context.Context, secret string) (bool, error) {
	if !domain.ValidPeerSecret(secret) {
		return false, nil
	}
	_, found, err := p.store.PeerBySecret(ctx, secret)
	return found, err
}

// ChangeAddress replaces the peer's address; without a record it answers domain.ErrNotFound.
func (p *Pairing) ChangeAddress(ctx context.Context, raw string) (domain.PeerState, error) {
	address, err := domain.NormalizePeerAddress(raw)
	if err != nil {
		return domain.PeerState{}, &domain.InvalidPeerAddressError{Field: "address"}
	}
	if _, err := p.store.UpdatePeerAddress(ctx, address); err != nil {
		return domain.PeerState{}, err
	}
	return p.State(ctx)
}

// Unpair deletes the peer record, which also clears a revoked state; without a record it does nothing.
func (p *Pairing) Unpair(ctx context.Context) error {
	record, found, err := p.store.Peer(ctx)
	if err != nil || !found {
		return err
	}
	return p.Forget(ctx, record.NodeID)
}

// Forget deletes the record of nodeID, as the peer asks after it unpaired.
func (p *Pairing) Forget(ctx context.Context, nodeID string) error {
	deleted, err := p.store.DeletePeer(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("cannot unpair: %w", err)
	}
	if deleted {
		p.mu.Lock()
		p.loaded = true
		p.state, p.reason = domain.PeerUnpaired, ""
		p.mu.Unlock()
	}
	return nil
}
