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
	// folders answers each MediaFolders call in turn; the last one repeats.
	folders [][]domain.PeerFolder
}

func newStreamLink(steps ...watchStep) *streamLink {
	return &streamLink{
		node:  domain.PeerNode{ID: connectionPeerID, Name: "peer", Version: "1.2.0"},
		steps: steps, end: make(chan error),
	}
}

func (l *streamLink) RequestPairing(context.Context, domain.PeerTarget, domain.PairingRequest) (domain.PairingAnswer, error) {
	return domain.PairingAnswer{}, errors.New("not used")
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

func (l *streamLink) MediaFolders(context.Context, domain.PeerTarget, string) ([]domain.PeerFolder, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.folders) == 0 {
		return nil, nil
	}
	listed := l.folders[0]
	if len(l.folders) > 1 {
		l.folders = l.folders[1:]
	}
	return listed, nil
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

// peerEventLog records each peer event with the state and whether the link was marked revoked at that moment.
type peerEventLog struct {
	mu      sync.Mutex
	pairing *Pairing
	store   *fakePeerStore
	states  []domain.PeerState
	revoked []bool
	changed []domain.PeerChanged
	folders []domain.MediaFoldersChanged
}

func (l *peerEventLog) Publish(events ...domain.Event) {
	for _, event := range events {
		switch event := event.(type) {
		case domain.PeerStateChanged:
			state, _ := l.pairing.State(context.Background())
			link := l.store.storedLink()
			l.mu.Lock()
			l.states = append(l.states, state)
			l.revoked = append(l.revoked, link != nil && link.Revoked)
			l.mu.Unlock()
		case domain.PeerChanged:
			l.mu.Lock()
			l.changed = append(l.changed, event)
			l.mu.Unlock()
		case domain.MediaFoldersChanged:
			l.mu.Lock()
			l.folders = append(l.folders, event)
			l.mu.Unlock()
		}
	}
}

func (l *peerEventLog) stateNames() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	names := make([]string, len(l.states))
	for i, state := range l.states {
		names[i] = linkStateName(state)

	}
	return names
}

// snapshot copies the log under its lock, because the connection loop keeps publishing.
func (l *peerEventLog) snapshot() (states []domain.PeerState, revoked []bool, changed []domain.PeerChanged) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.states), slices.Clone(l.revoked), slices.Clone(l.changed)
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
	record := domain.PeerLink{NodeID: connectionPeerID, Name: "peer", Address: connectionPeer, Secret: "vvp_outgoing"}
	if _, err := store.AddLink(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	events := &peerEventLog{store: store}
	pairing := NewPairing(PairingOptions{
		Store: store, Client: link, Version: "1.2.0", NodeName: "self", Publisher: events, Logger: logger,
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

// linkStateName names the state of the first link, with its reason, or unpaired without a link.
func linkStateName(state domain.PeerState) string {
	if len(state.Links) == 0 {
		return "unpaired"
	}
	link := state.Links[0]
	name := string(link.State)
	if link.Reason != "" {
		name += "/" + string(link.Reason)
	}
	return name
}

func (n *connectionNode) state(t *testing.T) domain.PeerState {
	t.Helper()
	state, err := n.pairing.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return state
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
		if got := linkStateName(node.state(t)); got != "unreachable/network" {
			t.Errorf("State = %s, want unreachable/network", got)
		}
	})
}

func TestConnectionStatesAndEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		forwarded := domain.PeerChanged{VideoIDs: []int64{7}}
		node := startConnection(t, newStreamLink(
			watchStep{open: true, changed: &forwarded},
			watchStep{open: true},
		))
		assertStates(t, "ストリームが開く", node.events, "connected")
		states, _, changed := node.events.snapshot()
		if version := states[0].Links[0].PeerVersion; version != "1.2.0" {
			t.Errorf("connected の PeerVersion = %q, want 1.2.0", version)
		}
		if len(changed) != 1 || changed[0].VideoIDs[0] != domain.RemoteID(1, 7) {
			t.Errorf("転送されたイベント = %+v, want スロット 1 の 7", changed)
		}

		node.link.end <- &domain.PeerUnreachableError{Message: "connection reset"}
		synctest.Wait()
		assertStates(t, "ストリームが切れる", node.events, "connected", "unreachable/network")
		states, _, _ = node.events.snapshot()
		if version := states[1].Links[0].PeerVersion; version != "1.2.0" {
			t.Errorf("切れた後の PeerVersion = %q, want 最後に見た 1.2.0", version)
		}
		lost := time.Now()

		time.Sleep(domain.PeerRetryFirst)
		synctest.Wait()
		assertStates(t, "5 秒後の再接続", node.events, "connected", "unreachable/network", "connected")
		if times := node.link.attemptTimes(); times[1].Sub(lost) != domain.PeerRetryFirst {
			t.Errorf("再接続までの待ち = %v, want 5s", times[1].Sub(lost))
		}

		// The read node withdrew the grant, so the stream refuses the credential.
		node.link.end <- domain.ErrPeerUnknown
		synctest.Wait()
		assertStates(t, "401 peer_unknown", node.events, "connected", "unreachable/network", "connected", "revoked")
		if _, revoked, _ := node.events.snapshot(); !revoked[3] {
			t.Error("revoked のイベントの時点で link に revoked が記録されていない")
		}

		if err := node.pairing.Unpair(context.Background(), connectionPeerID); err != nil {
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

// This node runs 1.2.0; a peer built with another label still connects and shows its own.
func TestConnectionIgnoresTheVersionLabel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		link := newStreamLink(watchStep{open: true})
		link.node.Version = domain.DefaultVersion
		node := startConnection(t, link)

		assertStates(t, "版の違う相手", node.events, "connected")
		if version := node.state(t).Links[0].PeerVersion; version != domain.DefaultVersion {
			t.Errorf("PeerVersion = %q, want %s", version, domain.DefaultVersion)
		}
	})
}

func TestConnectionRefusesAnotherNode(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		link := newStreamLink(watchStep{open: true})
		link.node.ID = "0b9d3f1e-2c4a-4e6b-9d8c-7a6b5c4d3e2f"
		node := startConnection(t, link)

		if got := linkStateName(node.state(t)); got != "unreachable/network" {
			t.Errorf("State = %s, want unreachable/network", got)
		}
		if attempts := len(link.attemptTimes()); attempts != 0 {
			t.Errorf("別の VVMDM のストリームを %d 回開いた", attempts)
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
		if link := node.state(t).Links[0]; link.Reason != domain.PeerReasonCertificate || link.Fingerprint != "spki-new" {
			t.Fatalf("State = %+v, want certificate と spki-new", link)
		}

		if _, err := node.pairing.ConfirmCertificate(ctx, connectionPeerID, "spki-old"); !errors.Is(err, domain.ErrFingerprintChanged) {
			t.Errorf("別の指紋の ConfirmCertificate err = %v, want ErrFingerprintChanged", err)
		}
		if _, err := node.pairing.ConfirmCertificate(ctx, connectionPeerID, "spki-new"); err != nil {
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

		node.pairing.ReportFailure(context.Background(), 1, &domain.PeerUnreachableError{Message: "list timed out"})
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

		target, secret, ok, err := node.pairing.Connection(context.Background(), 1)
		if err != nil || !ok || target.Address != connectionPeer || secret != "vvp_outgoing" {
			t.Fatalf("接続中の Connection = %+v, %q, %v, %v", target, secret, ok, err)
		}
		node.pairing.ReportFailure(context.Background(), 1, &domain.PeerUnreachableError{Message: "lost"})
		if _, secret, ok, err := node.pairing.Connection(context.Background(), 1); err != nil || ok || secret != "" {
			t.Errorf("届かない間の Connection = %q, %v, %v, want ok なし", secret, ok, err)
		}
		if _, _, _, err := node.pairing.Connection(context.Background(), 2); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("リンクの無いスロットの Connection err = %v, want ErrNotFound", err)
		}
	})
}

// An attempt that an address change interrupted cannot revoke the link with the corrected address.
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

		if _, err := node.pairing.ChangeAddress(context.Background(), connectionPeerID, corrected); err != nil {
			t.Fatal(err)
		}
		close(answered)
		synctest.Wait()

		if link := node.store.storedLink(); link == nil || link.Revoked {
			t.Fatalf("link = %+v, want revoked でない", link)
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

		if err := node.pairing.Unpair(ctx, connectionPeerID); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if want := "vvp_outgoing " + self.ID; len(node.link.forgotten) != 1 || node.link.forgotten[0] != want {
			t.Errorf("相手への解除 = %v, want [%s]", node.link.forgotten, want)
		}
		if node.store.storedLink() != nil {
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
			"the read VVMDM is connected", "the read VVMDM became unreachable", "the read VVMDM is connected",
		}
		if got := logs.all(); !slices.Equal(got, want) {
			t.Errorf("info のログ = %q, want %q", got, want)
		}
	})
}

// routedLinks answers each call by the streamLink at the target's address, as several read nodes.
type routedLinks map[string]*streamLink

func (r routedLinks) RequestPairing(ctx context.Context, target domain.PeerTarget, req domain.PairingRequest) (domain.PairingAnswer, error) {
	return r[target.Address].RequestPairing(ctx, target, req)
}

func (r routedLinks) Handshake(ctx context.Context, target domain.PeerTarget, secret string) (domain.PeerNode, error) {
	return r[target.Address].Handshake(ctx, target, secret)
}

func (r routedLinks) Watch(
	ctx context.Context, target domain.PeerTarget, secret string, opened func(), changed func(domain.PeerChanged),
) error {
	return r[target.Address].Watch(ctx, target, secret, opened, changed)
}

func (r routedLinks) Forget(ctx context.Context, target domain.PeerTarget, secret, nodeID string) error {
	return r[target.Address].Forget(ctx, target, secret, nodeID)
}

func (r routedLinks) MediaFolders(ctx context.Context, target domain.PeerTarget, secret string) ([]domain.PeerFolder, error) {
	return r[target.Address].MediaFolders(ctx, target, secret)
}

// Each link has its own loop: two links connect on their own, a lost link leaves the other connected, and only
// the lost one retries.
func TestLinksConnectIndependently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			secondID   = "0b9d3f1e-2c4a-4e6b-9d8c-7a6b5c4d3e2f"
			secondPeer = "http://second.local:8080"
		)
		first := newStreamLink(watchStep{open: true}, watchStep{open: true})
		second := newStreamLink(watchStep{open: true, changed: &domain.PeerChanged{GroupIDs: []int64{3}}})
		second.node.ID = secondID
		store := &fakePeerStore{}
		ctx := context.Background()
		for _, link := range []domain.PeerLink{
			{NodeID: connectionPeerID, Name: "first", Address: connectionPeer, Secret: "vvp_first"},
			{NodeID: secondID, Name: "second", Address: secondPeer, Secret: "vvp_second"},
		} {
			if _, err := store.AddLink(ctx, link); err != nil {
				t.Fatal(err)
			}
		}
		events := &peerEventLog{store: store}
		pairing := NewPairing(PairingOptions{
			Store: store, Client: routedLinks{connectionPeer: first, secondPeer: second}, Publisher: events,
		})
		events.pairing = pairing
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			pairing.Run(runCtx)
		}()
		defer func() {
			cancel()
			<-done
		}()
		synctest.Wait()

		states := func() []string {
			state, err := pairing.State(ctx)
			if err != nil {
				t.Fatal(err)
			}
			names := make([]string, len(state.Links))
			for i, link := range state.Links {
				names[i] = link.Link.Name + ":" + string(link.State)
			}
			return names
		}
		if got := states(); !slices.Equal(got, []string{"first:connected", "second:connected"}) {
			t.Fatalf("接続後の State = %v, want 両方 connected", got)
		}
		if _, _, changed := events.snapshot(); len(changed) != 1 || changed[0].GroupIDs[0] != domain.RemoteID(2, 3) {
			t.Errorf("2 本目から転送されたイベント = %+v, want スロット 2 の 3", changed)
		}

		first.end <- &domain.PeerUnreachableError{Message: "connection reset"}
		synctest.Wait()
		if got := states(); !slices.Equal(got, []string{"first:unreachable", "second:connected"}) {
			t.Errorf("1 本目が切れた後の State = %v, want 2 本目は connected のまま", got)
		}
		if _, _, ok, err := pairing.Connection(ctx, 2); err != nil || !ok {
			t.Errorf("1 本目が切れた後の 2 本目の Connection = %v, %v, want ok", ok, err)
		}

		time.Sleep(domain.PeerRetryFirst)
		synctest.Wait()
		if got := states(); !slices.Equal(got, []string{"first:connected", "second:connected"}) {
			t.Errorf("再接続後の State = %v, want 両方 connected", got)
		}
		if len(first.attemptTimes()) != 2 || len(second.attemptTimes()) != 1 {
			t.Errorf("試行 = 1 本目 %d 回, 2 本目 %d 回, want 2 回と 1 回", len(first.attemptTimes()), len(second.attemptTimes()))
		}
	})
}
