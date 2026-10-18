// Package password hashes and verifies Argon2id PHC strings using unpadded standard Base64 salt/key
// fields. Verification uses stored parameters, so stronger defaults do not invalidate existing
// hashes.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// OWASP minimum Argon2id parameters: 19 MiB, two iterations, parallelism one.
const (
	memoryKiB  = 19456
	iterations = 2
	threads    = 1
	saltLength = 16
	keyLength  = 32
)

// Cap untrusted stored parameters to prevent resource exhaustion; the RFC 9106 secondary memory
// recommendation keeps two concurrent verifications within 128 MiB while allowing stronger
// defaults.
const (
	maxMemoryKiB  = 1 << 16 // 64 MiB
	maxIterations = 64
	maxSaltLength = 1024
	minKeyLength  = 16
	maxKeyLength  = 1024
)

var ErrMalformedHash = errors.New("cannot parse the password hash")

// Hash uses a random salt and default Argon2id parameters.
func Hash(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("cannot create a salt: %w", err)
	}
	p := params{memory: memoryKiB, iterations: iterations, threads: threads}
	key := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.threads, keyLength)
	return encode(p, salt, key), nil
}

// Verify uses PHC-encoded parameters and returns ErrMalformedHash for invalid encodings.
func Verify(password, encoded string) (bool, error) {
	p, salt, key, err := decode(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1, nil
}

type params struct {
	memory     uint32
	iterations uint32
	threads    uint8
}

var b64 = base64.RawStdEncoding

func encode(p params, salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.iterations, p.threads,
		b64.EncodeToString(salt), b64.EncodeToString(key))
}

func decode(encoded string) (params, []byte, []byte, error) {
	var p params
	parts := strings.Split(encoded, "$")
	// Leading empty component before $, followed by five fields.
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return p, nil, nil, ErrMalformedHash
	}
	if parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return p, nil, nil, ErrMalformedHash
	}
	if err := parseParams(parts[3], &p); err != nil {
		return p, nil, nil, err
	}
	salt, err := b64.Strict().DecodeString(parts[4])
	if err != nil || len(salt) == 0 || len(salt) > maxSaltLength {
		return p, nil, nil, ErrMalformedHash
	}
	key, err := b64.Strict().DecodeString(parts[5])
	if err != nil || len(key) < minKeyLength || len(key) > maxKeyLength {
		return p, nil, nil, ErrMalformedHash
	}
	return p, salt, key, nil
}

// Check bounds before narrowing parsed parameter types.
func parseParams(s string, p *params) error {
	fields := strings.Split(s, ",")
	if len(fields) != 3 {
		return ErrMalformedHash
	}
	m, ok := parseField(fields[0], "m=")
	if !ok || m > maxMemoryKiB {
		return ErrMalformedHash
	}
	t, ok := parseField(fields[1], "t=")
	if !ok || t > maxIterations {
		return ErrMalformedHash
	}
	par, ok := parseField(fields[2], "p=")
	if !ok || par > math.MaxUint8 {
		return ErrMalformedHash
	}
	// Argon2 requires memory of at least eight times parallelism.
	if m < 8*par {
		return ErrMalformedHash
	}
	p.memory = uint32(m)
	p.iterations = uint32(t)
	p.threads = uint8(par)
	return nil
}

// Accept positive uint32 decimal values without signs or leading zeros; callers enforce parameter
// limits.
func parseField(field, prefix string) (uint64, bool) {
	digits, ok := strings.CutPrefix(field, prefix)
	if !ok || digits == "" || (len(digits) > 1 && digits[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(digits, 10, 32)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}
