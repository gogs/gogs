package context

import (
	"net/http"
	"path"
	"strings"

	"github.com/unknwon/com"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/repoutil"
)

// ServeGoGet does quick responses for appropriate go-get meta with status OK
// regardless of whether the user has access to the repository, or the repository
// does exist at all. This is particular a workaround for "go get" command which
// does not respect .netrc file.
func ServeGoGet() macaron.Handler {
	return func(c *macaron.Context) {
		if c.Query("go-get") != "1" {
			return
		}

		ownerName := c.Params(":username")
		repoName := c.Params(":reponame")
		branchName := "master"

		owner, err := db.Users.GetByUsername(c.Req.Context(), ownerName)
		if err == nil {
			repo, err := db.Repos.GetByName(c.Req.Context(), owner.ID, repoName)
			if err == nil && repo.DefaultBranch != "" {
				branchName = repo.DefaultBranch
			}
		}

		prefix := conf.Server.ExternalURL + path.Join(ownerName, repoName, "src", branchName)
		insecureFlag := ""
		if !strings.HasPrefix(conf.Server.ExternalURL, "https://") {
			insecureFlag = "--insecure "
		}
		c.PlainText(http.StatusOK, []byte(com.Expand(`<!doctype html>
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
