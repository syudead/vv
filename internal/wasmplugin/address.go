package wasmplugin

import (
	"net"
	"net/netip"
)

// Reject IANA special-purpose ranges not marked Globally Reachable, plus application-specific
// exclusions; tests cover one address from every range.
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

	// Additional application exclusions.
	netip.MustParsePrefix("fec0::/10"),    // Deprecated site-local addresses can still route within a LAN.
	netip.MustParsePrefix("64:ff9b::/96"), // Embedded IPv4: translation.
	netip.MustParsePrefix("2001::/32"),    // Embedded IPv4: Teredo.
	netip.MustParsePrefix("2002::/16"),    // Embedded IPv4: 6to4.
}

var ipv6GlobalUnicast = netip.MustParsePrefix("2000::/3")

// local is replaceable for tests of server-interface exclusions.
type addressRule struct {
	local func() []netip.Addr
}

// Unmap IPv4-mapped IPv6 before requiring global unicast and excluding denied prefixes and local
// interfaces.
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

// Reread interfaces for each check because they may change during execution; lookup failure leaves
// prefix rules in effect.
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
