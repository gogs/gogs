// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	"github.com/sourcegraph/run"
	"github.com/unknwon/com"
	"golang.org/x/crypto/ssh"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/osutil"
)

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
			for req := range in {
				payload := cleanCommand(string(req.Payload))
				switch req.Type {
				case "env":
					var env struct {
						Name  string
						Value string
					}
					if err := ssh.Unmarshal(req.Payload, &env); err != nil {
						log.Warn("SSH: Invalid env payload %q: %v", req.Payload, err)
						continue
					}
					// Sometimes the client could send malformed command (i.e. missing "="),
					// see https://discuss.gogs.io/t/ssh/3106.
					if env.Name == "" || env.Value == "" {
						log.Warn("SSH: Invalid env arguments: %+v", env)
						continue
					}

					_, stderr, err := com.ExecCmd("env", fmt.Sprintf("%s=%s", env.Name, env.Value))
					if err != nil {
						log.Error("env: %v - %s", err, stderr)
						return
					}

				case "exec":
					cmdName := strings.TrimLeft(payload, "'()")
					log.Trace("SSH: Payload: %v", cmdName)

					args := []string{"serv", "key-" + keyID, "--config=" + conf.CustomConf}
					log.Trace("SSH: Arguments: %v", args)
					cmd := exec.Command(conf.AppPath(), args...)
					cmd.Env = append(os.Environ(), "SSH_ORIGINAL_COMMAND="+cmdName)

					stdout, err := cmd.StdoutPipe()
					if err != nil {
						log.Error("SSH: StdoutPipe: %v", err)
						return
					}
					stderr, err := cmd.StderrPipe()
					if err != nil {
						log.Error("SSH: StderrPipe: %v", err)
						return
					}
					input, err := cmd.StdinPipe()
					if err != nil {
						log.Error("SSH: StdinPipe: %v", err)
						return
					}

					// FIXME: check timeout
					if err = cmd.Start(); err != nil {
						log.Error("SSH: Start: %v", err)
						return
					}

					_ = req.Reply(true, nil)
					go func() {
						_, _ = io.Copy(input, ch)
					}()
					_, _ = io.Copy(ch, stdout)
					_, _ = io.Copy(ch.Stderr(), stderr)

					if err = cmd.Wait(); err != nil {
						log.Error("SSH: Wait: %v", err)
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

func listen(config *ssh.ServerConfig, host string, port int) {
	listener, err := net.Listen("tcp", host+":"+com.ToStr(port))
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
			sConn, chans, reqs, err := ssh.NewServerConn(conn, config)
			if err != nil {
				if err == io.EOF || errors.Is(err, syscall.ECONNRESET) {
					log.Trace("SSH: Handshaking was terminated: %v", err)
				} else {
					log.Error("SSH: Error on handshaking: %v", err)
				}
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
		Config: ssh.Config{
			Ciphers: opts.ServerCiphers,
			MACs:    opts.ServerMACs,
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			pkey, err := database.SearchPublicKeyByContent(strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))))
			if err != nil {
				log.Error("SearchPublicKeyByContent: %v", err)
				return nil, err
			}
			return &ssh.Permissions{Extensions: map[string]string{"key-id": com.ToStr(pkey.ID)}}, nil
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
		if !osutil.IsExist(keyPath) {
			args := []string{
				conf.SSH.KeygenPath,
				"-t", algo,
				"-f", keyPath,
				"-m", "PEM",
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
