package domain

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// internal/wasmplugin captures stderr, internal/app redacts it, and internal/store persists it.

const WorkInfoSecretMask = "[secret]"

// Stderr retains the first WorkInfoPluginStderrKeptBytes; Truncated records discarded trailing bytes.
type WorkInfoRunLog struct {
	Operation WorkInfoOperation
	Failure   WorkInfoRunFailure
	Stderr    string
	Truncated bool
}

// WorkInfoPluginRun stores one latest run per plugin with secrets already masked.
type WorkInfoPluginRun struct {
	PluginID int64
	// Persist only while the plugin still matches its execution-start generation.
	Generation int64
	// RanAt is completion time.
	RanAt time.Time
	WorkInfoRunLog
}

// SecretWorkInfoPluginValues returns nonempty request values for declared secret settings.
func SecretWorkInfoPluginValues(settings []WorkInfoPluginSetting, values map[string]string) []string {
	var secrets []string
	for _, s := range settings {
		if v := values[s.Key]; s.Secret && v != "" {
			secrets = append(secrets, v)
		}
	}
	return secrets
}

// MaskWorkInfoRunLog merges overlapping matches in the original text and masks secret prefixes at truncated tails; empty secrets do not match, and plugin-encoded secrets cannot be detected.
func MaskWorkInfoRunLog(log WorkInfoRunLog, secrets []string) WorkInfoRunLog {
	secrets = slices.DeleteFunc(slices.Clone(secrets), func(s string) bool { return s == "" })
	if len(secrets) == 0 {
		return log
	}
	text := log.Stderr
	spans := secretSpans(text, secrets)
	if log.Truncated {
		if n := secretPrefixSuffix(text, secrets); n > 0 {
			spans = append(spans, span{len(text) - n, len(text)})
		}
	}
	var b strings.Builder
	at := 0
	for _, s := range mergeSpans(spans) {
		b.WriteString(text[at:s.start])
		b.WriteString(WorkInfoSecretMask)
		at = s.end
	}
	b.WriteString(text[at:])
	log.Stderr = b.String()
	return log
}

// span is a half-open byte range [start, end).
type span struct{ start, end int }

// secretSpans includes overlapping matches.
func secretSpans(text string, secrets []string) []span {
	var spans []span
	for _, s := range secrets {
		for from := 0; ; {
			i := strings.Index(text[from:], s)
			if i < 0 {
				break
			}
			spans = append(spans, span{from + i, from + i + len(s)})
			from += i + 1
		}
	}
	return spans
}

// Merge overlaps but not adjacent spans so adjacent secret values retain separate masks.
func mergeSpans(spans []span) []span {
	slices.SortFunc(spans, func(a, b span) int { return cmp.Compare(a.start, b.start) })
	var merged []span
	for _, s := range spans {
		if last := len(merged) - 1; last >= 0 && s.start < merged[last].end {
			merged[last].end = max(merged[last].end, s.end)
			continue
		}
		merged = append(merged, s)
	}
	return merged
}

// Return the longest trailing byte span matching a proper secret prefix, or zero.
func secretPrefixSuffix(text string, secrets []string) int {
	longest := 0
	for _, s := range secrets {
		for n := min(len(s)-1, len(text)); n > longest; n-- {
			if strings.HasSuffix(text, s[:n]) {
				longest = n
				break
			}
		}
	}
	return longest
}
