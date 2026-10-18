package domain

import (
	"encoding/json"
	"fmt"
)

// Each duplicate candidate pair retains one version_judgement entry whose state follows its judgement.

// VersionJudgementActivityState treats empty or unknown judgements as waiting and maps unresolved, failed, and disconnected judgements to attention.
func VersionJudgementActivityState(judgement VersionJudgement) ActivityState {
	switch judgement {
	case VersionJudgementRunning:
		return ActivityState{Status: ActivityRunning, Reason: ActivityProcessing}
	case VersionJudgementUndecided:
		return ActivityState{Status: ActivityAttention, Reason: ActivityUndecided}
	case VersionJudgementFailed:
		return ActivityState{Status: ActivityAttention, Reason: ActivityFailed}
	case VersionJudgementNoConnection:
		return ActivityState{Status: ActivityAttention, Reason: ActivityNoConnection}
	default:
		return ActivityState{Status: ActivityRunning, Reason: ActivityWaiting}
	}
}

// ResultDecider defaults to ActivityByRule.
func (r VersionJudgeResult) ResultDecider() ActivityDecider {
	if r.Decider == "" {
		return ActivityByRule
	}
	return r.Decider
}

// VersionBundleUndoKeys records joined members and their former representative so undo can detach that original bundle.
type VersionBundleUndoKeys struct {
	Members        []string `json:"members"`
	Representative string   `json:"representative"`
}

func (k VersionBundleUndoKeys) Encode() string {
	body, _ := json.Marshal(k) // Marshaling a struct containing only strings cannot fail.
	return string(body)
}

// DecodeVersionBundleUndoKeys rejects malformed JSON and empty member lists.
func DecodeVersionBundleUndoKeys(value string) (VersionBundleUndoKeys, error) {
	var keys VersionBundleUndoKeys
	if err := json.Unmarshal([]byte(value), &keys); err != nil {
		return VersionBundleUndoKeys{}, fmt.Errorf("cannot read the undo keys: %w", err)
	}
	if len(keys.Members) == 0 {
		return VersionBundleUndoKeys{}, fmt.Errorf("the undo keys have no members: %q", value)
	}
	return keys, nil
}
