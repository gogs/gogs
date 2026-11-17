package ssh

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/telemetry"
)

const accessDeniedMessage = "Repository does not exist or you do not have access"

// ServeError is an error of Serve with the message to show the client.
type ServeError struct {
	// The message to show the client, which never contains internal details.
	UserMessage string
	// The error to log, which is nil when there is nothing more than UserMessage.
	Err error
}

func (e *ServeError) Error() string {
	if e.Err == nil {
		return e.UserMessage
	}
	return e.UserMessage + ": " + e.Err.Error()
}

func serveError(userMessage, format string, args ...any) error {
	err := &ServeError{UserMessage: userMessage}
	if format != "" {
		err.Err = errors.Newf(format, args...)
	}
	return err
}

// ServeOptions contains the options of Serve.
type ServeOptions struct {
	// The ID of the public key that the client signed in with.
	KeyID int64
	// The command that the client asks to run, e.g., "git-upload-pack
	// 'owner/repo.git'".
	Command string
	// The Git protocol that the client asks for, e.g., "version=2".
	Protocol string
	// The streams connected to the client.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// TriggerPushTasks is called for every reference that a push updates, with
	// its short name, e.g., to queue webhook deliveries.
	TriggerPushTasks func(pusher *database.User, repo *database.Repository, branch string)
}

var allowedCommands = map[string]database.AccessMode{
	"git-upload-pack":    database.AccessModeRead,
	"git-upload-archive": database.AccessModeRead,
	"git-receive-pack":   database.AccessModeWrite,
}

// Serve checks that the client may run the Git command over SSH and runs it.
// After a push, it records the updated references and calls TriggerPushTasks.
// Errors are of type *ServeError.
func Serve(ctx context.Context, logger *logx.Logger, opts ServeOptions) error {
	if conf.SSH.Disabled {
		return serveError("SSH has been disabled", "")
	}
	if opts.Command == "" {
		_, _ = fmt.Fprintln(opts.Stderr, "Hi there, You've successfully authenticated, but Gogs does not provide shell access.")
		_, _ = fmt.Fprintln(opts.Stderr, "If this is unexpected, please log in with password and setup Gogs under another user.")
		return nil
	}

	verb, args := parseCommand(opts.Command)
	repoFullName := strings.ToLower(strings.Trim(args, "'"))
	repoFields := strings.SplitN(repoFullName, "/", 2)
	if len(repoFields) != 2 {
		return serveError("Invalid repository path", "Invalid repository path: %v", args)
	}
	ownerName := strings.ToLower(repoFields[0])
	repoName := strings.TrimSuffix(strings.ToLower(repoFields[1]), ".git")
	isWiki := strings.HasSuffix(repoName, ".wiki")
	repoName = strings.TrimSuffix(repoName, ".wiki")

	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return serveError("Repository owner does not exist", "Unregistered owner: %s", ownerName)
		}
		return serveError("Internal error", "Failed to get repository owner '%s': %v", ownerName, err)
	}

	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return serveError(accessDeniedMessage, "Repository does not exist: %s/%s", owner.Name, repoName)
		}
		return serveError("Internal error", "Failed to get repository: %v", err)
	}
	repo.Owner = owner
	repo.Owner = owner
	telemetry.SetRepository(ctx, repo.ID)

	requestMode, ok := allowedCommands[verb]
	if !ok {
		return serveError("Unknown git command", "Unknown git command '%s'", verb)
	}

	// Prohibit push to mirror repositories.
	if requestMode > database.AccessModeRead && repo.IsMirror {
		return serveError("Mirror repository is read-only", "")
	}

	// Allow anonymous (user is nil) clone for public repositories.
	var user *database.User

	key, err := database.Handle.PublicKeys().GetByID(ctx, opts.KeyID)
	if err != nil {
		return serveError("Invalid key ID", "Invalid key ID '%d': %v", opts.KeyID, err)
	}
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.Int64("ssh.key.id", key.ID),
		attribute.Bool("ssh.key.deploy", key.IsDeployKey()),
	)

	if requestMode == database.AccessModeWrite || repo.IsPrivate {
		// Check deploy key or user key.
		if key.IsDeployKey() {
			if key.Mode < requestMode {
				return serveError("Key permission denied", "Cannot push with deployment key: %d", key.ID)
			}
			if err = checkDeployKey(ctx, key, repo); err != nil {
				return err
			}
		} else {
			user, err = database.Handle.Users().GetByKeyID(ctx, key.ID)
			if err != nil {
				return serveError("Internal error", "Failed to get user by key ID '%d': %v", key.ID, err)
			}
			telemetry.SetUser(ctx, user.ID)
			if user.IsDisabled() {
				return serveError(accessDeniedMessage, "User '%s' is disabled", user.Name)
			}

			mode := database.Handle.Permissions().AccessMode(ctx, logger, user.ID, repo.ID,
				database.AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			)
			if mode < requestMode {
				clientMessage := accessDeniedMessage
				if mode >= database.AccessModeRead {
					clientMessage = "You do not have sufficient authorization for this action"
				}
				return serveError(clientMessage,
					"User '%s' does not have level '%v' access to repository '%s'",
					user.Name, requestMode, repoFullName)
			}
		}
	} else {
		// Check if the key can access to the repository in case of it is a deploy key (a deploy keys != user key).
		// A deploy key doesn't represent a signed in user, so in a site with Auth.RequireSignInView enabled,
		// we should give read access only in repositories where this deploy key is in use. In other cases,
		// a server or system using an active deploy key can get read access to all repositories on a Gogs instance.
		if conf.Auth.RequireSigninView {
			if key.IsDeployKey() {
				if err = checkDeployKey(ctx, key, repo); err != nil {
					return err
				}
			} else {
				keyOwner, err := database.Handle.Users().GetByKeyID(ctx, key.ID)
				if err != nil {
					return serveError("Internal error", "Failed to get user by key ID '%d': %v", key.ID, err)
				}
				if keyOwner.IsDisabled() {
					return serveError(accessDeniedMessage, "User '%s' is disabled", keyOwner.Name)
				}
			}
		}
	}

	// Update user key activity.
	if key.ID > 0 {
		if err = database.Handle.PublicKeys().Touch(ctx, key.ID); err != nil {
			return serveError("Internal error", "Touch public key: %v", err)
		}
	}

	// Git finds the repository without the suffix, but recording a push runs other
	// Git commands in the directory of the repository.
	repoPath := filepath.Join(conf.Repository.Root, repoFullName)
	if !strings.HasSuffix(repoPath, ".git") {
		repoPath += ".git"
	}
	serviceOpts := git.ServiceOptions{
		Stdin:    opts.Stdin,
		Stdout:   opts.Stdout,
		Stderr:   opts.Stderr,
		Protocol: opts.Protocol,
	}
	if requestMode != database.AccessModeWrite {
		err = git.RunService(ctx, git.Service(strings.TrimPrefix(verb, "git-")), repoPath, serviceOpts)
		if err != nil {
			return serveError("Internal error", "Failed to execute git command: %v", err)
		}
		return nil
	}

	serviceOpts.Envs = database.ComposeHookEnvs(database.ComposeHookEnvsOptions{
		AuthUser:  user,
		OwnerName: owner.Name,
		OwnerSalt: owner.Salt,
		RepoID:    repo.ID,
		RepoName:  repo.Name,
		RepoPath:  repo.RepoPath(),
	})
	// The client going away must not stop Git midway through a push, nor the
	// recording of what it pushed.
	ctx = context.WithoutCancel(ctx)
	updates, err := git.RunReceivePack(ctx, repoPath, serviceOpts)
	// TODO: support news feeds for wiki
	if !isWiki {
		database.PushUpdates(ctx, logger, user, owner.Name, repo.Name, updates, func(branch string) {
			opts.TriggerPushTasks(user, repo, branch)
		})
	}
	if err != nil {
		return serveError("Internal error", "Failed to execute git command: %v", err)
	}
	return nil
}

// parseCommand returns the verb and the arguments of the command a client asks
// to run.
func parseCommand(cmd string) (verb, args string) {
	ss := strings.SplitN(cmd, " ", 2)
	if len(ss) != 2 {
		return "", ""
	}
	return ss[0], strings.Replace(ss[1], "'/", "'", 1)
}

func checkDeployKey(ctx context.Context, key *database.PublicKey, repo *database.Repository) error {
	deployKey, err := database.Handle.DeployKeys().GetByRepoAndKey(ctx, repo.ID, key.ID)
	if err != nil {
		if database.IsErrDeployKeyNotExist(err) {
			return serveError("Key access denied", "Deploy key access denied: [key_id: %d, repo_id: %d]", key.ID, repo.ID)
		}
		return serveError("Internal error", "Get deploy key: %v", err)
	}

	if err = database.Handle.DeployKeys().Touch(ctx, deployKey.ID); err != nil {
		return serveError("Internal error", "Touch deploy key: %v", err)
	}
	return nil
}
