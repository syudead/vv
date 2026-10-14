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
