package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/gogits/gogs/models"
)

var (
	COMMANDS_READONLY = map[string]int{
		"git-upload-pack":    models.AU_WRITABLE,
		"git upload-pack":    models.AU_WRITABLE,
		"git-upload-archive": models.AU_WRITABLE,
	}

	COMMANDS_WRITE = map[string]int{
		"git-receive-pack": models.AU_READABLE,
		"git receive-pack": models.AU_READABLE,
	}
)

var CmdServ = cli.Command{
	Name:  "serv",
	Usage: "This command just should be called by ssh shell",
	Description: `
gogs serv provide access auth for repositories`,
	Action: runServ,
	Flags:  []cli.Flag{
	//cli.BoolFlag{"update, u", "update pakcage(s) and dependencies if any"},
	//cli.BoolFlag{"verbose, v", "show process details"},
	},
}

func In(b string, sl map[string]int) bool {
	_, e := sl[b]
	return e
}

func runServ(*cli.Context) {
	keys := strings.Split(os.Args[2], "-")
	if len(keys) != 2 {
		fmt.Println("auth file format error")
		return
	}

	keyId, err := strconv.ParseInt(keys[1], 10, 64)
	if err != nil {
		fmt.Println("auth file format error")
		return
	}
	user, err := models.GetUserByKeyId(keyId)
	if err != nil {
		fmt.Println("You have no right to access")
		return
	}

	cmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	if cmd == "" {
		println("Hi %s! You've successfully authenticated, but Gogits does not provide shell access.\n", user.Name)
		return
	}

	verb, args := parseCmd(cmd)
	rRepo := strings.Trim(args, "'")
	rr := strings.SplitN(rRepo, "/", 2)
	if len(rr) != 2 {
		println("Unavilable repository", args)
		return
	}
	repoName := rr[1]
	if strings.HasSuffix(repoName, ".git") {
		repoName = repoName[:len(repoName)-4]
	}
	isWrite := In(verb, COMMANDS_WRITE)
	isRead := In(verb, COMMANDS_READONLY)

	switch {
	case isWrite:
		has, err := models.HasAccess(user.Name, repoName, models.AU_WRITABLE)
		if err != nil {
			println("Inernel error:", err)
			return
		}
		if !has {
			println("You have no right to write this repository")
			return
		}
	case isRead:
		has, err := models.HasAccess(user.Name, repoName, models.AU_READABLE)
		if err != nil {
			println("Inernel error")
			return
		}
		if !has {
			has, err = models.HasAccess(user.Name, repoName, models.AU_WRITABLE)
			if err != nil {
				println("Inernel error")
				return
			}
		}
		if !has {
			println("You have no right to access this repository")
			return
		}
	default:
		println("Unknown command")
		return
	}

	isExist, err := models.IsRepositoryExist(user, repoName)
	if err != nil {
		println("Inernel error:", err.Error())
		return
	}

	if !isExist {
		if isRead {
			println("Repository", user.Name+"/"+repoName, "is not exist")
			return
		} else if isWrite {
			_, err := models.CreateRepository(user, repoName, "", "", false, true)
			if err != nil {
				println("Create repository failed")
				return
			}
		}
	}

	gitcmd := exec.Command(verb, rRepo)
	gitcmd.Dir = models.RepoRootPath
	gitcmd.Stdout = os.Stdout
	gitcmd.Stdin = os.Stdin
	gitcmd.Stderr = os.Stderr

	err = gitcmd.Run()
	if err != nil {
		println("execute command error:", err.Error())
	}
}

func parseCmd(cmd string) (string, string) {
	ss := strings.SplitN(cmd, " ", 2)
	if len(ss) != 2 {
		return "", ""
	}

	verb, args := ss[0], ss[1]
	if verb == "git" {
		ss = strings.SplitN(args, " ", 2)
		args = ss[1]
		verb = fmt.Sprintf("%s %s", verb, ss[0])
	}
	return verb, args
}
