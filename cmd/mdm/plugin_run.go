package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// mdm plugin run は作品情報プラグインを VVMDM と同じランタイム、同じ制限、同じアドレスの規則で 1 回動かし、
// VVMDM が残す結果を出す（specs/056-plugin-tooling/contracts/plugin-cli.md、research.md R-1）。要求は
// サーバーと同じ domain の規則（入力の確かめ、実効の設定値）で組み立てる。--set の秘密の値は伏せない。
// 伏せるのは所有者の画面のためで、値は作者が打ったものだからである（research.md R-6）。

// pluginRunArgs は mdm plugin run の引数を読んだものである。
type pluginRunArgs struct {
	modulePath string
	operation  domain.WorkInfoOperation
	input      string
	values     map[string]string
	proxy      string
}

// runPluginRun は mdm plugin run <module.wasm> <operation> <input> [--set KEY=VALUE]... [--proxy URL] を実行する。
func runPluginRun(ctx context.Context, args []string, env accountEnv) *accountFailure {
	parsed, failure := parsePluginRunArgs(args)
	if failure != nil {
		return failure
	}
	network, failure := pluginRunNetwork(parsed.proxy)
	if failure != nil {
		return failure
	}
	module, failure := readPackInput(parsed.modulePath, "module", "")
	if failure != nil {
		failure.withUsage = failure.code == exitAccountUsage
		return failure
	}
	runtime, failure := newPluginRuntime(ctx, env.PluginRuntime)
	if failure != nil {
		return failure
	}
	defer func() { _ = runtime.Close(context.WithoutCancel(ctx)) }()
	manifest, failure := inspectModule(ctx, runtime, module, "module", "", usageFailure)
	if failure != nil {
		return failure
	}
	req, failure := pluginRunRequest(manifest, parsed)
	if failure != nil {
		return failure
	}
	digest := sha256.Sum256(module)
	result, log, err := runtime.Run(ctx, hex.EncodeToString(digest[:]), module, req, network, nil)
	writeRunLog(env.Stderr, log, err != nil)
	var runErr *domain.WorkInfoRunError
	switch {
	case errors.As(err, &runErr):
		return &accountFailure{code: exitAccountError, message: runFailureLine(runErr)}
	case err != nil:
		return operationFailure("Cannot run the plugin: %v", err)
	}
	return writeKeptResult(env.Stdout, req.Operation, result)
}

// parsePluginRunArgs は位置の 3 つの引数と、--set KEY=VALUE（繰り返せる）、--proxy URL を読む。フラグは
// 位置の引数の前後どこにも置け、--set=KEY=VALUE と --proxy=URL の形も受け付ける。同じキーの --set は
// 後のものが勝つ。最初の -- の後はどれも位置の引数で、-- で始まる検索の語もそこに書ける。
func parsePluginRunArgs(args []string) (pluginRunArgs, *accountFailure) {
	parsed := pluginRunArgs{values: map[string]string{}}
	var positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		name, value, hasValue := strings.Cut(args[i], "=")
		if name != "--set" && name != "--proxy" {
			if strings.HasPrefix(args[i], "--") {
				return parsed, usageFailure("Unknown option: %s", args[i])
			}
			positional = append(positional, args[i])
			continue
		}
		if !hasValue {
			if i+1 == len(args) {
				return parsed, usageFailure("%s needs a value.", name)
			}
			i++
			value = args[i]
		}
		if failure := parsed.setOption(name, value); failure != nil {
			return parsed, failure
		}
	}
	if len(positional) != 3 {
		return parsed, usageFailure("run takes exactly three arguments: the module, the operation and its input.")
	}
	parsed.modulePath, parsed.operation, parsed.input = positional[0], domain.WorkInfoOperation(positional[1]), positional[2]
	return parsed, nil
}

// setOption は --set か --proxy の値を入れる。
func (p *pluginRunArgs) setOption(name, value string) *accountFailure {
	if name == "--proxy" {
		p.proxy = value
		return nil
	}
	key, setting, ok := strings.Cut(value, "=")
	if !ok || key == "" {
		return usageFailure("--set takes KEY=VALUE, not %q.", value)
	}
	p.values[key] = setting
	return nil
}

// pluginRunNetwork は --proxy の URL を PUT /api/settings/plugin-proxy と同じ規則で確かめる。
func pluginRunNetwork(raw string) (domain.WorkInfoRunNetwork, *accountFailure) {
	proxy, err := domain.ParseWorkInfoPluginProxyURL(raw)
	if err != nil {
		return domain.WorkInfoRunNetwork{}, usageFailure(
			"--proxy must be an http, https or socks5 URL with a host, an optional port and user information, "+
				"and no path, at most %d characters", domain.WorkInfoPluginProxyURLMaxLen)
	}
	return domain.WorkInfoRunNetwork{Proxy: proxy}, nil
}

// pluginRunRequest はサーバーと同じ規則で実行の要求を組み立てる。操作はマニフェストが宣言したもの、
// 入力は画面 API と同じ規則、設定は宣言されたキーだけで、値は画面が保存できるもの
// （domain.CheckWorkInfoPluginValues）に限り、実効値（既定値を含む）を入れる。
func pluginRunRequest(manifest domain.WorkInfoPluginManifest, args pluginRunArgs) (domain.WorkInfoRunRequest, *accountFailure) {
	req := domain.WorkInfoRunRequest{Operation: args.operation}
	if !manifest.Supports(args.operation) {
		return req, usageFailure("The plugin does not declare the operation %q; its manifest declares %s.",
			args.operation, joinOperations(manifest.Operations))
	}
	if failure := setRunInput(&req, args.input); failure != nil {
		return req, failure
	}
	for _, key := range slices.Sorted(maps.Keys(args.values)) {
		if !declaresSetting(manifest.Settings, key) {
			return req, usageFailure("The manifest declares no setting %q.", key)
		}
		value := args.values[key]
		if domain.CheckWorkInfoPluginValues(manifest.Settings, map[string]*string{key: &value}) != nil {
			return req, usageFailure("The value of %q must be valid UTF-8 of at most %d characters, as the settings screen saves it.",
				key, domain.WorkInfoPluginSettingValueMaxRunes)
		}
	}
	settings, err := domain.WorkInfoRunSettings(manifest.Settings, args.values)
	var missing *domain.WorkInfoSettingsMissingError
	if errors.As(err, &missing) {
		return req, usageFailure("Required settings have no value and no default: %s. Give them with --set KEY=VALUE.",
			strings.Join(missing.Keys, ", "))
	}
	if err != nil {
		return req, operationFailure("Cannot build the request: %v", err)
	}
	req.Settings = settings
	return req, nil
}

// setRunInput は操作の入力を画面 API と同じ規則で確かめて要求に入れる。
func setRunInput(req *domain.WorkInfoRunRequest, input string) *accountFailure {
	switch req.Operation {
	case domain.WorkInfoSearch:
		query, err := domain.NormalizeWorkInfoSearchQuery(input)
		if err != nil {
			return usageFailure("The search words must be 1 to %d characters after trimming spaces.",
				domain.WorkInfoSearchQueryMaxRunes)
		}
		req.Query = query
	case domain.WorkInfoFetch:
		// URL の形の入力も作品 ID としてそのまま渡す（親 Issue #1232 の要件 14・16）。
		id, err := domain.NormalizeWorkInfoSearchQuery(input)
		if err != nil {
			return usageFailure("The work's ID must be 1 to %d characters after trimming spaces.",
				domain.WorkInfoSearchQueryMaxRunes)
		}
		req.ID = id
	default:
		return usageFailure("This VVMDM cannot build the input of the operation %q.", req.Operation)
	}
	return nil
}

func declaresSetting(settings []domain.WorkInfoPluginSetting, key string) bool {
	for _, s := range settings {
		if s.Key == key {
			return true
		}
	}
	return false
}

func joinOperations(ops []domain.WorkInfoOperation) string {
	names := make([]string, 0, len(ops))
	for _, op := range ops {
		names = append(names, string(op))
	}
	return strings.Join(names, ", ")
}

// writeRunLog はプラグインの標準エラーを書かれたとおりに出し、切り詰めたならそのことを 1 行で足す。
// 切り詰めの行や、failed のとき後に続く失敗の行が、改行で終わらない標準エラーにつながらないよう、
// その前で行を区切る。
func writeRunLog(w io.Writer, log domain.WorkInfoRunLog, failed bool) {
	_, _ = io.WriteString(w, log.Stderr)
	if (log.Truncated || failed) && log.Stderr != "" && !strings.HasSuffix(log.Stderr, "\n") {
		_, _ = io.WriteString(w, "\n")
	}
	if log.Truncated {
		_, _ = fmt.Fprintf(w, "truncated after %d KiB\n", domain.WorkInfoPluginStderrKeptBytes>>10)
	}
}

// runFailureLine は実行の失敗の 1 行である。失敗の種類、ネットワークの 2 つの失敗ではホスト、プラグインが
// 報告した誤りの文があればそれを並べる。
func runFailureLine(err *domain.WorkInfoRunError) string {
	line := "Run failed: " + string(err.Failure)
	if err.Host != "" {
		line += ", host " + err.Host
	}
	if err.PluginMessage != "" {
		line += ": " + strings.Join(strings.Fields(err.PluginMessage), " ")
	}
	return line
}

// writeKeptResult は VVMDM が残す結果を、字下げした JSON で標準出力に書く。
func writeKeptResult(w io.Writer, op domain.WorkInfoOperation, result domain.WorkInfoRunResult) *accountFailure {
	var body any
	if op == domain.WorkInfoSearch {
		body = keptCandidates(result.Candidates)
	} else {
		body = struct {
			WorkInfo keptWorkInfo `json:"workInfo"`
		}{toKeptWorkInfo(result.WorkInfo)}
	}
	out, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return operationFailure("Cannot encode the result: %v", err)
	}
	if _, err := fmt.Fprintf(w, "%s\n", out); err != nil {
		return operationFailure("Cannot write the result: %v", err)
	}
	return nil
}

// 残す結果の JSON の形である。名前はプラグインの応答（docs/how-to/work-info-plugins.md の Response）と同じで、
// 値は VVMDM が上限と規則で整えたあとのものである。値の無い項目は省く。

type keptCandidate struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

type keptExternalID struct {
	Source string `json:"source"`
	Value  string `json:"value"`
}

type keptDate struct {
	Kind string `json:"kind"`
	Date string `json:"date"`
}

type keptCredit struct {
	Role        string           `json:"role"`
	Name        string           `json:"name"`
	ExternalIDs []keptExternalID `json:"externalIds,omitempty"`
}

type keptWorkInfo struct {
	Title         string           `json:"title,omitempty"`
	Tags          []string         `json:"tags"`
	Code          string           `json:"code,omitempty"`
	Description   string           `json:"description,omitempty"`
	Dates         []keptDate       `json:"dates,omitempty"`
	People        []keptCredit     `json:"people,omitempty"`
	Organizations []keptCredit     `json:"organizations,omitempty"`
	URLs          []string         `json:"urls,omitempty"`
	ExternalIDs   []keptExternalID `json:"externalIds,omitempty"`
}

func keptCandidates(candidates []domain.WorkInfoCandidate) any {
	out := make([]keptCandidate, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, keptCandidate{ID: c.ID, Title: c.Title, Detail: c.Detail})
	}
	return struct {
		Candidates []keptCandidate `json:"candidates"`
	}{out}
}

// toKeptWorkInfo は取得の結果を、サーバーが取得の結果として持つのと同じ題名と並びにする。
func toKeptWorkInfo(fetched domain.FetchedWorkInfo) keptWorkInfo {
	details := domain.OrderFetchedWorkDetails(fetched)
	out := keptWorkInfo{
		Title: domain.KeptWorkInfoTitle(fetched.Title), Tags: fetched.Tags,
		Code: details.Code, Description: details.Description, URLs: details.URLs,
		ExternalIDs: toKeptExternalIDs(details.ExternalIDs),
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	for _, d := range details.Dates {
		out.Dates = append(out.Dates, keptDate{Kind: string(d.Kind), Date: d.Date})
	}
	for _, p := range details.People {
		out.People = append(out.People, keptCredit{Role: string(p.Role), Name: p.Name, ExternalIDs: toKeptExternalIDs(p.ExternalIDs)})
	}
	for _, o := range details.Organizations {
		out.Organizations = append(out.Organizations,
			keptCredit{Role: string(o.Role), Name: o.Name, ExternalIDs: toKeptExternalIDs(o.ExternalIDs)})
	}
	return out
}

func toKeptExternalIDs(ids []domain.ExternalID) []keptExternalID {
	var out []keptExternalID
	for _, id := range ids {
		out = append(out, keptExternalID{Source: id.Source, Value: id.Value})
	}
	return out
}
