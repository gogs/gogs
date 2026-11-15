package form

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

func TestParseRemoteAddr(t *testing.T) {
	before := conf.Repository.EnableLocalPathMigration
	conf.Repository.EnableLocalPathMigration = true
	beforeAllowlist := conf.Security.LocalNetworkAllowlist
	conf.Security.LocalNetworkAllowlist = []string{"gitlab.example.com"}
	t.Cleanup(func() {
		conf.Repository.EnableLocalPathMigration = before
		conf.Security.LocalNetworkAllowlist = beforeAllowlist
	})

	admin := &database.User{IsAdmin: true}
	regular := &database.User{}
	localDir := t.TempDir()

	tests := []struct {
		name     string
		addr     string
		user     *database.User
		wantAddr string
		wantErr  error
	}{
		{name: "scp-like address", addr: "git@gitlab.example.com:owner/repo.git", user: regular, wantErr: database.ErrInvalidCloneAddr{IsSSHAddress: true}},
		{name: "scp-like address without user", addr: "gitlab.example.com:owner/repo.git", user: regular, wantErr: database.ErrInvalidCloneAddr{IsSSHAddress: true}},
		{name: "scp-like address for an admin", addr: "git@gitlab.example.com:owner/repo.git", user: admin, wantErr: database.ErrInvalidCloneAddr{IsSSHAddress: true}},
		{name: "ssh URL", addr: "ssh://git@gitlab.example.com:2222/owner/repo.git", user: regular, wantErr: database.ErrInvalidCloneAddr{IsSSHAddress: true}},
		{name: "git+ssh URL", addr: "git+ssh://git@gitlab.example.com/owner/repo.git", user: regular, wantErr: database.ErrInvalidCloneAddr{IsSSHAddress: true}},
		{name: "https URL", addr: "https://gitlab.example.com/owner/repo.git", user: regular, wantAddr: "https://gitlab.example.com/owner/repo.git"},
		{name: "git URL", addr: "git://gitlab.example.com/owner/repo.git", user: regular, wantAddr: "git://gitlab.example.com/owner/repo.git"},
		{name: "local path for a regular user", addr: localDir, user: regular, wantErr: database.ErrInvalidCloneAddr{IsPermissionDenied: true}},
		{name: "local path for an admin", addr: localDir, user: admin, wantAddr: localDir},
		{name: "local path with a colon", addr: localDir + "/a:b", user: admin, wantErr: database.ErrInvalidCloneAddr{IsInvalidPath: true}},
		{name: "Windows drive letter", addr: `C:\repos\repo.git`, user: admin, wantErr: database.ErrInvalidCloneAddr{IsInvalidPath: true}},
		{name: "file URL", addr: "file:///srv/repo.git", user: admin, wantErr: database.ErrInvalidCloneAddr{IsInvalidPath: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseRemoteAddr(ParseRemoteAddrOptions{CloneAddr: test.addr, User: test.user})
			assert.Equal(t, test.wantErr, err)
			assert.Equal(t, test.wantAddr, got)
		})
	}
}
