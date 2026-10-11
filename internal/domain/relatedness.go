package domain

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// 関連動画の近さ（issue 931）。
//
// 関連動画は、基準の動画との近さの点が高い順に並ぶ。点の付け方は RelatedScorer の
// 実装 1 つに閉じ込め、並べ方・ページ分け・読み出しはその点だけを見る。手がかりや重みを
// 変える、判定の方法を置き換えるときは、RelatedScorer を差し替えるか ContentRelatedness
// を書き換えるだけで済み、保存層・アプリケーション層・API は変わらない。

// RelatedCandidate は近さを測る動画 1 本の手がかりである。
type RelatedCandidate struct {
	VideoID int64
	// Path は代表の所在の絶対パス。ディレクトリとファイル名を手がかりにする。
	Path string
	// Title は有効な題名（表示名があればそれ）。
	Title string
	// AddedAt はライブラリに追加された日時。
	AddedAt time.Time
	// TagIDs は付いているタグの id。確定・仮・フォルダ名から付いたものを区別しない。
	TagIDs []int64
	// Fingerprint は映像の指紋。作られていなければ nil。
	Fingerprint *Fingerprint
}

// RelatedScore は近さの点である。1 点を RelatedScoreUnit で表す整数にし、同点の判定と
// ページの位置（RelatedCursor）が浮動小数の誤差で揺れないようにする。
type RelatedScore int64

// RelatedScoreUnit は 1 点の RelatedScore である。
const RelatedScoreUnit RelatedScore = 1_000_000

// RelatedScorer は基準の動画と他の動画の近さの点を決める部品である。
type RelatedScorer interface {
	// For は基準の動画 self に対する点の付け方を返す。self の手がかりの下ごしらえは
	// ここで 1 回だけ行い、返す関数を候補ごとに呼ぶ。
	For(self RelatedCandidate) func(other RelatedCandidate) RelatedScore
}

// ContentRelatedness は既定の近さの付け方である。下の手がかりの点を足す（満点 120）。
//
//   - 映像の指紋（満点 40）: 時刻を揃えずに比べ、似た場面が共通する割合に比例する。
//   - タグの一致率（30）: 2 本のタグを合わせた種類のうち、両方に付いているものの割合。
//   - 題名・ファイル名の似かた（25）: 数字を除いた語の重なりの割合。
//   - 同じディレクトリの直下（20）: 当たれば満点。
//   - 日付の近さ（5）: 追加日時の差が 1 日以内で満点、そこから半減期 30 日で下がる。
type ContentRelatedness struct{}

// DefaultRelatedScorer は関連動画の並びに使う付け方である。
var DefaultRelatedScorer RelatedScorer = ContentRelatedness{}

const (
	relatedFingerprintPoints = 40
	relatedTagPoints         = 30
	relatedTitlePoints       = 25
	relatedFolderPoints      = 20
	relatedDatePoints        = 5

	// relatedDateFullRange は日付の点が満点のままの差である（同じ日）。
	relatedDateFullRange = 24 * time.Hour
	// relatedDateHalfLife は日付の点が半分になる差の伸びである。1 年で満点の 1/4000 ほどになる。
	relatedDateHalfLife = 30 * 24 * time.Hour
	// relatedSceneMaxDistance は、2 つのコマを似た場面とみなすハミング距離の上限である
	// （63 ビットのうち）。同じ動画の判定と同じ FingerprintMatchMaxDistance を使い、
	// 撮り方や画質の違う似た場面も拾う。
	relatedSceneMaxDistance = FingerprintMatchMaxDistance
)

// For は ContentRelatedness の点の付け方を返す。
func (ContentRelatedness) For(self RelatedCandidate) func(RelatedCandidate) RelatedScore {
	selfDir := filepath.Dir(self.Path)
	selfTags := uniqueIDs(self.TagIDs)
	selfWords := titleWords(self.Title, self.Path)
	selfScenes := sceneHashes(self.Fingerprint)
	return func(other RelatedCandidate) RelatedScore {
		points := relatedFingerprintPoints*sceneOverlap(selfScenes, sceneHashes(other.Fingerprint)) +
			relatedTagPoints*jaccard(selfTags, uniqueIDs(other.TagIDs)) +
			relatedTitlePoints*jaccard(selfWords, titleWords(other.Title, other.Path)) +
			relatedDatePoints*dateCloseness(self.AddedAt, other.AddedAt)
		if filepath.Dir(other.Path) == selfDir {
			points += relatedFolderPoints
		}
		return RelatedScore(math.Round(points * float64(RelatedScoreUnit)))
	}
}

// sceneHashes は指紋のうち比べられるコマ（単色でないもの）のハッシュを返す。指紋が無い、
// 今の版でない、比べられるコマが FingerprintMinComparableFrames 未満なら nil。
func sceneHashes(fingerprint *Fingerprint) []uint64 {
	if fingerprint == nil || fingerprint.Version != FingerprintVersion {
		return nil
	}
	hashes := make([]uint64, 0, len(fingerprint.Frames))
	for _, frame := range fingerprint.Frames {
		if !frame.Flat {
			hashes = append(hashes, frame.Hash)
		}
	}
	if len(hashes) < FingerprintMinComparableFrames {
		return nil
	}
	return hashes
}

// sceneOverlap は、2 本のコマのうち相手のどこかに似た場面があるものの割合を返す（0〜1）。
// コマの時刻は揃えないので、切り出しや並べ替えをした動画も近くなる。どちらかが nil なら 0。
func sceneOverlap(a, b []uint64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	matchedB := make([]bool, len(b))
	matched := 0
	for _, ha := range a {
		hit := false
		for j, hb := range b {
			if bits.OnesCount64(ha^hb) <= relatedSceneMaxDistance {
				hit = true
				matchedB[j] = true
			}
		}
		if hit {
			matched++
		}
	}
	for _, hit := range matchedB {
		if hit {
			matched++
		}
	}
	return float64(matched) / float64(len(a)+len(b))
}

// jaccard は 2 つの集合を合わせた要素のうち両方にあるものの割合を返す。どちらかが空なら 0。
func jaccard[T comparable](a, b map[T]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for item := range b {
		if _, ok := a[item]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}

func uniqueIDs(ids []int64) map[int64]struct{} {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// titleWords は題名とファイル名（拡張子を除く）の語の集合を返す。照合の形（FoldForMatch）に
// してから、文字の続きを語にし、数字・記号・空白は区切りとして捨てる。連番や日付は語に
// 残らない。かなと漢字の続きは区切りが無いので 2 文字ずつ（1 文字だけならその 1 文字）を
// 語にする。それ以外の文字の 1 文字だけの語（`1080p` の `p` など）は捨てる。
func titleWords(title, path string) map[string]struct{} {
	words := map[string]struct{}{}
	base := filepath.Base(path)
	for _, text := range []string{title, strings.TrimSuffix(base, filepath.Ext(base))} {
		addWords(words, FoldForMatch(text))
	}
	return words
}

func addWords(words map[string]struct{}, text string) {
	var run []rune
	runCJK := false
	flush := func() {
		switch {
		case runCJK && len(run) == 1:
			words[string(run)] = struct{}{}
		case runCJK:
			for i := 0; i+1 < len(run); i++ {
				words[string(run[i:i+2])] = struct{}{}
			}
		case len(run) > 1:
			words[string(run)] = struct{}{}
		}
		run = run[:0]
	}
	for _, r := range text {
		if !unicode.IsLetter(r) && !unicode.IsMark(r) {
			flush()
			continue
		}
		cjk := isCJK(r)
		if len(run) > 0 && cjk != runCJK && !unicode.IsMark(r) {
			flush()
		}
		if len(run) == 0 {
			runCJK = cjk
		}
		run = append(run, r)
	}
	flush()
}

// isCJK は語の区切りを書かない文字（漢字・かな・ハングル）かを返す。長音符 `ー` はかなの続きに含める。
func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) || r == 'ー'
}

// dateCloseness は追加日時の差の近さを返す（0〜1）。
func dateCloseness(a, b time.Time) float64 {
	gap := a.Sub(b)
	if gap < 0 {
		gap = -gap
	}
	if gap <= relatedDateFullRange {
		return 1
	}
	return math.Exp2(-float64(gap-relatedDateFullRange) / float64(relatedDateHalfLife))
}

// RankedRelated は関連動画の並びの 1 本である。
type RankedRelated struct {
	Score RelatedScore
	// AddedAt は追加日時の Unix 秒。同点のときの並びに使う。
	AddedAt int64
	VideoID int64
}

// Cursor はこの動画の直後から続きを読むページの位置を返す。
func (r RankedRelated) Cursor() RelatedCursor {
	return RelatedCursor(r)
}

// RankRelated は候補を基準の動画 self との近さの点が高い順に並べる。同点なら追加日時の
// 新しい順、それも同じなら id の大きい順で、全順序になる。self 自身と、同じ id の候補の
// 2 回目以降は除く。
func RankRelated(scorer RelatedScorer, self RelatedCandidate, candidates []RelatedCandidate) []RankedRelated {
	score := scorer.For(self)
	seen := map[int64]struct{}{self.VideoID: {}}
	ranked := make([]RankedRelated, 0, len(candidates))
	for _, candidate := range candidates {
		if _, dup := seen[candidate.VideoID]; dup {
			continue
		}
		seen[candidate.VideoID] = struct{}{}
		ranked = append(ranked, RankedRelated{
			VideoID: candidate.VideoID,
			Score:   score(candidate),
			AddedAt: candidate.AddedAt.Unix(),
		})
	}
	slices.SortFunc(ranked, compareRanked)
	return ranked
}

// compareRanked は並びで a が先なら負を返す。
func compareRanked(a, b RankedRelated) int {
	if order := cmp.Compare(b.Score, a.Score); order != 0 {
		return order
	}
	if order := cmp.Compare(b.AddedAt, a.AddedAt); order != 0 {
		return order
	}
	return cmp.Compare(b.VideoID, a.VideoID)
}

// RelatedCursor は関連動画のページの位置で、その直前に返した動画の並びの値である。
// 続きのページは並びでこの値より後の動画から始める。読み込みの間に動画が足されたり
// 消えたり公開が変わったりしても、点は 2 本の手がかりだけで決まるので、すでに返した
// 動画が続きのページに再び出ることはない。
type RelatedCursor struct {
	Score   RelatedScore
	AddedAt int64
	VideoID int64
}

// ErrInvalidRelatedCursor はページの位置の文字列が読めないことを表す。
var ErrInvalidRelatedCursor = errors.New("the related videos cursor is malformed")

// String は位置を API に載せる文字列にする（ParseRelatedCursor が読む）。
func (c RelatedCursor) String() string {
	return fmt.Sprintf("%d.%d.%d", c.Score, c.AddedAt, c.VideoID)
}

// ParseRelatedCursor は String の形を読む。
func ParseRelatedCursor(text string) (RelatedCursor, error) {
	parts := strings.Split(text, ".")
	if len(parts) != 3 {
		return RelatedCursor{}, ErrInvalidRelatedCursor
	}
	var values [3]int64
	for i, part := range parts {
		value, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return RelatedCursor{}, ErrInvalidRelatedCursor
		}
		values[i] = value
	}
	if values[2] < 1 {
		return RelatedCursor{}, ErrInvalidRelatedCursor
	}
	return RelatedCursor{Score: RelatedScore(values[0]), AddedAt: values[1], VideoID: values[2]}, nil
}

const (
	// DefaultRelatedPageSize は関連動画を 1 回に返す本数の既定である。
	DefaultRelatedPageSize = 30
	// MaxRelatedPageSize は関連動画を 1 回に返す本数の上限である。
	MaxRelatedPageSize = 100
)

// PageRelated は並びのうち after より後の最大 limit 本を返す。続きがあれば、返した最後の
// 動画の位置も返す（無ければ nil）。after が nil なら先頭から返す。
func PageRelated(ranked []RankedRelated, after *RelatedCursor, limit int) ([]RankedRelated, *RelatedCursor) {
	start := 0
	if after != nil {
		start, _ = slices.BinarySearchFunc(ranked, RankedRelated(*after), compareRanked)
		if start < len(ranked) && ranked[start].Cursor() == *after {
			start++
		}
	}
	end := min(start+max(limit, 0), len(ranked))
	page := ranked[start:end]
	if end >= len(ranked) || len(page) == 0 {
		return page, nil
	}
	next := page[len(page)-1].Cursor()
	return page, &next
}
