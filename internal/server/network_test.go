package server

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"bosho/internal/config"
)

func TestNetworkOf(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.37":        "192.168.1.0/24", // app on the shop LAN: the whole Wi-Fi subnet
		"103.4.5.6":           "103.4.5.6/32",   // app online: the shop router's public address
		"2001:db8:1:2:3::4":   "2001:db8:1:2::/64",
		"::ffff:192.168.0.10": "192.168.0.0/24",
	} {
		if got := networkOf(netip.MustParseAddr(in).Unmap()).String(); got != want {
			t.Errorf("networkOf(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestNetworkAllowed(t *testing.T) {
	ip := netip.MustParseAddr
	stored := "192.168.1.0/24\n103.4.5.6/32\njunk"
	cases := []struct {
		stored string
		ip     string
		want   bool
	}{
		{"", "8.8.8.8", true}, // no list: anywhere
		{stored, "192.168.1.200", true},
		{stored, "192.168.2.1", false},
		{stored, "103.4.5.6", true},
		{stored, "103.4.5.7", false},
	}
	for _, c := range cases {
		if got := networkAllowed(c.stored, ip(c.ip), true); got != c.want {
			t.Errorf("networkAllowed(%q, %s) = %v, want %v", c.stored, c.ip, got, c.want)
		}
	}
	if networkAllowed(stored, netip.Addr{}, false) {
		t.Error("unknown address must be refused when a list is set")
	}
}

func TestParseNetwork(t *testing.T) {
	for in, want := range map[string]string{"103.4.5.6": "103.4.5.6/32", " 192.168.1.9/24 ": "192.168.1.0/24", "2001:db8::/48": "2001:db8::/48"} {
		if p, ok := parseNetwork(in); !ok || p.String() != want {
			t.Errorf("parseNetwork(%q) = %v %v, want %s", in, p, ok, want)
		}
	}
	for _, bad := range []string{"", "wifi", "1.2.3.4/40"} {
		if _, ok := parseNetwork(bad); ok {
			t.Errorf("parseNetwork(%q) should fail", bad)
		}
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.5:1234"
	r.Header.Set("X-Forwarded-For", "1.1.1.1, 103.4.5.6")
	direct := &App{}
	if ip, _ := direct.clientIP(r); ip.String() != "10.0.0.5" {
		t.Errorf("without TRUST_PROXY got %s, want the socket address", ip)
	}
	proxied := &App{cfg: config.Config{TrustProxy: true}}
	if ip, _ := proxied.clientIP(r); ip.String() != "103.4.5.6" {
		t.Errorf("with TRUST_PROXY got %s, want the last forwarded address", ip)
	}
}
