package domain

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// PeerRecord is the one peer this node is paired with. The incoming secret is never held in plain text.
type PeerRecord struct {
	NodeID  string
	Name    string
	Address string
	// OutgoingSecret is what this node presents to the peer.
	OutgoingSecret string
	// IncomingRevoked is true once the incoming secret hash was cleared; the peer credential is then refused.
	IncomingRevoked bool
	// PinnedSPKI is empty when the system roots verify the peer or the address is http://.
	PinnedSPKI string
	PairedAt   time.Time
}

// PeerNode identifies one node to the other: its node id, host name and release version.
type PeerNode struct {
	ID      string
	Name    string
	Version string
}

var (
	ErrAlreadyPaired       = errors.New("this node is already paired")
	ErrInvalidPairingCode  = errors.New("the pairing code is unknown, used or expired")
	ErrUnsupportedVersion  = errors.New("the peer runs another version")
	ErrPeerUnreachable     = errors.New("the peer could not be reached")
	ErrInvalidPeerAddress  = errors.New("invalid peer address")
	ErrInvalidPairingInput = errors.New("invalid pairing request")
)

// UnsupportedVersionError carries both versions so the screen can name them.
type UnsupportedVersionError struct {
	Local, Peer string
}

func (e *UnsupportedVersionError) Error() string {
	return fmt.Sprintf("the peer runs %s and this node %s", e.Peer, e.Local)
}

func (e *UnsupportedVersionError) Unwrap() error { return ErrUnsupportedVersion }

// PeerUnreachableError says which call failed; Message is English and free of secrets.
type PeerUnreachableError struct {
	Message string
	Cause   error
}

func (e *PeerUnreachableError) Error() string { return e.Message }

func (e *PeerUnreachableError) Unwrap() []error { return []error{ErrPeerUnreachable, e.Cause} }

// CertificateUntrustedError names the SPKI fingerprint of a certificate the system roots did not verify.
type CertificateUntrustedError struct {
	Fingerprint string
}

func (e *CertificateUntrustedError) Error() string {
	return "the peer certificate is not trusted (SPKI " + e.Fingerprint + ")"
}

// OwnCertificateUntrustedError is the other node refusing the certificate this node's address presents, with
// that certificate's SPKI fingerprint.
type OwnCertificateUntrustedError struct {
	Fingerprint string
}

func (e *OwnCertificateUntrustedError) Error() string {
	return "The other VVMDM does not trust the certificate of this one (SPKI " + e.Fingerprint + ")."
}

func (e *OwnCertificateUntrustedError) Unwrap() error { return ErrPeerUnreachable }

// OwnCertificateUnverifiedError is this node's own https address presenting a certificate the system roots do
// not verify and the owner has not confirmed; Fingerprint is the SPKI it presents now.
type OwnCertificateUnverifiedError struct {
	Fingerprint string
}

func (e *OwnCertificateUnverifiedError) Error() string {
	return "the certificate of this VVMDM's own address is not confirmed (SPKI " + e.Fingerprint + ")"
}

// PresentedCertificate is the certificate an https address presents in a TLS handshake.
type PresentedCertificate struct {
	Fingerprint string
	// Verified reports that the system roots verify it for the address's host.
	Verified bool
}

// ValidFingerprint accepts the standard base64 of a 32-byte SHA-256, the form Fingerprint values take.
func ValidFingerprint(fingerprint string) bool {
	raw, err := base64.StdEncoding.Strict().DecodeString(fingerprint)
	return err == nil && len(raw) == 32
}

// CheckPeerVersions accepts only an equal release version; "dev" therefore matches only "dev".
func CheckPeerVersions(local, peer string) error {
	if local != peer {
		return &UnsupportedVersionError{Local: local, Peer: peer}
	}
	return nil
}

// NormalizePeerAddress accepts an http or https URL with a host and no path, query, fragment or user,
// and returns it without a trailing slash.
func NormalizePeerAddress(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.Opaque != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", ErrInvalidPeerAddress
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// PeerNameMaxLength counts code points, like tag names.
const PeerNameMaxLength = 100

// ValidPeerName accepts a host name of 1 to 100 code points without control characters.
func ValidPeerName(name string) bool {
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > PeerNameMaxLength {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool { return unicode.Is(unicode.Cc, r) })
}

// NewNodeID returns a random UUID v4 in its canonical lowercase form.
func NewNodeID(random io.Reader) (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(random, b[:]); err != nil {
		return "", fmt.Errorf("cannot create a node id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// ValidNodeID accepts the canonical lowercase form of a UUID.
func ValidNodeID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, r := range id {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdef", r) {
				return false
			}
		}
	}
	return true
}

// PeerSecretPrefix makes a peer credential recognizable, as vvt_ does for API tokens.
const PeerSecretPrefix = "vvp_"

const peerSecretBytes = 32

var peerSecretEncoding = base64.RawURLEncoding

// NewPeerSecret returns vvp_ followed by 32 random bytes in unpadded base64url (43 characters).
func NewPeerSecret(random io.Reader) (string, error) {
	raw := make([]byte, peerSecretBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", fmt.Errorf("cannot create a peer secret: %w", err)
	}
	return PeerSecretPrefix + peerSecretEncoding.EncodeToString(raw), nil
}

// ValidPeerSecret checks the format only, so a malformed value is refused without a store read.
func ValidPeerSecret(secret string) bool {
	rest, ok := strings.CutPrefix(secret, PeerSecretPrefix)
	if !ok || len(rest) != peerSecretEncoding.EncodedLen(peerSecretBytes) {
		return false
	}
	raw, err := peerSecretEncoding.Strict().DecodeString(rest)
	return err == nil && len(raw) == peerSecretBytes
}

// The pairing code avoids characters that read alike (0/O, 1/I/L, U/V).
const (
	PairingCodeAlphabet = "ABCDEFGHJKMNPQRSTWXYZ23456789"
	PairingCodeLength   = 8
	PairingCodeLifetime = 10 * time.Minute
)

type PairingCode struct {
	Value     string
	ExpiresAt time.Time
}

// PairingCodeSlot holds a node's one pairing code; the caller serializes access.
type PairingCodeSlot struct {
	code *PairingCode
}

// Issue replaces any unused code with a new one valid for PairingCodeLifetime.
func (s *PairingCodeSlot) Issue(random io.Reader, now time.Time) (PairingCode, error) {
	value, err := newPairingCode(random)
	if err != nil {
		return PairingCode{}, err
	}
	code := PairingCode{Value: value, ExpiresAt: now.Add(PairingCodeLifetime)}
	s.code = &code
	return code, nil
}

// Matches reports whether input is the current, unexpired code; an expired code is dropped. The code stays
// until Clear, so a pairing whose callback fails can be repeated with it. Letter case and surrounding spaces
// in input are ignored.
func (s *PairingCodeSlot) Matches(input string, now time.Time) bool {
	if s.code == nil {
		return false
	}
	if !now.Before(s.code.ExpiresAt) {
		s.code = nil
		return false
	}
	given := strings.ToUpper(strings.TrimSpace(input))
	return subtle.ConstantTimeCompare([]byte(given), []byte(s.code.Value)) == 1
}

// Clear drops the current code, as a successful pairing does.
func (s *PairingCodeSlot) Clear() { s.code = nil }

// Rejection sampling keeps every character equally likely.
func newPairingCode(random io.Reader) (string, error) {
	const limit = 256 - 256%len(PairingCodeAlphabet)
	code := make([]byte, 0, PairingCodeLength)
	buf := make([]byte, PairingCodeLength*2)
	for len(code) < PairingCodeLength {
		if _, err := io.ReadFull(random, buf); err != nil {
			return "", fmt.Errorf("cannot create a pairing code: %w", err)
		}
		for _, b := range buf {
			if int(b) < limit && len(code) < PairingCodeLength {
				code = append(code, PairingCodeAlphabet[int(b)%len(PairingCodeAlphabet)])
			}
		}
	}
	return string(code), nil
}

// PeerConnection is the state the Settings page shows.
type PeerConnection string

const (
	PeerUnpaired    PeerConnection = "unpaired"
	PeerConnected   PeerConnection = "connected"
	PeerUnreachable PeerConnection = "unreachable"
	PeerRevoked     PeerConnection = "revoked"
)

// PeerUnreachableReason is present only with PeerUnreachable.
type PeerUnreachableReason string

const (
	PeerReasonNetwork     PeerUnreachableReason = "network"
	PeerReasonVersion     PeerUnreachableReason = "version"
	PeerReasonCertificate PeerUnreachableReason = "certificate"
)

// PeerState is what GET /api/peer answers; Peer is nil when unpaired.
type PeerState struct {
	State       PeerConnection
	Reason      PeerUnreachableReason
	Peer        *PeerRecord
	PeerVersion string
	Fingerprint string
	Node        PeerNode
}

// ErrPeerUnknown answers a peer credential or handshake secret this node does not hold.
var ErrPeerUnknown = errors.New("the peer credential is unknown")

// PeerTarget is how this node reaches the peer; PinnedSPKI is empty unless the owner confirmed a certificate.
type PeerTarget struct {
	Address    string
	PinnedSPKI string
}

// PairInput is what the owner typed on the Pairing page.
type PairInput struct {
	Address    string
	Code       string
	OwnAddress string
	// Fingerprint is the other node's certificate the owner confirmed.
	Fingerprint string
	// OwnFingerprint is the certificate of OwnAddress the owner confirmed.
	OwnFingerprint string
}

// PairingRequest is what the pairing node sends with the other node's code.
type PairingRequest struct {
	Code    string
	Node    PeerNode
	Address string
	// Secret is what the receiving node will present to the sender.
	Secret string
	// Fingerprint is the SPKI fingerprint of the certificate Address presents, as the sender's owner confirmed
	// it when the system roots do not verify it; the receiving node pins it. Empty otherwise.
	Fingerprint string
}

// PairingAnswer is the receiving node's identity and the secret the sender will present to it.
type PairingAnswer struct {
	Node   PeerNode
	Secret string
}

// InvalidPeerAddressError names the request field holding the rejected address.
type InvalidPeerAddressError struct {
	Field string
}

func (e *InvalidPeerAddressError) Error() string { return "invalid peer address in " + e.Field }

func (e *InvalidPeerAddressError) Unwrap() error { return ErrInvalidPeerAddress }

// InitialPeerState is the state after a start, before any connection: unpaired without a record,
// revoked when the record lost its incoming secret, and unreachable otherwise.
func InitialPeerState(record *PeerRecord) (PeerConnection, PeerUnreachableReason) {
	switch {
	case record == nil:
		return PeerUnpaired, ""
	case record.IncomingRevoked:
		return PeerRevoked, ""
	default:
		return PeerUnreachable, PeerReasonNetwork
	}
}

// ErrFingerprintChanged answers a certificate confirmation whose fingerprint is not the one waiting.
var ErrFingerprintChanged = errors.New("the certificate fingerprint changed")

// The reconnect loop waits PeerRetryFirst after the first failure and doubles up to PeerRetryMax.
const (
	PeerRetryFirst = 5 * time.Second
	PeerRetryMax   = 60 * time.Second
)

// PeerRetryDelay is the wait after the given number of consecutive failures, counted from 1.
func PeerRetryDelay(failures int) time.Duration {
	delay := PeerRetryFirst
	for i := 1; i < failures && delay < PeerRetryMax; i++ {
		delay *= 2
	}
	return min(delay, PeerRetryMax)
}

// PeerStateChanged tells the screens to read the connection state again.
type PeerStateChanged struct{}

func (PeerStateChanged) event() {}

// PeerChanged forwards the peer's change notifications to the screens, with remote ids.
type PeerChanged struct {
	VideoIDs       []int64
	GroupIDs       []int64
	SmartFolderIDs []int64
}

func (PeerChanged) event() {}
