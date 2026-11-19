package server

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Order networks: the owner can limit guest ordering to the restaurant's own Wi-Fi, so a photo
// of a table QR taken home cannot be used to send orders from outside. The restaurant stores a
// list of IP ranges; an empty list means guests can order from anywhere (the default).

// clientIP is the address the request came from. Behind a reverse proxy (TRUST_PROXY=1) it is the
// last X-Forwarded-For entry, the one our own proxy added; earlier entries are set by the client
// and could be forged.
func (a *App) clientIP(r *http.Request) (netip.Addr, bool) {
	raw := r.RemoteAddr
	if a.cfg.TrustProxy {
		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			parts := strings.Split(xff[len(xff)-1], ",")
			raw = strings.TrimSpace(parts[len(parts)-1])
		}
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

// networkOf turns one address into the range a whole Wi-Fi shares:
//   - a private IPv4 address (the app runs inside the restaurant): the /24 around it, since every
//     phone on the Wi-Fi gets a different address in it;
//   - a public IPv4 address (the app runs online): exactly that address, the router's, which all
//     phones on the Wi-Fi share through NAT;
//   - IPv6: the /64, which is what one home or shop network gets.
func networkOf(ip netip.Addr) netip.Prefix {
	bits := 32
	switch {
	case ip.Is6():
		bits = 64
	case ip.IsPrivate() || ip.IsLoopback():
		bits = 24
	}
	p, _ := ip.Prefix(bits)
	return p
}

// parseNetwork reads an owner-typed address or range: "103.4.5.6", "192.168.0.0/24", "2001:db8::/48".
func parseNetwork(s string) (netip.Prefix, bool) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, false
		}
		return p.Masked(), true
	}
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, false
	}
	ip = ip.Unmap()
	return netip.PrefixFrom(ip, ip.BitLen()), true
}

// splitNetworks parses the stored list, skipping anything that does not parse.
func splitNetworks(stored string) []netip.Prefix {
	var out []netip.Prefix
	for _, line := range strings.Split(stored, "\n") {
		if p, ok := parseNetwork(line); ok {
			out = append(out, p)
		}
	}
	return out
}

func joinNetworks(ps []netip.Prefix) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.String()
	}
	return strings.Join(s, "\n")
}

// networkAllowed reports whether ip may order. An empty list allows everyone.
func networkAllowed(stored string, ip netip.Addr, known bool) bool {
	nets := splitNetworks(stored)
	if len(nets) == 0 {
		return true
	}
	if !known {
		return false
	}
	for _, p := range nets {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// guestNetworkOK lets the request through when the restaurant has no network list, when it comes
// from a listed network, or when a logged-in staff member of this restaurant sends it (a waiter
// taking an order on mobile data).
func (a *App) guestNetworkOK(r *http.Request, rid int64, stored string) bool {
	ip, known := a.clientIP(r)
	if networkAllowed(stored, ip, known) {
		return true
	}
	s := a.currentStaff(r)
	return s != nil && s.R.ID == rid
}

// networks shows the owner's list and the address this browser has right now.
func (a *App) networks(w http.ResponseWriter, r *http.Request, s *Staff) {
	m := M{"Staff": s, "Page": "settings", "Nets": splitNetworks(s.R.Networks), "Err": r.URL.Query().Get("err")}
	if ip, ok := a.clientIP(r); ok {
		m["MyIP"] = ip.String()
		m["MyNet"] = networkOf(ip).String()
		m["MyAllowed"] = networkAllowed(s.R.Networks, ip, true)
	}
	a.render(w, "owner_networks", m)
}

// networksSave handles add-mine, add (typed), remove and clear.
func (a *App) networksSave(w http.ResponseWriter, r *http.Request, s *Staff) {
	nets := splitNetworks(s.R.Networks)
	var add netip.Prefix
	switch r.FormValue("act") {
	case "mine":
		ip, ok := a.clientIP(r)
		if !ok {
			back(w, r, "/owner/networks?err="+urlQuery("Could not read this device's address."))
			return
		}
		add = networkOf(ip)
	case "add":
		p, ok := parseNetwork(r.FormValue("net"))
		if !ok {
			back(w, r, "/owner/networks?err="+urlQuery("That is not an IP address or range. Examples: 103.4.5.6 or 192.168.1.0/24"))
			return
		}
		add = p
	case "remove":
		drop, _ := parseNetwork(r.FormValue("net"))
		kept := nets[:0]
		for _, p := range nets {
			if p != drop {
				kept = append(kept, p)
			}
		}
		nets = kept
	case "clear":
		nets = nil
	}
	if add.IsValid() {
		dup := false
		for _, p := range nets {
			dup = dup || p == add
		}
		if !dup && len(nets) < 20 {
			nets = append(nets, add)
		}
	}
	a.db.Exec(`UPDATE restaurants SET order_networks=? WHERE id=?`, joinNetworks(nets), s.R.ID)
	back(w, r, "/owner/networks")
}
