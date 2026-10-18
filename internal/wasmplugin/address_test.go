package wasmplugin

import (
	"net/netip"
	"testing"
)

func TestAddressRuleRefusesEveryRangeOfTheTable(t *testing.T) {
	rule := addressRule{local: func() []netip.Addr { return nil }}
	for _, p := range refusedPrefixes {
		first := p.Addr()
		if rule.allows(first) {
			t.Errorf("allows(%v) = true, want refused by %v", first, p)
		}
		if last := lastAddr(p); rule.allows(last) {
			t.Errorf("allows(%v) = true, want refused by %v", last, p)
		}
		if first.Is4() {
			mapped := netip.AddrFrom16(first.As16())
			if rule.allows(mapped) {
				t.Errorf("allows(%v) = true, want the IPv4-mapped form refused by %v", mapped, p)
			}
		}
	}
}

func TestAddressRuleRefusesNonPublicAddresses(t *testing.T) {
	own := netip.MustParseAddr("93.184.215.99")
	rule := addressRule{local: func() []netip.Addr { return []netip.Addr{own} }}
	for _, s := range []string{
		"0.1.2.3", "::ffff:0.1.2.3", "fec0::1", "fefe::1", "127.0.0.1", "::ffff:127.0.0.1",
		"10.1.2.3", "::ffff:192.168.1.20", "100.64.0.1", "240.0.0.1", "224.0.0.1", "ff02::1",
		"::", "::1", "fc00::1", "fd12:3456::1", "fe80::1%eth0", "100::1", "64:ff9b::a00:1",
		"64:ff9b:1::1", "2002:a00:1::1", "2001:0:a00:1::1", "2001:db8::1", "3fff::1", "5f00::1",
		"::7f00:1", "93.184.215.99", "::ffff:93.184.215.99",
	} {
		if rule.allows(netip.MustParseAddr(s)) {
			t.Errorf("allows(%s) = true, want refused", s)
		}
	}
}

func TestAddressRuleAcceptsPublicAddresses(t *testing.T) {
	rule := addressRule{local: func() []netip.Addr { return nil }}
	for _, s := range []string{"93.184.215.14", "::ffff:93.184.215.14", "8.8.8.8", "2606:4700:4700::1111", "2a00:1450:4001::200e"} {
		if !rule.allows(netip.MustParseAddr(s)) {
			t.Errorf("allows(%s) = false, want accepted", s)
		}
	}
}

// Check addresses just outside each global-unicast exclusion so overly broad masks cannot pass by
// rejecting only the first address; IPv6 outside 2000::/3 is already rejected.
func TestAddressRuleRangesEndWhereTheTableSays(t *testing.T) {
	rule := addressRule{local: func() []netip.Addr { return nil }}
	for _, s := range []string{
		"1.0.0.0",                   // 0.0.0.0/8
		"9.255.255.255", "11.0.0.0", // 10.0.0.0/8
		"100.63.255.255", "100.128.0.0", // 100.64.0.0/10
		"126.255.255.255", "128.0.0.0", // 127.0.0.0/8
		"169.253.255.255", "169.255.0.0", // 169.254.0.0/16
		"172.15.255.255", "172.32.0.0", // 172.16.0.0/12
		"191.255.255.255", "192.0.1.0", // 192.0.0.0/24
		"192.0.1.255", "192.0.3.0", // 192.0.2.0/24
		"192.88.98.255", "192.88.100.0", // 192.88.99.0/24
		"192.167.255.255", "192.169.0.0", // 192.168.0.0/16
		"198.17.255.255", "198.20.0.0", // 198.18.0.0/15
		"198.51.99.255", "198.51.101.0", // 198.51.100.0/24
		"203.0.112.255", "203.0.114.0", // 203.0.113.0/24
		"2000:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "2001:200::", // 2001::/23 and 2001::/32.
		"2001:db7:ffff:ffff:ffff:ffff:ffff:ffff", "2001:db9::", // 2001:db8::/32
		"2001:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "2003::", // 2002::/16
		"3ffe:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "3fff:1000::", // 3fff::/20
	} {
		if !rule.allows(netip.MustParseAddr(s)) {
			t.Errorf("allows(%s) = false, want accepted just outside its range", s)
		}
	}
}

func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Masked().Addr().AsSlice()
	for i := p.Bits(); i < len(b)*8; i++ {
		b[i/8] |= 0x80 >> (i % 8)
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}
