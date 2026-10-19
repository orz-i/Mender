package oauthflow

import (
	"context"
	"fmt"
	"net"
	"net/netip"
)

var blockedOAuthNetworks = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), // Carrier-grade NAT
	netip.MustParsePrefix("198.18.0.0/15"), // Benchmark/internal testing
	netip.MustParsePrefix("192.0.0.0/24"),
}

// isPermittedOAuthIP allows explicit loopback use for local MCP test servers,
// while metadata-discovered external hostnames cannot turn into private,
// loopback, link-local or otherwise reserved destinations at dial time.
func isPermittedOAuthIP(ip net.IP, explicitLoopback bool) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return explicitLoopback
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range blockedOAuthNetworks {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func dialVerifiedOAuthAddress(ctx context.Context, _ string, address string) (net.Conn, error) {
	hostname, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid dial address", errHostDenied)
	}
	if hostname == "" {
		return nil, fmt.Errorf("%w: missing hostname", errHostDenied)
	}
	var ips []net.IP
	if ip := net.ParseIP(hostname); ip != nil {
		ips = []net.IP{ip}
	} else {
		answers, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, hostname)
		if lookupErr != nil {
			return nil, fmt.Errorf("%w: DNS lookup failed", errHostDenied)
		}
		for _, answer := range answers {
			ips = append(ips, answer.IP)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("%w: no DNS answers", errHostDenied)
	}
	// A mixed public/private answer is rejected in full rather than letting
	// the dialer select a private IP after a benign public-address check.
	for _, ip := range ips {
		if !isPermittedOAuthIP(ip, isLoopbackHost(hostname)) {
			return nil, fmt.Errorf("%w: blocked DNS address", errHostDenied)
		}
	}
	dialer := net.Dialer{}
	for _, ip := range ips {
		conn, dialErr := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		err = dialErr
	}
	return nil, fmt.Errorf("OAuth verified address connection failed: %w", err)
}
