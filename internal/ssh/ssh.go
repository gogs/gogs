package ssh

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/sourcegraph/run"
	"golang.org/x/crypto/ssh"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
)

func cleanCommand(cmd string) string {
	i := strings.Index(cmd, "git")
	if i == -1 {
		return cmd
	}
	return cmd[i:]
}

func handleServerConn(logger *logx.Logger, keyID string, chans <-chan ssh.NewChannel) {
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		ch, reqs, err := newChan.Accept()
		if err != nil {
			logger.Error("Failed to accept channel", "error", err)
			continue
		}

		go func(in <-chan *ssh.Request) {
			defer func() {
				_ = ch.Close()
			}()
			// The Git protocol the client asks for, e.g., "version=2".
			var gitProtocol string
			for req := range in {
				payload := cleanCommand(string(req.Payload))
				switch req.Type {
				case "env":
					var env struct{ Name, Value string }
					if err := ssh.Unmarshal(req.Payload, &env); err != nil {
						logger.Error("Failed to unmarshal env request", "error", err)
						_ = req.Reply(false, nil)
						continue
					}
					// Other environment variables are not passed on to Git.
					accepted := env.Name == "GIT_PROTOCOL" && git.IsSupportedProtocol(env.Value)
					if accepted {
						gitProtocol = env.Value
					}
					_ = req.Reply(accepted, nil)

				case "exec":
					cmdName := strings.TrimLeft(payload, "'()")
					logger.Debug("Received exec request", "command", cmdName)

					args := []string{"serv", "key-" + keyID, "--config=" + conf.CustomConf}
					logger.Debug("Running serv command", "args", args)
					cmd := exec.Command(conf.AppPath(), args...)
					cmd.Env = append(os.Environ(), "SSH_ORIGINAL_COMMAND="+cmdName)
					if gitProtocol != "" {
						cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+gitProtocol)
					}

					stdout, err := cmd.StdoutPipe()
					if err != nil {
						logger.Error("Failed to get stdout pipe", "error", err)
						return
					}
					stderr, err := cmd.StderrPipe()
					if err != nil {
						logger.Error("Failed to get stderr pipe", "error", err)
						return
					}
					input, err := cmd.StdinPipe()
					if err != nil {
						logger.Error("Failed to get stdin pipe", "error", err)
						return
					}

					// FIXME: check timeout
					if err = cmd.Start(); err != nil {
						logger.Error("Failed to start command", "error", err)
						return
					}

					_ = req.Reply(true, nil)
					go func() {
						_, _ = io.Copy(input, ch)
						input.Close()
					}()
					_, _ = io.Copy(ch, stdout)
					// When the client is gone, the copy stops early. Closing the pipe makes
					// the next write of the command fail, so that it exits rather than
					// waiting forever for someone to read its output.
					_ = stdout.Close()
					_, _ = io.Copy(ch.Stderr(), stderr)

					if err = cmd.Wait(); err != nil {
						logger.Error("Failed to wait for command", "error", err)
						return
					}

					_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
					return
				default:
				}
			}
		}(reqs)
	}
}

func listen(logger *logx.Logger, config *ssh.ServerConfig, host string, port int) {
	listener, err := net.Listen("tcp", host+":"+strconv.Itoa(port))
	if err != nil {
		logger.FatalContext(context.Background(), "Failed to start SSH server", "error", err)
	}
	for {
		// Once a ServerConfig has been configured, connections can be accepted.
		conn, err := listener.Accept()
		if err != nil {
			logger.Error("Failed to accept incoming connection", "error", err)
			continue
		}

		// Before use, a handshake must be performed on the incoming net.Conn.
		// It must be handled in a separate goroutine,
		// otherwise one user could easily block entire loop.
		// For example, user could be asked to trust server key fingerprint and hangs.
		go func() {
			logger.Debug("Handshaking", "remoteAddr", conn.RemoteAddr())
			if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
				logger.Error("Failed to set handshake deadline", "remoteAddr", conn.RemoteAddr(), "error", err)
				_ = conn.Close()
				return
			}
			sConn, chans, reqs, err := ssh.NewServerConn(conn, config)
			if err != nil {
				if err == io.EOF || errors.Is(err, syscall.ECONNRESET) {
					logger.Debug("Handshake was terminated", "remoteAddr", conn.RemoteAddr(), "error", err)
				} else {
					logger.Error("Failed to handshake", "remoteAddr", conn.RemoteAddr(), "error", err)
				}
				return
			}
			if err := conn.SetDeadline(time.Time{}); err != nil {
				logger.Error("Failed to clear handshake deadline", "remoteAddr", conn.RemoteAddr(), "error", err)
				_ = sConn.Close()
				return
			}

			logger.Debug("Accepted connection", "remoteAddr", sConn.RemoteAddr(), "clientVersion", string(sConn.ClientVersion()))
			// The incoming Request channel must be serviced.
			go ssh.DiscardRequests(reqs)
			go handleServerConn(logger, sConn.Permissions.Extensions["key-id"], chans)
		}()
	}
}

// Listen starts a SSH server listens on given port.
func Listen(logger *logx.Logger, opts conf.SSHOpts, appDataPath string) {
	config := &ssh.ServerConfig{
		Ciphers: opts.ServerCiphers,
		MACs:    opts.ServerMACs,
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			pkey, err := database.SearchPublicKeyByContent(strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))))
			if err != nil {
				if !database.IsErrKeyNotExist(err) {
					logger.Error("Failed to search public key by content", "error", err)
				}
				return nil, err
			}
			return &ssh.Permissions{Extensions: map[string]string{"key-id": strconv.FormatInt(pkey.ID, 10)}}, nil
		},
	}

	keys, err := setupHostKeys(logger, appDataPath, opts.ServerAlgorithms)
	if err != nil {
		logger.FatalContext(context.Background(), "Failed to set up host keys", "error", err)
	}
	for _, key := range keys {
		config.AddHostKey(key)
	}

	go listen(logger, config, opts.ListenHost, opts.ListenPort)
}

func setupHostKeys(logger *logx.Logger, appDataPath string, algorithms []string) ([]ssh.Signer, error) {
	dir := filepath.Join(appDataPath, "ssh")
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return nil, errors.Wrapf(err, "create host key directory")
	}

	var hostKeys []ssh.Signer
	for _, algo := range algorithms {
		keyPath := filepath.Join(dir, "gogs."+algo)
		if !osx.Exist(keyPath) {
			args := []string{
				conf.SSH.KeygenPath,
				"-t", algo,
				"-f", keyPath,
				"-N", run.Arg(""),
			}
			err = run.Cmd(context.Background(), args...).Run().Wait()
			if err != nil {
				return nil, errors.Wrapf(err, "generate host key with args %v", args)
			}
			logger.Debug("Generated new private key", "keyPath", keyPath)
		}

		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, errors.Wrapf(err, "read host key %q", keyPath)
		}
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return nil, errors.Wrapf(err, "parse host key %q", keyPath)
		}

		hostKeys = append(hostKeys, signer)
	}
	return hostKeys, nil
}
