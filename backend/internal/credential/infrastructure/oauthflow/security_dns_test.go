package oauthflow

import (
	"net"
	"net/url"
	"testing"
)

func TestOAuthDiscoveryRejectsPrivateAndReservedRemoteNetworkTargets(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"10.0.0.3", "192.168.1.20", "172.16.10.3", "169.254.169.254", "100.64.0.8", "198.18.0.10", "[fc00::1]"} {
		u, err := url.Parse("https://" + host + "/oauth/metadata")
		if err != nil {
			t.Fatal(err)
		}
		if err := checkRequestURL(u); err == nil {
			t.Errorf("private OAuth metadata target allowed: %s", host)
		}
	}
	for _, test := range []struct {
		ip            string
		allowLoopback bool
		want          bool
	}{
		{"127.0.0.1", true, true},
		{"127.0.0.1", false, false},
		{"::1", true, true},
		{"::1", false, false},
		{"10.1.1.1", false, false},
		{"192.168.1.1", false, false},
		{"100.64.0.2", false, false},
		{"169.254.169.254", false, false},
		{"8.8.8.8", false, true},
	} {
		if got := isPermittedOAuthIP(net.ParseIP(test.ip), test.allowLoopback); got != test.want {
			t.Errorf("isPermittedOAuthIP(%q,%t)=%t want %t", test.ip, test.allowLoopback, got, test.want)
		}
	}
}
