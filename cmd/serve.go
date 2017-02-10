// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"crypto/tls"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Unknwon/com"
	git "github.com/gogits/git-module"
	gouuid "github.com/satori/go.uuid"
	"github.com/urfave/cli"
	log "gopkg.in/clog.v1"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/httplib"
	"github.com/gogits/gogs/modules/setting"
)

const (
	_ACCESS_DENIED_MESSAGE = "Repository does not exist or you do not have access"
)

var CmdServ = cli.Command{
	Name:        "serv",
	Usage:       "This command should only be called by SSH shell",
	Description: `Serv provide access auth for repositories`,
	Action:      runServ,
	Flags: []cli.Flag{
		stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
	},
}

func setup(logPath string) {
	setting.NewContext()
	setting.NewService()
	log.New(log.FILE, log.FileConfig{
		Filename: filepath.Join(setting.LogRootPath, logPath),
		FileRotationConfig: log.FileRotationConfig{
			Rotate:  true,
			Daily:   true,
			MaxDays: 3,
		},
	})
	log.Delete(log.CONSOLE) // Remove primary logger

	models.LoadConfigs()

	if setting.UseSQLite3 || setting.UseTiDB {
		workDir, _ := setting.WorkDir()
		os.Chdir(workDir)
	}

	models.SetEngine()
}

func parseCmd(cmd string) (string, string) {
	ss := strings.SplitN(cmd, " ", 2)
	if len(ss) != 2 {
		return "", ""
	}
	return ss[0], strings.Replace(ss[1], "'/", "'", 1)
}

func checkDeployKey(key *models.PublicKey, repo *models.Repository) {
	// Check if this deploy key belongs to current repository.
	if !models.HasDeployKey(key.ID, repo.ID) {
		fail("Key access denied", "Deploy key access denied: [key_id: %d, repo_id: %d]", key.ID, repo.ID)
	}

	// Update deploy key activity.
	deployKey, err := models.GetDeployKeyByRepo(key.ID, repo.ID)
	if err != nil {
		fail("Internal error", "GetDeployKey: %v", err)
	}

	deployKey.Updated = time.Now()
	if err = models.UpdateDeployKey(deployKey); err != nil {
		fail("Internal error", "UpdateDeployKey: %v", err)
	}
}

var (
	allowedCommands = map[string]models.AccessMode{
		"git-upload-pack":    models.ACCESS_MODE_READ,
		"git-upload-archive": models.ACCESS_MODE_READ,
		"git-receive-pack":   models.ACCESS_MODE_WRITE,
	}
)

func fail(userMessage, logMessage string, args ...interface{}) {
	fmt.Fprintln(os.Stderr, "Gogs:", userMessage)

	if len(logMessage) > 0 {
		if !setting.ProdMode {
			fmt.Fprintf(os.Stderr, logMessage+"\n", args...)
		}
		log.Fatal(3, logMessage, args...)
	}

	log.Shutdown()
	os.Exit(1)
}

func handleUpdateTask(uuid string, user, repoUser *models.User, reponame string, isWiki bool) {
	task, err := models.GetUpdateTaskByUUID(uuid)
	if err != nil {
		if models.IsErrUpdateTaskNotExist(err) {
			log.Trace("No update task is presented: %s", uuid)
			return
		}
		log.Fatal(2, "GetUpdateTaskByUUID: %v", err)
	} else if err = models.DeleteUpdateTaskByUUID(uuid); err != nil {
		log.Fatal(2, "DeleteUpdateTaskByUUID: %v", err)
	}

	if isWiki {
		return
	}

	if err = models.PushUpdate(models.PushUpdateOptions{
		RefFullName:  task.RefName,
		OldCommitID:  task.OldCommitID,
		NewCommitID:  task.NewCommitID,
		PusherID:     user.ID,
		PusherName:   user.Name,
		RepoUserName: repoUser.Name,
		RepoName:     reponame,
	}); err != nil {
		log.Error(2, "Update: %v", err)
	}

	// Ask for running deliver hook and test pull request tasks.
	reqURL := setting.LocalURL + repoUser.Name + "/" + reponame + "/tasks/trigger?branch=" +
		strings.TrimPrefix(task.RefName, git.BRANCH_PREFIX) + "&secret=" + base.EncodeMD5(repoUser.Salt) + "&pusher=" + com.ToStr(user.ID)
	log.Trace("Trigger task: %s", reqURL)

	resp, err := httplib.Head(reqURL).SetTLSClientConfig(&tls.Config{
		InsecureSkipVerify: true,
	}).Response()
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			log.Error(2, "Fail to trigger task: not 2xx response code")
		}
	} else {
		log.Error(2, "Fail to trigger task: %v", err)
	}
}

func runServ(c *cli.Context) error {
	if c.IsSet("config") {
		setting.CustomConf = c.String("config")
	}

	setup("serv.log")

	if setting.SSH.Disabled {
		println("Gogs: SSH has been disabled")
		return nil
	}

	if len(c.Args()) < 1 {
		fail("Not enough arguments", "Not enough arguments")
	}

	cmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	if len(cmd) == 0 {
		println("Hi there, You've successfully authenticated, but Gogs does not provide shell access.")
		println("If this is unexpected, please log in with password and setup Gogs under another user.")
		return nil
	}

	verb, args := parseCmd(cmd)
	repoPath := strings.ToLower(strings.Trim(args, "'"))
	rr := strings.SplitN(repoPath, "/", 2)
	if len(rr) != 2 {
		fail("Invalid repository path", "Invalid repository path: %v", args)
	}
	username := strings.ToLower(rr[0])
	reponame := strings.ToLower(strings.TrimSuffix(rr[1], ".git"))

	isWiki := false
	if strings.HasSuffix(reponame, ".wiki") {
		isWiki = true
		reponame = reponame[:len(reponame)-5]
	}

	repoUser, err := models.GetUserByName(username)
	if err != nil {
		if models.IsErrUserNotExist(err) {
			fail("Repository owner does not exist", "Unregistered owner: %s", username)
		}
		fail("Internal error", "Failed to get repository owner (%s): %v", username, err)
	}

	repo, err := models.GetRepositoryByName(repoUser.ID, reponame)
	if err != nil {
		if models.IsErrRepoNotExist(err) {
			fail(_ACCESS_DENIED_MESSAGE, "Repository does not exist: %s/%s", repoUser.Name, reponame)
		}
		fail("Internal error", "Failed to get repository: %v", err)
	}

	requestedMode, has := allowedCommands[verb]
	if !has {
		fail("Unknown git command", "Unknown git command %s", verb)
	}

	// Prohibit push to mirror repositories.
	if requestedMode > models.ACCESS_MODE_READ && repo.IsMirror {
		fail("mirror repository is read-only", "")
	}

	// Allow anonymous (user is nil) clone for public repositories.
	var user *models.User

	key, err := models.GetPublicKeyByID(com.StrTo(strings.TrimPrefix(c.Args()[0], "key-")).MustInt64())
	if err != nil {
		fail("Invalid key ID", "Invalid key ID [%s]: %v", c.Args()[0], err)
	}

	if requestedMode == models.ACCESS_MODE_WRITE || repo.IsPrivate {
		// Check deploy key or user key.
		if key.IsDeployKey() {
			if key.Mode < requestedMode {
				fail("Key permission denied", "Cannot push with deployment key: %d", key.ID)
			}
			checkDeployKey(key, repo)
		} else {
			user, err = models.GetUserByKeyID(key.ID)
			if err != nil {
				fail("internal error", "Failed to get user by key ID(%d): %v", key.ID, err)
			}

			mode, err := models.AccessLevel(user, repo)
			if err != nil {
				fail("Internal error", "Fail to check access: %v", err)
			} else if mode < requestedMode {
				clientMessage := _ACCESS_DENIED_MESSAGE
				if mode >= models.ACCESS_MODE_READ {
					clientMessage = "You do not have sufficient authorization for this action"
				}
				fail(clientMessage,
					"User %s does not have level %v access to repository %s",
					user.Name, requestedMode, repoPath)
			}
		}
	} else {
		// Check if the key can access to the repository in case of it is a deploy key (a deploy keys != user key).
		// A deploy key doesn't represent a signed in user, so in a site with Service.RequireSignInView activated
		// we should give read access only in repositories where this deploy key is in use. In other case, a server
		// or system using an active deploy key can get read access to all the repositories in a Gogs service.
		if key.IsDeployKey() && setting.Service.RequireSignInView {
			checkDeployKey(key, repo)
		}
	}

	uuid := gouuid.NewV4().String()
	os.Setenv("uuid", uuid)

	// Special handle for Windows.
	if setting.IsWindows {
		verb = strings.Replace(verb, "-", " ", 1)
	}

	var gitcmd *exec.Cmd
	verbs := strings.Split(verb, " ")
	if len(verbs) == 2 {
		gitcmd = exec.Command(verbs[0], verbs[1], repoPath)
	} else {
		gitcmd = exec.Command(verb, repoPath)
	}
	gitcmd.Dir = setting.RepoRootPath
	gitcmd.Stdout = os.Stdout
	gitcmd.Stdin = os.Stdin
	gitcmd.Stderr = os.Stderr
	if err = gitcmd.Run(); err != nil {
		fail("Internal error", "Failed to execute git command: %v", err)
	}

	if requestedMode == models.ACCESS_MODE_WRITE {
		handleUpdateTask(uuid, user, repoUser, reponame, isWiki)
	}

	// Update user key activity.
	if key.ID > 0 {
		key, err := models.GetPublicKeyByID(key.ID)
		if err != nil {
			fail("Internal error", "GetPublicKeyByID: %v", err)
		}

		key.Updated = time.Now()
		if err = models.UpdatePublicKey(key); err != nil {
			fail("Internal error", "UpdatePublicKey: %v", err)
		}
	}

	return nil
}
