// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package netutil

import (
	"fmt"
	"net"
)

var localCIDRs []*net.IPNet

func init() {
	// Parsing hardcoded CIDR strings should never fail, if in case it does, let's
	// fail it at start.
	rawCIDRs := []string{
		// https://datatracker.ietf.org/doc/html/rfc5735:
		"127.0.0.0/8",        // Loopback
		"0.0.0.0/8",          // "This" network
		"100.64.0.0/10",      // Shared address space
		"169.254.0.0/16",     // Link local
		"172.16.0.0/12",      // Private-use networks
		"192.0.0.0/24",       // IETF Protocol assignments
		"192.0.2.0/24",       // TEST-NET-1
		"192.88.99.0/24",     // 6to4 Relay anycast
		"192.168.0.0/16",     // Private-use networks
		"198.18.0.0/15",      // Network interconnect
		"198.51.100.0/24",    // TEST-NET-2
		"203.0.113.0/24",     // TEST-NET-3
		"255.255.255.255/32", // Limited broadcast

		// https://datatracker.ietf.org/doc/html/rfc1918:
		"10.0.0.0/8", // Private-use networks

		// https://datatracker.ietf.org/doc/html/rfc6890:
		"::1/128",   // Loopback
		"FC00::/7",  // Unique local address
		"FE80::/10", // Multicast address
	}
	for _, raw := range rawCIDRs {
		_, cidr, err := net.ParseCIDR(raw)
		if err != nil {
			panic(fmt.Sprintf("parse CIDR %q: %v", raw, err))
		}
		localCIDRs = append(localCIDRs, cidr)
	}
}

// IsBlockedLocalHostname returns true if given hostname is resolved to a local
// network address that is implicitly blocked (i.e. not exempted from the
// allowlist).
func IsBlockedLocalHostname(hostname string, allowlist []string) bool {
	for _, allow := range allowlist {
		if hostname == allow {
			return false
		}
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return true
	}
	for _, ip := range ips {
		for _, cidr := range localCIDRs {
			if cidr.Contains(ip) {
				return true
			}
		}
	}
	return false
}
