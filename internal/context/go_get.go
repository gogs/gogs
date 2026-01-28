package context

import (
	"net/http"
	"path"
	"strings"

	"github.com/flamego/flamego"
	"github.com/unknwon/com"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/repoutil"
)

// ServeGoGet does quick responses for appropriate go-get meta with status OK
// regardless of whether the user has access to the repository, or the repository
// does exist at all. This is particular a workaround for "go get" command which
// does not respect .netrc file.
func ServeGoGet() flamego.Handler {
	return func(fctx flamego.Context, w http.ResponseWriter, req *http.Request) {
		if fctx.Query("go-get") != "1" {
			return
		}

		ownerName := fctx.Param("username")
		repoName := fctx.Param("reponame")
		branchName := "master"

		owner, err := database.Handle.Users().GetByUsername(req.Context(), ownerName)
		if err == nil {
			repo, err := database.Handle.Repositories().GetByName(req.Context(), owner.ID, repoName)
			if err == nil && repo.DefaultBranch != "" {
				branchName = repo.DefaultBranch
			}
		}

		prefix := conf.Server.ExternalURL + path.Join(ownerName, repoName, "src", branchName)
		insecureFlag := ""
		if !strings.HasPrefix(conf.Server.ExternalURL, "https://") {
			insecureFlag = "--insecure "
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(com.Expand(`<!doctype html>
<html>
	<head>
		<meta name="go-import" content="{GoGetImport} git {CloneLink}">
		<meta name="go-source" content="{GoGetImport} _ {GoDocDirectory} {GoDocFile}">
	</head>
	<body>
		go get {InsecureFlag}{GoGetImport}
	</body>
</html>
`,
			map[string]string{
				"GoGetImport":    path.Join(conf.Server.URL.Host, conf.Server.Subpath, ownerName, repoName),
				"CloneLink":      repoutil.HTTPSCloneURL(ownerName, repoName),
				"GoDocDirectory": prefix + "{/dir}",
				"GoDocFile":      prefix + "{/dir}/{file}#L{line}",
				"InsecureFlag":   insecureFlag,
			},
		)))
	}
}
