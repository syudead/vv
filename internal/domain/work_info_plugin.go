package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"time"
	"unicode/utf8"
)

// 作品情報プラグイン（WebAssembly モジュール）の値と規則である。モジュールの検査と実行は
// internal/wasmplugin が行い、ここには I/O を伴わない型と規則だけを置く
// （specs/049-work-info-plugins/contracts/plugin-contract.md、data-model.md、research.md R-3・R-4）。

// WorkInfoPluginContract は vv が動かせるプラグインの取り決めの版である。
const WorkInfoPluginContract = 1

// 作品情報プラグインの上限である（research.md R-4、plugin-contract.md）。
const (
	// WorkInfoPluginModuleMaxBytes はモジュールの大きさの上限である。
	WorkInfoPluginModuleMaxBytes = 16 << 20
	// WorkInfoPluginMemoryLimitPages は 1 回の実行の線形メモリの上限（64 KiB のページ数）である。
	WorkInfoPluginMemoryLimitPages = 2048
	// WorkInfoPluginTableMaxElements は各テーブルの要素数の上限である。最大値を宣言しないテーブルには vv がこれを最大値として書き足す。
	WorkInfoPluginTableMaxElements = 100_000
	// WorkInfoPluginRunTimeout は 1 回の実行の時間の上限で、HTTP の時間も含む。
	WorkInfoPluginRunTimeout = 30 * time.Second
	// WorkInfoPluginOutputMaxBytes は標準出力の上限である。超えた実行は止める。
	WorkInfoPluginOutputMaxBytes = 1 << 20
	// WorkInfoPluginStderrKeptBytes は標準エラーのうちサーバーログに残す大きさである。
	WorkInfoPluginStderrKeptBytes = 64 << 10
	// WorkInfoPluginBusyWait は同時に動く実行が 1 本のとき、2 本目が空きを待つ時間である。
	WorkInfoPluginBusyWait = 10 * time.Second

	// WorkInfoPluginMaxSettings はマニフェストが宣言できる設定の数の上限である。
	WorkInfoPluginMaxSettings = 10
	// WorkInfoPluginMessageMaxRunes は所有者に見せるプラグインの誤りの文の長さである。
	WorkInfoPluginMessageMaxRunes = 300
	// WorkInfoPluginMaxCandidates は検索の結果のうち vv が残す候補の数である。
	WorkInfoPluginMaxCandidates = 50
	// WorkInfoPluginMaxTags は取得の結果のうち vv が残すタグの数である。
	WorkInfoPluginMaxTags = 100
)

// ErrWorkInfoPluginTooLarge はモジュールが WorkInfoPluginModuleMaxBytes を超えることを表す。
var ErrWorkInfoPluginTooLarge = errors.New("work-info plugin module is larger than 16 MiB")

// WorkInfoPluginProblem はプラグインを承認できない理由である（data-model.md の work_info_plugins）。
// 空は問題が無いことを表す。
type WorkInfoPluginProblem string

const (
	// WorkInfoPluginNotWasm はバイト列が WebAssembly モジュールとしてコンパイルできないことを表す。
	WorkInfoPluginNotWasm WorkInfoPluginProblem = "not_wasm"
	// WorkInfoPluginManifestMissing は vv.plugin のカスタムセクションが無いことを表す。
	WorkInfoPluginManifestMissing WorkInfoPluginProblem = "manifest_missing"
	// WorkInfoPluginManifestInvalid はセクションがマニフェストの JSON でないか、項目が規則に反することを表す。
	WorkInfoPluginManifestInvalid WorkInfoPluginProblem = "manifest_invalid"
	// WorkInfoPluginContractUnsupported は contract がこの vv の動かせる版でないことを表す。
	WorkInfoPluginContractUnsupported WorkInfoPluginProblem = "contract_unsupported"
	// WorkInfoPluginImportUnsupported は wasi_snapshot_preview1 と vv の関数の外を import することを表す。
	WorkInfoPluginImportUnsupported WorkInfoPluginProblem = "import_unsupported"
	// WorkInfoPluginExportInvalid は _start（引数も戻り値も無い関数）か memory（メモリ）を export しないことを表す。
	WorkInfoPluginExportInvalid WorkInfoPluginProblem = "export_invalid"
	// WorkInfoPluginTableTooLarge はテーブルの最小値か、宣言した最大値が WorkInfoPluginTableMaxElements を超えることを表す。
	WorkInfoPluginTableTooLarge WorkInfoPluginProblem = "table_too_large"
)

// WorkInfoPluginState は所有者に見せるプラグインの状態である（data-model.md の State transitions）。
type WorkInfoPluginState string

const (
	// WorkInfoPluginApproved は所有者が今のモジュールを承認した状態である。
	WorkInfoPluginApproved WorkInfoPluginState = "approved"
	// WorkInfoPluginUnapproved は使えるが、今のモジュールが承認されていない状態である。
	WorkInfoPluginUnapproved WorkInfoPluginState = "unapproved"
	// WorkInfoPluginInvalid は問題があり、承認できない状態である。
	WorkInfoPluginInvalid WorkInfoPluginState = "invalid"
)

// WorkInfoPluginStateOf はプラグインの状態を決める。承認済みは、問題が無く、承認したダイジェストが
// 今のモジュールのダイジェストと等しいときだけである（data-model.md の Rules）。
func WorkInfoPluginStateOf(problem WorkInfoPluginProblem, digest, approvedDigest string) WorkInfoPluginState {
	switch {
	case problem != "":
		return WorkInfoPluginInvalid
	case approvedDigest != "" && approvedDigest == digest:
		return WorkInfoPluginApproved
	default:
		return WorkInfoPluginUnapproved
	}
}

// WorkInfoOperation はプラグインの操作である。
type WorkInfoOperation string

const (
	// WorkInfoSearch は語で作品を探し、候補を返す操作である。
	WorkInfoSearch WorkInfoOperation = "search"
	// WorkInfoFetch は作品の URL から作品情報を取る操作である。
	WorkInfoFetch WorkInfoOperation = "fetch"
)

// WorkInfoPluginSetting はマニフェストが宣言する設定の 1 つである。値は所有者が入力する。
type WorkInfoPluginSetting struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
}

// WorkInfoPluginManifest はモジュールの vv.plugin セクションのマニフェストである
// （plugin-contract.md の Manifest）。
type WorkInfoPluginManifest struct {
	Contract    int
	Name        string
	Version     string
	Description string
	Operations  []WorkInfoOperation
	Settings    []WorkInfoPluginSetting
}

// Supports はマニフェストが操作 op を宣言するかを返す。
func (m WorkInfoPluginManifest) Supports(op WorkInfoOperation) bool {
	return slices.Contains(m.Operations, op)
}

// manifestJSON は vv.plugin セクションの JSON の形である。知らない項目は無視する。
type manifestJSON struct {
	Contract    json.RawMessage         `json:"contract"`
	Name        string                  `json:"name"`
	Version     string                  `json:"version"`
	Description string                  `json:"description"`
	Operations  []WorkInfoOperation     `json:"operations"`
	Settings    []WorkInfoPluginSetting `json:"settings"`
}

// ParseWorkInfoPluginManifest は vv.plugin セクションの中身を読み、規則を確かめる。
// 読めなければ manifest_invalid、contract が 1 でなければ contract_unsupported を返す。
// contract は他の項目より先に確かめる。後の版は他の項目の形を変えうるからである。
func ParseWorkInfoPluginManifest(data []byte) (WorkInfoPluginManifest, WorkInfoPluginProblem) {
	var raw manifestJSON
	if err := json.Unmarshal(data, &raw); err != nil || !isJSONObject(data) {
		return WorkInfoPluginManifest{}, WorkInfoPluginManifestInvalid
	}
	var contract int
	if len(raw.Contract) == 0 || json.Unmarshal(raw.Contract, &contract) != nil {
		return WorkInfoPluginManifest{}, WorkInfoPluginManifestInvalid
	}
	if contract != WorkInfoPluginContract {
		return WorkInfoPluginManifest{}, WorkInfoPluginContractUnsupported
	}
	m := WorkInfoPluginManifest{
		Contract:    contract,
		Name:        raw.Name,
		Version:     raw.Version,
		Description: raw.Description,
		Operations:  raw.Operations,
		Settings:    raw.Settings,
	}
	if !validManifest(m) {
		return WorkInfoPluginManifest{}, WorkInfoPluginManifestInvalid
	}
	return m, ""
}

func isJSONObject(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func validManifest(m WorkInfoPluginManifest) bool {
	return ValidWorkInfoPluginName(m.Name) &&
		runeCountBetween(m.Version, 1, 32) &&
		utf8.RuneCountInString(m.Description) <= 500 && utf8.ValidString(m.Description) &&
		validOperations(m.Operations) &&
		validSettings(m.Settings)
}

// ValidWorkInfoPluginName はプラグインの名前が a-z・0-9・-・. の 1〜64 文字かを返す。
func ValidWorkInfoPluginName(name string) bool {
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

func runeCountBetween(s string, lo, hi int) bool {
	n := utf8.RuneCountInString(s)
	return utf8.ValidString(s) && n >= lo && n <= hi
}

func validOperations(ops []WorkInfoOperation) bool {
	if len(ops) == 0 {
		return false
	}
	seen := map[WorkInfoOperation]bool{}
	for _, op := range ops {
		if (op != WorkInfoSearch && op != WorkInfoFetch) || seen[op] {
			return false
		}
		seen[op] = true
	}
	return true
}

func validSettings(settings []WorkInfoPluginSetting) bool {
	if len(settings) > WorkInfoPluginMaxSettings {
		return false
	}
	seen := map[string]bool{}
	for _, s := range settings {
		if !validSettingKey(s.Key) || seen[s.Key] || !runeCountBetween(s.Label, 1, 60) {
			return false
		}
		seen[s.Key] = true
	}
	return true
}

// validSettingKey は設定の key が英字で始まる A-Z・a-z・0-9・_ の 1〜32 文字かを返す。
func validSettingKey(key string) bool {
	if len(key) < 1 || len(key) > 32 || !isASCIILetter(key[0]) {
		return false
	}
	for i := 1; i < len(key); i++ {
		c := key[i]
		if !isASCIILetter(c) && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// WorkInfoRunRequest は 1 回の実行でプラグインの標準入力に書く要求である（plugin-contract.md の Request）。
// 文字数と URL の確かめは要求を作る側（internal/app）が行う。
type WorkInfoRunRequest struct {
	Operation WorkInfoOperation
	// Query は search の語である。
	Query string
	// URL は fetch の作品の URL である。
	URL string
	// Settings は所有者が入力した、このプラグインの設定の値である。
	Settings map[string]string
}

// MarshalJSON は取り決めの版を添えた要求の JSON を返す。
func (r WorkInfoRunRequest) MarshalJSON() ([]byte, error) {
	settings := r.Settings
	if settings == nil {
		settings = map[string]string{}
	}
	body := struct {
		Contract  int               `json:"contract"`
		Operation WorkInfoOperation `json:"operation"`
		Query     string            `json:"query,omitempty"`
		URL       string            `json:"url,omitempty"`
		Settings  map[string]string `json:"settings"`
	}{WorkInfoPluginContract, r.Operation, "", "", settings}
	switch r.Operation {
	case WorkInfoSearch:
		body.Query = r.Query
	case WorkInfoFetch:
		body.URL = r.URL
	}
	return json.Marshal(body)
}

// WorkInfoCandidate は検索の候補の 1 つである。文字だけで、画像は持たない（research.md R-10）。
type WorkInfoCandidate struct {
	URL    string
	Title  string
	Detail string
}

// FetchedWorkInfo はプラグインが取った作品情報である。空の Title は値が無いことを表す。
// 動画に保存する作品情報（WorkInfo）とは別の値で、反映できるのは名前とタグだけである。
type FetchedWorkInfo struct {
	Title string
	Tags  []string
}

// WorkInfoRunResult は成功した実行の結果である。操作が search なら Candidates、
// fetch なら WorkInfo を持つ。
type WorkInfoRunResult struct {
	Candidates []WorkInfoCandidate
	WorkInfo   FetchedWorkInfo
}

// WorkInfoRunFailure は実行の失敗の種類である（contracts/screen-api.md の Run failures）。
type WorkInfoRunFailure string

const (
	// WorkInfoRunFailed はプラグインが誤りを報告したか、0 以外で終わったか、取り決めに無い出力を書いたことを表す。
	WorkInfoRunFailed WorkInfoRunFailure = "plugin_failed"
	// WorkInfoRunTimedOut は実行が WorkInfoPluginRunTimeout を超えたことを表す。
	WorkInfoRunTimedOut WorkInfoRunFailure = "timed_out"
	// WorkInfoRunMemoryExceeded は実行が線形メモリの上限を超えようとしたことを表す。
	WorkInfoRunMemoryExceeded WorkInfoRunFailure = "memory_exceeded"
	// WorkInfoRunOutputTooLarge は標準出力が WorkInfoPluginOutputMaxBytes を超えたことを表す。
	WorkInfoRunOutputTooLarge WorkInfoRunFailure = "output_too_large"
	// WorkInfoRunBusy は別の実行が WorkInfoPluginBusyWait の間、枠を空けなかったことを表す。
	WorkInfoRunBusy WorkInfoRunFailure = "busy"
)

// WorkInfoRunError は実行の失敗である。PluginMessage はプラグインが報告した誤りの文の
// 先頭 WorkInfoPluginMessageMaxRunes 文字で、Failure が WorkInfoRunFailed のときだけ入りうる。
type WorkInfoRunError struct {
	Failure       WorkInfoRunFailure
	PluginMessage string
}

func (e *WorkInfoRunError) Error() string {
	if e.PluginMessage != "" {
		return "work-info plugin run failed: " + string(e.Failure) + ": " + e.PluginMessage
	}
	return "work-info plugin run failed: " + string(e.Failure)
}

// responseJSON はプラグインの標準出力の JSON の形である（plugin-contract.md の Response）。
// 知らない項目は無視する。
type responseJSON struct {
	Candidates []struct {
		URL    string `json:"url"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	} `json:"candidates"`
	WorkInfo *struct {
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
	} `json:"workInfo"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ParseWorkInfoResponse は操作 op の実行が標準出力に書いた応答を読む。誤りの応答と、
// 取り決めのどれにも当たらない出力は *WorkInfoRunError（plugin_failed）を返す。
func ParseWorkInfoResponse(op WorkInfoOperation, output []byte) (WorkInfoRunResult, error) {
	var raw responseJSON
	if err := json.Unmarshal(output, &raw); err != nil || !isJSONObject(output) {
		return WorkInfoRunResult{}, &WorkInfoRunError{Failure: WorkInfoRunFailed}
	}
	if raw.Error != nil {
		return WorkInfoRunResult{}, &WorkInfoRunError{
			Failure:       WorkInfoRunFailed,
			PluginMessage: truncateRunes(raw.Error.Message, WorkInfoPluginMessageMaxRunes),
		}
	}
	switch {
	case op == WorkInfoSearch && raw.Candidates != nil:
		return parseCandidates(raw)
	case op == WorkInfoFetch && raw.WorkInfo != nil:
		return WorkInfoRunResult{WorkInfo: FetchedWorkInfo{
			Title: raw.WorkInfo.Title,
			Tags:  firstUniqueTags(raw.WorkInfo.Tags),
		}}, nil
	}
	return WorkInfoRunResult{}, &WorkInfoRunError{Failure: WorkInfoRunFailed}
}

func parseCandidates(raw responseJSON) (WorkInfoRunResult, error) {
	candidates := make([]WorkInfoCandidate, 0, min(len(raw.Candidates), WorkInfoPluginMaxCandidates))
	for _, c := range raw.Candidates {
		if c.Title == "" || !hasHTTPScheme(c.URL) {
			return WorkInfoRunResult{}, &WorkInfoRunError{Failure: WorkInfoRunFailed}
		}
		if len(candidates) < WorkInfoPluginMaxCandidates {
			candidates = append(candidates, WorkInfoCandidate{URL: c.URL, Title: c.Title, Detail: c.Detail})
		}
	}
	return WorkInfoRunResult{Candidates: candidates}, nil
}

// hasHTTPScheme は s が http か https の URL の形かを返す。URL の解釈は使う側が行う。
func hasHTTPScheme(s string) bool {
	lower := bytes.ToLower([]byte(s))
	return bytes.HasPrefix(lower, []byte("http://")) && len(s) > len("http://") ||
		bytes.HasPrefix(lower, []byte("https://")) && len(s) > len("https://")
}

// firstUniqueTags は重複を除いた先頭 WorkInfoPluginMaxTags 件のタグを返す。
func firstUniqueTags(tags []string) []string {
	out := make([]string, 0, min(len(tags), WorkInfoPluginMaxTags))
	seen := map[string]bool{}
	for _, t := range tags {
		if len(out) == WorkInfoPluginMaxTags {
			break
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func truncateRunes(s string, n int) string {
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}
