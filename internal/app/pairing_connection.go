package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// peerStatus is a link's connection state with what the Settings page shows beside it.
type peerStatus struct {
	state       domain.PeerConnection
	reason      domain.PeerUnreachableReason
	peerVersion string
	fingerprint string
}

// linkConn is one link's connection: its state and its loop. Each link has its own, so one link's failures and
// retry delays never reach another. Every field but revokeMu is guarded by Pairing.mu.
type linkConn struct {
	nodeID string
	name   string
	slot   int64
	status peerStatus
	// generation counts interruptions of the link's loop, so an attempt that was interrupted cannot overwrite
	// the state its interrupter set.
	generation uint64
	// stopAttempt ends the running connection attempt; nil while none runs.
	stopAttempt context.CancelFunc
	// stopLoop ends the link's loop; nil while none runs.
	stopLoop context.CancelFunc
	// wake ends the loop's wait after pairing, an address change or a certificate confirmation.
	wake chan struct{}
	// revokeMu makes marking the link revoked and interrupting the loop exclusive, so an interrupted
	// attempt cannot revoke the link its interrupter acts on.
	revokeMu sync.Mutex
}

func peerTarget(link domain.PeerLink) domain.PeerTarget {
	return domain.PeerTarget{Address: link.Address, PinnedSPKI: link.PinnedSPKI}
}

// conn returns the connection of link, made with the state derived from the link on its first read.
func (p *Pairing) conn(link domain.PeerLink) *linkConn {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.conns[link.NodeID]
	if c == nil {
		state, reason := domain.InitialPeerState(link)
		c = &linkConn{
			nodeID: link.NodeID, slot: link.Slot, status: peerStatus{state: state, reason: reason},
			wake: make(chan struct{}, 1),
		}
		p.conns[link.NodeID] = c
	}
	c.name = link.Name
	return c
}

// dropConn ends the loop of a deleted link and forgets its state.
func (p *Pairing) dropConn(c *linkConn) {
	p.mu.Lock()
	stop := c.stopLoop
	c.stopLoop = nil
	if p.conns[c.nodeID] == c {
		delete(p.conns, c.nodeID)
	}
	p.mu.Unlock()
	if stop != nil {
		stop()
	}
}

func (p *Pairing) loadStatus(link domain.PeerLink) peerStatus {
	c := p.conn(link)
	p.mu.Lock()
	defer p.mu.Unlock()
	return c.status
}

// setStatus applies next unless the link's loop was interrupted after generation was read, and publishes one
// domain.PeerStateChanged when the state changed.
func (p *Pairing) setStatus(c *linkConn, generation uint64, next peerStatus) {
	p.applyStatus(c, generation, next, nil)
}

// applyStatus is setStatus with the failure that caused next, which the transition's log line names.
func (p *Pairing) applyStatus(c *linkConn, generation uint64, next peerStatus, cause error) {
	p.mu.Lock()
	if generation != c.generation || c.status == next {
		p.mu.Unlock()
		return
	}
	previous := c.status.state
	c.status = next
	name := c.name
	p.mu.Unlock()
	p.logTransition(name, previous, next, cause)
	p.publish(domain.PeerStateChanged{})
}

// logTransition logs losing and regaining a link once per transition; each failed retry stays at debug.
func (p *Pairing) logTransition(name string, previous domain.PeerConnection, next peerStatus, cause error) {
	switch {
	case previous == next.state:
	case next.state == domain.PeerConnected:
		p.logger.Info("the read VVMDM is connected", slog.String("peer", name), slog.String("previous", string(previous)))
	case next.state == domain.PeerUnreachable && previous == domain.PeerConnected:
		p.logger.Info("the read VVMDM became unreachable", slog.String("peer", name),
			slog.String("reason", string(next.reason)), slog.Any("error", cause))
	}
}

func (p *Pairing) publish(event domain.Event) {
	if p.publisher != nil {
		p.publisher.Publish(event)
	}
}

// interrupt ends the link's running connection attempt and, with retry, also its loop's wait. It returns the
// new generation, under which the caller sets the state.
func (p *Pairing) interrupt(c *linkConn, retry bool) uint64 {
	c.revokeMu.Lock()
	p.mu.Lock()
	c.generation++
	generation := c.generation
	stop := c.stopAttempt
	p.mu.Unlock()
	c.revokeMu.Unlock()
	if stop != nil {
		stop()
	}
	if retry {
		select {
		case c.wake <- struct{}{}:
		default:
			// A wake while an attempt runs is kept for the next wait.
		}
	}
	return generation
}

// seenPeerVersion is the peer version the link's state last carried, kept while the peer cannot be reached.
func (p *Pairing) seenPeerVersion(c *linkConn) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return c.status.peerVersion
}

// Run starts one connection loop per stored link, and one for each link paired later, and returns when ctx
// ends and every loop returned.
func (p *Pairing) Run(ctx context.Context) {
	var loops sync.WaitGroup
	defer loops.Wait()
	p.startStoredLoops(ctx, &loops)
	for {
		select {
		case <-ctx.Done():
			return
		case c := <-p.paired:
			p.startLoop(ctx, &loops, c)
		}
	}
}

// startStoredLoops starts the loops of the stored links, reading them again after a failure until ctx ends.
func (p *Pairing) startStoredLoops(ctx context.Context, loops *sync.WaitGroup) {
	for failures := 1; ctx.Err() == nil; failures++ {
		links, err := p.store.Links(ctx)
		if err == nil {
			for _, link := range links {
				p.startLoop(ctx, loops, p.conn(link))
			}
			return
		}
		p.logger.Warn("could not read the peer links", slog.Any("error", err))
		timer := time.NewTimer(domain.PeerRetryDelay(failures))
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		timer.Stop()
	}
}

// startLoop starts the loop of c under ctx unless c has one or its link was removed.
func (p *Pairing) startLoop(ctx context.Context, loops *sync.WaitGroup, c *linkConn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c.stopLoop != nil || p.conns[c.nodeID] != c {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	c.stopLoop = cancel
	loops.Add(1)
	go func() {
		defer loops.Done()
		p.runLink(loopCtx, c)
	}()
}

// runLink keeps one event stream open to the link's node while the link exists unrevoked, and returns when ctx
// ends or the link is gone. After a failure it waits domain.PeerRetryDelay, unless an address change or a
// certificate confirmation wakes it.
func (p *Pairing) runLink(ctx context.Context, c *linkConn) {
	failures := 0
	for ctx.Err() == nil {
		link, found, err := p.linkByNode(ctx, c.nodeID)
		switch {
		case err != nil:
			p.logger.Warn("could not read the peer link", slog.Any("error", err))
			failures++
		case !found:
			return
		case link.Revoked:
			// Nothing to connect to until the owner removes the revoked link.
			failures = 0
			p.waitWake(ctx, c, 0)
			continue
		default:
			if p.connect(ctx, c, link) {
				failures = 0
			}
			failures++
		}
		if p.waitWake(ctx, c, domain.PeerRetryDelay(failures)) {
			failures = 0
		}
	}
}

// waitWake waits delay, or without a delay until woken, and reports whether it was woken.
func (p *Pairing) waitWake(ctx context.Context, c *linkConn, delay time.Duration) bool {
	var expired <-chan time.Time
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		expired = timer.C
	}
	select {
	case <-ctx.Done():
		return false
	case <-c.wake:
		return true
	case <-expired:
		return false
	}
}

// connect runs one attempt: the handshake checks the peer's node id, then the event stream is held open until it
// ends. The link's folder snapshot is refreshed when the stream opens and on each mediaFolders event, before the
// stream is read further. It reports whether the stream opened.
func (p *Pairing) connect(ctx context.Context, c *linkConn, link domain.PeerLink) bool {
	attempt, cancel := context.WithCancel(ctx)
	defer cancel()
	p.mu.Lock()
	generation := c.generation
	c.stopAttempt = cancel
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		c.stopAttempt = nil
		p.mu.Unlock()
	}()

	opened := false
	target := peerTarget(link)
	node, err := p.client.Handshake(attempt, target, link.Secret)
	if err == nil && node.ID != link.NodeID {
		err = &domain.PeerUnreachableError{Message: "The paired address answers as another VVMDM."}
	}
	if err == nil {
		err = p.client.Watch(attempt, target, link.Secret, func() {
			opened = true
			p.setStatus(c, generation, peerStatus{state: domain.PeerConnected, peerVersion: node.Version})
			p.refreshFolders(attempt, link, false)
		}, func(changed domain.PeerChanged) {
			if changed.MediaFolders {
				p.refreshFolders(attempt, link, true)
				return
			}
			p.publish(changed.InSlot(link.Slot))
		})
	}
	if ctx.Err() == nil {
		p.fail(ctx, c, generation, link, err)
	}
	return opened
}

// Connection returns how to reach the node of the link in slot and the secret to present while that link is
// connected; ok is false in any other state, so a forwarded request does not wait on a node the link's loop
// already lost. Without a link in slot it answers domain.ErrNotFound.
func (p *Pairing) Connection(ctx context.Context, slot int64) (target domain.PeerTarget, secret string, ok bool, err error) {
	link, found, err := p.findLink(ctx, func(link domain.PeerLink) bool { return link.Slot == slot })
	if err != nil {
		return domain.PeerTarget{}, "", false, err
	}
	if !found {
		return domain.PeerTarget{}, "", false, domain.ErrNotFound
	}
	if p.loadStatus(link).state != domain.PeerConnected {
		return domain.PeerTarget{}, "", false, nil
	}
	return peerTarget(link), link.Secret, true, nil
}

// ReportFailure records a failed request to the node of the link in slot: that link becomes unreachable at
// once, or revoked on domain.ErrPeerUnknown, and its stream is reopened by its loop alone.
func (p *Pairing) ReportFailure(ctx context.Context, slot int64, err error) {
	link, found, readErr := p.findLink(ctx, func(link domain.PeerLink) bool { return link.Slot == slot })
	if readErr != nil || !found || link.Revoked {
		return
	}
	c := p.conn(link)
	generation := p.interrupt(c, false)
	p.fail(ctx, c, generation, link, err)
}

func (p *Pairing) fail(ctx context.Context, c *linkConn, generation uint64, link domain.PeerLink, err error) {
	var untrusted *domain.CertificateUntrustedError
	switch {
	case errors.Is(err, domain.ErrPeerUnknown):
		p.revoke(ctx, c, generation, link)
	case errors.As(err, &untrusted):
		p.applyStatus(c, generation, peerStatus{
			state: domain.PeerUnreachable, reason: domain.PeerReasonCertificate, fingerprint: untrusted.Fingerprint,
			peerVersion: p.seenPeerVersion(c),
		}, err)
	default:
		p.logger.Debug("the read VVMDM is unreachable", slog.String("peer", link.Name), slog.Any("error", err))
		p.applyStatus(c, generation, peerStatus{
			state: domain.PeerUnreachable, reason: domain.PeerReasonNetwork, peerVersion: p.seenPeerVersion(c),
		}, err)
	}
}

// revoke marks the link revoked before the state shows it, so a start shows revoked too. An interrupted
// attempt revokes nothing, and an interruption waits until a running revocation has set its state.
func (p *Pairing) revoke(ctx context.Context, c *linkConn, generation uint64, link domain.PeerLink) {
	c.revokeMu.Lock()
	defer c.revokeMu.Unlock()
	p.mu.Lock()
	current := generation == c.generation
	p.mu.Unlock()
	if !current {
		return
	}
	if err := p.store.MarkLinkRevoked(ctx, link.NodeID); err != nil {
		p.logger.Warn("could not mark the peer link revoked", slog.Any("error", err))
		p.setStatus(c, generation, peerStatus{state: domain.PeerUnreachable, reason: domain.PeerReasonNetwork})
		return
	}
	p.setStatus(c, generation, peerStatus{state: domain.PeerRevoked})
}

// ConfirmCertificate pins the fingerprint the unreachable state of the link of nodeID shows and reconnects it.
// Another fingerprint answers domain.ErrFingerprintChanged, and an unknown node id domain.ErrNotFound.
func (p *Pairing) ConfirmCertificate(ctx context.Context, nodeID, fingerprint string) (domain.PeerState, error) {
	link, found, err := p.linkByNode(ctx, nodeID)
	if err != nil {
		return domain.PeerState{}, err
	}
	if !found {
		return domain.PeerState{}, domain.ErrNotFound
	}
	c := p.conn(link)
	p.mu.Lock()
	waiting := c.status.fingerprint
	p.mu.Unlock()
	if waiting == "" || fingerprint != waiting {
		return domain.PeerState{}, domain.ErrFingerprintChanged
	}
	if err := p.store.PinLinkCertificate(ctx, link.NodeID, fingerprint); err != nil {
		return domain.PeerState{}, err
	}
	p.interrupt(c, true)
	return p.State(ctx)
}
