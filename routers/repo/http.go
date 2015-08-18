// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

func authRequired(ctx *middleware.Context) {
	ctx.Resp.Header().Set("WWW-Authenticate", "Basic realm=\".\"")
	ctx.Data["ErrorMsg"] = "no basic auth and digit auth"
	ctx.HTML(401, base.TplName("status/401"))
}

func Http(ctx *middleware.Context) {
	username := ctx.Params(":username")
	reponame := ctx.Params(":reponame")
	if strings.HasSuffix(reponame, ".git") {
		reponame = reponame[:len(reponame)-4]
	}

	var isPull bool
	service := ctx.Query("service")
	if service == "git-receive-pack" ||
		strings.HasSuffix(ctx.Req.URL.Path, "git-receive-pack") {
		isPull = false
	} else if service == "git-upload-pack" ||
		strings.HasSuffix(ctx.Req.URL.Path, "git-upload-pack") {
		isPull = true
	} else {
		isPull = (ctx.Req.Method == "GET")
	}

	repoUser, err := models.GetUserByName(username)
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.Handle(404, "GetUserByName", nil)
		} else {
			ctx.Handle(500, "GetUserByName", err)
		}
		return
	}

	repo, err := models.GetRepositoryByName(repoUser.Id, reponame)
	if err != nil {
		if models.IsErrRepoNotExist(err) {
			ctx.Handle(404, "GetRepositoryByName", nil)
		} else {
			ctx.Handle(500, "GetRepositoryByName", err)
		}
		return
	}

	// Only public pull don't need auth.
	isPublicPull := !repo.IsPrivate && isPull
	var (
		askAuth      = !isPublicPull || setting.Service.RequireSignInView
		authUser     *models.User
		authUsername string
		authPasswd   string
	)

	// check access
	if askAuth {
		baHead := ctx.Req.Header.Get("Authorization")
		if baHead == "" {
			authRequired(ctx)
			return
		}

		auths := strings.Fields(baHead)
		// currently check basic auth
		// TODO: support digit auth
		// FIXME: middlewares/context.go did basic auth check already,
		// maybe could use that one.
		if len(auths) != 2 || auths[0] != "Basic" {
			ctx.HandleText(401, "no basic auth and digit auth")
			return
		}
		authUsername, authPasswd, err = base.BasicAuthDecode(auths[1])
		if err != nil {
			ctx.HandleText(401, "no basic auth and digit auth")
			return
		}

		authUser, err = models.UserSignIn(authUsername, authPasswd)
		if err != nil {
			if !models.IsErrUserNotExist(err) {
				ctx.Handle(500, "UserSignIn error: %v", err)
				return
			}

			// Assume username now is a token.
			token, err := models.GetAccessTokenBySHA(authUsername)
			if err != nil {
				if err == models.ErrAccessTokenNotExist {
					ctx.HandleText(401, "invalid token")
				} else {
					ctx.Handle(500, "GetAccessTokenBySha", err)
				}
				return
			}
			token.Updated = time.Now()
			if err = models.UpdateAccessToekn(token); err != nil {
				ctx.Handle(500, "UpdateAccessToekn", err)
			}
			authUser, err = models.GetUserByID(token.UID)
			if err != nil {
				ctx.Handle(500, "GetUserById", err)
				return
			}
			authUsername = authUser.Name
		}

		if !isPublicPull {
			var tp = models.ACCESS_MODE_WRITE
			if isPull {
				tp = models.ACCESS_MODE_READ
			}

			has, err := models.HasAccess(authUser, repo, tp)
			if err != nil {
				ctx.HandleText(401, "no basic auth and digit auth")
				return
			} else if !has {
				if tp == models.ACCESS_MODE_READ {
					has, err = models.HasAccess(authUser, repo, models.ACCESS_MODE_WRITE)
					if err != nil || !has {
						ctx.HandleText(401, "no basic auth and digit auth")
						return
					}
				} else {
					ctx.HandleText(401, "no basic auth and digit auth")
					return
				}
			}

			if !isPull && repo.IsMirror {
				ctx.HandleText(401, "can't push to mirror")
				return
			}
		}
	}

	callback := func(rpc string, input []byte) {
		if rpc == "receive-pack" {
			var lastLine int64 = 0

			for {
				head := input[lastLine : lastLine+2]
				if head[0] == '0' && head[1] == '0' {
					size, err := strconv.ParseInt(string(input[lastLine+2:lastLine+4]), 16, 32)
					if err != nil {
						log.Error(4, "%v", err)
						return
					}

					if size == 0 {
						//fmt.Println(string(input[lastLine:]))
						break
					}

					line := input[lastLine : lastLine+size]
					idx := bytes.IndexRune(line, '\000')
					if idx > -1 {
						line = line[:idx]
					}
					fields := strings.Fields(string(line))
					if len(fields) >= 3 {
						oldCommitId := fields[0][4:]
						newCommitId := fields[1]
						refName := fields[2]

						// FIXME: handle error.
						if err = models.Update(refName, oldCommitId, newCommitId, authUsername, username, reponame, authUser.Id); err == nil {
							models.HookQueue.AddRepoID(repo.ID)
						}

					}
					lastLine = lastLine + size
				} else {
					break
				}
			}
		}
	}

	HTTPBackend(&Config{
		RepoRootPath: setting.RepoRootPath,
		GitBinPath:   "git",
		UploadPack:   true,
		ReceivePack:  true,
		OnSucceed:    callback,
	})(ctx.Resp, ctx.Req.Request)

	runtime.GC()
}

type Config struct {
	RepoRootPath string
	GitBinPath   string
	UploadPack   bool
	ReceivePack  bool
	OnSucceed    func(rpc string, input []byte)
}

type handler struct {
	*Config
	w    http.ResponseWriter
	r    *http.Request
	Dir  string
	File string
}

type route struct {
	cr      *regexp.Regexp
	method  string
	handler func(handler)
}

var routes = []route{
	{regexp.MustCompile("(.*?)/git-upload-pack$"), "POST", serviceUploadPack},
	{regexp.MustCompile("(.*?)/git-receive-pack$"), "POST", serviceReceivePack},
	{regexp.MustCompile("(.*?)/info/refs$"), "GET", getInfoRefs},
	{regexp.MustCompile("(.*?)/HEAD$"), "GET", getTextFile},
	{regexp.MustCompile("(.*?)/objects/info/alternates$"), "GET", getTextFile},
	{regexp.MustCompile("(.*?)/objects/info/http-alternates$"), "GET", getTextFile},
	{regexp.MustCompile("(.*?)/objects/info/packs$"), "GET", getInfoPacks},
	{regexp.MustCompile("(.*?)/objects/info/[^/]*$"), "GET", getTextFile},
	{regexp.MustCompile("(.*?)/objects/[0-9a-f]{2}/[0-9a-f]{38}$"), "GET", getLooseObject},
	{regexp.MustCompile("(.*?)/objects/pack/pack-[0-9a-f]{40}\\.pack$"), "GET", getPackFile},
	{regexp.MustCompile("(.*?)/objects/pack/pack-[0-9a-f]{40}\\.idx$"), "GET", getIdxFile},
}

// Request handling function
func HTTPBackend(config *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, route := range routes {
			r.URL.Path = strings.ToLower(r.URL.Path) // blue: In case some repo name has upper case name
			if m := route.cr.FindStringSubmatch(r.URL.Path); m != nil {
				if route.method != r.Method {
					renderMethodNotAllowed(w, r)
					return
				}

				file := strings.Replace(r.URL.Path, m[1]+"/", "", 1)
				dir, err := getGitDir(config, m[1])

				if err != nil {
					log.GitLogger.Error(4, err.Error())
					renderNotFound(w)
					return
				}

				hr := handler{config, w, r, dir, file}
				route.handler(hr)
				return
			}
		}

		renderNotFound(w)
		return
	}
}

// Actual command handling functions
func serviceUploadPack(hr handler) {
	serviceRpc("upload-pack", hr)
}

func serviceReceivePack(hr handler) {
	serviceRpc("receive-pack", hr)
}

func serviceRpc(rpc string, hr handler) {
	w, r, dir := hr.w, hr.r, hr.Dir
	defer r.Body.Close()

	if !hasAccess(r, hr.Config, dir, rpc, true) {
		renderNoAccess(w)
		return
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-result", rpc))

	var (
		reqBody = r.Body
		input   []byte
		br      io.Reader
		err     error
	)

	// Handle GZIP.
	if r.Header.Get("Content-Encoding") == "gzip" {
		reqBody, err = gzip.NewReader(reqBody)
		if err != nil {
			log.GitLogger.Error(2, "fail to create gzip reader: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	if hr.Config.OnSucceed != nil {
		input, err = ioutil.ReadAll(reqBody)
		if err != nil {
			log.GitLogger.Error(2, "fail to read request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		br = bytes.NewReader(input)
	} else {
		br = reqBody
	}

	args := []string{rpc, "--stateless-rpc", dir}
	cmd := exec.Command(hr.Config.GitBinPath, args...)
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stdin = br

	if err := cmd.Run(); err != nil {
		log.GitLogger.Error(2, "fail to serve RPC(%s): %v", rpc, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if hr.Config.OnSucceed != nil {
		hr.Config.OnSucceed(rpc, input)
	}
}

func getInfoRefs(hr handler) {
	w, r, dir := hr.w, hr.r, hr.Dir
	serviceName := getServiceType(r)
	access := hasAccess(r, hr.Config, dir, serviceName, false)

	if access {
		args := []string{serviceName, "--stateless-rpc", "--advertise-refs", "."}
		refs := gitCommand(hr.Config.GitBinPath, dir, args...)

		hdrNocache(w)
		w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-advertisement", serviceName))
		w.WriteHeader(http.StatusOK)
		w.Write(packetWrite("# service=git-" + serviceName + "\n"))
		w.Write(packetFlush())
		w.Write(refs)
	} else {
		updateServerInfo(hr.Config.GitBinPath, dir)
		hdrNocache(w)
		sendFile("text/plain; charset=utf-8", hr)
	}
}

func getInfoPacks(hr handler) {
	hdrCacheForever(hr.w)
	sendFile("text/plain; charset=utf-8", hr)
}

func getLooseObject(hr handler) {
	hdrCacheForever(hr.w)
	sendFile("application/x-git-loose-object", hr)
}

func getPackFile(hr handler) {
	hdrCacheForever(hr.w)
	sendFile("application/x-git-packed-objects", hr)
}

func getIdxFile(hr handler) {
	hdrCacheForever(hr.w)
	sendFile("application/x-git-packed-objects-toc", hr)
}

func getTextFile(hr handler) {
	hdrNocache(hr.w)
	sendFile("text/plain", hr)
}

// Logic helping functions

func sendFile(contentType string, hr handler) {
	w, r := hr.w, hr.r
	reqFile := path.Join(hr.Dir, hr.File)

	// fmt.Println("sendFile:", reqFile)

	f, err := os.Stat(reqFile)
	if os.IsNotExist(err) {
		renderNotFound(w)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", f.Size()))
	w.Header().Set("Last-Modified", f.ModTime().Format(http.TimeFormat))
	http.ServeFile(w, r, reqFile)
}

func getGitDir(config *Config, fPath string) (string, error) {
	root := config.RepoRootPath

	if root == "" {
		cwd, err := os.Getwd()

		if err != nil {
			log.GitLogger.Error(4, err.Error())
			return "", err
		}

		root = cwd
	}

	if !strings.HasSuffix(fPath, ".git") {
		fPath = fPath + ".git"
	}

	f := filepath.Join(root, fPath)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		return "", err
	}

	return f, nil
}

func getServiceType(r *http.Request) string {
	serviceType := r.FormValue("service")

	if s := strings.HasPrefix(serviceType, "git-"); !s {
		return ""
	}

	return strings.Replace(serviceType, "git-", "", 1)
}

func hasAccess(r *http.Request, config *Config, dir string, rpc string, checkContentType bool) bool {
	if checkContentType {
		if r.Header.Get("Content-Type") != fmt.Sprintf("application/x-git-%s-request", rpc) {
			return false
		}
	}

	if !(rpc == "upload-pack" || rpc == "receive-pack") {
		return false
	}
	if rpc == "receive-pack" {
		return config.ReceivePack
	}
	if rpc == "upload-pack" {
		return config.UploadPack
	}

	return getConfigSetting(config.GitBinPath, rpc, dir)
}

func getConfigSetting(gitBinPath, serviceName string, dir string) bool {
	serviceName = strings.Replace(serviceName, "-", "", -1)
	setting := getGitConfig(gitBinPath, "http."+serviceName, dir)

	if serviceName == "uploadpack" {
		return setting != "false"
	}

	return setting == "true"
}

func getGitConfig(gitBinPath, configName string, dir string) string {
	args := []string{"config", configName}
	out := string(gitCommand(gitBinPath, dir, args...))
	return out[0 : len(out)-1]
}

func updateServerInfo(gitBinPath, dir string) []byte {
	args := []string{"update-server-info"}
	return gitCommand(gitBinPath, dir, args...)
}

func gitCommand(gitBinPath, dir string, args ...string) []byte {
	command := exec.Command(gitBinPath, args...)
	command.Dir = dir
	out, err := command.Output()

	if err != nil {
		log.GitLogger.Error(4, err.Error())
	}

	return out
}

// HTTP error response handling functions

func renderMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	if r.Proto == "HTTP/1.1" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method Not Allowed"))
	} else {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}
}

func renderNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("Not Found"))
}

func renderNoAccess(w http.ResponseWriter) {
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte("Forbidden"))
}

// Packet-line handling function

func packetFlush() []byte {
	return []byte("0000")
}

func packetWrite(str string) []byte {
	s := strconv.FormatInt(int64(len(str)+4), 16)

	if len(s)%4 != 0 {
		s = strings.Repeat("0", 4-len(s)%4) + s
	}

	return []byte(s + str)
}

// Header writing functions

func hdrNocache(w http.ResponseWriter) {
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
}

func hdrCacheForever(w http.ResponseWriter) {
	now := time.Now().Unix()
	expires := now + 31536000
	w.Header().Set("Date", fmt.Sprintf("%d", now))
	w.Header().Set("Expires", fmt.Sprintf("%d", expires))
	w.Header().Set("Cache-Control", "public, max-age=31536000")
}
