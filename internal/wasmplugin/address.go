package wasmplugin

import (
	"net"
	"net/netip"
)

// refusedPrefixes はプラグインの要求が届いてはならない範囲の表である（research.md R-5）。
// IANA の IPv4 と IPv6 の Special-Purpose Address Registry のうち「Globally Reachable」が True で
// ない範囲と、それに加えて vv が拒む範囲を並べる。テストはどの行からも 1 つのアドレスを確かめる。
var refusedPrefixes = []netip.Prefix{
	// IANA IPv4 Special-Purpose Address Registry
	netip.MustParsePrefix("0.0.0.0/8"),          // This network
	netip.MustParsePrefix("10.0.0.0/8"),         // Private-Use
	netip.MustParsePrefix("100.64.0.0/10"),      // Shared Address Space
	netip.MustParsePrefix("127.0.0.0/8"),        // Loopback
	netip.MustParsePrefix("169.254.0.0/16"),     // Link Local
	netip.MustParsePrefix("172.16.0.0/12"),      // Private-Use
	netip.MustParsePrefix("192.0.0.0/24"),       // IETF Protocol Assignments
	netip.MustParsePrefix("192.0.2.0/24"),       // Documentation (TEST-NET-1)
	netip.MustParsePrefix("192.88.99.0/24"),     // Deprecated 6to4 Relay Anycast
	netip.MustParsePrefix("192.168.0.0/16"),     // Private-Use
	netip.MustParsePrefix("198.18.0.0/15"),      // Benchmarking
	netip.MustParsePrefix("198.51.100.0/24"),    // Documentation (TEST-NET-2)
	netip.MustParsePrefix("203.0.113.0/24"),     // Documentation (TEST-NET-3)
	netip.MustParsePrefix("240.0.0.0/4"),        // Reserved
	netip.MustParsePrefix("255.255.255.255/32"), // Limited Broadcast

	// IANA IPv6 Special-Purpose Address Registry
	netip.MustParsePrefix("::/128"),         // Unspecified Address
	netip.MustParsePrefix("::1/128"),        // Loopback Address
	netip.MustParsePrefix("::ffff:0:0/96"),  // IPv4-mapped Address
	netip.MustParsePrefix("64:ff9b:1::/48"), // IPv4-IPv6 Translation (local use)
	netip.MustParsePrefix("100::/64"),       // Discard-Only Address Block
	netip.MustParsePrefix("100:0:0:1::/64"), // Dummy IPv6 Prefix
	netip.MustParsePrefix("2001::/23"),      // IETF Protocol Assignments
	netip.MustParsePrefix("2001:db8::/32"),  // Documentation
	netip.MustParsePrefix("3fff::/20"),      // Documentation
	netip.MustParsePrefix("5f00::/16"),      // Segment Routing (SRv6) SIDs
	netip.MustParsePrefix("fc00::/7"),       // Unique-Local
	netip.MustParsePrefix("fe80::/10"),      // Link-Local Unicast

	// vv が加える範囲
	netip.MustParsePrefix("fec0::/10"),    // 廃止された site-local。今も LAN で経路を持つことがある
	netip.MustParsePrefix("64:ff9b::/96"), // IPv4 を埋め込む: IPv4-IPv6 Translation
	netip.MustParsePrefix("2001::/32"),    // IPv4 を埋め込む: TEREDO
	netip.MustParsePrefix("2002::/16"),    // IPv4 を埋め込む: 6to4
}

// ipv6GlobalUnicast は IANA が IPv6 の Global Unicast に割り当てた範囲である。
var ipv6GlobalUnicast = netip.MustParsePrefix("2000::/3")

// addressRule はプラグインの要求が接続してよいアドレスかを決める。local はサーバー自身の
// インターフェースのアドレスを返し、テストでは差し替える。
type addressRule struct {
	local func() []netip.Addr
}

// allows は、IPv4-mapped IPv6 を IPv4 に戻したアドレスが global unicast で、refusedPrefixes の
// どれにも入らず、サーバー自身のアドレスでもないときだけ true を返す。
func (r addressRule) allows(addr netip.Addr) bool {
	addr = addr.Unmap().WithZone("")
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.Is6() && !ipv6GlobalUnicast.Contains(addr) {
		return false
	}
	for _, p := range refusedPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	for _, own := range r.local() {
		if own.Unmap().WithZone("") == addr {
			return false
		}
	}
	return true
}

// interfaceAddrs はサーバーのインターフェースのアドレスを返す。インターフェースは実行の間にも
// 変わりうるので、確かめるたびに読む。読めなければ何も返さず、表の規則だけが効く。
func interfaceAddrs() []netip.Addr {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			out = append(out, p.Addr())
		} else if ip, err := netip.ParseAddr(a.String()); err == nil {
			out = append(out, ip)
		}
	}
	return out
}
