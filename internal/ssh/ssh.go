package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/sourcegraph/run"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/crypto/ssh"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
)

var tracer = otel.Tracer("gogs.io/gogs/internal/ssh")

func cleanCommand(cmd string) string {
	i := strings.Index(cmd, "git")
	if i == -1 {
		return cmd
	}
	return cmd[i:]
}

func handleServerConn(keyID string, chans <-chan ssh.NewChannel) {
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		ch, reqs, err := newChan.Accept()
		if err != nil {
			log.Error("Error accepting channel: %v", err)
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
						log.Error("SSH: Unmarshal env: %v", err)
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
					log.Trace("SSH: Payload: %v", cmdName)

					_ = req.Reply(true, nil)
					status := serve(keyID, cmdName, gitProtocol, ch)
					_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
					return
				default:
				}
			}
		}(reqs)
	}
}

// serve runs the command for the client on the channel, and returns the exit
// status of the command.
func serve(keyID, command, protocol string, ch ssh.Channel) (status uint32) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("SSH: Panic while serving %q: %v\n%s", command, r, debug.Stack())
			_, _ = fmt.Fprintln(ch.Stderr(), "Gogs: Internal error")
			status = 1
		}
	}()

	verb, _, _ := strings.Cut(command, " ")
	ctx, span := tracer.Start(context.Background(), "ssh "+verb)
	defer span.End()

	id, _ := strconv.ParseInt(keyID, 10, 64)
	err := Serve(ctx, ServeOptions{
		KeyID:    id,
		Command:  command,
		Protocol: protocol,
		Stdin:    ch,
		Stdout:   ch,
		Stderr:   ch.Stderr(),
		TriggerPushTasks: func(pusher *database.User, repo *database.Repository, branch string) {
			database.TriggerPushTasks(pusher, repo.ID, branch)
		},
	})
	if err == nil {
		return 0
	}

	span.SetStatus(codes.Error, err.Error())
	userMessage := "Internal error"
	var serveErr *ServeError
	if errors.As(err, &serveErr) {
		userMessage = serveErr.UserMessage
		err = serveErr.Err
	}
	_, _ = fmt.Fprintln(ch.Stderr(), "Gogs:", userMessage)
	if err != nil {
		if !conf.IsProdMode() {
			_, _ = fmt.Fprintln(ch.Stderr(), err)
		}
		log.Error("SSH: Failed to serve %q: %v", command, err)
	}
	return 1
}

func listen(config *ssh.ServerConfig, host string, port int) {
	listener, err := net.Listen("tcp", host+":"+strconv.Itoa(port))
	if err != nil {
		log.Fatal("Failed to start SSH server: %v", err)
	}
	for {
		// Once a ServerConfig has been configured, connections can be accepted.
		conn, err := listener.Accept()
		if err != nil {
			log.Error("SSH: Error accepting incoming connection: %v", err)
			continue
		}

		// Before use, a handshake must be performed on the incoming net.Conn.
		// It must be handled in a separate goroutine,
		// otherwise one user could easily block entire loop.
		// For example, user could be asked to trust server key fingerprint and hangs.
		go func() {
			log.Trace("SSH: Handshaking for %s", conn.RemoteAddr())
			if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
				log.Error("SSH: Failed to set handshake deadline: %v", err)
				_ = conn.Close()
				return
			}
			sConn, chans, reqs, err := ssh.NewServerConn(conn, config)
			if err != nil {
				if err == io.EOF || errors.Is(err, syscall.ECONNRESET) {
					log.Trace("SSH: Handshaking was terminated: %v", err)
				} else {
					log.Error("SSH: Error on handshaking: %v", err)
				}
				return
			}
			if err := conn.SetDeadline(time.Time{}); err != nil {
				log.Error("SSH: Failed to clear handshake deadline: %v", err)
				_ = sConn.Close()
				return
			}

			log.Trace("SSH: Connection from %s (%s)", sConn.RemoteAddr(), sConn.ClientVersion())
			// The incoming Request channel must be serviced.
			go ssh.DiscardRequests(reqs)
			go handleServerConn(sConn.Permissions.Extensions["key-id"], chans)
		}()
	}
}

// Listen starts a SSH server listens on given port.
func Listen(opts conf.SSHOpts, appDataPath string) {
	config := &ssh.ServerConfig{
		Ciphers: opts.ServerCiphers,
		MACs:    opts.ServerMACs,
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			pkey, err := database.SearchPublicKeyByContent(strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))))
			if err != nil {
				if !database.IsErrKeyNotExist(err) {
					log.Error("SearchPublicKeyByContent: %v", err)
				}
				return nil, err
			}
			return &ssh.Permissions{Extensions: map[string]string{"key-id": strconv.FormatInt(pkey.ID, 10)}}, nil
		},
	}

	keys, err := setupHostKeys(appDataPath, opts.ServerAlgorithms)
	if err != nil {
		log.Fatal("SSH: Failed to setup host keys: %v", err)
	}
	for _, key := range keys {
		config.AddHostKey(key)
	}

	go listen(config, opts.ListenHost, opts.ListenPort)
}

func setupHostKeys(appDataPath string, algorithms []string) ([]ssh.Signer, error) {
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
			log.Trace("SSH: New private key is generated: %s", keyPath)
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
