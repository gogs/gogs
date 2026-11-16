package main

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"

	"gogs.io/gogs/internal/conf"
)

var healthcheckCommand = cli.Command{
	Name:  "healthcheck",
	Usage: "Check whether the web server is healthy",
	Description: `Requests the health check of the web server at the address set in [server],
and exits with a non-zero status when the server is unreachable or unhealthy,
e.g., for the HEALTHCHECK of a Docker image.`,
	Action: runHealthcheck,
	Flags: []cli.Flag{
		stringFlag("config, c", filepath.Join(conf.CustomDir(), "conf", "app.ini"), "Custom configuration file path"),
	},
}

func runHealthcheck(ctx context.Context, cmd *cli.Command) error {
	err := conf.InitServer(configFromLineage(cmd))
	if err != nil {
		return errors.Wrap(err, "init configuration")
	}

	client, url, err := healthcheckTarget(conf.Server.Protocol, conf.Server.HTTPAddr, conf.Server.HTTPPort)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrap(err, "new request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "request health check")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return errors.Newf("unhealthy: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// healthcheckTarget returns the client and the URL that reach the health check
// of a web server listening with the given protocol, address and port.
func healthcheckTarget(protocol, addr string, port int) (*http.Client, string, error) {
	// The server is checked directly, never through a proxy from the environment.
	transport := &http.Transport{}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	switch protocol {
	case "http", "https":
		host := addr
		if ip := net.ParseIP(addr); addr == "" || (ip != nil && ip.IsUnspecified()) {
			host = "127.0.0.1"
			if ip != nil && ip.To4() == nil {
				host = "::1"
			}
		}
		if protocol == "https" {
			// The certificate is issued for the external domain, not the listen address.
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		}
		return client, protocol + "://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/healthcheck", nil

	case "unix":
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", addr)
		}
		return client, "http://unix/healthcheck", nil
	}
	return nil, "", errors.Newf("health check is not supported for server protocol %q", protocol)
}
