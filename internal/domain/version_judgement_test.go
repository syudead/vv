package domain

import (
	"testing"
	"time"
)

func TestVersionJudgeRetryAt(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, c := range []struct {
		attempts int
		want     time.Duration
	}{
		{0, 10 * time.Minute},
		{1, 10 * time.Minute},
		{2, time.Hour},
		{3, 6 * time.Hour},
		{4, 24 * time.Hour},
		{40, 24 * time.Hour},
	} {
		if got := VersionJudgeRetryAt(c.attempts, now).Sub(now); got != c.want {
			t.Errorf("%d 回目の失敗の待ち = %v, want %v", c.attempts, got, c.want)
		}
	}
}

func TestVersionSidesConflict(t *testing.T) {
	a, b := []string{"a", "c"}, []string{"b"}
	for _, c := range []struct {
		name      string
		differing [][2]string
		want      bool
	}{
		{"組なし", nil, false},
		{"同じ側の中の組", [][2]string{{"a", "c"}}, false},
		{"側をまたぐ組", [][2]string{{"b", "c"}}, true},
		{"鍵の順に依らない", [][2]string{{"c", "b"}}, true},
		{"どちらの側でもない鍵", [][2]string{{"b", "x"}}, false},
	} {
		if got := VersionSidesConflict(a, b, c.differing); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
