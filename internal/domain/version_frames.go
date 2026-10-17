package domain

// 候補の 1 組を、保存した 2 本の指紋のコマごとの距離でまず判定する規則と、モデルに聞く問い。コマがすべて一致すればモデルに聞かずに「同じ動画」、
// コマの連なりが食い違えば、その区間のコマを左右に並べてモデルに聞く。

const (
	// VersionJudgeQuestionKey はモデルに聞く問いの答えを引き当てる名前である。
	VersionJudgeQuestionKey = "same_picture"
	// VersionJudgeQuestion はモデルに聞く問いの文である。
	VersionJudgeQuestion = "Each image puts two frames side by side, taken at the same moment from two video files. " +
		"In every image, do the left half and the right half show the same picture " +
		"(ignoring differences in sharpness, resolution or compression)?"
	// VersionSameProbability 以上の答えは「同じ動画」である。
	VersionSameProbability = 0.8
	// VersionDifferentProbability 以下の答えは「違う動画」である。
	VersionDifferentProbability = 0.2
	// VersionJudgeMaxImages はモデルに見せる画像の数の上限である。
	VersionJudgeMaxImages = 4
	// versionFramesDifferingDivisor は「同じ動画」とみなす食い違いの割合の上限（1/20 = 5%）の分母である。
	versionFramesDifferingDivisor = 20
)

// VersionFrameOutcome はコマごとの規則の結果である。
type VersionFrameOutcome int

const (
	// VersionFramesSame はコマがすべて（食い違いが 5% 以下で連続せず）一致し、モデルに聞かずに同じ動画とする。
	VersionFramesSame VersionFrameOutcome = iota + 1
	// VersionFramesDiffer はコマが食い違い、その区間のコマをモデルに聞く。
	VersionFramesDiffer
	// VersionFramesIncomparable は 2 本の指紋を比べられない（版が違う、組が少ない）。
	VersionFramesIncomparable
)

// String はログに出す名前である。
func (o VersionFrameOutcome) String() string {
	switch o {
	case VersionFramesSame:
		return "frames_match"
	case VersionFramesDiffer:
		return "frames_differ"
	case VersionFramesIncomparable:
		return "incomparable"
	default:
		return "unknown"
	}
}

// VersionFrameRule はコマごとの規則を 1 組に当てた結果である。Compared は比べたコマの組の数、
// Differing はそのうち食い違った数。Shown は VersionFramesDiffer のときモデルに見せるコマの組で、
// 最大 VersionJudgeMaxImages 個、時刻の順。
type VersionFrameRule struct {
	Outcome   VersionFrameOutcome
	Compared  int
	Differing int
	Shown     []FramePair
}

// JudgeVersionFrames は 2 本の指紋をコマごとに比べる。コマは
// ComparableFramePairs で時刻で組にし（単色のコマは飛ばす）、距離が FingerprintMatchMaxDistance を
// 超える組を食い違いとする。連なりは時刻で隣り合う組の食い違いで、飛ばしたコマを挟めば切れる
// （framePairsAdjacent）。
// 食い違いが 5% 以下で 2 つ続かなければ VersionFramesSame。そうでなければ
// VersionFramesDiffer で、食い違いの最も長い連なり（同じ長さなら先のもの）からコマを選ぶ。連なりが
// 無く（どの食い違いも 1 つだけで）5% を超えるときは、食い違ったコマ全体から選ぶ。選ぶコマが
// VersionJudgeMaxImages より多ければ、その中に等間隔に散らす。
func JudgeVersionFrames(a, b Fingerprint) VersionFrameRule {
	pairs, ok := ComparableFramePairs(a, b)
	if !ok {
		return VersionFrameRule{Outcome: VersionFramesIncomparable}
	}
	rule := VersionFrameRule{Compared: len(pairs)}
	var differing, longest, current []FramePair
	for _, pair := range pairs {
		if pair.Distance <= FingerprintMatchMaxDistance {
			current = nil
			continue
		}
		differing = append(differing, pair)
		if len(current) > 0 && !framePairsAdjacent(a, b, current[len(current)-1], pair) {
			// 間のコマが単色か相手が無くて飛ばされた。時刻で隣り合わない食い違いは連なりではない。
			current = nil
		}
		current = append(current, pair)
		if len(current) > len(longest) {
			longest = current
		}
	}
	rule.Differing = len(differing)
	if len(longest) < 2 && rule.Differing*versionFramesDifferingDivisor <= rule.Compared {
		rule.Outcome = VersionFramesSame
		return rule
	}
	rule.Outcome = VersionFramesDiffer
	shown := longest
	if len(shown) < 2 {
		shown = differing
	}
	for _, index := range SpreadFrameIndexes(len(shown), VersionJudgeMaxImages) {
		rule.Shown = append(rule.Shown, shown[index])
	}
	return rule
}

// framePairsAdjacent は組 p の次の組 q が時刻で隣り合うかを返す。どちらの指紋の側でも、
// 2 つの組のコマの先頭の隔たりが粗い方の間隔以下なら隣り合う。a と b の両方の側を同じ規則で
// 見るので、間隔の違う 2 本の指紋でも引数の順で結果が変わらない。間のコマが飛ばされていれば、
// どちらかの側の隔たりが粗い方の間隔を超える。
func framePairsAdjacent(a, b Fingerprint, p, q FramePair) bool {
	step := max(a.IntervalMs, b.IntervalMs)
	return int64(q.A-p.A)*a.IntervalMs <= step && int64(q.B-p.B)*b.IntervalMs <= step
}

// VersionVerdictFor はモデルの答えの確率を判定の結果の種類にする。
// VersionSameProbability 以上は同じ、VersionDifferentProbability 以下は違う、間は決められない。
func VersionVerdictFor(probability float64) VersionVerdict {
	switch {
	case probability >= VersionSameProbability:
		return VersionSame
	case probability <= VersionDifferentProbability:
		return VersionDifferent
	default:
		return VersionUndecided
	}
}
