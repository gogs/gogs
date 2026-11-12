package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/cmd/gogs/internal/tracing"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/httplib"
	"gogs.io/gogs/internal/osx"
)

var (
	hookCommand = cli.Command{
		Name:        "hook",
		Usage:       "Delegate commands to corresponding Git hooks",
		Description: "All sub-commands should only be called by Git",
		Flags: []cli.Flag{
			stringFlag("config, c", "", "Custom configuration file path"),
		},
		Commands: []*cli.Command{
			&subcmdHookPreReceive,
			&subcmdHookUpadte,
			&subcmdHookPostReceive,
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

func runHookPreReceive(ctx context.Context, cmd *cli.Command) error {
	if os.Getenv("SSH_ORIGINAL_COMMAND") == "" {
		return nil
	}
	setup(cmd, "pre-receive.log", true)
	ctx = startHookSpan(ctx, "pre-receive")
	defer endHookSpan()

	isWiki := strings.Contains(os.Getenv(database.EnvRepoCustomHooksPath), ".wiki.git/")

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
		branchName := git.RefShortName(string(fields[2]))

		// Branch protection
		repoID, _ := strconv.ParseInt(os.Getenv(database.EnvRepoID), 10, 64)
		protectBranch, err := database.GetProtectBranchOfRepoByName(repoID, branchName)
		if err != nil {
			if database.IsErrBranchNotExist(err) {
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
		userID, _ := strconv.ParseInt(os.Getenv(database.EnvAuthUserID), 10, 64)
		if protectBranch.EnableWhitelist {
			if !database.IsUserInProtectBranchWhitelist(repoID, userID, branchName) {
				fail(fmt.Sprintf("Branch '%s' is protected and you are not in the push whitelist", branchName), "")
			}

			bypassRequirePullRequest = true
		}

		// Check if branch allows direct push
		if !bypassRequirePullRequest && protectBranch.RequirePullRequest {
			fail(fmt.Sprintf("Branch '%s' is protected and commits must be merged through pull request", branchName), "")
		}

		// check and deletion
		if git.IsEmptyID(newCommitID) {
			fail(fmt.Sprintf("Branch '%s' is protected from deletion", branchName), "")
		}

		// Check force push
		repoPath := database.RepoPath(os.Getenv(database.EnvRepoOwnerName), os.Getenv(database.EnvRepoName))
		removedCommits, err := git.RepoRevListCount(ctx, repoPath, []string{oldCommitID, "^" + newCommitID})
		if err != nil {
			fail("Internal error", "Failed to detect force push: %v", err)
		} else if removedCommits > 0 {
			fail(fmt.Sprintf("Branch '%s' is protected from force push", branchName), "")
		}
	}

	customHooksPath := filepath.Join(os.Getenv(database.EnvRepoCustomHooksPath), "pre-receive")
	if !osx.IsFile(customHooksPath) {
		return nil
	}

	var hookCmd *exec.Cmd
	if conf.IsWindowsRuntime() {
		hookCmd = exec.Command("bash.exe", "custom_hooks/pre-receive")
	} else {
		hookCmd = exec.Command(customHooksPath)
	}
	hookCmd.Dir = database.RepoPath(os.Getenv(database.EnvRepoOwnerName), os.Getenv(database.EnvRepoName))
	hookCmd.Stdout = os.Stdout
	hookCmd.Stdin = buf
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		fail("Internal error", "Failed to execute custom pre-receive hook: %v", err)
	}
	return nil
}

func runHookUpdate(ctx context.Context, cmd *cli.Command) error {
	if os.Getenv("SSH_ORIGINAL_COMMAND") == "" {
		return nil
	}
	setup(cmd, "update.log", false)
	_ = startHookSpan(ctx, "update")
	defer endHookSpan()

	args := cmd.Args().Slice()
	if len(args) != 3 {
		fail("Arguments received are not equal to three", "Arguments received are not equal to three")
	} else if args[0] == "" {
		fail("First argument 'refName' is empty", "First argument 'refName' is empty")
	}

	customHooksPath := filepath.Join(os.Getenv(database.EnvRepoCustomHooksPath), "update")
	if !osx.IsFile(customHooksPath) {
		return nil
	}

	var hookCmd *exec.Cmd
	if conf.IsWindowsRuntime() {
		hookCmd = exec.Command("bash.exe", append([]string{"custom_hooks/update"}, args...)...)
	} else {
		hookCmd = exec.Command(customHooksPath, args...)
	}
	hookCmd.Dir = database.RepoPath(os.Getenv(database.EnvRepoOwnerName), os.Getenv(database.EnvRepoName))
	hookCmd.Stdout = os.Stdout
	hookCmd.Stdin = os.Stdin
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		fail("Internal error", "Failed to execute custom pre-receive hook: %v", err)
	}
	return nil
}

func runHookPostReceive(ctx context.Context, cmd *cli.Command) error {
	if os.Getenv("SSH_ORIGINAL_COMMAND") == "" {
		return nil
	}
	setup(cmd, "post-receive.log", true)
	ctx = startHookSpan(ctx, "post-receive")
	defer endHookSpan()

	// Post-receive hook does more than just gather Git information,
	// so we need to setup additional services for email notifications.
	email.NewContext()

	isWiki := strings.Contains(os.Getenv(database.EnvRepoCustomHooksPath), ".wiki.git/")

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

		pusherID, _ := strconv.ParseInt(os.Getenv(database.EnvAuthUserID), 10, 64)
		options := database.PushUpdateOptions{
			OldCommitID:  string(fields[0]),
			NewCommitID:  string(fields[1]),
			FullRefspec:  string(fields[2]),
			PusherID:     pusherID,
			PusherName:   os.Getenv(database.EnvAuthUserName),
			RepoUserName: os.Getenv(database.EnvRepoOwnerName),
			RepoName:     os.Getenv(database.EnvRepoName),
		}
		if err := database.PushUpdate(ctx, options); err != nil {
			log.Error("PushUpdate: %v", err)
		}

		// Ask for running deliver hook and test pull request tasks
		q := make(url.Values)
		q.Add("branch", git.RefShortName(options.FullRefspec))
		q.Add("secret", os.Getenv(database.EnvRepoOwnerSaltMd5))
		q.Add("pusher", os.Getenv(database.EnvAuthUserID))
		reqURL := fmt.Sprintf("%s%s/%s/tasks/trigger?%s", conf.Server.LocalRootURL, options.RepoUserName, options.RepoName, q.Encode())
		log.Trace("Trigger task: %s", reqURL)

		req := httplib.Get(reqURL).
			SetTLSClientConfig(&tls.Config{
				InsecureSkipVerify: true,
			})
		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Headers()))
		resp, err := req.Response()
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				log.Error("Failed to trigger task: unsuccessful response code %d", resp.StatusCode)
			}
		} else {
			log.Error("Failed to trigger task: %v", err)
		}
	}

	customHooksPath := filepath.Join(os.Getenv(database.EnvRepoCustomHooksPath), "post-receive")
	if !osx.IsFile(customHooksPath) {
		return nil
	}

	var hookCmd *exec.Cmd
	if conf.IsWindowsRuntime() {
		hookCmd = exec.Command("bash.exe", "custom_hooks/post-receive")
	} else {
		hookCmd = exec.Command(customHooksPath)
	}
	hookCmd.Dir = database.RepoPath(os.Getenv(database.EnvRepoOwnerName), os.Getenv(database.EnvRepoName))
	hookCmd.Stdout = os.Stdout
	hookCmd.Stdin = buf
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		fail("Internal error", "Failed to execute custom post-receive hook: %v", err)
	}
	return nil
}

// The span of the running hook, and the function that ends it and exports it.
// fail uses them because it exits without returning.
var (
	hookSpan    = trace.SpanFromContext(context.Background())
	endHookSpan = func() {}
)

// startHookSpan starts the span of the hook when tracing is enabled, under the
// span of the Git process that runs the hook, which Git passes on through the
// TRACEPARENT and TRACESTATE environment variables.
func startHookSpan(ctx context.Context, name string) context.Context {
	shutdown, err := tracing.Init(ctx)
	if err != nil {
		log.Error("Failed to initialize tracing: %v", err)
		return ctx
	} else if shutdown == nil {
		return ctx
	}

	ctx = propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{
		"traceparent": os.Getenv("TRACEPARENT"),
		"tracestate":  os.Getenv("TRACESTATE"),
	})
	ctx, hookSpan = otel.Tracer("gogs.io/gogs/cmd/gogs").Start(ctx, "hook "+name)
	endHookSpan = func() {
		hookSpan.End()
		shutdown()
	}
	return ctx
}
