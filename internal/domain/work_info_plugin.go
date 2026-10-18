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

// Plugin inspection/execution belongs to internal/wasmplugin; domain contains only pure types and rules.

const WorkInfoPluginContract = 1

const (
	WorkInfoPluginModuleMaxBytes = 16 << 20
	// Memory limits use 64 KiB pages per execution.
	WorkInfoPluginMemoryLimitPages = 2048
	// The table limit applies individually and in aggregate; missing declared maxima are rewritten to this cap.
	WorkInfoPluginTableMaxElements = 100_000
	// The run timeout includes compilation and HTTP after acquiring the execution slot.
	WorkInfoPluginRunTimeout = 30 * time.Second
	// Exceeding stdout capacity stops execution.
	WorkInfoPluginOutputMaxBytes  = 1 << 20
	WorkInfoPluginStderrKeptBytes = 64 << 10
	// BusyWait bounds how long a second run waits for the single execution slot.
	WorkInfoPluginBusyWait = 10 * time.Second

	WorkInfoPluginMaxSettings     = 10
	WorkInfoPluginMessageMaxRunes = 300
	WorkInfoPluginMaxCandidates   = 50
	WorkInfoPluginMaxTags         = 100
)

const (
	WorkInfoPluginHTTPMaxRequests  = 16
	WorkInfoPluginHTTPMaxRedirects = 5
	// HTTPTimeout includes reading the response body.
	WorkInfoPluginHTTPTimeout      = 10 * time.Second
	WorkInfoPluginHTTPMaxBodyBytes = 4 << 20
	// Oversized response headers count as an undelivered response; keep this cap smaller because http_meta retains headers for the run.
	WorkInfoPluginHTTPMaxHeaderBytes = 1 << 20
)

var ErrWorkInfoPluginTooLarge = errors.New("work-info plugin module is larger than 16 MiB")

type WorkInfoPluginProblem string

const (
	WorkInfoPluginNotWasm             WorkInfoPluginProblem = "not_wasm"
	WorkInfoPluginManifestMissing     WorkInfoPluginProblem = "manifest_missing"
	WorkInfoPluginManifestInvalid     WorkInfoPluginProblem = "manifest_invalid"
	WorkInfoPluginContractUnsupported WorkInfoPluginProblem = "contract_unsupported"
	WorkInfoPluginImportUnsupported   WorkInfoPluginProblem = "import_unsupported"
	// Plugins must export a parameterless/resultless _start function and memory.
	WorkInfoPluginExportInvalid WorkInfoPluginProblem = "export_invalid"
	// Missing table maxima count as WorkInfoPluginTableMaxElements when enforcing aggregate limits.
	WorkInfoPluginTableTooLarge WorkInfoPluginProblem = "table_too_large"
)

type WorkInfoPluginState string

const (
	WorkInfoPluginApproved   WorkInfoPluginState = "approved"
	WorkInfoPluginUnapproved WorkInfoPluginState = "unapproved"
	WorkInfoPluginInvalid    WorkInfoPluginState = "invalid"
)

// Approval requires no problem and an approved digest matching the current module.
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

type WorkInfoOperation string

const (
	WorkInfoSearch WorkInfoOperation = "search"
	// Plugins, not vv, embed work IDs into source URLs.
	WorkInfoFetch WorkInfoOperation = "fetch"
)

type WorkInfoPluginSetting struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
	// Default also replaces saved values outside Options; empty means no default.
	Default string `json:"default,omitempty"`
	// Options follows manifest order; nil permits free-form input.
	Options []string `json:"options,omitempty"`
}

const (
	workInfoPluginMinOptions     = 2
	workInfoPluginMaxOptions     = 50
	workInfoPluginOptionMaxRunes = 200
)

// Raw JSON distinguishes omitted default/options from explicit null.
type settingJSON struct {
	Key      string          `json:"key"`
	Label    string          `json:"label"`
	Secret   bool            `json:"secret"`
	Required bool            `json:"required"`
	Default  json.RawMessage `json:"default"`
	Options  json.RawMessage `json:"options"`
}

// optionalJSON treats absence as valid and rejects explicit null or incorrect types.
func optionalJSON(raw json.RawMessage, dst any) (present, ok bool) {
	if raw == nil {
		return false, true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return true, false
	}
	return true, json.Unmarshal(raw, dst) == nil
}

type WorkInfoPluginManifest struct {
	Contract    int
	Name        string
	Version     string
	Description string
	Operations  []WorkInfoOperation
	Settings    []WorkInfoPluginSetting
}

func (m WorkInfoPluginManifest) Supports(op WorkInfoOperation) bool {
	return slices.Contains(m.Operations, op)
}

// Unknown manifest fields are ignored.
type manifestJSON struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Description string              `json:"description"`
	Operations  []WorkInfoOperation `json:"operations"`
	Settings    []json.RawMessage   `json:"settings"`
}

// Field identifies the rejected property path, or empty for the whole manifest; Rule is phrased to follow Field.
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

func ParseWorkInfoPluginManifest(data []byte) (WorkInfoPluginManifest, WorkInfoPluginProblem) {
	m, rejection := CheckWorkInfoPluginManifest(data)
	if rejection != nil {
		return WorkInfoPluginManifest{}, rejection.Problem
	}
	return m, ""
}

// Validate contract before other fields because newer contracts may change their shapes.
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

// Decode array items separately to retain indices missing from UnmarshalTypeError.Field.
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

// Explicit defaults must be nonempty; explicit null defaults/options are rejected.
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

// Secret settings cannot declare defaults/options; choices are 2–50 unique strings of 1–200 code points, and defaults must be among them.
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

// Request builders in internal/app and cmd/mdm validate string lengths.
type WorkInfoRunRequest struct {
	Operation WorkInfoOperation
	Query     string
	// ID is opaque; the plugin embeds it into source URLs.
	ID       string
	Settings map[string]string
}

func (r WorkInfoRunRequest) MarshalJSON() ([]byte, error) {
	settings := r.Settings
	if settings == nil {
		settings = map[string]string{}
	}
	body := struct {
		Contract  int               `json:"contract"`
		Operation WorkInfoOperation `json:"operation"`
		Query     string            `json:"query,omitempty"`
		ID        string            `json:"id,omitempty"`
		Settings  map[string]string `json:"settings"`
	}{WorkInfoPluginContract, r.Operation, "", "", settings}
	switch r.Operation {
	case WorkInfoSearch:
		body.Query = r.Query
	case WorkInfoFetch:
		body.ID = r.ID
	}
	return json.Marshal(body)
}

// Candidate IDs are trimmed fetch inputs; URLs are display links only and are never fetched or passed to fetch by vv.
type WorkInfoCandidate struct {
	ID     string
	URL    string
	Title  string
	Detail string
}

// FetchedWorkInfo retains potentially invalid field values for later apply validation; credits carry names/external IDs rather than stored IDs.
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
	CoverURL      string
}

type WorkInfoRunResult struct {
	Candidates []WorkInfoCandidate
	WorkInfo   FetchedWorkInfo
	// CoverImage is unvalidated response data, nil when absent or fetching fails.
	CoverImage []byte
	// PersonImages aligns with WorkInfo.People and holds unvalidated response data, nil for a person without one.
	PersonImages [][]byte
}

type WorkInfoRunFailure string

const (
	WorkInfoRunFailed         WorkInfoRunFailure = "plugin_failed"
	WorkInfoRunTimedOut       WorkInfoRunFailure = "timed_out"
	WorkInfoRunMemoryExceeded WorkInfoRunFailure = "memory_exceeded"
	WorkInfoRunOutputTooLarge WorkInfoRunFailure = "output_too_large"
	WorkInfoRunBusy           WorkInfoRunFailure = "busy"
	// Blocked private/LAN/vv addresses fail the run regardless of plugin output.
	WorkInfoRunBlockedAddress WorkInfoRunFailure = "blocked_address"
	WorkInfoRunNetworkError   WorkInfoRunFailure = "network_error"
)

// PluginMessage is bounded and applies only to plugin/network failures; Host applies only to blocked-address/network failures.
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

// Unknown response fields are ignored.
type responseJSON struct {
	Candidates []struct {
		ID     string `json:"id"`
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
	// Ignore candidates beyond the retained limit; retained IDs follow fetch-input rules, while display-only links need absolute HTTP(S) URLs.
	for _, c := range raw.Candidates[:min(len(raw.Candidates), WorkInfoPluginMaxCandidates)] {
		id, err := NormalizeWorkInfoSearchQuery(c.ID)
		if c.Title == "" || err != nil || !isWorkURL(c.URL) {
			return WorkInfoRunResult{}, &WorkInfoRunError{Failure: WorkInfoRunFailed}
		}
		candidates = append(candidates, WorkInfoCandidate{ID: id, URL: c.URL, Title: c.Title, Detail: c.Detail})
	}
	return WorkInfoRunResult{Candidates: candidates}, nil
}

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
