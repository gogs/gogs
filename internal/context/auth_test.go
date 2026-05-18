package context

import (
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
)

func TestIsRequestFromTrustedProxy(t *testing.T) {
	mustCIDR := func(s string) *net.IPNet {
		_, n, err := net.ParseCIDR(s)
		require.NoError(t, err)
		return n
	}

	original := conf.Auth.TrustedProxyNets
	t.Cleanup(func() { conf.Auth.TrustedProxyNets = original })
	conf.Auth.TrustedProxyNets = []*net.IPNet{
		mustCIDR("127.0.0.0/8"),
		mustCIDR("::1/128"),
		mustCIDR("10.1.0.0/16"),
	}

	tests := []struct {
		name       string
		remoteAddr string
		want       bool
	}{
		{name: "loopback IPv4 with port", remoteAddr: "127.0.0.1:54321", want: true},
		{name: "loopback IPv6 with port", remoteAddr: "[::1]:54321", want: true},
		{name: "within configured CIDR", remoteAddr: "10.1.2.3:8080", want: true},
		{name: "outside configured CIDR", remoteAddr: "203.0.113.5:443", want: false},
		{name: "bare IP without port", remoteAddr: "127.0.0.1", want: true},
		{name: "unparseable remote", remoteAddr: "not-an-ip", want: false},
		{name: "empty remote", remoteAddr: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{RemoteAddr: tc.remoteAddr}
			require.Equal(t, tc.want, isRequestFromTrustedProxy(req))
		})
	}
}
