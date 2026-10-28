package domain

import (
	"errors"
	"testing"
	"time"
)

// Duplicate judging keeps its own delays through the job retry policy and never fails for good.
func TestVersionJudgeRetryPolicy(t *testing.T) {
	policy := JobRetryPolicyFor(JobVersionJudge)
	for _, c := range []struct {
		attempts int
		want     time.Duration
	}{
		{1, 10 * time.Minute},
		{2, time.Hour},
		{3, 6 * time.Hour},
		{4, 24 * time.Hour},
		{40, 24 * time.Hour},
	} {
		got := policy.AfterFailure(c.attempts, errors.New("no answer"), true, 0.5)
		if got.State != JobQueued || got.Delay != c.want {
			t.Errorf("%d 回目の失敗 = %+v, want %v 待って queued", c.attempts, got, c.want)
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
