package domain

import (
	"encoding/json"
	"fmt"
)

// 重複の候補の判定の記録。組ごとに 1 件の
// version_judgement の記録を保ち、その状態は候補の判定に従う。

// VersionJudgementActivityState は候補の判定 judgement（空は待ち）のときの、組の開いている記録の状態である。
// 待ちは running/waiting、判定中は running/processing、決められない・失敗・接続なしは attention とその理由。
// 知らない値は待ちとみなす。
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

// ResultDecider は結果を決めたものを返す。空は ActivityByRule である。
func (r VersionJudgeResult) ResultDecider() ActivityDecider {
	if r.Decider == "" {
		return ActivityByRule
	}
	return r.Decider
}

// VersionBundleUndoKeys は自動で束ねた組の、加わった側のメンバーの鍵と、その側の元の代表の鍵である
// （activity_entries.undo_keys）。取り消しはこの側を元の代表のまとまりとして外す。
type VersionBundleUndoKeys struct {
	Members        []string `json:"members"`
	Representative string   `json:"representative"`
}

// Encode は undo_keys の列に書く JSON を返す。
func (k VersionBundleUndoKeys) Encode() string {
	body, _ := json.Marshal(k) // 文字列だけの構造体の Marshal は失敗しない。
	return string(body)
}

// DecodeVersionBundleUndoKeys は Encode の逆である。読めないか、メンバーが無いものは誤りを返す。
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
