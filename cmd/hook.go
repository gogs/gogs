// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"
	"github.com/urfave/cli"
	log "gopkg.in/clog.v1"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/httplib"
	"github.com/gogits/gogs/pkg/mailer"
	"github.com/gogits/gogs/pkg/setting"
	"github.com/gogits/gogs/pkg/template"
	http "github.com/gogits/gogs/routes/repo"
)

var (
	Hook = cli.Command{
		Name:        "hook",
		Usage:       "Delegate commands to corresponding Git hooks",
		Description: "All sub-commands should only be called by Git",
		Flags: []cli.Flag{
			stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		},
		Subcommands: []cli.Command{
			subcmdHookPreReceive,
			subcmdHookUpadte,
			subcmdHookPostReceive,
		},
	}

	subcmdHookPreReceive = cli.Command{
		Name:        "pre-receive",
		Usage:       "Delegate pre-receive Git hook",
		Description: "This command should only be called by Git",
		Action:      runHookPreReceive,
	}
	subcmdHookUpadte = cli.Command{
		Name:        "update",
		Usage:       "Delegate update Git hook",
		Description: "This command should only be called by Git",
		Action:      runHookUpdate,
	}
	subcmdHookPostReceive = cli.Command{
		Name:        "post-receive",
		Usage:       "Delegate post-receive Git hook",
		Description: "This command should only be called by Git",
		Action:      runHookPostReceive,
	}
)

func runHookPreReceive(c *cli.Context) error {
	if len(os.Getenv("SSH_ORIGINAL_COMMAND")) == 0 {
		return nil
	}
	setup(c, "hooks/pre-receive.log", true)

	isWiki := strings.Contains(os.Getenv(http.ENV_REPO_CUSTOM_HOOKS_PATH), ".wiki.git/")

	buf := bytes.NewBuffer(nil)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		buf.Write(scanner.Bytes())
		buf.WriteByte('\n')

		if isWiki {
			continue
		}

		fields := bytes.Fields(scanner.Bytes())
		if len(fields) != 3 {
			continue
		}
		oldCommitID := string(fields[0])
		newCommitID := string(fields[1])
		branchName := strings.TrimPrefix(string(fields[2]), git.BRANCH_PREFIX)

		// Branch protection
		repoID := com.StrTo(os.Getenv(http.ENV_REPO_ID)).MustInt64()
		protectBranch, err := models.GetProtectBranchOfRepoByName(repoID, branchName)
		if err != nil {
			if models.IsErrBranchNotExist(err) {
				continue
			}
			fail("Internal error", "GetProtectBranchOfRepoByName [repo_id: %d, branch: %s]: %v", repoID, branchName, err)
		}
		if !protectBranch.Protected {
			continue
		}

		// Whitelist users can bypass require pull request check
		bypassRequirePullRequest := false

		// Check if user is in whitelist when enabled
		userID := com.StrTo(os.Getenv(http.ENV_AUTH_USER_ID)).MustInt64()
		if protectBranch.EnableWhitelist {
			if !models.IsUserInProtectBranchWhitelist(repoID, userID, branchName) {
				fail(fmt.Sprintf("Branch '%s' is protected and you are not in the push whitelist", branchName), "")
			}

			bypassRequirePullRequest = true
		}

		// Check if branch allows direct push
		if !bypassRequirePullRequest && protectBranch.RequirePullRequest {
			fail(fmt.Sprintf("Branch '%s' is protected and commits must be merged through pull request", branchName), "")
		}

		// check and deletion
		if newCommitID == git.EMPTY_SHA {
			fail(fmt.Sprintf("Branch '%s' is protected from deletion", branchName), "")
		}

		// Check force push
		output, err := git.NewCommand("rev-list", "--max-count=1", oldCommitID, "^"+newCommitID).
			RunInDir(models.RepoPath(os.Getenv(http.ENV_REPO_OWNER_NAME), os.Getenv(http.ENV_REPO_NAME)))
		if err != nil {
			fail("Internal error", "Fail to detect force push: %v", err)
		} else if len(output) > 0 {
			fail(fmt.Sprintf("Branch '%s' is protected from force push", branchName), "")
		}
	}

	customHooksPath := filepath.Join(os.Getenv(http.ENV_REPO_CUSTOM_HOOKS_PATH), "pre-receive")
	if !com.IsFile(customHooksPath) {
		return nil
	}

	var hookCmd *exec.Cmd
	if setting.IsWindows {
		hookCmd = exec.Command("bash.exe", "custom_hooks/pre-receive")
	} else {
		hookCmd = exec.Command(customHooksPath)
	}
	hookCmd.Dir = models.RepoPath(os.Getenv(http.ENV_REPO_OWNER_NAME), os.Getenv(http.ENV_REPO_NAME))
	hookCmd.Stdout = os.Stdout
	hookCmd.Stdin = buf
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		fail("Internal error", "Fail to execute custom pre-receive hook: %v", err)
	}
	return nil
}

func runHookUpdate(c *cli.Context) error {
	if len(os.Getenv("SSH_ORIGINAL_COMMAND")) == 0 {
		return nil
	}
	setup(c, "hooks/update.log", false)

	args := c.Args()
	if len(args) != 3 {
		fail("Arguments received are not equal to three", "Arguments received are not equal to three")
	} else if len(args[0]) == 0 {
		fail("First argument 'refName' is empty", "First argument 'refName' is empty")
	}

	customHooksPath := filepath.Join(os.Getenv(http.ENV_REPO_CUSTOM_HOOKS_PATH), "update")
	if !com.IsFile(customHooksPath) {
		return nil
	}

	var hookCmd *exec.Cmd
	if setting.IsWindows {
		hookCmd = exec.Command("bash.exe", append([]string{"custom_hooks/update"}, args...)...)
	} else {
		hookCmd = exec.Command(customHooksPath, args...)
	}
	hookCmd.Dir = models.RepoPath(os.Getenv(http.ENV_REPO_OWNER_NAME), os.Getenv(http.ENV_REPO_NAME))
	hookCmd.Stdout = os.Stdout
	hookCmd.Stdin = os.Stdin
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		fail("Internal error", "Fail to execute custom pre-receive hook: %v", err)
	}
	return nil
}

func runHookPostReceive(c *cli.Context) error {
	if len(os.Getenv("SSH_ORIGINAL_COMMAND")) == 0 {
		return nil
	}
	setup(c, "hooks/post-receive.log", true)

	// Post-receive hook does more than just gather Git information,
	// so we need to setup additional services for email notifications.
	setting.NewPostReceiveHookServices()
	mailer.NewContext()
	mailer.InitMailRender(path.Join(setting.StaticRootPath, "templates/mail"),
		path.Join(setting.CustomPath, "templates/mail"), template.NewFuncMap())

	isWiki := strings.Contains(os.Getenv(http.ENV_REPO_CUSTOM_HOOKS_PATH), ".wiki.git/")

	buf := bytes.NewBuffer(nil)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		buf.Write(scanner.Bytes())
		buf.WriteByte('\n')

		// TODO: support news feeds for wiki
		if isWiki {
			continue
		}

		fields := bytes.Fields(scanner.Bytes())
		if len(fields) != 3 {
			continue
		}

		options := models.PushUpdateOptions{
			OldCommitID:  string(fields[0]),
			NewCommitID:  string(fields[1]),
			RefFullName:  string(fields[2]),
			PusherID:     com.StrTo(os.Getenv(http.ENV_AUTH_USER_ID)).MustInt64(),
			PusherName:   os.Getenv(http.ENV_AUTH_USER_NAME),
			RepoUserName: os.Getenv(http.ENV_REPO_OWNER_NAME),
			RepoName:     os.Getenv(http.ENV_REPO_NAME),
		}
		if err := models.PushUpdate(options); err != nil {
			log.Error(2, "PushUpdate: %v", err)
		}

		// Ask for running deliver hook and test pull request tasks.
		reqURL := setting.LocalURL + options.RepoUserName + "/" + options.RepoName + "/tasks/trigger?branch=" +
			strings.TrimPrefix(options.RefFullName, git.BRANCH_PREFIX) +
			"&secret=" + os.Getenv(http.ENV_REPO_OWNER_SALT_MD5) +
			"&pusher=" + os.Getenv(http.ENV_AUTH_USER_ID)
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

	customHooksPath := filepath.Join(os.Getenv(http.ENV_REPO_CUSTOM_HOOKS_PATH), "post-receive")
	if !com.IsFile(customHooksPath) {
		return nil
	}

	var hookCmd *exec.Cmd
	if setting.IsWindows {
		hookCmd = exec.Command("bash.exe", "custom_hooks/post-receive")
	} else {
		hookCmd = exec.Command(customHooksPath)
	}
	hookCmd.Dir = models.RepoPath(os.Getenv(http.ENV_REPO_OWNER_NAME), os.Getenv(http.ENV_REPO_NAME))
	hookCmd.Stdout = os.Stdout
	hookCmd.Stdin = buf
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		fail("Internal error", "Fail to execute custom post-receive hook: %v", err)
	}
	return nil
}
