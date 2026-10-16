package domain

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// WorkInfoPluginProxyURLMaxLen はプラグインの要求を送るプロキシの URL の最大の長さ（バイト）である。
const WorkInfoPluginProxyURLMaxLen = 2048

// ErrWorkInfoPluginProxyURLInvalid はプロキシの URL が規則に外れることを表す。
var ErrWorkInfoPluginProxyURLInvalid = errors.New("work-info plugin proxy URL is invalid")

// proxySchemes はプロキシの URL に使える scheme である。どれも Go の transport がプロキシとして扱える。
var proxySchemes = map[string]bool{"http": true, "https": true, "socks5": true}

// WorkInfoRunNetwork は 1 回の実行のネットワークの選択肢である（specs/056-plugin-tooling/research.md R-4）。
// ゼロ値はプロキシを使わない実行である。
type WorkInfoRunNetwork struct {
	// Proxy はプラグインの要求を送るプロキシである。nil ならプロキシを使わない。
	Proxy *url.URL
}

// ParseWorkInfoPluginProxyURL はプロキシの URL を確かめて返す。URL は http、https、socks5 の scheme、
// ホスト、任意のポートと任意のユーザー情報からなり、パスもクエリもフラグメントも持たず、
// WorkInfoPluginProxyURLMaxLen 以内である（specs/056-plugin-tooling/contracts/screen-api.md）。
// "" はプロキシなしで、nil を返す。規則に外れれば ErrWorkInfoPluginProxyURLInvalid を返す。
func ParseWorkInfoPluginProxyURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > WorkInfoPluginProxyURLMaxLen {
		return nil, ErrWorkInfoPluginProxyURLInvalid
	}
	// url.Parse は "#" や ":" だけを書いた空のフラグメントとポートを、書かなかったときと
	// 区別しないので、生の URL で拒む。
	if strings.Contains(raw, "#") {
		return nil, ErrWorkInfoPluginProxyURLInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || strings.HasSuffix(u.Host, ":") || !proxySchemes[u.Scheme] || u.Opaque != "" || u.Hostname() == "" ||
		u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" ||
		!validProxyPort(u.Port()) {
		return nil, ErrWorkInfoPluginProxyURLInvalid
	}
	return u, nil
}

// validProxyPort は、ポートが無いか、1 から 65535 の数かを確かめる。
func validProxyPort(port string) bool {
	if port == "" {
		return true
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}
