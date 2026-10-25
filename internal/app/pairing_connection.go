package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// peerStatus is the connection state with what the Settings page shows beside it.
type peerStatus struct {
	state       domain.PeerConnection
	reason      domain.PeerUnreachableReason
	peerVersion string
	fingerprint string
}

func peerTarget(record domain.PeerRecord) domain.PeerTarget {
	return domain.PeerTarget{Address: record.Address, PinnedSPKI: record.PinnedSPKI}
}

// loadStatus derives the state from the record on the first read; without a record it is unpaired
// whatever was held.
func (p *Pairing) loadStatus(record *domain.PeerRecord) peerStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.loaded || record == nil {
		state, reason := domain.InitialPeerState(record)
		p.status = peerStatus{state: state, reason: reason}
		p.loaded = true
	}
	return p.status
}

// setStatus applies next unless the loop was interrupted after generation was read, and publishes one
// domain.PeerStateChanged when the state changed.
func (p *Pairing) setStatus(generation uint64, next peerStatus) {
	p.mu.Lock()
	if generation != p.generation || (p.loaded && p.status == next) {
		p.mu.Unlock()
		return
	}
	p.status, p.loaded = next, true
	p.mu.Unlock()
	p.publish(domain.PeerStateChanged{})
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
		select {
		case p.wake <- struct{}{}:
		default:
		}
	}
	return generation
}

// Run keeps one event stream open to the peer while a record with an incoming secret exists, and returns
// when ctx ends. After a failure it waits domain.PeerRetryDelay, unless pairing, an address change or a
// certificate confirmation wakes it.
func (p *Pairing) Run(ctx context.Context) {
	failures := 0
	for ctx.Err() == nil {
		record, found, err := p.store.Peer(ctx)
		switch {
		case err != nil:
			p.logger.Warn("could not read the peer record", slog.Any("error", err))
			failures++
		case !found || record.IncomingRevoked:
			// Nothing to connect to until the owner pairs or clears the revoked record.
			var held *domain.PeerRecord
			if found {
				held = &record
			}
			p.loadStatus(held)
			failures = 0
			p.waitWake(ctx, 0)
			continue
		default:
			p.loadStatus(&record)
			if p.connect(ctx, record) {
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

// connect runs one attempt: the handshake checks the peer's node id and version, then the event stream
// is held open until it ends. It reports whether the stream opened.
func (p *Pairing) connect(ctx context.Context, record domain.PeerRecord) bool {
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
	target := peerTarget(record)
	node, err := p.link.Handshake(attempt, target, record.OutgoingSecret)
	if err == nil {
		err = p.checkPeerNode(record, node)
	}
	if err == nil {
		err = p.link.Watch(attempt, target, record.OutgoingSecret, func() {
			opened = true
			p.setStatus(generation, peerStatus{state: domain.PeerConnected})
		}, func(changed domain.PeerChanged) { p.publish(changed) })
		err = p.confirmUnknown(attempt, record, err)
	}
	if ctx.Err() == nil {
		p.fail(ctx, generation, record, err)
	}
	return opened
}

// checkPeerNode refuses a peer that answers as another node or runs another release.
func (p *Pairing) checkPeerNode(record domain.PeerRecord, node domain.PeerNode) error {
	if node.ID != record.NodeID {
		return &domain.UnsupportedVersionError{Local: p.version, Peer: node.Version}
	}
	return domain.CheckPeerVersions(p.version, node.Version)
}

// Connection returns how to reach the peer and the secret to present while it is connected; ok is false in
// any other state, so a forwarded request does not wait on a peer the connection loop already lost.
func (p *Pairing) Connection(ctx context.Context) (target domain.PeerTarget, secret string, ok bool, err error) {
	record, found, err := p.store.Peer(ctx)
	if err != nil || !found {
		return domain.PeerTarget{}, "", false, err
	}
	if p.loadStatus(&record).state != domain.PeerConnected {
		return domain.PeerTarget{}, "", false, nil
	}
	return peerTarget(record), record.OutgoingSecret, true, nil
}

// ReportFailure records a failed request to the peer: the state becomes unreachable at once, or revoked
// on domain.ErrPeerUnknown that the handshake confirms, and the stream is reopened by the loop alone.
func (p *Pairing) ReportFailure(ctx context.Context, err error) {
	record, found, readErr := p.store.Peer(ctx)
	if readErr != nil || !found || record.IncomingRevoked {
		return
	}
	generation := p.interrupt(false)
	p.fail(ctx, generation, record, p.confirmUnknown(ctx, record, err))
}

// confirmUnknown asks the handshake again when another request answered domain.ErrPeerUnknown. A peer
// that requested the pairing accepts the secret at its handshake alone until it has stored its record, so
// only a refused handshake means the peer no longer knows this node.
func (p *Pairing) confirmUnknown(ctx context.Context, record domain.PeerRecord, err error) error {
	if !errors.Is(err, domain.ErrPeerUnknown) {
		return err
	}
	if _, err := p.link.Handshake(ctx, peerTarget(record), record.OutgoingSecret); err != nil {
		return err
	}
	return &domain.PeerUnreachableError{Message: "The other VVMDM has not stored the pairing yet."}
}

func (p *Pairing) fail(ctx context.Context, generation uint64, record domain.PeerRecord, err error) {
	var (
		untrusted *domain.CertificateUntrustedError
		version   *domain.UnsupportedVersionError
	)
	switch {
	case errors.Is(err, domain.ErrPeerUnknown):
		p.revoke(ctx, generation, record)
	case errors.As(err, &untrusted):
		p.setStatus(generation, peerStatus{
			state: domain.PeerUnreachable, reason: domain.PeerReasonCertificate, fingerprint: untrusted.Fingerprint,
		})
	case errors.As(err, &version):
		p.setStatus(generation, peerStatus{
			state: domain.PeerUnreachable, reason: domain.PeerReasonVersion, peerVersion: version.Peer,
		})
	default:
		p.logger.Debug("the paired VVMDM is unreachable", slog.Any("error", err))
		p.setStatus(generation, peerStatus{state: domain.PeerUnreachable, reason: domain.PeerReasonNetwork})
	}
}

// revoke clears the incoming secret before the state shows revoked, so the peer that no longer knows this
// node cannot use its credential here. An interrupted attempt revokes nothing, and an interruption waits
// until a running revocation has set its state.
func (p *Pairing) revoke(ctx context.Context, generation uint64, record domain.PeerRecord) {
	p.revokeMu.Lock()
	defer p.revokeMu.Unlock()
	p.mu.Lock()
	current := generation == p.generation
	p.mu.Unlock()
	if !current {
		return
	}
	if err := p.store.RevokeIncomingSecret(ctx, record.NodeID); err != nil {
		p.logger.Warn("could not revoke the paired VVMDM's credential", slog.Any("error", err))
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
	record, found, err := p.store.Peer(ctx)
	if err != nil {
		return domain.PeerState{}, err
	}
	if !found {
		return domain.PeerState{}, domain.ErrNotFound
	}
	if err := p.store.PinPeerCertificate(ctx, record.NodeID, fingerprint); err != nil {
		return domain.PeerState{}, err
	}
	p.interrupt(true)
	return p.State(ctx)
}
