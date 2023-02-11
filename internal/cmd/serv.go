// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/unknwon/com"
	"github.com/urfave/cli"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db"
)

const (
	_ACCESS_DENIED_MESSAGE = "Repository does not exist or you do not have access"
)

var Serv = cli.Command{
	Name:        "serv",
	Usage:       "This command should only be called by SSH shell",
	Description: `Serv provide access auth for repositories`,
	Action:      runServ,
	Flags: []cli.Flag{
		stringFlag("config, c", "", "Custom configuration file path"),
	},
}

// fail prints user message to the Git client (i.e. os.Stderr) and
// logs error message on the server side. When not in "prod" mode,
// error message is also printed to the client for easier debugging.
func fail(userMessage, errMessage string, args ...any) {
	_, _ = fmt.Fprintln(os.Stderr, "Gogs:", userMessage)

	if len(errMessage) > 0 {
		if !conf.IsProdMode() {
			fmt.Fprintf(os.Stderr, errMessage+"\n", args...)
		}
		log.Error(errMessage, args...)
	}

	log.Stop()
	os.Exit(1)
}

func setup(c *cli.Context, logFile string, connectDB bool) {
	conf.HookMode = true

	var customConf string
	if c.IsSet("config") {
		customConf = c.String("config")
	} else if c.GlobalIsSet("config") {
		customConf = c.GlobalString("config")
	}

	err := conf.Init(customConf)
	if err != nil {
		fail("Internal error", "Failed to init configuration: %v", err)
	}
	conf.InitLogging(true)

	level := log.LevelTrace
	if conf.IsProdMode() {
		level = log.LevelError
	}

	err = log.NewFile(log.FileConfig{
		Level:    level,
		Filename: filepath.Join(conf.Log.RootPath, "hooks", logFile),
		FileRotationConfig: log.FileRotationConfig{
			Rotate:  true,
			Daily:   true,
			MaxDays: 3,
		},
	})
	if err != nil {
		fail("Internal error", "Failed to init file logger: %v", err)
	}
	log.Remove(log.DefaultConsoleName) // Remove the primary logger

	if !connectDB {
		return
	}

	if conf.UseSQLite3 {
		_ = os.Chdir(conf.WorkDir())
	}

	if _, err := db.SetEngine(); err != nil {
		fail("Internal error", "Failed to set database engine: %v", err)
	}
}

func parseSSHCmd(cmd string) (string, string) {
	ss := strings.SplitN(cmd, " ", 2)
	if len(ss) != 2 {
		return "", ""
	}
	return ss[0], strings.Replace(ss[1], "'/", "'", 1)
}

func checkDeployKey(key *db.PublicKey, repo *db.Repository) {
	// Check if this deploy key belongs to current repository.
	if !db.HasDeployKey(key.ID, repo.ID) {
		fail("Key access denied", "Deploy key access denied: [key_id: %d, repo_id: %d]", key.ID, repo.ID)
	}

	// Update deploy key activity.
	deployKey, err := db.GetDeployKeyByRepo(key.ID, repo.ID)
	if err != nil {
		fail("Internal error", "GetDeployKey: %v", err)
	}

	deployKey.Updated = time.Now()
	if err = db.UpdateDeployKey(deployKey); err != nil {
		fail("Internal error", "UpdateDeployKey: %v", err)
	}
}

var allowedCommands = map[string]db.AccessMode{
	"git-upload-pack":    db.AccessModeRead,
	"git-upload-archive": db.AccessModeRead,
	"git-receive-pack":   db.AccessModeWrite,
}

func runServ(c *cli.Context) error {
	ctx := context.Background()
	setup(c, "serv.log", true)

	if conf.SSH.Disabled {
		println("Gogs: SSH has been disabled")
		return nil
	}

	if len(c.Args()) < 1 {
		fail("Not enough arguments", "Not enough arguments")
	}

	sshCmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	if sshCmd == "" {
		println("Hi there, You've successfully authenticated, but Gogs does not provide shell access.")
		println("If this is unexpected, please log in with password and setup Gogs under another user.")
		return nil
	}

	verb, args := parseSSHCmd(sshCmd)
	repoFullName := strings.ToLower(strings.Trim(args, "'"))
	repoFields := strings.SplitN(repoFullName, "/", 2)
	if len(repoFields) != 2 {
		fail("Invalid repository path", "Invalid repository path: %v", args)
	}
	ownerName := strings.ToLower(repoFields[0])
	repoName := strings.TrimSuffix(strings.ToLower(repoFields[1]), ".git")
	repoName = strings.TrimSuffix(repoName, ".wiki")

	owner, err := db.Users.GetByUsername(ctx, ownerName)
	if err != nil {
		if db.IsErrUserNotExist(err) {
			fail("Repository owner does not exist", "Unregistered owner: %s", ownerName)
		}
		fail("Internal error", "Failed to get repository owner '%s': %v", ownerName, err)
	}

	repo, err := db.GetRepositoryByName(owner.ID, repoName)
	if err != nil {
		if db.IsErrRepoNotExist(err) {
			fail(_ACCESS_DENIED_MESSAGE, "Repository does not exist: %s/%s", owner.Name, repoName)
		}
		fail("Internal error", "Failed to get repository: %v", err)
	}
	repo.Owner = owner

	requestMode, ok := allowedCommands[verb]
	if !ok {
		fail("Unknown git command", "Unknown git command '%s'", verb)
	}

	// Prohibit push to mirror repositories.
	if requestMode > db.AccessModeRead && repo.IsMirror {
		fail("Mirror repository is read-only", "")
	}

	// Allow anonymous (user is nil) clone for public repositories.
	var user *db.User

	key, err := db.GetPublicKeyByID(com.StrTo(strings.TrimPrefix(c.Args()[0], "key-")).MustInt64())
	if err != nil {
		fail("Invalid key ID", "Invalid key ID '%s': %v", c.Args()[0], err)
	}

	if requestMode == db.AccessModeWrite || repo.IsPrivate {
		// Check deploy key or user key.
		if key.IsDeployKey() {
			if key.Mode < requestMode {
				fail("Key permission denied", "Cannot push with deployment key: %d", key.ID)
			}
			checkDeployKey(key, repo)
		} else {
			user, err = db.Users.GetByKeyID(ctx, key.ID)
			if err != nil {
				fail("Internal error", "Failed to get user by key ID '%d': %v", key.ID, err)
			}

			mode := db.Perms.AccessMode(ctx, user.ID, repo.ID,
				db.AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			)
			if mode < requestMode {
				clientMessage := _ACCESS_DENIED_MESSAGE
				if mode >= db.AccessModeRead {
					clientMessage = "You do not have sufficient authorization for this action"
				}
				fail(clientMessage,
					"User '%s' does not have level '%v' access to repository '%s'",
					user.Name, requestMode, repoFullName)
			}
		}
	} else {
		// Check if the key can access to the repository in case of it is a deploy key (a deploy keys != user key).
		// A deploy key doesn't represent a signed in user, so in a site with Auth.RequireSignInView enabled,
		// we should give read access only in repositories where this deploy key is in use. In other cases,
		// a server or system using an active deploy key can get read access to all repositories on a Gogs instance.
		if key.IsDeployKey() && conf.Auth.RequireSigninView {
			checkDeployKey(key, repo)
		}
	}

	// Update user key activity.
	if key.ID > 0 {
		key, err := db.GetPublicKeyByID(key.ID)
		if err != nil {
			fail("Internal error", "GetPublicKeyByID: %v", err)
		}

		key.Updated = time.Now()
		if err = db.UpdatePublicKey(key); err != nil {
			fail("Internal error", "UpdatePublicKey: %v", err)
		}
	}

	// Special handle for Windows.
	if conf.IsWindowsRuntime() {
		verb = strings.Replace(verb, "-", " ", 1)
	}

	var gitCmd *exec.Cmd
	verbs := strings.Split(verb, " ")
	if len(verbs) == 2 {
		gitCmd = exec.Command(verbs[0], verbs[1], repoFullName)
	} else {
		gitCmd = exec.Command(verb, repoFullName)
	}
	if requestMode == db.AccessModeWrite {
		gitCmd.Env = append(os.Environ(), db.ComposeHookEnvs(db.ComposeHookEnvsOptions{
			AuthUser:  user,
			OwnerName: owner.Name,
			OwnerSalt: owner.Salt,
			RepoID:    repo.ID,
			RepoName:  repo.Name,
			RepoPath:  repo.RepoPath(),
		})...)
	}
	gitCmd.Dir = conf.Repository.Root
	gitCmd.Stdout = os.Stdout
	gitCmd.Stdin = os.Stdin
	gitCmd.Stderr = os.Stderr
	if err = gitCmd.Run(); err != nil {
		fail("Internal error", "Failed to execute git command: %v", err)
	}

	return nil
}
