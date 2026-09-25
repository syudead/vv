package httpapi

import (
	"net/http"
	"net/netip"
	"strings"
)

// 送信元と HTTPS の判定（specs/016-single-account-auth/plan.md Structural Decisions 7）。
//
// 転送ヘッダーは、直接の接続元が信頼するプロキシ（Options.TrustedProxies）のときだけ
// 読む。それ以外の接続元が付けたヘッダーは、直接つないだ利用者が偽ったものかもしれない
// ので無視する。Forwarded（RFC 7239）と X-Forwarded-Host は読まない。プロキシは Host を
// そのまま渡す前提である。
//
// この判定を、ログインの試行制限と認証の記録の送信元、セッション Cookie の名前と
// Secure、同一オリジンの確認の期待するスキーム、既定アプリで開く操作のループバックの
// 確認に使う。

// clientOrigin は要求の送信元と、それが HTTPS で届いたかである。
type clientOrigin struct {
	// source は送信元の IP アドレスである。解釈できなければゼロ値。
	source netip.Addr
	// https は利用者から vv（または信頼するプロキシ）までが HTTPS だったかである。
	https bool
}

// trustedProxies は転送ヘッダーを信じてよい直接の接続元の集合である。
type trustedProxies []netip.Prefix

func (p trustedProxies) contains(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	for _, prefix := range p {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// clientOrigin は要求の送信元と HTTPS かを決める。
//
// 直接の接続元が信頼するプロキシなら、X-Forwarded-For を右から辿って最初の信頼しない
// アドレスを送信元とし、X-Forwarded-Proto の最後の値で HTTPS かを決める。それ以外は
// 接続元のアドレスと、TLS で受けたかで決める。
func (s *server) clientOrigin(r *http.Request) clientOrigin {
	remote := parseForwardedAddr(r.RemoteAddr)
	origin := clientOrigin{source: remote, https: r.TLS != nil}
	if !s.trustedProxies.contains(remote) {
		return origin
	}
	origin.source = s.forwardedSource(r.Header.Values("X-Forwarded-For"), remote)
	if proto, ok := lastHeaderValue(r.Header.Values("X-Forwarded-Proto")); ok {
		origin.https = strings.EqualFold(proto, "https")
	}
	return origin
}

// forwardedSource は X-Forwarded-For を右から辿り、最初の信頼しないアドレスを返す。
// どれも信頼するアドレスなら、いちばん左のアドレスを返す。解釈できない値に当たったら、
// それより左は誰が書いたか確かめられないので、そこで止めて直前に見たアドレスを返す。
func (s *server) forwardedSource(values []string, remote netip.Addr) netip.Addr {
	source := remote
	entries := headerListValues(values)
	for i := len(entries) - 1; i >= 0; i-- {
		addr := parseForwardedAddr(entries[i])
		if !addr.IsValid() {
			return source
		}
		source = addr
		if !s.trustedProxies.contains(addr) {
			return source
		}
	}
	return source
}

// parseForwardedAddr は RemoteAddr や X-Forwarded-For の1項を IP アドレスとして読む。
// 「アドレス:ポート」「[IPv6]」「[IPv6]:ポート」も受け付ける。解釈できなければゼロ値。
// ゾーン付きのアドレスはプレフィックスと一致しないので、ゾーンを落とす。
func parseForwardedAddr(value string) netip.Addr {
	value = strings.TrimSpace(value)
	if addrPort, err := netip.ParseAddrPort(value); err == nil {
		return addrPort.Addr().WithZone("").Unmap()
	}
	value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	if addr, err := netip.ParseAddr(value); err == nil {
		return addr.WithZone("").Unmap()
	}
	return netip.Addr{}
}

// headerListValues はカンマ区切りのヘッダーの値を、複数の行をつないだ順に1項ずつ返す。
// 空の項は除く。
func headerListValues(values []string) []string {
	var entries []string
	for _, value := range values {
		for entry := range strings.SplitSeq(value, ",") {
			if entry = strings.TrimSpace(entry); entry != "" {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

// lastHeaderValue はカンマ区切りのヘッダーの最後の項を返す。無ければ false。
func lastHeaderValue(values []string) (string, bool) {
	entries := headerListValues(values)
	if len(entries) == 0 {
		return "", false
	}
	return entries[len(entries)-1], true
}
