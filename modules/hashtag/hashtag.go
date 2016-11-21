// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package hashtag

import (
	"regexp"
	"strings"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/models"
)

func ConvertHashtagsToLinks(repo *models.Repository, html []byte) []byte {
	repoName := repo.LowerName
	indexOfUbn := strings.Index(repoName, "-ubn-")
	if indexOfUbn > 0 {
		ubnRepo := repoName[0:indexOfUbn+4]
		hashtagsUrl := setting.AppSubUrl + "/" + repo.Owner.LowerName + "/" + ubnRepo + "/hashtags"
		re, _ := regexp.Compile(`(^|\n|<p>)#([A-Za-uw-z0-9_-][\w-]+|v\d+[A-Za-z_-][\w-]*|v[A-Za-z_-][^\w-]*)`)
		html = re.ReplaceAll(html, []byte("$1<a href=\"" + hashtagsUrl +"/$2\">#$2</a>$3"))
	}
	return html
}
