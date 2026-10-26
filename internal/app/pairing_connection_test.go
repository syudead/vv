package app

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	connectionPeerID = "7c2f4c3a-1d2e-4f50-8a6b-0c1d2e3f4a5b"
	connectionPeer   = "http://peer.local:8080"
	// connectionIncoming is the peer's credential here, well-formed so Authenticate reaches the store.
	connectionIncoming = "vvp_aW5jb21pbmctc2VjcmV0LWZvci10aGUtcGVlci0wMDA"
)

// watchStep scripts one connection attempt: open holds the stream until the test ends it through
// streamLink.end; otherwise err is returned at once.
type watchStep struct {
	open    bool
	changed *domain.PeerChanged
	err     error
}

type watchAttempt struct {
	at     time.Time
	target domain.PeerTarget
}

// streamLink is the peer seen by the connection loop: a handshake answer and scripted streams.
type streamLink struct {
	mu   sync.Mutex
	node domain.PeerNode
	// handshake, when set, answers each handshake's error in place of nil.
	handshake func(ctx context.Context, target domain.PeerTarget) error
	steps     []watchStep
	attempts  []watchAttempt
	end       chan error
	forgotten []string
}

func newStreamLink(steps ...watchStep) *streamLink {
	return &streamLink{
		node:  domain.PeerNode{ID: connectionPeerID, Name: "peer", Version: "1.2.0"},
		steps: steps, end: make(chan error),
	}
}

func (l *streamLink) Version(context.Context, domain.PeerTarget) (string, error) {
	return "", errors.New("not used")
}

func (l *streamLink) RequestPairing(context.Context, domain.PeerTarget, domain.PairingRequest) (domain.PairingAnswer, error) {
	return domain.PairingAnswer{}, errors.New("not used")
}

func (l *streamLink) Certificate(context.Context, string) (domain.PresentedCertificate, error) {
	return domain.PresentedCertificate{}, errors.New("not used")
}

func (l *streamLink) Handshake(ctx context.Context, target domain.PeerTarget, _ string) (domain.PeerNode, error) {
	l.mu.Lock()
	node, handshake := l.node, l.handshake
	l.mu.Unlock()
	if handshake != nil {
		if err := handshake(ctx, target); err != nil {
			return domain.PeerNode{}, err
		}
	}
	return node, nil
}

func (l *streamLink) refuseHandshake(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.handshake = func(context.Context, domain.PeerTarget) error { return err }
}

func (l *streamLink) attemptTargets() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	targets := make([]string, len(l.attempts))
	for i, attempt := range l.attempts {
		targets[i] = attempt.target.Address
	}
	return targets
}

func (l *streamLink) Watch(
	ctx context.Context, target domain.PeerTarget, _ string, opened func(), changed func(domain.PeerChanged),
) error {
	l.mu.Lock()
	l.attempts = append(l.attempts, watchAttempt{at: time.Now(), target: target})
	var step *watchStep
	if len(l.steps) > 0 {
		step = &l.steps[0]
		l.steps = l.steps[1:]
	}
	l.mu.Unlock()
	if step == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	if !step.open {
		return step.err
	}
	opened()
	if step.changed != nil {
		changed(*step.changed)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-l.end:
		return err
	}
}

func (l *streamLink) Forget(_ context.Context, _ domain.PeerTarget, secret, nodeID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.forgotten = append(l.forgotten, secret+" "+nodeID)
	return &domain.PeerUnreachableError{Message: "the peer is down"}
}

func (l *streamLink) attemptTimes() []time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	times := make([]time.Time, len(l.attempts))
	for i, attempt := range l.attempts {
		times[i] = attempt.at
	}
	return times
}

// peerEventLog records each peer event with the state and the incoming secret at that moment.
type peerEventLog struct {
	mu      sync.Mutex
	pairing *Pairing
	store   *fakePeerStore
	states  []domain.PeerState
	revoked []bool
	changed []domain.PeerChanged
}

func (l *peerEventLog) Publish(events ...domain.Event) {
	for _, event := range events {
		switch event := event.(type) {
		case domain.PeerStateChanged:
			state, _ := l.pairing.State(context.Background())
			record := l.store.stored()
			l.mu.Lock()
			l.states = append(l.states, state)
			l.revoked = append(l.revoked, record != nil && record.IncomingRevoked)
			l.mu.Unlock()
		case domain.PeerChanged:
			l.mu.Lock()
			l.changed = append(l.changed, event)
			l.mu.Unlock()
		}
	}
}

func (l *peerEventLog) stateNames() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	names := make([]string, len(l.states))
	for i, state := range l.states {
		names[i] = string(state.State)
		if state.Reason != "" {
			names[i] += "/" + string(state.Reason)
		}
	}
	return names
}

type connectionNode struct {
	pairing *Pairing
	store   *fakePeerStore
	link    *streamLink
	events  *peerEventLog
	stop    func()
}

// startConnection runs the loop of a paired node inside the caller's synctest bubble.
func startConnection(t *testing.T, link *streamLink) *connectionNode {
	t.Helper()
	return startLoggedConnection(t, link, nil)
}

// startLoggedConnection is startConnection with the loop's logger; nil uses slog.Default.
func startLoggedConnection(t *testing.T, link *streamLink, logger *slog.Logger) *connectionNode {
	t.Helper()
	store := &fakePeerStore{}
	record := domain.PeerRecord{
		NodeID: connectionPeerID, Name: "peer", Address: connectionPeer, OutgoingSecret: "vvp_outgoing",
	}
	if err := store.AddPeer(context.Background(), record, connectionIncoming); err != nil {
		t.Fatal(err)
	}
	events := &peerEventLog{store: store}
	pairing := NewPairing(PairingOptions{
		Store: store, Link: link, Version: "1.2.0", NodeName: "self", Publisher: events, Logger: logger,
	})
	events.pairing = pairing
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		pairing.Run(ctx)
	}()
	node := &connectionNode{pairing: pairing, store: store, link: link, events: events}
	node.stop = func() {
		cancel()
		<-done
	}
	t.Cleanup(node.stop)
	synctest.Wait()
	return node
}

func assertStates(t *testing.T, label string, events *peerEventLog, want ...string) {
	t.Helper()
	got := events.stateNames()
	if len(got) != len(want) {
		t.Fatalf("%s: peer イベント = %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: peer イベント = %v, want %v", label, got, want)
		}
	}
}

func TestConnectionRetriesFromFiveToSixtySeconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failing := make([]watchStep, 8)
		for i := range failing {
			failing[i] = watchStep{err: &domain.PeerUnreachableError{Message: "down"}}
		}
		node := startConnection(t, newStreamLink(failing...))
		time.Sleep(4 * time.Minute)
		synctest.Wait()

		times := node.link.attemptTimes()
		want := []time.Duration{5, 10, 20, 40, 60, 60}
		if len(times) < len(want)+1 {
			t.Fatalf("試行 %d 回, want %d 回以上", len(times), len(want)+1)
		}
		for i, delay := range want {
			if got := times[i+1].Sub(times[i]); got != delay*time.Second {
				t.Errorf("%d 回目の後の待ち = %v, want %v", i+1, got, delay*time.Second)
			}
		}
		// The start already shows unreachable, so failing again publishes nothing.
		assertStates(t, "失敗の繰り返し", node.events)
		if state, _ := node.pairing.State(context.Background()); state.State != domain.PeerUnreachable ||
			state.Reason != domain.PeerReasonNetwork {
			t.Errorf("State = %s/%s, want unreachable/network", state.State, state.Reason)
		}
	})
}

func TestConnectionStatesAndEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		forwarded := domain.PeerChanged{VideoIDs: []int64{domain.ToRemoteID(7)}}
		node := startConnection(t, newStreamLink(
			watchStep{open: true, changed: &forwarded},
			watchStep{open: true},
		))
		assertStates(t, "ストリームが開く", node.events, "connected")
		if version := node.events.states[0].PeerVersion; version != "1.2.0" {
			t.Errorf("connected の PeerVersion = %q, want 1.2.0", version)
		}
		if len(node.events.changed) != 1 || node.events.changed[0].VideoIDs[0] != domain.ToRemoteID(7) {
			t.Errorf("転送されたイベント = %+v, want %+v", node.events.changed, forwarded)
		}

		node.link.end <- &domain.PeerUnreachableError{Message: "connection reset"}
		synctest.Wait()
		assertStates(t, "ストリームが切れる", node.events, "connected", "unreachable/network")
		if version := node.events.states[1].PeerVersion; version != "1.2.0" {
			t.Errorf("切れた後の PeerVersion = %q, want 最後に見た 1.2.0", version)
		}
		lost := time.Now()

		time.Sleep(domain.PeerRetryFirst)
		synctest.Wait()
		assertStates(t, "5 秒後の再接続", node.events, "connected", "unreachable/network", "connected")
		if times := node.link.attemptTimes(); times[1].Sub(lost) != domain.PeerRetryFirst {
			t.Errorf("再接続までの待ち = %v, want 5s", times[1].Sub(lost))
		}

		// The peer forgot this node, so its handshake refuses the credential too.
		node.link.refuseHandshake(domain.ErrPeerUnknown)
		node.link.end <- domain.ErrPeerUnknown
		synctest.Wait()
		assertStates(t, "401 peer_unknown", node.events, "connected", "unreachable/network", "connected", "revoked")
		if !node.events.revoked[3] {
			t.Error("revoked のイベントの時点で受け取る secret が取り消されていない")
		}
		if valid, _ := node.pairing.Authenticate(context.Background(), connectionIncoming); valid {
			t.Error("revoked の後も相手の資格情報が通る")
		}

		if err := node.pairing.Unpair(context.Background()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		assertStates(t, "記録を消す", node.events,
			"connected", "unreachable/network", "connected", "revoked", "unpaired")
		if len(node.link.forgotten) != 0 {
			t.Errorf("自分を知らない相手に解除を伝えた: %v", node.link.forgotten)
		}
		if attempts := len(node.link.attemptTimes()); attempts != 2 {
			t.Errorf("試行 %d 回, want 2 回（revoked と unpaired では接続しない）", attempts)
		}
	})
}

func TestConnectionRefusesAnotherVersion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		link := newStreamLink()
		link.node.Version = "1.3.0"
		node := startConnection(t, link)

		state, _ := node.pairing.State(context.Background())
		if state.State != domain.PeerUnreachable || state.Reason != domain.PeerReasonVersion || state.PeerVersion != "1.3.0" {
			t.Errorf("State = %+v, want unreachable/version と 1.3.0", state)
		}
		assertStates(t, "版の不一致", node.events, "unreachable/version")
		if attempts := len(link.attemptTimes()); attempts != 0 {
			t.Errorf("版の違う相手のストリームを %d 回開いた", attempts)
		}
	})
}

func TestConnectionConfirmsAChangedCertificate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		node := startConnection(t, newStreamLink(
			watchStep{err: &domain.CertificateUntrustedError{Fingerprint: "spki-new"}},
			watchStep{open: true},
		))
		ctx := context.Background()
		state, _ := node.pairing.State(ctx)
		if state.Reason != domain.PeerReasonCertificate || state.Fingerprint != "spki-new" {
			t.Fatalf("State = %+v, want certificate と spki-new", state)
		}

		if _, err := node.pairing.ConfirmCertificate(ctx, "spki-old"); !errors.Is(err, domain.ErrFingerprintChanged) {
			t.Errorf("別の指紋の ConfirmCertificate err = %v, want ErrFingerprintChanged", err)
		}
		if _, err := node.pairing.ConfirmCertificate(ctx, "spki-new"); err != nil {
			t.Fatal(err)
		}
		// The confirmation reconnects at once, not after the retry delay.
		synctest.Wait()
		assertStates(t, "確認して再接続", node.events, "unreachable/certificate", "connected")
		attempts := node.link.attempts
		if len(attempts) != 2 || attempts[1].at != attempts[0].at || attempts[1].target.PinnedSPKI != "spki-new" {
			t.Errorf("試行 = %+v, want 待たずに spki-new を固定した 2 回目", attempts)
		}
	})
}

func TestReportFailureMarksUnreachableAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		node := startConnection(t, newStreamLink(watchStep{open: true}, watchStep{open: true}))
		reported := time.Now()

		node.pairing.ReportFailure(context.Background(), &domain.PeerUnreachableError{Message: "list timed out"})
		assertStates(t, "読み取りの失敗", node.events, "connected", "unreachable/network")

		time.Sleep(domain.PeerRetryFirst)
		synctest.Wait()
		assertStates(t, "ループの再接続", node.events, "connected", "unreachable/network", "connected")
		if times := node.link.attemptTimes(); len(times) != 2 || times[1].Sub(reported) != domain.PeerRetryFirst {
			t.Errorf("試行 = %v, want 報告の 5 秒後に 2 回目", times)
		}
	})
}

func TestConnectionOffersThePeerOnlyWhileConnected(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		node := startConnection(t, newStreamLink(watchStep{open: true}))

		target, secret, ok, err := node.pairing.Connection(context.Background())
		if err != nil || !ok || target.Address != connectionPeer || secret != "vvp_outgoing" {
			t.Fatalf("接続中の Connection = %+v, %q, %v, %v", target, secret, ok, err)
		}
		node.pairing.ReportFailure(context.Background(), &domain.PeerUnreachableError{Message: "lost"})
		if _, secret, ok, err := node.pairing.Connection(context.Background()); err != nil || ok || secret != "" {
			t.Errorf("届かない間の Connection = %q, %v, %v, want ok なし", secret, ok, err)
		}
	})
}

// A peer that requested the pairing accepts the secret at its handshake before it stores the record, so its
// event stream refusing the secret then is not a revocation. Its handshake after storing reconnects at once.
func TestConnectionWaitsForThePeerStoringThePairing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		node := startConnection(t, newStreamLink(watchStep{err: domain.ErrPeerUnknown}, watchStep{open: true}))
		if record := node.store.stored(); record == nil || record.IncomingRevoked {
			t.Fatalf("記録 = %+v, want 受け取る secret が残る", record)
		}
		refused := time.Now()
		if _, err := node.pairing.Handshake(context.Background(), connectionIncoming); err != nil {
			t.Fatalf("相手の資格情報の Handshake: %v", err)
		}
		synctest.Wait()
		assertStates(t, "相手の保存後の再接続", node.events, "connected")
		if times := node.link.attemptTimes(); len(times) != 2 || !times[1].Equal(refused) {
			t.Errorf("試行の時刻 = %v, want 相手の Handshake の直後 %v", times, refused)
		}
	})
}

// An attempt that an address change interrupted cannot revoke the record with the corrected address.
func TestInterruptedAttemptRevokesNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const corrected = "http://peer-new.local:8080"
		answered := make(chan struct{})
		link := newStreamLink(watchStep{open: true})
		link.handshake = func(_ context.Context, target domain.PeerTarget) error {
			if target.Address == corrected {
				return nil
			}
			// The node at the old address refuses after the address was already changed.
			<-answered
			return domain.ErrPeerUnknown
		}
		node := startConnection(t, link)

		if _, err := node.pairing.ChangeAddress(context.Background(), corrected); err != nil {
			t.Fatal(err)
		}
		close(answered)
		synctest.Wait()

		if record := node.store.stored(); record == nil || record.IncomingRevoked {
			t.Fatalf("記録 = %+v, want 受け取る secret が残る", record)
		}
		assertStates(t, "直したアドレスで接続", node.events, "connected")
		if targets := link.attemptTargets(); len(targets) != 1 || targets[0] != corrected {
			t.Errorf("ストリームの相手 = %v, want [%s]", targets, corrected)
		}
	})
}

func TestUnpairTellsThePeerAndDeletesOnFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		node := startConnection(t, newStreamLink(watchStep{open: true}))
		ctx := context.Background()
		self, _ := node.pairing.Node(ctx)

		if err := node.pairing.Unpair(ctx); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if want := "vvp_outgoing " + self.ID; len(node.link.forgotten) != 1 || node.link.forgotten[0] != want {
			t.Errorf("相手への解除 = %v, want [%s]", node.link.forgotten, want)
		}
		if node.store.stored() != nil {
			t.Error("相手に伝えられなくても記録は消える")
		}
		// The stream the unpairing stopped is not reported as a lost connection.
		assertStates(t, "解除", node.events, "connected", "unpaired")
	})
}

// infoMessages records the messages of log records at info and above.
type infoMessages struct {
	mu       sync.Mutex
	messages []string
}

func (h *infoMessages) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

func (h *infoMessages) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messages = append(h.messages, record.Message)
	return nil
}

func (h *infoMessages) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *infoMessages) WithGroup(string) slog.Handler { return h }

func (h *infoMessages) all() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.messages)
}

// Losing and regaining the peer is logged once per transition, not once per failed retry.
func TestConnectionLogsEachTransitionOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		down := &domain.PeerUnreachableError{Message: "down"}
		logs := &infoMessages{}
		node := startLoggedConnection(t, newStreamLink(
			watchStep{open: true}, watchStep{err: down}, watchStep{err: down}, watchStep{open: true},
		), slog.New(logs))
		node.link.end <- down
		time.Sleep(domain.PeerRetryFirst + 2*domain.PeerRetryFirst + 4*domain.PeerRetryFirst)
		synctest.Wait()

		assertStates(t, "切断と 2 回の失敗の後の再接続", node.events, "connected", "unreachable/network", "connected")
		want := []string{
			"the paired VVMDM is connected", "the paired VVMDM became unreachable", "the paired VVMDM is connected",
		}
		if got := logs.all(); !slices.Equal(got, want) {
			t.Errorf("info のログ = %q, want %q", got, want)
		}
	})
}
