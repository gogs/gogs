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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/database"
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

	processSpan.SetStatus(codes.Error, userMessage)
	endProcessSpan()
	log.Stop()
	os.Exit(1)
}

func setup(cmd *cli.Command, logFile string, connectDB bool) {
	// The delegate hooks and the "authorized_keys" file run Gogs with the garbage
	// collector turned off, because collecting garbage slows down the startup more
	// than it saves. It is turned back on for the work, e.g., of a large push, and
	// for the custom hooks the process runs.
	if os.Getenv("GOGC") == "off" {
		debug.SetGCPercent(100)
		_ = os.Unsetenv("GOGC")
	}
	conf.HookMode = true

	customConf := configFromLineage(cmd)

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
		Rotate:   true,
		Daily:    true,
		MaxDays:  3,
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

	if _, err := database.SetEngine(); err != nil {
		fail("Internal error", "Failed to set database engine: %v", err)
	}
}

func runServ(ctx context.Context, cmd *cli.Command) error {
	setup(cmd, "serv.log", true)

	if conf.SSH.Disabled {
		println("Gogs: SSH has been disabled")
		return nil
	}

	if cmd.Args().Len() < 1 {
		fail("Not enough arguments", "Not enough arguments")
	}

	sshCmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	verb, _, _ := strings.Cut(sshCmd, " ")
	// Hooks that Git runs record their spans under this one.
	ctx = startProcessSpan(ctx, "ssh "+verb, time.Second)
	defer endProcessSpan()

	keyID, _ := strconv.ParseInt(strings.TrimPrefix(cmd.Args().Get(0), "key-"), 10, 64)
	err := ssh.Serve(ctx, ssh.ServeOptions{
		KeyID:            keyID,
		Command:          sshCmd,
		Protocol:         os.Getenv("GIT_PROTOCOL"),
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		TriggerPushTasks: triggerPushTasks,
	})
	if err == nil {
		return nil
	}

	var serveErr *ssh.ServeError
	if !errors.As(err, &serveErr) {
		fail("Internal error", "%v", err)
	} else if serveErr.Err == nil {
		fail(serveErr.UserMessage, "")
	}
	fail(serveErr.UserMessage, "%v", serveErr.Err)
	return nil
}

// triggerPushTasks asks the web server, which runs the tasks, to run the ones
// of a push, e.g., webhook deliveries.
func triggerPushTasks(pusher *database.User, repo *database.Repository, branch string) {
	ctx := context.Background()
	q := make(url.Values)
	q.Add("branch", branch)
	q.Add("secret", cryptox.MD5(repo.Owner.Salt))
	q.Add("pusher", strconv.FormatInt(pusher.ID, 10))
	reqURL := fmt.Sprintf("%s%s/%s/tasks/trigger?%s", conf.Server.LocalRootURL, repo.Owner.Name, repo.Name, q.Encode())
	log.Trace("Trigger task: %s", reqURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		log.Error("Failed to trigger task: %v", err)
		return
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
	if err != nil {
		log.Error("Failed to trigger task: %v", err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		log.Error("Failed to trigger task: unsuccessful response code %d", resp.StatusCode)
	}
}
