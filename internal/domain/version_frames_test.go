package domain

import (
	"reflect"
	"slices"
	"testing"
)

// distances[i] sets that many low bits in b relative to a's zero hash at one-second intervals.
func frameRuleFingerprints(distances []int) (Fingerprint, Fingerprint) {
	a := Fingerprint{Version: FingerprintVersion, IntervalMs: 1000}
	b := Fingerprint{Version: FingerprintVersion, IntervalMs: 1000}
	for _, distance := range distances {
		a.Frames = append(a.Frames, FrameHash{})
		b.Frames = append(b.Frames, FrameHash{Hash: 1<<uint(distance) - 1})
	}
	return a, b
}

func shownFramesA(rule VersionFrameRule) []int {
	var indexes []int
	for _, pair := range rule.Shown {
		indexes = append(indexes, pair.A)
	}
	return indexes
}

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

func TestJudgeVersionFramesOneIsolatedDifference(t *testing.T) {
	distances := make([]int, 20)
	distances[9] = 30
	a, b := frameRuleFingerprints(distances)
	rule := JudgeVersionFrames(a, b)
	if rule.Outcome != VersionFramesSame || rule.Differing != 1 {
		t.Fatalf("rule = %+v", rule)
	}
}

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

// Both argument orders must detect the run even when the finer timeline advances two frame indices per pair.
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

func TestJudgeVersionFramesIncomparable(t *testing.T) {
	a, b := frameRuleFingerprints([]int{0, 0, 0, 0})
	for i := 1; i < 4; i++ {
		a.Frames[i].Flat = true
	}
	if rule := JudgeVersionFrames(a, b); rule.Outcome != VersionFramesIncomparable {
		t.Fatalf("rule = %+v", rule)
	}
}

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

func TestJudgeVersionFramesRecordsTheLongestRun(t *testing.T) {
	distances := make([]int, 20)
	distances[3] = 30
	for i := 8; i < 12; i++ {
		distances[i] = 33
	}
	a, b := frameRuleFingerprints(distances)
	rule := JudgeVersionFrames(a, b)
	if rule.LongestRun != 4 || rule.Run[0].A != 8 || rule.Run[1].A != 11 {
		t.Fatalf("最長の連続 = %d（%+v）, want 4（8〜11）", rule.LongestRun, rule.Run)
	}
}

func TestNewVersionJudgeEvidenceTimesEachSideOnItsOwnInterval(t *testing.T) {
	a := Fingerprint{Version: FingerprintVersion, IntervalMs: 1000}
	b := Fingerprint{Version: FingerprintVersion + 1, IntervalMs: 500}
	rule := VersionFrameRule{
		Outcome: VersionFramesDiffer, Compared: 10, Differing: 3, LongestRun: 3,
		Run:   [2]FramePair{{A: 2, B: 4}, {A: 4, B: 8}},
		Shown: []FramePair{{A: 3, B: 6, Distance: 20}},
	}
	got := NewVersionJudgeEvidence(a, b, rule)
	want := &VersionJudgeEvidence{
		Rule: "frames_differ", Compared: 10, Differing: 3, LongestRun: 3, RunStartMs: 2000, RunEndMs: 5000,
		VersionMismatch: true, Frames: []VersionEvidenceFrame{{A: 3, B: 6, AMs: 3000, BMs: 3000, Distance: 20}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("根拠 = %+v, want %+v", got, want)
	}
	decoded, err := DecodeVersionJudgeEvidence(got.Encode())
	if err != nil || !reflect.DeepEqual(decoded, want) {
		t.Errorf("往復 = %+v, %v", decoded, err)
	}
}
