package domain

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestPairingCodeUsesTheAlphabetAndLength(t *testing.T) {
	var slot PairingCodeSlot
	code, err := slot.Issue(rand.Reader, time.Unix(1_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(code.Value) != 8 {
		t.Errorf("コードの長さ = %d, want 8: %q", len(code.Value), code.Value)
	}
	for _, r := range code.Value {
		if !strings.ContainsRune(PairingCodeAlphabet, r) {
			t.Errorf("コード %q に字母にない文字 %q がある", code.Value, r)
		}
	}
	for _, ambiguous := range "0O1IL" {
		if strings.ContainsRune(PairingCodeAlphabet, ambiguous) {
			t.Errorf("字母に紛らわしい文字 %q がある", ambiguous)
		}
	}
	if want := time.Unix(1_000, 0).Add(10 * time.Minute); !code.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", code.ExpiresAt, want)
	}
}

func TestPairingCodeExpiresAfterTenMinutes(t *testing.T) {
	start := time.Unix(1_000, 0)
	var slot PairingCodeSlot
	code, err := slot.Issue(rand.Reader, start)
	if err != nil {
		t.Fatal(err)
	}
	if slot.Matches(code.Value, start.Add(10*time.Minute)) {
		t.Error("10 分ちょうどのコードが通った")
	}

	code, err = slot.Issue(rand.Reader, start)
	if err != nil {
		t.Fatal(err)
	}
	if !slot.Matches(" "+strings.ToLower(code.Value)+" ", start.Add(10*time.Minute-time.Second)) {
		t.Fatal("期限内のコードが通らない")
	}
}

// A failed callback must leave the code usable; only a pairing that succeeded clears it.
func TestPairingCodeStaysUntilCleared(t *testing.T) {
	now := time.Unix(1_000, 0)
	var slot PairingCodeSlot
	code, err := slot.Issue(rand.Reader, now)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		if !slot.Matches(code.Value, now) {
			t.Fatalf("%d 回目の照合でコードが通らない", attempt+1)
		}
	}
	slot.Clear()
	if slot.Matches(code.Value, now) {
		t.Error("消したコードが通った")
	}
}

func TestSecondPairingCodeReplacesTheFirst(t *testing.T) {
	now := time.Unix(1_000, 0)
	var slot PairingCodeSlot
	// Fixed bytes give two different codes: all zeros, then all ones.
	first, err := slot.Issue(bytes.NewReader(bytes.Repeat([]byte{0}, 16)), now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := slot.Issue(bytes.NewReader(bytes.Repeat([]byte{1}, 16)), now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Value == second.Value {
		t.Fatalf("2 つのコードが同じ %q", first.Value)
	}
	if slot.Matches(first.Value, now) {
		t.Error("置き換えられた 1 つ目のコードが通った")
	}
	if !slot.Matches(second.Value, now) {
		t.Error("2 つ目のコードが通らない")
	}
}

func TestValidFingerprint(t *testing.T) {
	valid := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	if !ValidFingerprint(valid) {
		t.Errorf("ValidFingerprint(%q) = false", valid)
	}
	for _, bad := range []string{
		"", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 31)),
		base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)), valid + " ",
	} {
		if ValidFingerprint(bad) {
			t.Errorf("ValidFingerprint(%q) = true", bad)
		}
	}
}

func TestPeerSecretFormat(t *testing.T) {
	secret, err := NewPeerSecret(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "vvp_") || len(secret) != len("vvp_")+43 {
		t.Errorf("secret = %q, want vvp_ と 43 文字", secret)
	}
	if !ValidPeerSecret(secret) {
		t.Errorf("作った secret %q が形式に合わない", secret)
	}
	for _, bad := range []string{
		"", strings.TrimPrefix(secret, "vvp_"), "vvt_" + secret[4:], secret[:len(secret)-1], secret + "A",
		"vvp_" + strings.Repeat("!", 43),
	} {
		if ValidPeerSecret(bad) {
			t.Errorf("ValidPeerSecret(%q) = true", bad)
		}
	}
}

func TestNormalizePeerAddress(t *testing.T) {
	for raw, want := range map[string]string{
		"http://192.168.1.20:8080": "http://192.168.1.20:8080",
		" https://nas.local/ ":     "https://nas.local",
		"http://[fe80::1]:8080":    "http://[fe80::1]:8080",
	} {
		got, err := NormalizePeerAddress(raw)
		if err != nil || got != want {
			t.Errorf("NormalizePeerAddress(%q) = %q, %v, want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"", "nas.local:8080", "ftp://nas.local", "http://", "http://nas.local/vv", "http://nas.local?x=1",
		"http://user@nas.local", "http://nas.local#top",
	} {
		if _, err := NormalizePeerAddress(raw); err == nil {
			t.Errorf("NormalizePeerAddress(%q) が通った", raw)
		}
	}
}
