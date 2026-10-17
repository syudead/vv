package domain

import (
	"slices"
	"testing"
)

// 組の記録の状態は候補の判定に従う（specs/055-activity/data-model.md「Reasons」）。
func TestVersionJudgementActivityState(t *testing.T) {
	for judgement, want := range map[VersionJudgement]ActivityState{
		"":                           {Status: ActivityRunning, Reason: ActivityWaiting},
		VersionJudgementRunning:      {Status: ActivityRunning, Reason: ActivityProcessing},
		VersionJudgementUndecided:    {Status: ActivityAttention, Reason: ActivityUndecided},
		VersionJudgementFailed:       {Status: ActivityAttention, Reason: ActivityFailed},
		VersionJudgementNoConnection: {Status: ActivityAttention, Reason: ActivityNoConnection},
	} {
		if got := VersionJudgementActivityState(judgement); got != want {
			t.Errorf("VersionJudgementActivityState(%q) = %+v, want %+v", judgement, got, want)
		}
	}
	if got := (VersionJudgeResult{}).ResultDecider(); got != ActivityByRule {
		t.Errorf("空の判断したもの = %q, want rule", got)
	}
}

// 取り消しの鍵は JSON で往復し、メンバーの無いものは読まない。
func TestVersionBundleUndoKeysRoundTrip(t *testing.T) {
	keys := VersionBundleUndoKeys{Members: []string{"key-a", "key-c"}, Representative: "key-a"}
	got, err := DecodeVersionBundleUndoKeys(keys.Encode())
	if err != nil || !slices.Equal(got.Members, keys.Members) || got.Representative != keys.Representative {
		t.Errorf("往復 = %+v, %v, want %+v", got, err, keys)
	}
	for _, bad := range []string{"", "{", `{"members":[]}`} {
		if _, err := DecodeVersionBundleUndoKeys(bad); err == nil {
			t.Errorf("DecodeVersionBundleUndoKeys(%q) に誤りが無い", bad)
		}
	}
}
