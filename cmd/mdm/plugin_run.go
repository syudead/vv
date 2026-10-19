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

// Run with server validation, runtime limits, and address rules; explicit author-supplied secret
// settings remain visible in CLI output.

type pluginRunArgs struct {
	modulePath string
	operation  domain.WorkInfoOperation
	input      string
	values     map[string]string
	proxy      string
	// hasSource tells --source with an empty value apart from no --source.
	source    string
	hasSource bool
}

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
	return writeKeptResult(env.Stdout, req, result)
}

// Options may appear around positional arguments; repeated --set keys use the last value, and --
// ends option parsing.
func parsePluginRunArgs(args []string) (pluginRunArgs, *accountFailure) {
	parsed := pluginRunArgs{values: map[string]string{}}
	var positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		name, value, hasValue := strings.Cut(args[i], "=")
		if name != "--set" && name != "--proxy" && name != "--source" {
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

func (p *pluginRunArgs) setOption(name, value string) *accountFailure {
	switch name {
	case "--proxy":
		p.proxy = value
		return nil
	case "--source":
		p.source, p.hasSource = value, true
		return nil
	}
	key, setting, ok := strings.Cut(value, "=")
	if !ok || key == "" {
		return usageFailure("--set takes KEY=VALUE, not %q.", value)
	}
	p.values[key] = setting
	return nil
}

// Validate proxy URLs with the same rules as the settings API.
func pluginRunNetwork(raw string) (domain.WorkInfoRunNetwork, *accountFailure) {
	proxy, err := domain.ParseWorkInfoPluginProxyURL(raw)
	if err != nil {
		return domain.WorkInfoRunNetwork{}, usageFailure(
			"--proxy must be an http, https or socks5 URL with a host, an optional port and user information, "+
				"and no path, at most %d characters", domain.WorkInfoPluginProxyURLMaxLen)
	}
	return domain.WorkInfoRunNetwork{Proxy: proxy}, nil
}

// Build requests with declared operations, API input validation, declared settings, and effective
// defaults, just like the server.
func pluginRunRequest(manifest domain.WorkInfoPluginManifest, args pluginRunArgs) (domain.WorkInfoRunRequest, *accountFailure) {
	req := manifest.RunRequest(args.operation)
	if !manifest.Supports(args.operation) {
		return req, usageFailure("The plugin does not declare the operation %q; its manifest declares %s.",
			args.operation, joinOperations(manifest.Operations))
	}
	if failure := setRunInput(&req, args.input); failure != nil {
		return req, failure
	}
	if failure := setRunSource(&req, manifest, args); failure != nil {
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
		// Pass URL-shaped inputs through as work IDs.
		id, err := domain.NormalizeWorkInfoSearchQuery(input)
		if err != nil {
			owner := "work"
			if req.Kind == domain.WorkInfoPluginKindPerson {
				owner = "person"
			}
			return usageFailure("The %s's ID must be 1 to %d characters after trimming spaces.",
				owner, domain.WorkInfoSearchQueryMaxRunes)
		}
		req.ID = id
	default:
		return usageFailure("This VVMDM cannot build the input of the operation %q.", req.Operation)
	}
	return nil
}

// A person plugin's fetch needs a declared --source; any other run refuses one.
func setRunSource(req *domain.WorkInfoRunRequest, manifest domain.WorkInfoPluginManifest, args pluginRunArgs) *accountFailure {
	needed := manifest.Kind == domain.WorkInfoPluginKindPerson && req.Operation == domain.WorkInfoFetch
	switch {
	case !needed && args.hasSource:
		return usageFailure("--source applies only to a person plugin's fetch.")
	case !needed:
		return nil
	case !args.hasSource || !manifest.DeclaresSource(args.source):
		return usageFailure("A person plugin's fetch needs --source with one of its sources: %s.",
			strings.Join(manifest.Sources, ", "))
	}
	req.Source = args.source
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

// Preserve plugin stderr and insert a newline before truncation or failure notices if needed.
func writeRunLog(w io.Writer, log domain.WorkInfoRunLog, failed bool) {
	_, _ = io.WriteString(w, log.Stderr)
	if (log.Truncated || failed) && log.Stderr != "" && !strings.HasSuffix(log.Stderr, "\n") {
		_, _ = io.WriteString(w, "\n")
	}
	if log.Truncated {
		_, _ = fmt.Fprintf(w, "truncated after %d KiB\n", domain.WorkInfoPluginStderrKeptBytes>>10)
	}
}

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

func writeKeptResult(w io.Writer, req domain.WorkInfoRunRequest, result domain.WorkInfoRunResult) *accountFailure {
	person := req.Kind == domain.WorkInfoPluginKindPerson
	var body any
	switch {
	case req.Operation == domain.WorkInfoSearch && person:
		body = keptPersonCandidates(result.PersonCandidates)
	case req.Operation == domain.WorkInfoSearch:
		body = keptCandidates(result.Candidates)
	case person:
		body = struct {
			Person keptPerson `json:"person"`
		}{toKeptPerson(result)}
	default:
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

// Use plugin response field names with server-normalized values; omit absent fields.

type keptCandidate struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
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
		out = append(out, keptCandidate{ID: c.ID, URL: c.URL, Title: c.Title, Detail: c.Detail})
	}
	return struct {
		Candidates []keptCandidate `json:"candidates"`
	}{out}
}

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
