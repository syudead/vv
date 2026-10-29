package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// peerStatus is the link's connection state with what the Settings page shows beside it.
type peerStatus struct {
	state       domain.PeerConnection
	reason      domain.PeerUnreachableReason
	peerVersion string
	fingerprint string
}

func peerTarget(link domain.PeerLink) domain.PeerTarget {
	return domain.PeerTarget{Address: link.Address, PinnedSPKI: link.PinnedSPKI}
}

// loadStatus derives the state from the link on the first read; without a link it is unpaired
// whatever was held.
func (p *Pairing) loadStatus(link *domain.PeerLink) peerStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.loaded || link == nil {
		state, reason := domain.InitialPeerState(link)
		p.status = peerStatus{state: state, reason: reason}
		p.loaded = true
	}
	return p.status
}

// setStatus applies next unless the loop was interrupted after generation was read, and publishes one
// domain.PeerStateChanged when the state changed.
func (p *Pairing) setStatus(generation uint64, next peerStatus) {
	p.applyStatus(generation, next, nil)
}

// applyStatus is setStatus with the failure that caused next, which the transition's log line names.
func (p *Pairing) applyStatus(generation uint64, next peerStatus, cause error) {
	p.mu.Lock()
	if generation != p.generation || (p.loaded && p.status == next) {
		p.mu.Unlock()
		return
	}
	previous := p.status.state
	p.status, p.loaded = next, true
	p.mu.Unlock()
	p.logTransition(previous, next, cause)
	p.publish(domain.PeerStateChanged{})
}

// logTransition logs losing and regaining the peer once per transition; each failed retry stays at debug.
func (p *Pairing) logTransition(previous domain.PeerConnection, next peerStatus, cause error) {
	switch {
	case previous == next.state:
	case next.state == domain.PeerConnected:
		p.logger.Info("the read VVMDM is connected", slog.String("previous", string(previous)))
	case next.state == domain.PeerUnreachable && previous == domain.PeerConnected:
		p.logger.Info("the read VVMDM became unreachable",
			slog.String("reason", string(next.reason)), slog.Any("error", cause))
	}
}

func (p *Pairing) publish(event domain.Event) {
	if p.publisher != nil {
		p.publisher.Publish(event)
	}
}

// interrupt ends the running connection attempt and, with retry, also the loop's wait. It returns the
// new generation, under which the caller sets the state.
func (p *Pairing) interrupt(retry bool) uint64 {
	p.revokeMu.Lock()
	p.mu.Lock()
	p.generation++
	generation := p.generation
	stop := p.stopAttempt
	p.mu.Unlock()
	p.revokeMu.Unlock()
	if stop != nil {
		stop()
	}
	if retry {
		p.wakeLoop()
	}
	return generation
}

// wakeLoop ends the loop's wait before its retry delay; a wake while an attempt runs is kept for the next wait.
func (p *Pairing) wakeLoop() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// seenPeerVersion is the peer version the state last carried, kept while the peer cannot be reached.
func (p *Pairing) seenPeerVersion() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status.peerVersion
}

// Run keeps one event stream open to the read node while an unrevoked link exists, and returns when ctx
// ends. After a failure it waits domain.PeerRetryDelay, unless pairing, an address change or a certificate
// confirmation wakes it.
func (p *Pairing) Run(ctx context.Context) {
	failures := 0
	for ctx.Err() == nil {
		link, found, err := p.store.Link(ctx)
		switch {
		case err != nil:
			p.logger.Warn("could not read the peer link", slog.Any("error", err))
			failures++
		case !found || link.Revoked:
			// Nothing to connect to until the owner pairs or removes the revoked link.
			var held *domain.PeerLink
			if found {
				held = &link
			}
			p.loadStatus(held)
			failures = 0
			p.waitWake(ctx, 0)
			continue
		default:
			p.loadStatus(&link)
			if p.connect(ctx, link) {
				failures = 0
			}
			failures++
		}
		if p.waitWake(ctx, domain.PeerRetryDelay(failures)) {
			failures = 0
		}
	}
}

// waitWake waits delay, or without a delay until woken, and reports whether it was woken.
func (p *Pairing) waitWake(ctx context.Context, delay time.Duration) bool {
	var expired <-chan time.Time
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		expired = timer.C
	}
	select {
	case <-ctx.Done():
		return false
	case <-p.wake:
		return true
	case <-expired:
		return false
	}
}

// connect runs one attempt: the handshake checks the peer's node id, then the event stream
// is held open until it ends. It reports whether the stream opened.
func (p *Pairing) connect(ctx context.Context, link domain.PeerLink) bool {
	attempt, cancel := context.WithCancel(ctx)
	defer cancel()
	p.mu.Lock()
	generation := p.generation
	p.stopAttempt = cancel
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.stopAttempt = nil
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
			p.setStatus(generation, peerStatus{state: domain.PeerConnected, peerVersion: node.Version})
		}, func(changed domain.PeerChanged) { p.publish(changed) })
	}
	if ctx.Err() == nil {
		p.fail(ctx, generation, link, err)
	}
	return opened
}

// Connection returns how to reach the read node and the secret to present while it is connected; ok is
// false in any other state, so a forwarded request does not wait on a peer the connection loop already lost.
func (p *Pairing) Connection(ctx context.Context) (target domain.PeerTarget, secret string, ok bool, err error) {
	link, found, err := p.store.Link(ctx)
	if err != nil || !found {
		return domain.PeerTarget{}, "", false, err
	}
	if p.loadStatus(&link).state != domain.PeerConnected {
		return domain.PeerTarget{}, "", false, nil
	}
	return peerTarget(link), link.Secret, true, nil
}

// ReportFailure records a failed request to the read node: the state becomes unreachable at once, or
// revoked on domain.ErrPeerUnknown, and the stream is reopened by the loop alone.
func (p *Pairing) ReportFailure(ctx context.Context, err error) {
	link, found, readErr := p.store.Link(ctx)
	if readErr != nil || !found || link.Revoked {
		return
	}
	generation := p.interrupt(false)
	p.fail(ctx, generation, link, err)
}

func (p *Pairing) fail(ctx context.Context, generation uint64, link domain.PeerLink, err error) {
	var untrusted *domain.CertificateUntrustedError
	switch {
	case errors.Is(err, domain.ErrPeerUnknown):
		p.revoke(ctx, generation, link)
	case errors.As(err, &untrusted):
		p.applyStatus(generation, peerStatus{
			state: domain.PeerUnreachable, reason: domain.PeerReasonCertificate, fingerprint: untrusted.Fingerprint,
			peerVersion: p.seenPeerVersion(),
		}, err)
	default:
		p.logger.Debug("the read VVMDM is unreachable", slog.Any("error", err))
		p.applyStatus(generation, peerStatus{
			state: domain.PeerUnreachable, reason: domain.PeerReasonNetwork, peerVersion: p.seenPeerVersion(),
		}, err)
	}
}

// revoke marks the link revoked before the state shows it, so a start shows revoked too. An interrupted
// attempt revokes nothing, and an interruption waits until a running revocation has set its state.
func (p *Pairing) revoke(ctx context.Context, generation uint64, link domain.PeerLink) {
	p.revokeMu.Lock()
	defer p.revokeMu.Unlock()
	p.mu.Lock()
	current := generation == p.generation
	p.mu.Unlock()
	if !current {
		return
	}
	if err := p.store.MarkLinkRevoked(ctx, link.NodeID); err != nil {
		p.logger.Warn("could not mark the peer link revoked", slog.Any("error", err))
		p.setStatus(generation, peerStatus{state: domain.PeerUnreachable, reason: domain.PeerReasonNetwork})
		return
	}
	p.setStatus(generation, peerStatus{state: domain.PeerRevoked})
}

// ConfirmCertificate pins the fingerprint the unreachable state shows and reconnects. Another fingerprint
// answers domain.ErrFingerprintChanged.
func (p *Pairing) ConfirmCertificate(ctx context.Context, fingerprint string) (domain.PeerState, error) {
	p.mu.Lock()
	waiting := p.status.fingerprint
	p.mu.Unlock()
	if waiting == "" || fingerprint != waiting {
		return domain.PeerState{}, domain.ErrFingerprintChanged
	}
	link, found, err := p.store.Link(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	if !found {
		return domain.PeerState{}, domain.ErrNotFound
	}
	if err := p.store.PinLinkCertificate(ctx, link.NodeID, fingerprint); err != nil {
		return domain.PeerState{}, err
	}
	p.interrupt(true)
	return p.State(ctx)
}
