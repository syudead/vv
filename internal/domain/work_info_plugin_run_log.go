package domain

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// 作品情報プラグインの実行が残した標準エラーと、その秘密の値の伏せ方
//
// 実行は internal/wasmplugin、伏せて保存するのは internal/app、保存は internal/store が行う。

// WorkInfoSecretMask は標準エラーの中の秘密の設定の値と置き換える文字列である。
const WorkInfoSecretMask = "[secret]"

// WorkInfoRunLog は 1 回の実行が残した標準エラーである。Stderr は先頭 WorkInfoPluginStderrKeptBytes
// バイトで、Truncated はそれより後ろを捨てたかである。
type WorkInfoRunLog struct {
	Operation WorkInfoOperation
	// Failure は実行の失敗の種類で、成功した実行では空である。
	Failure   WorkInfoRunFailure
	Stderr    string
	Truncated bool
}

// WorkInfoPluginRun はプラグインごとに 1 つ持つ、直近の実行の標準エラーである。Stderr は秘密の値を
// 伏せたあとのものである。
type WorkInfoPluginRun struct {
	PluginID int64
	// Generation は実行を始めたときのプラグインの行の世代である。保存は、行がまだこの世代のときだけ書く。
	Generation int64
	// RanAt は実行が終わった時刻である。
	RanAt time.Time
	WorkInfoRunLog
}

// SecretWorkInfoPluginValues は宣言 settings のうち秘密の設定について、実行の要求の値 values にある
// 空でない値を返す。
func SecretWorkInfoPluginValues(settings []WorkInfoPluginSetting, values map[string]string) []string {
	var secrets []string
	for _, s := range settings {
		if v := values[s.Key]; s.Secret && v != "" {
			secrets = append(secrets, v)
		}
	}
	return secrets
}

// MaskWorkInfoRunLog は log の標準エラーのうち、秘密の値 secrets のどれかに一致する部分をすべて
// WorkInfoSecretMask に置き換える。一致はもとの標準エラーの上ですべて探し、重なる一致（2 つの値の重なりや、
// 同じ値の自己重複）はまとめて 1 つの WorkInfoSecretMask にするので、どの一致も一部も残らない。空の値は
// どこにも一致しない。log が切り詰められていれば、末尾に残った秘密の値の先頭にあたる部分も伏せ、64 KiB で
// 切れた値を一部も残さない。プラグインが符号化してから書いた値は見つけられない
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

// span は文字列の [start, end) のバイトの範囲である。
type span struct{ start, end int }

// secretSpans は text の中の、秘密の値 secrets の一致をすべて返す。重なる一致も返す。
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

// mergeSpans は spans を始まりの順に並べ、重なる範囲をまとめる。接するだけの範囲はまとめないので、
// 続けて書かれた 2 つの値は 2 つの WorkInfoSecretMask になる。
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

// secretPrefixSuffix は text の末尾のうち、秘密の値のどれかの（値そのものより短い）先頭と等しい最も長い
// 部分のバイト数を返す。無ければ 0 を返す。
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
