// Package password は Argon2id によるパスワードのハッシュ化と照合を受け持つ
// アダプタである（specs/016-single-account-auth/plan.md Structural Decisions 9）。
//
// ハッシュは PHC 文字列 `$argon2id$v=19$m=<KiB>,t=<回数>,p=<並列度>$<ソルト>$<鍵>`
// で表す。ソルトと鍵はパディングの無い標準の Base64 である。照合は文字列に
// 書かれたパラメータで行うので、新しく作るハッシュのパラメータを後で強めても、
// 既存のハッシュはそのまま照合できる。
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// 新しく作るハッシュのパラメータ。OWASP の Argon2id の最低推奨（19 MiB・2 回・
// 並列度 1）に合わせる。
const (
	memoryKiB  = 19456
	iterations = 2
	threads    = 1
	saltLength = 16
	keyLength  = 32
)

// 照合で受け入れるパラメータの上限。壊れた、または悪意のある保存値で、照合が
// 過大なメモリや時間を使わないようにする。
const (
	maxMemoryKiB  = 1 << 22 // 4 GiB
	maxIterations = 64
	maxSaltLength = 1024
	minKeyLength  = 16
	maxKeyLength  = 1024
)

// ErrMalformedHash は保存されたハッシュが Argon2id の PHC 文字列として
// 解釈できないことを表す。
var ErrMalformedHash = errors.New("パスワードのハッシュを解釈できません")

// Hash はパスワードを、ランダムなソルトと既定のパラメータで Argon2id の PHC 文字列にする。
func Hash(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("ソルトを作れません: %w", err)
	}
	p := params{memory: memoryKiB, iterations: iterations, threads: threads}
	key := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.threads, keyLength)
	return encode(p, salt, key), nil
}

// Verify はパスワードが PHC 文字列のハッシュと一致するかを返す。照合には文字列に
// 書かれたパラメータを使う。文字列を解釈できなければ ErrMalformedHash を返す。
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
	// 先頭の `$` の前の空文字列と、5つの欄。
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

// parseParams は `m=…,t=…,p=…` をこの順で読む。
func parseParams(s string, p *params) error {
	fields := strings.Split(s, ",")
	if len(fields) != 3 {
		return ErrMalformedHash
	}
	m, ok := parseField(fields[0], "m=", maxMemoryKiB)
	if !ok {
		return ErrMalformedHash
	}
	t, ok := parseField(fields[1], "t=", maxIterations)
	if !ok {
		return ErrMalformedHash
	}
	par, ok := parseField(fields[2], "p=", 255)
	if !ok {
		return ErrMalformedHash
	}
	// Argon2 はメモリを並列度の 8 倍以上に求める。
	if m < 8*par {
		return ErrMalformedHash
	}
	p.memory = uint32(m)
	p.iterations = uint32(t)
	p.threads = uint8(par)
	return nil
}

// parseField は `<prefix><10進の数>` を読み、1 以上 limit 以下なら返す。
// 先頭の 0 や符号は受け付けない。
func parseField(field, prefix string, limit uint64) (uint64, bool) {
	digits, ok := strings.CutPrefix(field, prefix)
	if !ok || digits == "" || (len(digits) > 1 && digits[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(digits, 10, 64)
	if err != nil || n < 1 || n > limit {
		return 0, false
	}
	return n, true
}
