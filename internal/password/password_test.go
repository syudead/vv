package password

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestHashAndVerify(t *testing.T) {
	const pw = "correct horse Battery staple"
	first, err := Hash(pw)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	second, err := Hash(pw)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if first == second {
		t.Fatalf("同じパスワードのハッシュが同じ文字列になった: %q", first)
	}
	wantPrefix := "$argon2id$v=19$m=19456,t=2,p=1$"
	for _, h := range []string{first, second} {
		if !strings.HasPrefix(h, wantPrefix) {
			t.Errorf("ハッシュ %q が %q で始まらない", h, wantPrefix)
		}
		parts := strings.Split(h, "$")
		if salt, _ := base64.RawStdEncoding.DecodeString(parts[4]); len(salt) != 16 {
			t.Errorf("ソルトの長さ = %d, want 16", len(salt))
		}
		if key, _ := base64.RawStdEncoding.DecodeString(parts[5]); len(key) != 32 {
			t.Errorf("鍵の長さ = %d, want 32", len(key))
		}
		ok, err := Verify(pw, h)
		if err != nil || !ok {
			t.Errorf("Verify(正しいパスワード) = %v, %v; want true, nil", ok, err)
		}
	}

	for _, wrong := range []string{
		"correct horse Battery stapl",  // 1文字欠け
		"correct horse Battery stapla", // 1文字違い
		"correct horse battery staple", // 大文字小文字違い
		"CORRECT HORSE BATTERY STAPLE",
		"",
	} {
		ok, err := Verify(wrong, first)
		if err != nil || ok {
			t.Errorf("Verify(%q) = %v, %v; want false, nil", wrong, ok, err)
		}
	}
}

func TestVerifyUsesEncodedParameters(t *testing.T) {
	const pw = "パスワード"
	salt := []byte("0123456789abcdefXYZ")
	key := argon2.IDKey([]byte(pw), salt, 1, 64, 2, 24)
	encoded := fmt.Sprintf("$argon2id$v=19$m=64,t=1,p=2$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))

	ok, err := Verify(pw, encoded)
	if err != nil || !ok {
		t.Fatalf("Verify(別のパラメータ) = %v, %v; want true, nil", ok, err)
	}
	ok, err = Verify("ぱすわーど", encoded)
	if err != nil || ok {
		t.Fatalf("Verify(違うパスワード) = %v, %v; want false, nil", ok, err)
	}
}

func TestVerifyAcceptsMemoryAtLimit(t *testing.T) {
	const pw = "パスワード"
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte(pw), salt, 1, 65536, 1, 32)
	encoded := fmt.Sprintf("$argon2id$v=19$m=65536,t=1,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))

	ok, err := Verify(pw, encoded)
	if err != nil || !ok {
		t.Fatalf("Verify(m=65536) = %v, %v; want true, nil", ok, err)
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	valid, err := Hash("x")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	parts := strings.Split(valid, "$")
	salt, key := parts[4], parts[5]
	join := func(fields ...string) string { return strings.Join(fields, "$") }

	for name, encoded := range map[string]string{
		"空":             "",
		"区切りだけ":         "$",
		"欄が足りない":        join("", "argon2id", "v=19", "m=19456,t=2,p=1", salt),
		"欄が多い":          join("", "argon2id", "v=19", "m=19456,t=2,p=1", salt, key, ""),
		"先頭が$でない":       join("x", "argon2id", "v=19", "m=19456,t=2,p=1", salt, key),
		"argon2i":       join("", "argon2i", "v=19", "m=19456,t=2,p=1", salt, key),
		"argon2d":       join("", "argon2d", "v=19", "m=19456,t=2,p=1", salt, key),
		"版が違う":          join("", "argon2id", "v=16", "m=19456,t=2,p=1", salt, key),
		"版が無い":          join("", "argon2id", "m=19456,t=2,p=1", salt, key, ""),
		"パラメータの順が違う":    join("", "argon2id", "v=19", "t=2,m=19456,p=1", salt, key),
		"パラメータが足りない":    join("", "argon2id", "v=19", "m=19456,t=2", salt, key),
		"数でない":          join("", "argon2id", "v=19", "m=abc,t=2,p=1", salt, key),
		"負の数":           join("", "argon2id", "v=19", "m=19456,t=-2,p=1", salt, key),
		"0回":            join("", "argon2id", "v=19", "m=19456,t=0,p=1", salt, key),
		"並列度0":          join("", "argon2id", "v=19", "m=19456,t=2,p=0", salt, key),
		"並列度が大きすぎる":     join("", "argon2id", "v=19", "m=19456,t=2,p=256", salt, key),
		"並列度が32ビットを超える": join("", "argon2id", "v=19", "m=19456,t=2,p=4294967297", salt, key),
		"回数が大きすぎる":      join("", "argon2id", "v=19", "m=19456,t=65,p=1", salt, key),
		"メモリが大きすぎる":     join("", "argon2id", "v=19", "m=99999999999,t=2,p=1", salt, key),
		"メモリが64MiBを超える": join("", "argon2id", "v=19", "m=65537,t=2,p=1", salt, key),
		"メモリが4GiB":      join("", "argon2id", "v=19", "m=4194304,t=2,p=1", salt, key),
		"メモリが並列度に足りない":  join("", "argon2id", "v=19", "m=8,t=1,p=2", salt, key),
		"先頭の0":          join("", "argon2id", "v=19", "m=019456,t=2,p=1", salt, key),
		"ソルトが空":         join("", "argon2id", "v=19", "m=19456,t=2,p=1", "", key),
		"ソルトがBase64でない": join("", "argon2id", "v=19", "m=19456,t=2,p=1", "!!!", key),
		"鍵が空":           join("", "argon2id", "v=19", "m=19456,t=2,p=1", salt, ""),
		"鍵が短すぎる":        join("", "argon2id", "v=19", "m=19456,t=2,p=1", salt, "AAAA"),
		"鍵にパディング":       join("", "argon2id", "v=19", "m=19456,t=2,p=1", salt, key+"=="),
	} {
		ok, err := Verify("x", encoded)
		if !errors.Is(err, ErrMalformedHash) || ok {
			t.Errorf("%s: Verify(%q) = %v, %v; want false, ErrMalformedHash", name, encoded, ok, err)
		}
	}
}
