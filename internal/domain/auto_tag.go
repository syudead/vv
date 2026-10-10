package domain

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"
)

// 自動タグ付けは、動画のサムネイルと題名・ファイル名・パスを判定モデル（Ollama の Clef）に
// 渡し、既存のタグごとに「当てはまる確率」を聞いて、閾値以上のタグを付ける
// （docs/design-docs/auto-tagging.md）。タグを新しく作ることはしない。

// 自動タグ付けの設定の既定値である。保存値が無いときに使う。
const (
	// DefaultAutoTagEndpoint は Ollama が既定で待ち受ける場所である。
	DefaultAutoTagEndpoint = "http://127.0.0.1:11434"
	// DefaultAutoTagModel は速さを優先した Clef の小さい版である。
	DefaultAutoTagModel = "clef-flash"
	// DefaultAutoTagThreshold は付けるのに要る確率の既定値である。誤って付くタグを
	// 少なくする側に寄せる。
	DefaultAutoTagThreshold = 0.8
)

// AutoTagMaxQuestions は判定モデルへの 1 回の問い合わせに載せられる質問の上限である
// （Ollama の /v1/systemone の制限）。候補のタグがこれより多ければ分けて問い合わせる。
const AutoTagMaxQuestions = 64

// autoTagMaxModelLength は模型名の長さの上限である。
const autoTagMaxModelLength = 200

// ErrInvalidAutoTagSettings は、自動タグ付けの設定の値が規則に合わないことを表す。
var ErrInvalidAutoTagSettings = errors.New("invalid auto-tagging settings")

// AutoTagSettings は自動タグ付けの設定である。
type AutoTagSettings struct {
	// Enabled は、取り込みで代表サムネイルの段階を終えた動画を自動で判定に回すかである。
	// 切でも、所有者が画面から始める判定は動く。
	Enabled bool
	// Endpoint は Ollama の基底 URL（http か https）である。
	Endpoint string
	// Model は Ollama の模型名（clef か clef-flash）である。
	Model string
	// Threshold はタグを付けるのに要る確率（0 より大きく 1 以下）である。
	Threshold float64
}

// DefaultAutoTagSettings は保存値が無いときの設定を返す。自動の判定は切である。
func DefaultAutoTagSettings() AutoTagSettings {
	return AutoTagSettings{
		Endpoint:  DefaultAutoTagEndpoint,
		Model:     DefaultAutoTagModel,
		Threshold: DefaultAutoTagThreshold,
	}
}

// NormalizeAutoTagSettings は前後の空白と Endpoint の末尾の / を落とし、規則に合うかを
// 確かめる。合わなければ ErrInvalidAutoTagSettings を包んだ誤りを返す。
func NormalizeAutoTagSettings(s AutoTagSettings) (AutoTagSettings, error) {
	s.Endpoint = strings.TrimRight(strings.TrimSpace(s.Endpoint), "/")
	s.Model = strings.TrimSpace(s.Model)
	parsed, err := url.Parse(s.Endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return AutoTagSettings{}, fmt.Errorf("%w: the endpoint must be an http or https URL", ErrInvalidAutoTagSettings)
	}
	if s.Model == "" || len(s.Model) > autoTagMaxModelLength || strings.ContainsFunc(s.Model, isControlOrSpace) {
		return AutoTagSettings{}, fmt.Errorf("%w: the model must be a model name", ErrInvalidAutoTagSettings)
	}
	if math.IsNaN(s.Threshold) || s.Threshold <= 0 || s.Threshold > 1 {
		return AutoTagSettings{}, fmt.Errorf("%w: the threshold must be greater than 0 and at most 1", ErrInvalidAutoTagSettings)
	}
	return s, nil
}

func isControlOrSpace(r rune) bool {
	return r <= ' ' || r == 0x7f
}

// AutoTagScope は所有者が始める判定の対象の選び方である。
type AutoTagScope string

const (
	// AutoTagScopeMissing は、まだ判定を終えていない動画（判定していない・失敗した）を回す。
	AutoTagScopeMissing AutoTagScope = "missing"
	// AutoTagScopeAll は、判定を終えた動画も含めてすべて回し直す。
	AutoTagScopeAll AutoTagScope = "all"
)

// Valid は値が既知の選び方かを返す。
func (s AutoTagScope) Valid() bool {
	return s == AutoTagScopeMissing || s == AutoTagScopeAll
}

// AutoTagQueueMode は待ち行列へ積むときに、既にある行をどう扱うかである。
type AutoTagQueueMode int

const (
	// AutoTagQueueIfNew は行が無い動画だけを積む。取り込みのあとの自動の判定が使い、
	// 所有者が外したタグを付け直さない。
	AutoTagQueueIfNew AutoTagQueueMode = iota
	// AutoTagQueueUnlessDone は行が無いか失敗した動画を積む。
	AutoTagQueueUnlessDone
	// AutoTagQueueAgain は判定中のもの以外をすべて積み直す。
	AutoTagQueueAgain
)

// QueueMode は所有者が選んだ範囲を積み方へ写す。
func (s AutoTagScope) QueueMode() AutoTagQueueMode {
	if s == AutoTagScopeAll {
		return AutoTagQueueAgain
	}
	return AutoTagQueueUnlessDone
}

// AutoTagJob は待ち行列から専有した判定 1 件である。
type AutoTagJob struct {
	ContentKey string
}

// AutoTagCounts は待ち行列の状態ごとの件数と、直近の失敗の理由である。
type AutoTagCounts struct {
	Queued  int
	Running int
	Done    int
	Failed  int
	// LastError は直近に失敗した判定の理由（英語）。無ければ空である。
	LastError string
}

// AutoTagStatus は設定と待ち行列の今の状態である。
type AutoTagStatus struct {
	Settings AutoTagSettings
	Counts   AutoTagCounts
}

// AutoTagCandidate は判定に回す既存のタグ 1 つである。仮タグ（tentative）は含めない。
type AutoTagCandidate struct {
	ID   int64
	Name string
	// Synonyms はシノニム（名前の順）。判定モデルへの説明に添える。
	Synonyms []string
}

// SortAutoTagCandidates は候補を名前の自然順に並べる（SortTagRefs と同じ規則）。
func SortAutoTagCandidates(candidates []AutoTagCandidate) {
	slices.SortStableFunc(candidates, func(a, b AutoTagCandidate) int {
		return compareTagRefs(TagRef{ID: a.ID, Name: a.Name}, TagRef{ID: b.ID, Name: b.Name})
	})
}

// AutoTagSubject は判定に渡す動画の手がかりである。
type AutoTagSubject struct {
	// Title は有効な題名（表示名があればそれ）。
	Title string
	// FileName はファイル名（拡張子を含む）。
	FileName string
	// Folder はファイルのあるフォルダのパス。
	Folder string
	// Thumbnail は代表サムネイルの JPEG。無ければ nil で、文字だけで判定する。
	Thumbnail []byte
}

// NewAutoTagSubject は動画の有効な題名と代表の所在のパスから手がかりを作る。パスの区切りは
// / と \ のどちらも受け付ける（Windows 版がある）。
func NewAutoTagSubject(title, path string) AutoTagSubject {
	folder, name := "", path
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		folder, name = path[:i], path[i+1:]
	}
	return AutoTagSubject{Title: title, FileName: name, Folder: folder}
}

// AutoTagQuestion は判定モデルへの、はい／いいえの質問 1 つである。
type AutoTagQuestion struct {
	// Key は答えを引き当てる名前である。
	Key string
	// Instructions は質問の文（英語）である。
	Instructions string
}

// AutoTagQuestionKey はタグの id から質問の名前を作る。
func AutoTagQuestionKey(tagID int64) string {
	return fmt.Sprintf("tag_%d", tagID)
}

// AutoTagQuestionFor はタグ 1 つを質問の文にする。シノニムがあれば別名として添える。
func AutoTagQuestionFor(candidate AutoTagCandidate) AutoTagQuestion {
	var b strings.Builder
	fmt.Fprintf(&b, "Does the tag %q describe this video?", candidate.Name)
	if len(candidate.Synonyms) > 0 {
		quoted := make([]string, len(candidate.Synonyms))
		for i, synonym := range candidate.Synonyms {
			quoted[i] = fmt.Sprintf("%q", synonym)
		}
		fmt.Fprintf(&b, " The tag is also known as %s.", strings.Join(quoted, ", "))
	}
	b.WriteString(" Judge from the thumbnail image if one is attached, and from the title, file name and folder.")
	return AutoTagQuestion{Key: AutoTagQuestionKey(candidate.ID), Instructions: b.String()}
}

// ChunkAutoTagCandidates は候補を 1 回の問い合わせに載る大きさ（AutoTagMaxQuestions）に分ける。
func ChunkAutoTagCandidates(candidates []AutoTagCandidate) [][]AutoTagCandidate {
	var chunks [][]AutoTagCandidate
	for start := 0; start < len(candidates); start += AutoTagMaxQuestions {
		end := min(start+AutoTagMaxQuestions, len(candidates))
		chunks = append(chunks, candidates[start:end])
	}
	return chunks
}

// SelectAutoTags は候補のうち、確率が閾値以上のタグの id を候補の順で返す。答えの無い
// 候補は付けない。
func SelectAutoTags(candidates []AutoTagCandidate, probabilities map[string]float64, threshold float64) []int64 {
	var selected []int64
	for _, candidate := range candidates {
		p, ok := probabilities[AutoTagQuestionKey(candidate.ID)]
		if ok && !math.IsNaN(p) && p >= threshold {
			selected = append(selected, candidate.ID)
		}
	}
	return selected
}

// AutoTagApplied は判定で動画にタグが付いた（付かなかったことも含め判定が終わった）ことを
// 表す。画面への知らせに写し、一覧と動画ページにタグを読み直させる。
type AutoTagApplied struct {
	VideoIDs []int64
}

func (AutoTagApplied) event() {}

// AutoTagRequest は判定モデルへの問い合わせ 1 回分である。
type AutoTagRequest struct {
	// Endpoint と Model は問い合わせ先（設定の値）である。
	Endpoint string
	Model    string
	Subject  AutoTagSubject
	// Questions は 1 から AutoTagMaxQuestions 個の、はい／いいえの質問である。
	Questions []AutoTagQuestion
}
