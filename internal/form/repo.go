package form

import (
	"net/url"
	"strings"

	"github.com/go-macaron/binding"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/netx"
	"gogs.io/gogs/internal/osx"
)

// _______________________________________    _________.______________________ _______________.___.
// \______   \_   _____/\______   \_____  \  /   _____/|   \__    ___/\_____  \\______   \__  |   |
//  |       _/|    __)_  |     ___//   |   \ \_____  \ |   | |    |    /   |   \|       _//   |   |
//  |    |   \|        \ |    |   /    |    \/        \|   | |    |   /    |    \    |   \\____   |
//  |____|_  /_______  / |____|   \_______  /_______  /|___| |____|   \_______  /____|_  // ______|
//         \/        \/                   \/        \/                        \/       \/ \/

type MigrateRepo struct {
	CloneAddr    string `json:"clone_addr" binding:"Required"`
	AuthUsername string `json:"auth_username"`
	AuthPassword string `json:"auth_password"`
	UID          int64  `json:"uid" binding:"Required"`
	RepoName     string `json:"repo_name" binding:"Required;AlphaDashDot;MaxSize(100)"`
	Mirror       bool   `json:"mirror"`
	Private      bool   `json:"private"`
	Unlisted     bool   `json:"unlisted"`
	Description  string `json:"description" binding:"MaxSize(512)"`
}

func (f *MigrateRepo) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}

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
		// To prevent CRLF injection in git protocol, see https://github.com/gogs/gogs/issues/6413
		if u.Scheme == "git" && (strings.Contains(remoteAddr, "%0d") || strings.Contains(remoteAddr, "%0a")) {
			return "", database.ErrInvalidCloneAddr{IsURLError: true}
		}
		remoteAddr = u.String()
	} else if !options.User.CanImportLocal() {
		return "", database.ErrInvalidCloneAddr{IsPermissionDenied: true}
	} else if !osx.IsDir(remoteAddr) {
		return "", database.ErrInvalidCloneAddr{IsInvalidPath: true}
	}

	return remoteAddr, nil
}

// .___
// |   | ______ ________ __   ____
// |   |/  ___//  ___/  |  \_/ __ \
// |   |\___ \ \___ \|  |  /\  ___/
// |___/____  >____  >____/  \___  >
//          \/     \/            \/

type NewIssue struct {
	Title       string `binding:"Required;MaxSize(255)"`
	LabelIDs    string `form:"label_ids"`
	MilestoneID int64
	AssigneeID  int64
	Content     string
	Files       []string
}

func (f *NewIssue) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validate(errs, ctx.Data, f, ctx.Locale)
}
