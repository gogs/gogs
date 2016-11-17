// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package misc

import (
        api "code.gitea.io/sdk/gitea"

	"code.gitea.io/gitea/modules/context"
	"code.gitea.io/gitea/modules/yaml"
)

// https://github.com/gogits/go-gogs-client/wiki/Miscellaneous#render-an-arbitrary-markdown-document
func Yaml(ctx *context.APIContext, form api.YamlOption) {
	if ctx.HasApiError() {
		ctx.Error(422, "", ctx.GetErrMsg())
		return
	}

	if len(form.Text) == 0 {
		ctx.Write([]byte(""))
		return
	}

	ctx.Write(yaml.Render([]byte(form.Text)))
}

