package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"
	"go.opentelemetry.io/otel/codes"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/cmd/gogs/internal/options"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/ssh"
)

var servCommand = cli.Command{
	Name:        "serv",
	Usage:       "This command should only be called by SSH shell",
	Description: `Serv provide access auth for repositories`,
	Action:      runServ,
	Flags: []cli.Flag{
		stringFlag("config, c", "", "Custom configuration file path"),
	},
}

// fail prints the user message to the Git client (i.e., os.Stderr), logs the
// error message on the server side, and exits the process. When not in "prod"
// mode, the error message is also printed to the client for easier debugging.
func fail(logger *logx.Logger, prodMode bool, userMessage, errMessage string, args ...any) {
	_, _ = fmt.Fprintln(os.Stderr, "Gogs:", userMessage)

	if len(errMessage) > 0 {
		if !prodMode {
			fmt.Fprintf(os.Stderr, errMessage+"\n", args...)
		}
		logger.Error(fmt.Sprintf(errMessage, args...))
	}

	processSpan.SetStatus(codes.Error, userMessage)
	endProcessSpan()
	os.Exit(1)
}

// setup returns the hook file logger, which replaces the given console logger,
// and the configuration loaded in hook mode.
func setup(logger *logx.Logger, cmd *cli.Command, logFile string, connectDB bool) (*logx.Logger, *conf.Config) {
	// The delegate hooks run Gogs with the garbage collector turned off, because
	// collecting garbage slows down the startup more than it saves. It is turned
	// back on for the work and for the custom hooks the process runs.
	if os.Getenv("GOGC") == "off" {
		debug.SetGCPercent(100)
		_ = os.Unsetenv("GOGC")
	}

	cfg, err := conf.Init(logger, configFromLineage(cmd), true)
	if err != nil {
		fail(logger, false, "Internal error", "Failed to init configuration: %v", err)
	}
	prodMode := cfg.App.IsProdMode()
	if _, err = conf.InitLogging(logger, cfg, true); err != nil {
		fail(logger, prodMode, "Internal error", "Failed to init logging: %v", err)
	}
	hookLogger, err := conf.InitHookLogging(filepath.Join(cfg.Log.RootPath, "hooks", logFile), prodMode)
	if err != nil {
		fail(logger, prodMode, "Internal error", "Failed to init hook logging: %v", err)
	}
	logger = hookLogger

	if !connectDB {
		return logger, cfg
	}

	if cfg.Database.Type == "sqlite3" {
		_ = os.Chdir(conf.WorkDir())
	}

	if _, err := database.SetEngine(logger.Scoped("db"), cfg.Database, !prodMode); err != nil {
		fail(logger, prodMode, "Internal error", "Failed to set database engine: %v", err)
	}
	return logger, cfg
}

func runServ(ctx context.Context, cmd *cli.Command) error {
	logger, cfg := setup(conf.NewConsoleLogger(os.Stderr), cmd, "serv.log", true)
	prodMode := cfg.App.IsProdMode()
	// Recording a push may send emails, e.g., when a commit closes an issue.
	email.NewContext(logger, cfg.Email)

	if cmd.Args().Len() < 1 {
		fail(logger, prodMode, "Not enough arguments", "Not enough arguments")
	}

	sshCmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	verb, _, _ := strings.Cut(sshCmd, " ")
	// Hooks that Git runs record their spans under this one.
	ctx = startProcessSpan(ctx, logger, cfg.Tracing.Enabled, "ssh "+verb, time.Second)
	defer endProcessSpan()

	keyID, _ := strconv.ParseInt(strings.TrimPrefix(cmd.Args().Get(0), "key-"), 10, 64)
	err := ssh.Serve(ctx, logger, ssh.ServeOptions{
		KeyID:    keyID,
		Command:  sshCmd,
		Protocol: os.Getenv("GIT_PROTOCOL"),
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		TriggerPushTasks: func(pusher *database.User, repo *database.Repository, branch string) {
			triggerPushTasks(logger, cfg.Server.LocalRootURL, pusher, repo, branch)
		},
		Settings: options.SSHServe(cfg, true),
	})
	if err == nil {
		return nil
	}

	var serveErr *ssh.ServeError
	if !errors.As(err, &serveErr) {
		fail(logger, prodMode, "Internal error", "%v", err)
	} else if serveErr.Err == nil {
		fail(logger, prodMode, serveErr.UserMessage, "")
	}
	fail(logger, prodMode, serveErr.UserMessage, "%v", serveErr.Err)
	return nil
}

// triggerPushTasks asks the web server at localRootURL, which runs the tasks, to
// run the ones of a push, e.g., webhook deliveries.
func triggerPushTasks(logger *logx.Logger, localRootURL string, pusher *database.User, repo *database.Repository, branch string) {
	ctx := context.Background()
	q := make(url.Values)
	q.Add("branch", branch)
	q.Add("secret", cryptox.MD5(repo.Owner.Salt))
	q.Add("pusher", strconv.FormatInt(pusher.ID, 10))
	reqURL := fmt.Sprintf("%s%s/%s/tasks/trigger?%s", localRootURL, repo.Owner.Name, repo.Name, q.Encode())
	logger.Debug("Triggering tasks", "requestURL", reqURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		logger.Error("Failed to create task trigger request", "error", err)
		return
	}
	req.Header.Set("User-Agent", "GogsServer")

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
	if err != nil {
		logger.Error("Failed to trigger tasks", "error", err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		logger.Error("Unsuccessful response when triggering tasks", "statusCode", resp.StatusCode)
	}
}
