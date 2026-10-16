package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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
	// WorkInfoPluginTableMaxElements は各テーブルと、モジュールの全テーブルの合計の要素数の上限である。最大値を宣言しないテーブルには vv がこれを最大値として書き足す。
	WorkInfoPluginTableMaxElements = 100_000
	// WorkInfoPluginRunTimeout は 1 回の実行の時間の上限で、実行の枠を取ってからのコンパイルと HTTP の時間も含む。
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

// 作品情報プラグインの HTTP 要求の上限である（research.md R-5、plugin-contract.md の Host functions）。
const (
	// WorkInfoPluginHTTPMaxRequests は 1 回の実行で送れる要求の数である。
	WorkInfoPluginHTTPMaxRequests = 16
	// WorkInfoPluginHTTPMaxRedirects は 1 つの要求でたどるリダイレクトの数である。
	WorkInfoPluginHTTPMaxRedirects = 5
	// WorkInfoPluginHTTPTimeout は 1 つの要求の、本文を読み終えるまでの時間の上限である。
	WorkInfoPluginHTTPTimeout = 10 * time.Second
	// WorkInfoPluginHTTPMaxBodyBytes は 1 つの応答の本文の上限である。
	WorkInfoPluginHTTPMaxBodyBytes = 4 << 20
	// WorkInfoPluginHTTPMaxHeaderBytes は 1 つの応答のヘッダーの上限である。超えた応答は届かなかった
	// ものとして扱う。http_meta はヘッダーを実行の間 vv の側に持つので、本文より小さく抑える。
	WorkInfoPluginHTTPMaxHeaderBytes = 1 << 20
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
	// WorkInfoPluginTableTooLarge はテーブルの最小値か、宣言した最大値か、全テーブルの要素数の合計（最大値の無いテーブルは
	// WorkInfoPluginTableMaxElements で数える）が WorkInfoPluginTableMaxElements を超えることを表す。
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
	// Default は値が保存されていないとき（Options の外の値が保存されているときも）に実行が受け取る値である。
	// 空なら既定値は無い（specs/056-plugin-tooling/research.md R-3）。
	Default string `json:"default,omitempty"`
	// Options は値の選択肢で、マニフェストの順である。nil なら値は自由に入力する。
	Options []string `json:"options,omitempty"`
}

// 設定の既定値と選択肢の上限である（specs/056-plugin-tooling/contracts/plugin-contract.md の Manifest）。
const (
	workInfoPluginMinOptions     = 2
	workInfoPluginMaxOptions     = 50
	workInfoPluginOptionMaxRunes = 200
)

// settingJSON は vv.plugin セクションの設定 1 つの JSON の形である。default と options は
// 項目の有無と明示された null を区別するために生の JSON のまま読む（optionalJSON）。
type settingJSON struct {
	Key      string          `json:"key"`
	Label    string          `json:"label"`
	Secret   bool            `json:"secret"`
	Required bool            `json:"required"`
	Default  json.RawMessage `json:"default"`
	Options  json.RawMessage `json:"options"`
}

// optionalJSON は任意の項目 raw を dst に読み、項目があったかを返す。項目が無ければ (false, true)。
// 明示された null や型の違う値は契約の型（文字列・文字列の配列）でないので ok を false にする。
func optionalJSON(raw json.RawMessage, dst any) (present, ok bool) {
	if raw == nil {
		return false, true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return true, false
	}
	return true, json.Unmarshal(raw, dst) == nil
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
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Description string              `json:"description"`
	Operations  []WorkInfoOperation `json:"operations"`
	Settings    []json.RawMessage   `json:"settings"`
}

// WorkInfoPluginManifestError は拒んだマニフェストの理由である（specs/056-plugin-tooling/contracts/plugin-cli.md）。
// Field は規則に反した項目の道筋（"settings[0].default"）で、マニフェスト全体の誤りなら空である。
// Rule は Field に続けて読める、反した規則の文である。
type WorkInfoPluginManifestError struct {
	Problem WorkInfoPluginProblem
	Field   string
	Rule    string
}

func (e *WorkInfoPluginManifestError) Error() string {
	field := e.Field
	if field == "" {
		field = "the manifest"
	}
	return fmt.Sprintf("%s: %s %s", e.Problem, field, e.Rule)
}

func manifestInvalid(field, rule string) *WorkInfoPluginManifestError {
	return &WorkInfoPluginManifestError{Problem: WorkInfoPluginManifestInvalid, Field: field, Rule: rule}
}

func settingInvalid(i int, field, rule string) *WorkInfoPluginManifestError {
	return manifestInvalid(fmt.Sprintf("settings[%d].%s", i, field), rule)
}

// ParseWorkInfoPluginManifest は vv.plugin セクションの中身を読み、規則を確かめる。
// 読めなければ manifest_invalid、contract が 1 でなければ contract_unsupported を返す。
// 反した項目と規則が要るときは CheckWorkInfoPluginManifest を使う。
func ParseWorkInfoPluginManifest(data []byte) (WorkInfoPluginManifest, WorkInfoPluginProblem) {
	m, rejection := CheckWorkInfoPluginManifest(data)
	if rejection != nil {
		return WorkInfoPluginManifest{}, rejection.Problem
	}
	return m, ""
}

// CheckWorkInfoPluginManifest は ParseWorkInfoPluginManifest と同じ規則でマニフェストを読み、拒むときは
// 問題に、反した項目と規則を添えて返す。contract は他の項目より先に確かめる。後の版は他の項目の形を
// 変えうるからである。
func CheckWorkInfoPluginManifest(data []byte) (WorkInfoPluginManifest, *WorkInfoPluginManifestError) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top == nil {
		return WorkInfoPluginManifest{}, manifestInvalid("", "must be a JSON object")
	}
	var contract int
	if raw, ok := top["contract"]; !ok || json.Unmarshal(raw, &contract) != nil {
		return WorkInfoPluginManifest{}, manifestInvalid("contract", "must be an integer")
	}
	if contract != WorkInfoPluginContract {
		return WorkInfoPluginManifest{}, &WorkInfoPluginManifestError{
			Problem: WorkInfoPluginContractUnsupported, Field: "contract", Rule: fmt.Sprintf("must be %d", WorkInfoPluginContract),
		}
	}
	var raw manifestJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return WorkInfoPluginManifest{}, typeRejection("", err)
	}
	settings, rejection := parseSettings(raw.Settings)
	if rejection != nil {
		return WorkInfoPluginManifest{}, rejection
	}
	m := WorkInfoPluginManifest{
		Contract:    contract,
		Name:        raw.Name,
		Version:     raw.Version,
		Description: raw.Description,
		Operations:  raw.Operations,
		Settings:    settings,
	}
	if rejection := checkManifest(m); rejection != nil {
		return WorkInfoPluginManifest{}, rejection
	}
	return m, nil
}

// typeRejection は項目の JSON の型の誤りを理由にする。prefix は読んだ値の道筋で、マニフェスト全体なら空、
// 配列の要素なら添字つき（"settings[1]"）である。UnmarshalTypeError.Field は配列の添字を含まないので、
// 配列の要素は 1 つずつ読んでここで添字を足す。
func typeRejection(prefix string, err error) *WorkInfoPluginManifestError {
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) {
		return manifestInvalid(prefix, "must be a JSON object")
	}
	field := typeErr.Field
	if prefix != "" && field != "" {
		field = prefix + "." + field
	} else if prefix != "" {
		field = prefix
	}
	return manifestInvalid(field, "must be "+jsonTypeName(typeErr.Type.Kind()))
}

func jsonTypeName(kind reflect.Kind) string {
	switch kind {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "a boolean"
	case reflect.Slice:
		return "an array"
	default:
		return "an object"
	}
}

// parseSettings は設定の JSON を宣言にし、既定値と選択肢の規則を確かめる。
// 設定は 1 つずつ読み、型の誤りの項目に添字（settings[1].label）を付ける。
func parseSettings(raw []json.RawMessage) ([]WorkInfoPluginSetting, *WorkInfoPluginManifestError) {
	if raw == nil {
		return nil, nil
	}
	settings := make([]WorkInfoPluginSetting, 0, len(raw))
	for i, element := range raw {
		var r settingJSON
		if err := json.Unmarshal(element, &r); err != nil {
			return nil, typeRejection(fmt.Sprintf("settings[%d]", i), err)
		}
		s, rejection := parseSetting(i, r)
		if rejection != nil {
			return nil, rejection
		}
		settings = append(settings, s)
	}
	return settings, nil
}

// parseSetting は i 番目の設定を読む。default を書いた設定は 1 文字以上の既定値を持たなければならないので、
// 空の default はここで拒む。null の default や options も拒む。
func parseSetting(i int, r settingJSON) (WorkInfoPluginSetting, *WorkInfoPluginManifestError) {
	s := WorkInfoPluginSetting{Key: r.Key, Label: r.Label, Secret: r.Secret, Required: r.Required}
	hasDefault, ok := optionalJSON(r.Default, &s.Default)
	if !ok {
		return WorkInfoPluginSetting{}, settingInvalid(i, "default", "must be a string")
	}
	if hasDefault && !runeCountBetween(s.Default, 1, WorkInfoPluginSettingValueMaxRunes) {
		return WorkInfoPluginSetting{}, settingInvalid(i, "default",
			fmt.Sprintf("must be 1 to %d characters", WorkInfoPluginSettingValueMaxRunes))
	}
	if _, ok := optionalJSON(r.Options, &s.Options); !ok {
		return WorkInfoPluginSetting{}, settingInvalid(i, "options", "must be an array of strings")
	}
	if rejection := checkSettingChoices(i, s, hasDefault); rejection != nil {
		return WorkInfoPluginSetting{}, rejection
	}
	return s, nil
}

// checkSettingChoices は設定の既定値と選択肢の規則を確かめる。秘密の設定はどちらも持てず、選択肢は重複の無い
// 2〜50 個の 1〜200 文字で、既定値は選択肢のどれかでなければならない。
func checkSettingChoices(i int, s WorkInfoPluginSetting, hasDefault bool) *WorkInfoPluginManifestError {
	switch {
	case s.Secret && hasDefault:
		return settingInvalid(i, "default", "is not allowed on a secret setting")
	case s.Secret && s.Options != nil:
		return settingInvalid(i, "options", "is not allowed on a secret setting")
	case s.Options == nil:
		return nil
	case len(s.Options) < workInfoPluginMinOptions || len(s.Options) > workInfoPluginMaxOptions:
		return settingInvalid(i, "options",
			fmt.Sprintf("must hold %d to %d values", workInfoPluginMinOptions, workInfoPluginMaxOptions))
	}
	seen := make(map[string]bool, len(s.Options))
	for _, option := range s.Options {
		if !runeCountBetween(option, 1, workInfoPluginOptionMaxRunes) {
			return settingInvalid(i, "options",
				fmt.Sprintf("must hold values of 1 to %d characters", workInfoPluginOptionMaxRunes))
		}
		if seen[option] {
			return settingInvalid(i, "options", "must hold distinct values")
		}
		seen[option] = true
	}
	if hasDefault && !seen[s.Default] {
		return settingInvalid(i, "default", "must be one of options")
	}
	return nil
}

func isJSONObject(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// checkManifest は読めたマニフェストの項目の規則を確かめる。
func checkManifest(m WorkInfoPluginManifest) *WorkInfoPluginManifestError {
	switch {
	case !ValidWorkInfoPluginName(m.Name):
		return manifestInvalid("name", "must be 1 to 64 characters of a-z, 0-9, - and .")
	case !runeCountBetween(m.Version, 1, 32):
		return manifestInvalid("version", "must be 1 to 32 characters")
	case utf8.RuneCountInString(m.Description) > 500 || !utf8.ValidString(m.Description):
		return manifestInvalid("description", "must be at most 500 characters")
	case !validOperations(m.Operations):
		return manifestInvalid("operations", "must list search, fetch or both, each at most once")
	}
	return checkSettings(m.Settings)
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

// checkSettings は設定の数と、各設定の key と label の規則を確かめる。
func checkSettings(settings []WorkInfoPluginSetting) *WorkInfoPluginManifestError {
	if len(settings) > WorkInfoPluginMaxSettings {
		return manifestInvalid("settings", fmt.Sprintf("must declare at most %d settings", WorkInfoPluginMaxSettings))
	}
	seen := map[string]bool{}
	for i, s := range settings {
		switch {
		case !validSettingKey(s.Key):
			return settingInvalid(i, "key", "must be 1 to 32 characters of A-Z, a-z, 0-9 and _, starting with a letter")
		case seen[s.Key]:
			return settingInvalid(i, "key", "must be unique within the manifest")
		case !runeCountBetween(s.Label, 1, 60):
			return settingInvalid(i, "label", "must be 1 to 60 characters")
		}
		seen[s.Key] = true
	}
	return nil
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

// FetchedWorkInfo はプラグインが取った作品情報である。空の文字列と nil の一覧は値が無いことを表す。
// 動画に保存する作品情報（WorkInfo）とは別の値で、人物と団体は名前と外部 ID で持ち、項目の規則に
// 合わない値も残す（specs/052-plugin-work-info-fields/contracts/plugin-contract.md）。
type FetchedWorkInfo struct {
	Title         string
	Tags          []string
	Code          string
	Description   string
	Dates         []WorkDate
	People        []FetchedPerson
	Organizations []FetchedOrganization
	URLs          []string
	ExternalIDs   []ExternalID
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
	// WorkInfoRunBlockedAddress はプラグインが vv、LAN か他の公開でないアドレスに要求を送ろうとしたことを表す。
	// プラグインが何を返しても実行はこの失敗になる（research.md R-5）。
	WorkInfoRunBlockedAddress WorkInfoRunFailure = "blocked_address"
	// WorkInfoRunNetworkError は要求の接続が失敗し、そのあとプラグインも失敗したことを表す。
	WorkInfoRunNetworkError WorkInfoRunFailure = "network_error"
)

// WorkInfoRunError は実行の失敗である。PluginMessage はプラグインが報告した誤りの文の
// 先頭 WorkInfoPluginMessageMaxRunes 文字で、Failure が WorkInfoRunFailed か
// WorkInfoRunNetworkError のときだけ入りうる。Host は拒んだか届かなかったホストで、Failure が
// WorkInfoRunBlockedAddress か WorkInfoRunNetworkError のときだけ入る（contracts/screen-api.md の Run failures）。
type WorkInfoRunError struct {
	Failure       WorkInfoRunFailure
	PluginMessage string
	Host          string
}

func (e *WorkInfoRunError) Error() string {
	msg := "work-info plugin run failed: " + string(e.Failure)
	if e.Host != "" {
		msg += ": host " + e.Host
	}
	if e.PluginMessage != "" {
		msg += ": " + e.PluginMessage
	}
	return msg
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
		workDetailsJSON
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
		info := FetchedWorkInfo{
			Title: raw.WorkInfo.Title,
			Tags:  firstUniqueTags(raw.WorkInfo.Tags),
		}
		parseWorkDetails(raw.WorkInfo.workDetailsJSON, &info)
		return WorkInfoRunResult{WorkInfo: info}, nil
	}
	return WorkInfoRunResult{}, &WorkInfoRunError{Failure: WorkInfoRunFailed}
}

func parseCandidates(raw responseJSON) (WorkInfoRunResult, error) {
	candidates := make([]WorkInfoCandidate, 0, min(len(raw.Candidates), WorkInfoPluginMaxCandidates))
	// 保持するのは先頭 WorkInfoPluginMaxCandidates 件だけなので、それより後の候補は確かめない。
	for _, c := range raw.Candidates[:min(len(raw.Candidates), WorkInfoPluginMaxCandidates)] {
		if c.Title == "" || !isWorkURL(c.URL) {
			return WorkInfoRunResult{}, &WorkInfoRunError{Failure: WorkInfoRunFailed}
		}
		candidates = append(candidates, WorkInfoCandidate{URL: c.URL, Title: c.Title, Detail: c.Detail})
	}
	return WorkInfoRunResult{Candidates: candidates}, nil
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
