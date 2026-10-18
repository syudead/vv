package httpapi

import (
	"net/http"
	"net/netip"
	"strings"
)

// Trust forwarding headers only from TrustedProxies; ignore Forwarded and X-Forwarded-Host and require proxies to preserve Host for origin, authentication, and loopback decisions.

type clientOrigin struct {
	// Unparseable source addresses use the zero value.
	source netip.Addr
	// https describes the client-to-vv or client-to-trusted-proxy connection.
	https bool
}

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

// Trusted proxies use the rightmost untrusted X-Forwarded-For address and final X-Forwarded-Proto; direct requests use RemoteAddr and TLS.
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

// Stop at malformed forwarded addresses because earlier values have unverifiable authorship; if all are trusted, use the leftmost address.
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

// Strip IPv6 zones for prefix matching; accept bare IPs and optional bracketed/port forms.
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

// Preserve comma-list order across header lines and omit empty entries.
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

func lastHeaderValue(values []string) (string, bool) {
	entries := headerListValues(values)
	if len(entries) == 0 {
		return "", false
	}
	return entries[len(entries)-1], true
}
