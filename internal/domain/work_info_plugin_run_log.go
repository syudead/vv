package domain

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// 作品情報プラグインの実行が残した標準エラーと、その秘密の値の伏せ方
// （specs/056-plugin-tooling/research.md R-6、data-model.md の work_info_plugin_runs・Rules）。
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
// WorkInfoSecretMask に置き換える。空の値はどこにも一致しない。log が切り詰められていれば、末尾に残った
// 秘密の値の先頭にあたる部分も伏せ、64 KiB で切れた値を一部も残さない。プラグインが符号化してから
// 書いた値は見つけられない（contracts/plugin-contract.md の Standard error）。
func MaskWorkInfoRunLog(log WorkInfoRunLog, secrets []string) WorkInfoRunLog {
	secrets = slices.DeleteFunc(slices.Clone(secrets), func(s string) bool { return s == "" })
	if len(secrets) == 0 {
		return log
	}
	// 長い値を先に比べ、ほかの値を含む値を丸ごと伏せる。
	slices.SortFunc(secrets, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	text, tail := log.Stderr, ""
	if log.Truncated {
		if n := secretPrefixSuffix(text, secrets); n > 0 {
			text, tail = text[:len(text)-n], WorkInfoSecretMask
		}
	}
	pairs := make([]string, 0, 2*len(secrets))
	for _, s := range secrets {
		pairs = append(pairs, s, WorkInfoSecretMask)
	}
	log.Stderr = strings.NewReplacer(pairs...).Replace(text) + tail
	return log
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
