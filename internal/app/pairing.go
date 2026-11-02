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

// PeerStore holds the outgoing links, the incoming grant and this node's id; it hashes the grant's secret itself.
type PeerStore interface {
	// EnsureNodeID saves candidate when no node id exists and returns the stored one.
	EnsureNodeID(ctx context.Context, candidate string) (string, error)
	// Links answers the links in slot order.
	Links(ctx context.Context) ([]domain.PeerLink, error)
	// AddLink stores link in the next slot and returns it with that slot; it enforces domain.CheckNewLink.
	AddLink(ctx context.Context, link domain.PeerLink) (domain.PeerLink, error)
	// UpdateLinkAddress answers domain.ErrNotFound without the link of nodeID.
	UpdateLinkAddress(ctx context.Context, nodeID, address string) (domain.PeerLink, error)
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
	// PeerFolders answers the snapshot of the link's media folders in the link's id order.
	PeerFolders(ctx context.Context, nodeID string) ([]domain.PeerMediaFolder, error)
	// ReplacePeerFolders applies domain.ReconcilePeerFolders and reports a change; without the link it answers
	// domain.ErrNotFound.
	ReplacePeerFolders(ctx context.Context, nodeID string, listed []domain.PeerFolder, now time.Time) (bool, error)
	// SetPeerFolderEnabled answers the row and whether it changed, or domain.ErrNotFound without it.
	SetPeerFolderEnabled(ctx context.Context, nodeID string, folderID int64, enabled bool) (domain.PeerMediaFolder, bool, error)
}

// PeerClient calls a node this one reads. A failed call answers *domain.PeerUnreachableError or
// *domain.CertificateUntrustedError; a refusal answers the domain error the other node named.
type PeerClient interface {
	RequestPairing(ctx context.Context, target domain.PeerTarget, req domain.PairingRequest) (domain.PairingAnswer, error)
	// Handshake presents secret and returns the identity the node at target answers with.
	Handshake(ctx context.Context, target domain.PeerTarget, secret string) (domain.PeerNode, error)
	// Watch holds the peer's event stream open with secret: it calls opened once the stream is open and
	// changed for each forwarded notification in the peer's own ids, and returns a non-nil error when the
	// stream ends.
	Watch(ctx context.Context, target domain.PeerTarget, secret string, opened func(), changed func(domain.PeerChanged)) error
	// Forget tells the peer that the node nodeID stopped reading it.
	Forget(ctx context.Context, target domain.PeerTarget, secret, nodeID string) error
	// MediaFolders lists the peer's media folders in its own ids.
	MediaFolders(ctx context.Context, target domain.PeerTarget, secret string) ([]domain.PeerFolder, error)
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
	// Publisher receives domain.PeerStateChanged, the links' forwarded domain.PeerChanged and the
	// domain.MediaFoldersChanged of the links' snapshots; nil drops them.
	Publisher Publisher
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// Pairing owns the pairing code, the outgoing links this node reads with one connection loop and state each, and
// the incoming grant of the node that reads this one. The two directions are independent: pairing creates one
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

	mu     sync.Mutex
	nodeID string
	code   domain.PairingCodeSlot
	// conns holds each link's connection by node id, made on the first read of the link.
	conns map[string]*linkConn
	// paired hands Run the connection of each new link; a link paired while Run is not running is started from
	// the store when it starts.
	paired chan *linkConn
}

func NewPairing(opts PairingOptions) *Pairing {
	p := &Pairing{
		store: opts.Store, client: opts.Client, version: opts.Version, nodeName: opts.NodeName,
		now: opts.Now, random: opts.Random, publisher: opts.Publisher, logger: opts.Logger,
		conns: map[string]*linkConn{}, paired: make(chan *linkConn, domain.MaxPeerLinks),
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

// State returns the links in slot order with their connection states, the grant and this node's identity.
func (p *Pairing) State(ctx context.Context) (domain.PeerState, error) {
	node, err := p.Node(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	links, err := p.store.Links(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	grant, found, err := p.store.Grant(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	state := domain.PeerState{Node: node, Links: make([]domain.PeerLinkState, len(links))}
	if found {
		state.Grant = &grant
	}
	for i, link := range links {
		status := p.loadStatus(link)
		state.Links[i] = domain.PeerLinkState{
			Link: link, State: status.state, Reason: status.reason, PeerVersion: status.peerVersion,
			Fingerprint: status.fingerprint,
		}
	}
	return state, nil
}

// linkByNode reads the link of nodeID, false without one.
func (p *Pairing) linkByNode(ctx context.Context, nodeID string) (domain.PeerLink, bool, error) {
	return p.findLink(ctx, func(link domain.PeerLink) bool { return link.NodeID == nodeID })
}

func (p *Pairing) findLink(ctx context.Context, match func(domain.PeerLink) bool) (domain.PeerLink, bool, error) {
	links, err := p.store.Links(ctx)
	if err != nil {
		return domain.PeerLink{}, false, err
	}
	for _, link := range links {
		if match(link) {
			return link, true, nil
		}
	}
	return domain.PeerLink{}, false, nil
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
// that node answered with the secret to present. A ninth link answers domain.ErrPeerLimit, and a node this
// one already reads domain.ErrAlreadyPaired. The other node never connects back.
func (p *Pairing) Pair(ctx context.Context, input domain.PairInput) (domain.PeerState, error) {
	address, err := domain.NormalizePeerAddress(input.Address)
	if err != nil {
		return domain.PeerState{}, &domain.InvalidPeerAddressError{Field: "address"}
	}
	p.pairMu.Lock()
	defer p.pairMu.Unlock()
	links, err := p.store.Links(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	// The node id is known only from the answer; the limit is checked before the other node stores a grant.
	if err := domain.CheckNewLink(links, ""); err != nil {
		return domain.PeerState{}, err
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
	link, err := p.store.AddLink(ctx, domain.PeerLink{
		NodeID: answer.Node.ID, Name: answer.Node.Name, Address: address, Secret: answer.Secret,
		PinnedSPKI: input.Fingerprint, PairedAt: p.now(),
	})
	if err != nil {
		// The other node already stored a grant for this refused link, so it is asked to drop it.
		if forgetErr := p.client.Forget(ctx, target, answer.Secret, node.ID); forgetErr != nil {
			p.logger.Info("could not withdraw a refused pairing", slog.Any("error", forgetErr))
		}
		return domain.PeerState{}, err
	}
	c := p.conn(link)
	generation := p.interrupt(c, true)
	p.setStatus(c, generation, peerStatus{state: domain.PeerConnected, peerVersion: answer.Node.Version})
	select {
	case p.paired <- c:
	default:
		// Run takes every new link at once, so a full buffer means Run is not running.
	}
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

// ChangeAddress replaces the address of the link of nodeID; without that link it answers domain.ErrNotFound.
func (p *Pairing) ChangeAddress(ctx context.Context, nodeID, raw string) (domain.PeerState, error) {
	address, err := domain.NormalizePeerAddress(raw)
	if err != nil {
		return domain.PeerState{}, &domain.InvalidPeerAddressError{Field: "address"}
	}
	link, err := p.store.UpdateLinkAddress(ctx, nodeID, address)
	if err != nil {
		return domain.PeerState{}, err
	}
	p.interrupt(p.conn(link), true)
	return p.State(ctx)
}

// Unpair tells the read node of nodeID, best effort, and deletes that link, which also clears a revoked state;
// without that link it does nothing. A node that was not told learns it from the 401 its reader's next request
// gets.
func (p *Pairing) Unpair(ctx context.Context, nodeID string) error {
	link, found, err := p.linkByNode(ctx, nodeID)
	if err != nil || !found {
		return err
	}
	c := p.conn(link)
	// Stop the stream first, so the peer dropping it while it forgets this node is not a lost connection.
	p.interrupt(c, false)
	if !link.Revoked {
		p.tellPeerUnpaired(ctx, link)
	}
	deleted, err := p.store.DeleteLink(ctx, link.NodeID)
	if err != nil {
		return fmt.Errorf("cannot unpair: %w", err)
	}
	if deleted {
		p.dropConn(c)
		p.publish(domain.PeerStateChanged{})
	}
	return nil
}

func (p *Pairing) tellPeerUnpaired(ctx context.Context, link domain.PeerLink) {
	node, err := p.Node(ctx)
	if err == nil {
		err = p.client.Forget(ctx, peerTarget(link), link.Secret, node.ID)
	}
	if err != nil {
		p.logger.Info("could not tell the read VVMDM that this one unpaired",
			slog.String("peer", link.Name), slog.Any("error", err))
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
