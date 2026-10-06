package serve

import (
	"net"
	"strings"
)

// hostOK accepts a loopback name or address, with or without a port, and the
// names given with --allow-host. A browser sends the Host of the URL the
// person typed, so a DNS name that an attacker points at 127.0.0.1 arrives
// with the attacker's name and fails here. The check is no authentication.
func (s *Server) hostOK(host string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	h = strings.ToLower(strings.TrimSuffix(strings.Trim(h, "[]"), "."))
	if h == "localhost" || s.allowed[h] || s.allowed[strings.ToLower(host)] {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// loopbackPeer reports whether a connection's remote address is a loopback
// address: the person at the machine.
func loopbackPeer(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// Loopback reports whether an address "HOST:PORT" binds only the loopback
// interface: a loopback IP address or the name localhost. An empty host and an
// unspecified address bind every interface.
func Loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
