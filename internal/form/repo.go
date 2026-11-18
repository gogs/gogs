package form

import (
	"net/url"
	"strings"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/netx"
	"gogs.io/gogs/internal/osx"
)

type ParseRemoteAddrOptions struct {
	CloneAddr    string
	User         *database.User
	AuthUsername string
	AuthPassword string
}

// ParseRemoteAddr checks if given remote address is valid,
// and returns composed URL with needed username and password.
// It also checks if given user has permission when remote address
// is actually a local path.
func ParseRemoteAddr(options ParseRemoteAddrOptions) (string, error) {
	remoteAddr := strings.TrimSpace(options.CloneAddr)

	// Remote address can be HTTP/HTTPS/Git URL or local path.
	if strings.HasPrefix(remoteAddr, "http://") ||
		strings.HasPrefix(remoteAddr, "https://") ||
		strings.HasPrefix(remoteAddr, "git://") {
		u, err := url.Parse(remoteAddr)
		if err != nil {
			return "", database.ErrInvalidCloneAddr{IsURLError: true}
		}

		if netx.IsBlockedLocalHostname(u.Hostname(), conf.Security.LocalNetworkAllowlist) {
			return "", database.ErrInvalidCloneAddr{IsBlockedLocalAddress: true}
		}

		if len(options.AuthUsername)+len(options.AuthPassword) > 0 {
			u.User = url.UserPassword(options.AuthUsername, options.AuthPassword)
		}
		// To prevent CRLF injection in Git protocol, see https://github.com/gogs/gogs/issues/6413.
		if u.Scheme == "git" && (strings.Contains(remoteAddr, "%0d") || strings.Contains(remoteAddr, "%0a")) {
			return "", database.ErrInvalidCloneAddr{IsURLError: true}
		}
		remoteAddr = u.String()
	} else if isSSHAddr(remoteAddr) {
		return "", database.ErrInvalidCloneAddr{IsSSHAddress: true}
	} else if !options.User.CanImportLocal() {
		return "", database.ErrInvalidCloneAddr{IsPermissionDenied: true}
	} else if !osx.IsDir(remoteAddr) {
		return "", database.ErrInvalidCloneAddr{IsInvalidPath: true}
	}

	return remoteAddr, nil
}

// isSSHAddr reports whether addr is an SSH address that Git would accept, either
// a URL with an SSH scheme or the scp-like syntax, e.g., "git@example.com:owner/repo.git".
func isSSHAddr(addr string) bool {
	if scheme, _, ok := strings.Cut(addr, "://"); ok {
		switch strings.ToLower(scheme) {
		case "ssh", "git+ssh", "ssh+git":
			return true
		}
		return false
	}

	// Git treats an address as scp-like when it has a colon before any slash,
	// except for a Windows drive letter, e.g., `C:\repos`.
	colon := strings.IndexByte(addr, ':')
	return colon > 1 && !strings.ContainsAny(addr[:colon], `/\`)
}
