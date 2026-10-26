package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const (
	peerScheme         = "VV-Peer"
	peerAudienceHeader = "X-VV-Peer-Audience"
	maxPeerBodyBytes   = 8 << 10
)

// Pairing is app.Pairing: the pairing code, pairing in both directions, unpairing and the state.
type Pairing interface {
	State(ctx context.Context) (domain.PeerState, error)
	// IssueCode answers domain.ErrAlreadyPaired on a paired node.
	IssueCode(ctx context.Context) (domain.PairingCode, error)
	Pair(ctx context.Context, input domain.PairInput) (domain.PeerState, error)
	// ChangeAddress answers domain.ErrNotFound without a peer.
	ChangeAddress(ctx context.Context, address string) (domain.PeerState, error)
	// ConfirmCertificate answers domain.ErrFingerprintChanged unless fingerprint is the one waiting.
	ConfirmCertificate(ctx context.Context, fingerprint string) (domain.PeerState, error)
	Unpair(ctx context.Context) error
	Accept(ctx context.Context, req domain.PairingRequest) (domain.PairingAnswer, error)
	// Handshake answers domain.ErrPeerUnknown for a secret that is neither the credential nor pending.
	Handshake(ctx context.Context, secret string) (domain.PeerNode, error)
	Forget(ctx context.Context, nodeID string) error
	// Authenticate reports whether secret is the peer credential.
	Authenticate(ctx context.Context, secret string) (bool, error)
}

type peerRequestKey struct{}

// peerLedgerKey tracks every peer-credential request under one key, since a node has at most one peer;
// the colon namespace cannot collide with base64url session IDs.
const peerLedgerKey = "peer:"

// isPeerRequest reports a request the auth boundary accepted with the peer credential.
func isPeerRequest(ctx context.Context) bool {
	return peerRequestFrom(ctx) != nil
}

func peerRequestFrom(ctx context.Context) *trackedRequest {
	tracked, _ := ctx.Value(peerRequestKey{}).(*trackedRequest)
	return tracked
}

// revokePeerRequests cancels the in-flight peer-credential requests after the record is deleted,
// except the request asking for the deletion, which still has to answer.
func (s *server) revokePeerRequests(ctx context.Context) {
	s.sessions.revokeExcept(peerLedgerKey, peerRequestFrom(ctx))
}

// peerSecret reads one Authorization header with the VV-Peer scheme; present is true whenever that
// scheme is used, so a malformed value is refused rather than treated as a cookie request.
func peerSecret(r *http.Request) (secret string, present bool) {
	values := r.Header.Values("Authorization")
	for _, value := range values {
		scheme, rest, _ := strings.Cut(value, " ")
		if strings.EqualFold(scheme, peerScheme) {
			present = true
			secret = rest
		}
	}
	if len(values) != 1 || secret == "" || strings.ContainsAny(secret, " \t") {
		return "", present
	}
	return secret, present
}

// peerBoundary classifies a request carrying the peer credential: no cookie and, through the context
// value, no same-origin check. Like an API token it registers before validating, so unpairing cancels it.
func (s *server) peerBoundary(
	w http.ResponseWriter, r *http.Request, next http.Handler, class access, secret string,
	setAudience func(domain.Audience),
) {
	setAudience(domain.AudienceGuest)
	if s.pairing == nil || secret == "" {
		s.peerUnknown(w)
		return
	}
	check := func(ctx context.Context) (bool, error) { return s.pairing.Authenticate(ctx, secret) }
	ctx, tracked, release := s.sessions.openWith(r.Context(), w, peerLedgerKey, check)
	defer release()
	valid, err := check(ctx)
	if tracked.revoked() {
		valid, err = false, nil
	}
	if err != nil {
		s.internalError(w, "Could not check the peer credential.", err)
		return
	}
	if valid {
		ctx, valid = s.sessions.serve(ctx, tracked, time.Time{})
	}
	if !valid {
		s.peerUnknown(w)
		return
	}
	audience := domain.AudienceOwner
	switch values := r.Header.Values(peerAudienceHeader); {
	case len(values) == 0:
	case len(values) == 1 && values[0] == domain.AudienceGuest.String():
		audience = domain.AudienceGuest
	default:
		s.invalidRequest(w, "X-VV-Peer-Audience must be guest when present.")
		return
	}
	if !peerAllowed(r) {
		s.writeReasonError(w, http.StatusForbidden, codeForbidden, gen.ErrorReasonPeerNotAllowed,
			"The paired VVMDM cannot use this operation.")
		return
	}
	if audience == domain.AudienceGuest && class == accessOwner {
		s.unauthenticated(w)
		return
	}
	setAudience(audience)
	ctx = context.WithValue(withAudience(ctx, audience), peerRequestKey{}, tracked)
	next.ServeHTTP(w, r.WithContext(ctx))
}

func (s *server) peerUnknown(w http.ResponseWriter) {
	s.writeReasonError(w, http.StatusUnauthorized, gen.ErrorCodeUnauthenticated, gen.ErrorReasonPeerUnknown,
		"The paired VVMDM credential is not known here.")
}

func (s *server) requirePairing(w http.ResponseWriter) bool {
	if s.pairing == nil {
		s.internalError(w, "Pairing is not available.", errors.New("pairing is not configured"))
		return false
	}
	return true
}

func (s *server) GetPeer(w http.ResponseWriter, r *http.Request) {
	if !s.requirePairing(w) {
		return
	}
	state, err := s.pairing.State(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the pairing.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, peerStateBody(state, audienceFrom(r.Context())), s.logger)
}

// peerStateBody gives guests the state and reason only.
func peerStateBody(state domain.PeerState, audience domain.Audience) gen.PeerState {
	body := gen.PeerState{State: gen.PeerStateState(state.State)}
	if state.Reason != "" {
		reason := gen.PeerStateReason(state.Reason)
		body.Reason = &reason
	}
	if audience != domain.AudienceOwner {
		return body
	}
	node := peerNodeBody(state.Node)
	body.Node = &node
	if state.Peer != nil {
		body.Peer = &gen.PeerInfo{Name: state.Peer.Name, Address: state.Peer.Address, Pinned: state.Peer.PinnedSPKI != ""}
		if state.PeerVersion != "" {
			body.Peer.Version = &state.PeerVersion
		}
	}
	if state.Fingerprint != "" {
		body.Fingerprint = &state.Fingerprint
	}
	return body
}

func peerNodeBody(node domain.PeerNode) gen.PeerNode {
	return gen.PeerNode{Id: node.ID, Name: node.Name, Version: node.Version}
}

func (s *server) UnpairPeer(w http.ResponseWriter, r *http.Request) {
	if !s.requirePairing(w) {
		return
	}
	if err := s.pairing.Unpair(r.Context()); err != nil {
		s.internalError(w, "Could not unpair.", err)
		return
	}
	s.revokePeerRequests(r.Context())
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) IssuePairingCode(w http.ResponseWriter, r *http.Request) {
	if !s.requirePairing(w) {
		return
	}
	code, err := s.pairing.IssueCode(r.Context())
	if err != nil {
		s.writePairingError(w, err, "Could not issue a pairing code.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, gen.PairingCode{Code: code.Value, ExpiresAt: code.ExpiresAt}, s.logger)
}

func (s *server) PairPeer(w http.ResponseWriter, r *http.Request) {
	if !s.requirePairing(w) {
		return
	}
	var body gen.PairPeerRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxPeerBodyBytes)
	if !s.readJSONBody(w, r, &body) {
		return
	}
	input := domain.PairInput{Address: body.Address, Code: body.Code, OwnAddress: body.OwnAddress}
	if body.Fingerprint != nil {
		input.Fingerprint = *body.Fingerprint
	}
	state, err := s.pairing.Pair(r.Context(), input)
	if err != nil {
		s.writePairingError(w, err, "Could not pair.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, peerStateBody(state, domain.AudienceOwner), s.logger)
}

func (s *server) UpdatePeerAddress(w http.ResponseWriter, r *http.Request) {
	if !s.requirePairing(w) {
		return
	}
	var body gen.UpdatePeerAddressRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxPeerBodyBytes)
	if !s.readJSONBody(w, r, &body) {
		return
	}
	state, err := s.pairing.ChangeAddress(r.Context(), body.Address)
	if errors.Is(err, domain.ErrNotFound) {
		s.notFound(w, "This VVMDM is not paired.")
		return
	}
	if err != nil {
		s.writePairingError(w, err, "Could not change the address.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, peerStateBody(state, domain.AudienceOwner), s.logger)
}

func (s *server) ConfirmPeerCertificate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePairing(w) {
		return
	}
	var body gen.PeerCertificateRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxPeerBodyBytes)
	if !s.readJSONBody(w, r, &body) {
		return
	}
	state, err := s.pairing.ConfirmCertificate(r.Context(), body.Fingerprint)
	switch {
	case errors.Is(err, domain.ErrFingerprintChanged):
		s.fingerprintChanged(w, r)
		return
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "This VVMDM is not paired.")
		return
	case err != nil:
		s.internalError(w, "Could not trust the certificate.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, peerStateBody(state, domain.AudienceOwner), s.logger)
}

// fingerprintChanged names the fingerprint now waiting, so the page can show it in place of the old one.
func (s *server) fingerprintChanged(w http.ResponseWriter, r *http.Request) {
	body := gen.Error{
		Code: codeConflict, Reason: reasonRef(gen.ErrorReasonFingerprintChanged),
		Message: "The certificate of the other VVMDM changed again.",
	}
	if state, err := s.pairing.State(r.Context()); err == nil && state.Fingerprint != "" {
		body.Fingerprint = &state.Fingerprint
	}
	s.writeErrorBody(w, http.StatusConflict, body)
}

func (s *server) AcceptPairing(w http.ResponseWriter, r *http.Request) {
	if !s.requirePairing(w) {
		return
	}
	var body gen.PairingRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxPeerBodyBytes)
	if !s.readJSONBody(w, r, &body) {
		return
	}
	answer, err := s.pairing.Accept(r.Context(), domain.PairingRequest{
		Code:        body.Code,
		Node:        domain.PeerNode{ID: body.Node.Id, Name: body.Node.Name, Version: body.Node.Version},
		Address:     body.Node.Address,
		Secret:      body.Secret,
		Fingerprint: valueOrZero(body.Node.Fingerprint),
	})
	if err != nil {
		s.writePairingError(w, err, "Could not accept the pairing.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, gen.PairingAnswer{Node: peerNodeBody(answer.Node), Secret: answer.Secret}, s.logger)
}

// PeerHandshake is public at the boundary because a pairing in progress has no record yet; the
// handler checks the secret itself.
func (s *server) PeerHandshake(w http.ResponseWriter, r *http.Request) {
	if !s.requirePairing(w) {
		return
	}
	secret, _ := peerSecret(r)
	if secret == "" {
		s.peerUnknown(w)
		return
	}
	node, err := s.pairing.Handshake(r.Context(), secret)
	if errors.Is(err, domain.ErrPeerUnknown) {
		s.peerUnknown(w)
		return
	}
	if err != nil {
		s.internalError(w, "Could not answer the handshake.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.PeerHandshake{Node: peerNodeBody(node)}, s.logger)
}

func (s *server) ForgetPeer(w http.ResponseWriter, r *http.Request, nodeID string) {
	if !s.requirePairing(w) {
		return
	}
	if err := s.pairing.Forget(r.Context(), nodeID); err != nil {
		s.internalError(w, "Could not unpair.", err)
		return
	}
	s.revokePeerRequests(r.Context())
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func reasonRef(reason gen.ErrorReason) *gen.ErrorReason { return &reason }

func (s *server) writePairingError(w http.ResponseWriter, err error, fallback string) {
	var (
		address     *domain.InvalidPeerAddressError
		version     *domain.UnsupportedVersionError
		untrusted   *domain.CertificateUntrustedError
		distrusted  *domain.OwnCertificateUntrustedError
		unreachable *domain.PeerUnreachableError
	)
	switch {
	case errors.As(err, &address):
		s.writeErrorBody(w, http.StatusBadRequest, gen.Error{
			Code: codeInvalidRequest, Reason: reasonRef(gen.ErrorReasonInvalidAddress), Field: &address.Field,
			Message: "Enter an address starting with http:// or https://, with a host and no path.",
		})
	case errors.Is(err, domain.ErrInvalidPairingCode):
		s.invalidRequestReason(w, gen.ErrorReasonInvalidCode, "The pairing code is unknown, used or expired.")
	case errors.Is(err, domain.ErrInvalidPairingInput):
		s.invalidRequest(w, "The pairing request has invalid values.")
	case errors.Is(err, domain.ErrAlreadyPaired):
		s.conflictReason(w, gen.ErrorReasonAlreadyPaired, "One of the two VVMDMs is already paired.")
	case errors.As(err, &version):
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code: codeConflict, Reason: reasonRef(gen.ErrorReasonUnsupportedVersion),
			LocalVersion: &version.Local, PeerVersion: &version.Peer,
			Message: "The two VVMDMs run different versions.",
		})
	case errors.As(err, &untrusted):
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code: codeConflict, Reason: reasonRef(gen.ErrorReasonCertificateUntrusted), Fingerprint: &untrusted.Fingerprint,
			Message: "The certificate of the other VVMDM is not trusted.",
		})
	case errors.As(err, &distrusted):
		s.logger.Info("pairing failed", slog.Any("error", err))
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code: codeConflict, Reason: reasonRef(gen.ErrorReasonPeerUnreachable), Fingerprint: &distrusted.Fingerprint,
			Message: distrusted.Error(),
		})
	case errors.As(err, &unreachable):
		s.logger.Info("pairing failed", slog.Any("error", err))
		s.conflictReason(w, gen.ErrorReasonPeerUnreachable, unreachable.Message)
	default:
		s.internalError(w, fallback, err)
	}
}
