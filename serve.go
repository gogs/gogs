package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/utils/log"
)

var (
	COMMANDS_READONLY = map[string]int{
		"git-upload-pack": models.AU_WRITABLE,
		"git upload-pack": models.AU_WRITABLE,
	}

	COMMANDS_WRITE = map[string]int{
		"git-receive-pack": models.AU_READABLE,
		"git receive-pack": models.AU_READABLE,
	}
)

var CmdServ = cli.Command{
	Name:  "serv",
	Usage: "just run",
	Description: `
gogs serv`,
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
		fmt.Printf("Hi %s! You've successfully authenticated, but Gogits does not provide shell access.\n", user.Name)
		return
	}

	f, _ := os.Create("test2.log")
	f.WriteString(cmd)
	f.Close()

	log.Info("cmd is %s", cmd)

	verb, args := parseCmd(cmd)
	rr := strings.SplitN(strings.Trim(args, "'"), "/", 1)
	if len(rr) != 2 {
		fmt.Printf("Unavilable repository")
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
		has, err := models.HasAccess(user.Name, repoName, COMMANDS_WRITE[verb])
		if err != nil {
			fmt.Println("Inernel error")
			return
		}
		if !has {
			fmt.Println("You have no right to access this repository")
			return
		}
	case isRead:
		has, err := models.HasAccess(user.Name, repoName, COMMANDS_READONLY[verb])
		if err != nil {
			fmt.Println("Inernel error")
			return
		}
		if !has {
			has, err = models.HasAccess(user.Name, repoName, COMMANDS_WRITE[verb])
			if err != nil {
				fmt.Println("Inernel error")
				return
			}
		}
		if !has {
			fmt.Println("You have no right to access this repository")
			return
		}
	default:
		fmt.Println("Unknown command")
		return
	}

	isExist, err := models.IsRepositoryExist(user, repoName)
	if err != nil {
		fmt.Println("Inernel error")
		return
	}

	if !isExist {
		if isRead {
			fmt.Println("Repository is not exist")
			return
		} else if isWrite {
			_, err := models.CreateRepository(user, repoName)
			if err != nil {
				fmt.Println("Create repository failed")
				return
			}
		}
	}

	fullPath := filepath.Join(models.RepoRootPath, user.Name, repoName+".git")
	newcmd := fmt.Sprintf("%s '%s'", verb, fullPath)
	fmt.Println(newcmd)
	gitcmd := exec.Command("git", "shell", "-c", newcmd)
	gitcmd.Stdout = os.Stdout
	gitcmd.Stderr = os.Stderr

	err = gitcmd.Run()
	if err != nil {
		log.Error("execute command error: %s", err)
	}
}

func parseCmd(cmd string) (string, string) {
	ss := strings.SplitN(cmd, " ", 1)
	if len(ss) != 2 {
		return "", ""
	}
	verb, args := ss[0], ss[1]
	if verb == "git" {
		ss = strings.SplitN(args, " ", 1)
		args = ss[1]
		verb = fmt.Sprintf("%s %s", verb, ss[0])
	}
	return verb, args
}
