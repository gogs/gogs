package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/cmd/gogs/internal/tracing"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/git"
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
	logger := conf.NewConsoleLogger()
	if os.Getenv("SSH_ORIGINAL_COMMAND") == "" {
		return nil
	}
	logger = setup(logger, cmd, "pre-receive.log", true)
	ctx = startProcessSpan(ctx, logger, "hook pre-receive", time.Second)
	defer endProcessSpan()

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
			fail(logger, "Internal error", "GetProtectBranchOfRepoByName [repo_id: %d, branch: %s]: %v", repoID, branchName, err)
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
				fail(logger, fmt.Sprintf("Branch '%s' is protected and you are not in the push whitelist", branchName), "")
			}

			bypassRequirePullRequest = true
		}

		// Check if branch allows direct push
		if !bypassRequirePullRequest && protectBranch.RequirePullRequest {
			fail(logger, fmt.Sprintf("Branch '%s' is protected and commits must be merged through pull request", branchName), "")
		}

		// check and deletion
		if git.IsEmptyID(newCommitID) {
			fail(logger, fmt.Sprintf("Branch '%s' is protected from deletion", branchName), "")
		}

		// Check force push
		repoPath := database.RepoPath(os.Getenv(database.EnvRepoOwnerName), os.Getenv(database.EnvRepoName))
		removedCommits, err := git.RepoRevListCount(ctx, repoPath, []string{oldCommitID, "^" + newCommitID})
		if err != nil {
			fail(logger, "Internal error", "Failed to detect force push: %v", err)
		} else if removedCommits > 0 {
			fail(logger, fmt.Sprintf("Branch '%s' is protected from force push", branchName), "")
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
		fail(logger, "Internal error", "Failed to execute custom pre-receive hook: %v", err)
	}
	return nil
}

func runHookUpdate(ctx context.Context, cmd *cli.Command) error {
	logger := conf.NewConsoleLogger()
	if os.Getenv("SSH_ORIGINAL_COMMAND") == "" {
		return nil
	}
	logger = setup(logger, cmd, "update.log", false)

	args := cmd.Args().Slice()
	if len(args) != 3 {
		fail(logger, "Arguments received are not equal to three", "Arguments received are not equal to three")
	} else if args[0] == "" {
		fail(logger, "First argument 'refName' is empty", "First argument 'refName' is empty")
	}

	customHooksPath := filepath.Join(os.Getenv(database.EnvRepoCustomHooksPath), "update")
	if !osx.IsFile(customHooksPath) {
		return nil
	}

	// Git runs the hook for every ref it updates, so it is only traced when it
	// runs a custom hook, and gives up exporting sooner, to keep exporting spans
	// from slowing down every push.
	_ = startProcessSpan(ctx, logger, "hook update", 300*time.Millisecond)
	defer endProcessSpan()

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
		fail(logger, "Internal error", "Failed to execute custom pre-receive hook: %v", err)
	}
	return nil
}

func runHookPostReceive(ctx context.Context, cmd *cli.Command) error {
	logger := conf.NewConsoleLogger()
	if os.Getenv("SSH_ORIGINAL_COMMAND") == "" {
		return nil
	}
	logger = setup(logger, cmd, "post-receive.log", true)
	ctx = startProcessSpan(ctx, logger, "hook post-receive", time.Second)
	defer endProcessSpan()

	// Post-receive hook does more than just gather Git information,
	// so we need to setup additional services for email notifications.
	email.NewContext(logger)

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
		if err := database.PushUpdate(ctx, logger, options); err != nil {
			logger.Error("Failed to push update", "error", err)
		}

		// Ask for running deliver hook and test pull request tasks
		q := make(url.Values)
		q.Add("branch", git.RefShortName(options.FullRefspec))
		q.Add("secret", os.Getenv(database.EnvRepoOwnerSaltMd5))
		q.Add("pusher", os.Getenv(database.EnvAuthUserID))
		reqURL := fmt.Sprintf("%s%s/%s/tasks/trigger?%s", conf.Server.LocalRootURL, options.RepoUserName, options.RepoName, q.Encode())
		logger.Debug("Triggering tasks", "requestURL", reqURL)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			logger.Error("Failed to create task trigger request", "error", err)
			continue
		}
		req.Header.Set("User-Agent", "GogsServer")
		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

		client := &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
				DisableKeepAlives: true,
			},
			// The trigger endpoint is local and never redirects legitimately, so a
			// redirect response is reported as a failure instead of being followed.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				logger.Error("Unsuccessful response when triggering tasks", "statusCode", resp.StatusCode)
			}
		} else {
			logger.Error("Failed to trigger tasks", "error", err)
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
		fail(logger, "Internal error", "Failed to execute custom post-receive hook: %v", err)
	}
	return nil
}

// The span of the running hook or serv process, and the function that ends it
// and exports it. fail uses them because it exits without returning.
var (
	processSpan    = trace.SpanFromContext(context.Background())
	endProcessSpan = func() {}
)

// startProcessSpan starts the span of the process when tracing is enabled. For
// a hook, it is under the span of the Git process that runs the hook, which Git
// passes on through the TRACEPARENT and TRACESTATE environment variables. The
// Git client waits for the process to exit, so exporting its spans gives up
// after the timeout to not hold up the push for long on a collector that is
// down.
func startProcessSpan(ctx context.Context, logger *logx.Logger, name string, exportTimeout time.Duration) context.Context {
	shutdown, err := tracing.Init(ctx, logger)
	if err != nil {
		logger.Error("Failed to initialize tracing", "error", err)
		return ctx
	} else if shutdown == nil {
		return ctx
	}

	ctx = propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{
		"traceparent": os.Getenv("TRACEPARENT"),
		"tracestate":  os.Getenv("TRACESTATE"),
	})
	ctx, processSpan = otel.Tracer("gogs.io/gogs/cmd/gogs").Start(ctx, name)
	endProcessSpan = func() {
		processSpan.End()
		shutdown(exportTimeout)
	}
	return ctx
}
