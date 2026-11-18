package database

import (
	"path/filepath"
	"strconv"

	"gogs.io/gogs/internal/cryptox"
)

const (
	EnvAuthUserID          = "GOGS_AUTH_USER_ID"
	EnvAuthUserName        = "GOGS_AUTH_USER_NAME"
	EnvAuthUserEmail       = "GOGS_AUTH_USER_EMAIL"
	EnvRepoOwnerName       = "GOGS_REPO_OWNER_NAME"
	EnvRepoOwnerSaltMd5    = "GOGS_REPO_OWNER_SALT_MD5"
	EnvRepoID              = "GOGS_REPO_ID"
	EnvRepoName            = "GOGS_REPO_NAME"
	EnvRepoCustomHooksPath = "GOGS_REPO_CUSTOM_HOOKS_PATH"
)

type ComposeHookEnvsOptions struct {
	AuthUser  *User
	OwnerName string
	OwnerSalt string
	RepoID    int64
	RepoName  string
	RepoPath  string
}

func ComposeHookEnvs(opts ComposeHookEnvsOptions) []string {
	envs := []string{
		"SSH_ORIGINAL_COMMAND=1",
		EnvAuthUserID + "=" + strconv.FormatInt(opts.AuthUser.ID, 10),
		EnvAuthUserName + "=" + opts.AuthUser.Name,
		EnvAuthUserEmail + "=" + opts.AuthUser.Email,
		EnvRepoOwnerName + "=" + opts.OwnerName,
		EnvRepoOwnerSaltMd5 + "=" + cryptox.MD5(opts.OwnerSalt),
		EnvRepoID + "=" + strconv.FormatInt(opts.RepoID, 10),
		EnvRepoName + "=" + opts.RepoName,
		EnvRepoCustomHooksPath + "=" + filepath.Join(opts.RepoPath, "custom_hooks"),
	}
	return envs
}
