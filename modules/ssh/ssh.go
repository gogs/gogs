// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Prototype, git client looks like do not recognize req.Reply.
package ssh

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/Unknwon/com"
	"golang.org/x/crypto/ssh"

	"github.com/gogits/gogs/modules/log"
)

func handleServerConn(keyId string, chans <-chan ssh.NewChannel) {
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChan.Accept()
		if err != nil {
			log.Error(3, "Could not accept channel: %v", err)
			continue
		}

		go func(in <-chan *ssh.Request) {
			defer channel.Close()
			for req := range in {
				ok, payload := false, strings.TrimLeft(string(req.Payload), "\x00&")
				fmt.Println("Request:", req.Type, req.WantReply, payload)
				if req.WantReply {
					fmt.Println(req.Reply(true, nil))
				}
				switch req.Type {
				case "env":
					args := strings.Split(strings.Replace(payload, "\x00", "", -1), "\v")
					if len(args) != 2 {
						break
					}
					args[0] = strings.TrimLeft(args[0], "\x04")
					_, _, err := com.ExecCmdBytes("env", args[0]+"="+args[1])
					if err != nil {
						log.Error(3, "env: %v", err)
						channel.Stderr().Write([]byte(err.Error()))
						break
					}
					ok = true
				case "exec":
					os.Setenv("SSH_ORIGINAL_COMMAND", strings.TrimLeft(payload, "'("))
					log.Info("Payload: %v", strings.TrimLeft(payload, "'("))
					cmd := exec.Command("/Users/jiahuachen/Applications/Go/src/github.com/gogits/gogs/gogs", "serv", "key-"+keyId)
					cmd.Stdout = channel
					cmd.Stdin = channel
					cmd.Stderr = channel.Stderr()
					if err := cmd.Run(); err != nil {
						log.Error(3, "exec: %v", err)
					} else {
						ok = true
					}
				}
				fmt.Println("Done:", ok)
			}
			fmt.Println("Done!!!")
		}(requests)
	}
}

func listen(config *ssh.ServerConfig, port string) {
	listener, err := net.Listen("tcp", "0.0.0.0:"+port)
	if err != nil {
		panic(err)
	}
	for {
		// Once a ServerConfig has been configured, connections can be accepted.
		conn, err := listener.Accept()
		if err != nil {
			log.Error(3, "Fail to accept incoming connection: %v", err)
			continue
		}
		// Before use, a handshake must be performed on the incoming net.Conn.
		sConn, chans, reqs, err := ssh.NewServerConn(conn, config)
		if err != nil {
			log.Error(3, "Fail to handshake: %v", err)
			continue
		}
		// The incoming Request channel must be serviced.
		go ssh.DiscardRequests(reqs)
		go handleServerConn(sConn.Permissions.Extensions["key-id"], chans)
	}
}

// Listen starts a SSH server listens on given port.
func Listen(port string) {
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			// keyCache[string(ssh.MarshalAuthorizedKey(key))] = 2
			return &ssh.Permissions{Extensions: map[string]string{"key-id": "1"}}, nil
		},
	}

	privateBytes, err := ioutil.ReadFile("/Users/jiahuachen/.ssh/id_rsa")
	if err != nil {
		panic("failed to load private key")
	}
	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		panic("failed to parse private key")
	}
	config.AddHostKey(private)

	go listen(config, port)
}
