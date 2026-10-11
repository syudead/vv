package domain

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

// 自動タグ付けは、動画のサムネイルと題名・ファイル名・パスを判定モデル（Ollama の Clef）に
// 渡し、既存のタグごとに「当てはまる確率」を聞いて、閾値以上のタグを付ける
// （docs/design-docs/auto-tagging.md）。タグを新しく作ることはしない。

// 自動タグ付けの設定の既定値である。保存値が無いときに使う。
const (
	// DefaultAutoTagThreshold は付けるのに要る確率の既定値である。誤って付くタグを
	// 少なくする側に寄せる。
	DefaultAutoTagThreshold = 0.8
	// DefaultAutoTagTentativeThreshold は仮に付けるのに要る確率の既定値である。これ以上で
	// DefaultAutoTagThreshold 未満のタグは、所有者が確定か却下を選ぶ仮の付与になる。
	DefaultAutoTagTentativeThreshold = 0.5
)

// AutoTagMaxQuestions は判定モデルへの 1 回の問い合わせに載せる質問の数である。候補のタグが
// これより多ければ分けて問い合わせる。Ollama の /v1/systemone は 64 問まで受け付けるが、
// clef-flash は 64 問と画像で入力が約 7,400 トークンになり、メモリ 15GB の CPU 機では判定の
// プロセスがメモリ不足で落ちた。16 問（約 2,000 トークン）なら収まる
// （docs/design-docs/auto-tagging.md）。
const AutoTagMaxQuestions = 16

// ErrInvalidAutoTagSettings は、自動タグ付けの設定の値が規則に合わないことを表す。
var ErrInvalidAutoTagSettings = errors.New("invalid auto-tagging settings")

// ErrAutoTaggingOff は、自動タグ付けの機能が切で、判定を始められないことを表す。
var ErrAutoTaggingOff = errors.New("auto-tagging is turned off")

// AutoTagSettings は自動タグ付けの機能の設定である。判定モデルへの接続先は OllamaSettings が
// 持ち、ここには持たない。
type AutoTagSettings struct {
	// Enabled は自動タグ付けの機能を使うかである。切なら判定を始めず、待ち行列から取り出さず、
	// 判定モデルに接続しない。既定は切。
	Enabled bool
	// AfterImport は、取り込みでシーク用プレビューの段階を終えた動画を自動で判定に回すかである。
	// Enabled が切なら効かない。
	AfterImport bool
	// Threshold はタグを確定した付与として付けるのに要る確率（0 より大きく 1 以下）である。
	Threshold float64
	// TentativeThreshold はタグを仮に付けるのに要る確率（0 より大きく Threshold 以下）である。
	// Threshold と同じなら仮の付与はしない。
	TentativeThreshold float64
}

// DefaultAutoTagSettings は保存値が無いときの設定を返す。機能も自動の判定も切である。
func DefaultAutoTagSettings() AutoTagSettings {
	return AutoTagSettings{
		Threshold:          DefaultAutoTagThreshold,
		TentativeThreshold: DefaultAutoTagTentativeThreshold,
	}
}

// ValidateAutoTagSettings は確率が規則に合うかを確かめる。合わなければ
// ErrInvalidAutoTagSettings を包んだ誤りを返す。
func ValidateAutoTagSettings(s AutoTagSettings) (AutoTagSettings, error) {
	if math.IsNaN(s.Threshold) || s.Threshold <= 0 || s.Threshold > 1 {
		return AutoTagSettings{}, fmt.Errorf("%w: the threshold must be greater than 0 and at most 1", ErrInvalidAutoTagSettings)
	}
	if math.IsNaN(s.TentativeThreshold) || s.TentativeThreshold <= 0 || s.TentativeThreshold > s.Threshold {
		return AutoTagSettings{}, fmt.Errorf("%w: the tentative threshold must be greater than 0 and at most the threshold", ErrInvalidAutoTagSettings)
	}
	return s, nil
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

// AutoTagFrameCount は判定に添えるシーク用プレビューのコマの数である。代表サムネイル
// 1 枚だけでは動画の 1 場面しか見えないので、動画全体から等間隔に選んで添える。コマは
// 長辺 320px 以下で、1 コマあたりの入力はサムネイルより小さい。
const AutoTagFrameCount = 8

// SpreadFrameIndexes は frameCount 個のコマから、全体に等間隔に n 個を選んだ番号を昇順で
// 返す。各区間の中央のコマを選ぶ。frameCount が n 以下ならすべてを返す。
func SpreadFrameIndexes(frameCount, n int) []int {
	if frameCount <= 0 || n <= 0 {
		return nil
	}
	if frameCount <= n {
		indexes := make([]int, frameCount)
		for i := range indexes {
			indexes[i] = i
		}
		return indexes
	}
	indexes := make([]int, n)
	for i := range indexes {
		indexes[i] = (2*i + 1) * frameCount / (2 * n)
	}
	return indexes
}

// AutoTagSubject は判定に渡す動画の手がかりである。
type AutoTagSubject struct {
	// Title は有効な題名（表示名があればそれ）。
	Title string
	// FileName はファイル名（拡張子を含む）。
	FileName string
	// Folder はファイルのあるフォルダのパス。
	Folder string
	// Images は判定に添える JPEG で、代表サムネイル、続いて動画全体から等間隔に選んだ
	// シーク用プレビューのコマの順に並ぶ。無ければ空で、文字だけで判定する。
	Images [][]byte
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
	// 「この動画を表すか」と聞くと、動画の一部にだけ当てはまるタグの確率が下がった
	// （docs/design-docs/auto-tagging.md）。どこか一部に当てはまれば付く聞き方にする。
	fmt.Fprintf(&b, "Does %q appear in or apply to any part of this video?", candidate.Name)
	if len(candidate.Synonyms) > 0 {
		quoted := make([]string, len(candidate.Synonyms))
		for i, synonym := range candidate.Synonyms {
			quoted[i] = fmt.Sprintf("%q", synonym)
		}
		fmt.Fprintf(&b, " The tag is also known as %s.", strings.Join(quoted, ", "))
	}
	b.WriteString(" The images are the thumbnail and frames taken from across the video; a tag applies if it fits any of them, or the title, file name or folder.")
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

// AutoTagSelection は判定の結果、付けるタグの id である。
type AutoTagSelection struct {
	// Confirmed は確率が Threshold 以上で、確定した付与として付けるタグである。
	Confirmed []int64
	// Tentative は確率が TentativeThreshold 以上 Threshold 未満で、仮に付けるタグである。
	Tentative []int64
}

// SelectAutoTags は候補を確率で振り分け、候補の順で返す。答えの無い候補は付けない。
func SelectAutoTags(candidates []AutoTagCandidate, probabilities map[string]float64, settings AutoTagSettings) AutoTagSelection {
	var selection AutoTagSelection
	for _, candidate := range candidates {
		p, ok := probabilities[AutoTagQuestionKey(candidate.ID)]
		switch {
		case !ok || math.IsNaN(p):
		case p >= settings.Threshold:
			selection.Confirmed = append(selection.Confirmed, candidate.ID)
		case p >= settings.TentativeThreshold:
			selection.Tentative = append(selection.Tentative, candidate.ID)
		}
	}
	return selection
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
