package domain

import (
	"slices"
	"testing"
)

// frameRuleFingerprints は 1 秒間隔の 2 本の指紋を作る。distances[i] はコマ i の組の距離で、
// b のコマ i は a のコマ i（ハッシュ 0）から下位 distances[i] ビットを立てたもの。
func frameRuleFingerprints(distances []int) (Fingerprint, Fingerprint) {
	a := Fingerprint{Version: FingerprintVersion, IntervalMs: 1000}
	b := Fingerprint{Version: FingerprintVersion, IntervalMs: 1000}
	for _, distance := range distances {
		a.Frames = append(a.Frames, FrameHash{})
		b.Frames = append(b.Frames, FrameHash{Hash: 1<<uint(distance) - 1})
	}
	return a, b
}

// shownFramesA はモデルに見せるコマの組の、a の側の番号である。
func shownFramesA(rule VersionFrameRule) []int {
	var indexes []int
	for _, pair := range rule.Shown {
		indexes = append(indexes, pair.A)
	}
	return indexes
}

// 距離がすべて 12 以内なら、モデルに聞かずに同じ動画（specs/054-auto-dup/research.md R-1）。
func TestJudgeVersionFramesAllWithinCutOff(t *testing.T) {
	distances := make([]int, 20)
	for i := range distances {
		distances[i] = i % 13
	}
	a, b := frameRuleFingerprints(distances)
	rule := JudgeVersionFrames(a, b)
	if rule.Outcome != VersionFramesSame || rule.Compared != 20 || rule.Differing != 0 || rule.Shown != nil {
		t.Fatalf("rule = %+v", rule)
	}
}

// 20 コマ中 1 つだけ離れて食い違っても同じ動画（5% 以下で連続しない）。
func TestJudgeVersionFramesOneIsolatedDifference(t *testing.T) {
	distances := make([]int, 20)
	distances[9] = 30
	a, b := frameRuleFingerprints(distances)
	rule := JudgeVersionFrames(a, b)
	if rule.Outcome != VersionFramesSame || rule.Differing != 1 {
		t.Fatalf("rule = %+v", rule)
	}
}

// 4 つ連続で食い違えば、その連なりのコマをモデルに見せる（research.md R-1・R-2 の D）。
func TestJudgeVersionFramesRunOfFour(t *testing.T) {
	distances := make([]int, 20)
	for i := 8; i < 12; i++ {
		distances[i] = 33
	}
	a, b := frameRuleFingerprints(distances)
	rule := JudgeVersionFrames(a, b)
	if rule.Outcome != VersionFramesDiffer || rule.Differing != 4 {
		t.Fatalf("rule = %+v", rule)
	}
	if got := shownFramesA(rule); !slices.Equal(got, []int{8, 9, 10, 11}) {
		t.Fatalf("見せるコマ = %v", got)
	}
}

// 単色のコマを挟む 2 つの食い違いは連なりではない。比べられる組の 5% 以下なら同じ動画。
func TestJudgeVersionFramesFlatFrameBreaksRun(t *testing.T) {
	distances := make([]int, 41)
	distances[10] = 30
	distances[12] = 30
	a, b := frameRuleFingerprints(distances)
	a.Frames[11].Flat = true
	rule := JudgeVersionFrames(a, b)
	if rule.Outcome != VersionFramesSame || rule.Compared != 40 || rule.Differing != 2 {
		t.Fatalf("rule = %+v", rule)
	}
}

// b の側の単色のコマを挟む 2 つの食い違いも連なりではない。
func TestJudgeVersionFramesFlatFrameOnBBreaksRun(t *testing.T) {
	distances := make([]int, 41)
	distances[10] = 30
	distances[12] = 30
	a, b := frameRuleFingerprints(distances)
	b.Frames[11].Flat = true
	rule := JudgeVersionFrames(a, b)
	if rule.Outcome != VersionFramesSame || rule.Compared != 40 || rule.Differing != 2 {
		t.Fatalf("rule = %+v", rule)
	}
}

// 間隔の違う 2 本の指紋（5 秒と 2.5 秒）で時刻 50 秒と 55 秒の組だけが食い違うとき、どちらを
// a にしても連なりとしてモデルに聞く。細かい方を a にすると組の a の番号は 2 ずつ進む。
func TestJudgeVersionFramesRunAcrossIntervalsIsSymmetric(t *testing.T) {
	coarse := Fingerprint{Version: FingerprintVersion, IntervalMs: 5000}
	fine := Fingerprint{Version: FingerprintVersion, IntervalMs: 2500}
	for i := range 40 {
		coarse.Frames = append(coarse.Frames, FrameHash{})
		hash := uint64(0)
		if i == 10 || i == 11 {
			hash = 1<<30 - 1
		}
		fine.Frames = append(fine.Frames, FrameHash{Hash: hash}, FrameHash{Hash: hash})
	}
	for _, c := range []struct {
		name string
		a, b Fingerprint
	}{{"coarse first", coarse, fine}, {"fine first", fine, coarse}} {
		rule := JudgeVersionFrames(c.a, c.b)
		if rule.Outcome != VersionFramesDiffer || rule.Compared != 40 || rule.Differing != 2 || len(rule.Shown) != 2 {
			t.Errorf("%s: rule = %+v", c.name, rule)
		}
	}
}

// 2 つ連続の食い違いは、5% 以下でもモデルに聞く。複数の連なりのうち最も長いものを選ぶ。
func TestJudgeVersionFramesLongestRun(t *testing.T) {
	distances := make([]int, 60)
	distances[3], distances[4] = 20, 20
	for i := 30; i < 33; i++ {
		distances[i] = 20
	}
	a, b := frameRuleFingerprints(distances)
	rule := JudgeVersionFrames(a, b)
	if rule.Outcome != VersionFramesDiffer {
		t.Fatalf("rule = %+v", rule)
	}
	if got := shownFramesA(rule); !slices.Equal(got, []int{30, 31, 32}) {
		t.Fatalf("見せるコマ = %v", got)
	}
}

// 連なりが 4 より長ければ、その中に等間隔に 4 コマを散らす。
func TestJudgeVersionFramesSpreadsALongRun(t *testing.T) {
	distances := make([]int, 20)
	for i := 4; i < 12; i++ {
		distances[i] = 40
	}
	a, b := frameRuleFingerprints(distances)
	rule := JudgeVersionFrames(a, b)
	if got := shownFramesA(rule); !slices.Equal(got, []int{5, 7, 9, 11}) {
		t.Fatalf("見せるコマ = %v", got)
	}
}

// 連続しない食い違いでも 5% を超えればモデルに聞き、食い違ったコマ全体から選ぶ。
func TestJudgeVersionFramesScatteredOverShare(t *testing.T) {
	distances := make([]int, 20)
	distances[2], distances[10] = 25, 25
	a, b := frameRuleFingerprints(distances)
	rule := JudgeVersionFrames(a, b)
	if rule.Outcome != VersionFramesDiffer {
		t.Fatalf("rule = %+v", rule)
	}
	if got := shownFramesA(rule); !slices.Equal(got, []int{2, 10}) {
		t.Fatalf("見せるコマ = %v", got)
	}
}

// 単色のコマは比べず、組が足りなければ比べられない。
func TestJudgeVersionFramesIncomparable(t *testing.T) {
	a, b := frameRuleFingerprints([]int{0, 0, 0, 0})
	for i := 1; i < 4; i++ {
		a.Frames[i].Flat = true
	}
	if rule := JudgeVersionFrames(a, b); rule.Outcome != VersionFramesIncomparable {
		t.Fatalf("rule = %+v", rule)
	}
}

// 0.8 以上は同じ、0.2 以下は違う、間は決められない（research.md R-3）。
func TestVersionVerdictFor(t *testing.T) {
	for _, c := range []struct {
		probability float64
		want        VersionVerdict
	}{
		{1, VersionSame},
		{0.85, VersionSame},
		{0.8, VersionSame},
		{0.79, VersionUndecided},
		{0.5, VersionUndecided},
		{0.21, VersionUndecided},
		{0.2, VersionDifferent},
		{0.1, VersionDifferent},
		{0, VersionDifferent},
	} {
		if got := VersionVerdictFor(c.probability); got != c.want {
			t.Errorf("VersionVerdictFor(%v) = %v, want %v", c.probability, got, c.want)
		}
	}
}
