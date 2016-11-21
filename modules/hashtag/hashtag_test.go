package hashtag_test

import (
	"testing"

	. "code.gitea.io/gitea/modules/hashtag"
	"code.gitea.io/gitea/modules/setting"
	. "github.com/smartystreets/goconvey/convey"
	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/markdown"
)

func TestHashtag(t *testing.T) {
	var (
		user = &models.User{
			ID: 1,
			LowerName: "joesmith",
		}
	)
	setting.AppSubUrl = "http://example.com"

	Convey("Rendering markdown with hashtags of a <lang>-ubn-<book> repo", t, func() {
		var (
			repo = &models.Repository{
				ID: 1,
				LowerName: "en-ubn-act",
				Owner: user,
			}
		)

		Convey("Rendering a single hashtag", func() {
			So(string(ConvertHashtagsToLinks(repo, []byte(`#testtag`))), ShouldEqual, `<a href="http://example.com/joesmith/en-ubn/hashtags/testtag">#testtag</a>`)
		})

		Convey("Rendering markdown content into html with linked hashtag", func() {
			markdown_content := []byte(`# Acts 1
#author-luke

This is some text saying where a hashtag in this line such as #test should not be rendered, but the
following should be rendered as links except for #v12 which should not be a link:
#v12
#kingdomofgod
#da-god
`)
			html_content := markdown.Render(markdown_content, "content/01.md", repo.ComposeMetas())
			converted_hashtags := ConvertHashtagsToLinks(repo, html_content)
			So(string(converted_hashtags), ShouldEqual, `<h1>Acts 1</h1>

<p><a href="http://example.com/joesmith/en-ubn/hashtags/author-luke">#author-luke</a></p>

<p>This is some text saying where a hashtag in this line such as #test should not be rendered, but the
following should be rendered as links except for #v12 which should not be a link:
#v12
<a href="http://example.com/joesmith/en-ubn/hashtags/kingdomofgod">#kingdomofgod</a>
<a href="http://example.com/joesmith/en-ubn/hashtags/da-god">#da-god</a></p>
`)
		})
	})

	Convey("Rendering markdown of a NON <lang>-ubn-<book> repo", t, func() {
		var (
			repo = &models.Repository{
				ID: 1,
				LowerName: "en-act",
				Owner: user,
			}
		)

		Convey("Should not render a single hashtag", func() {
			So(string(ConvertHashtagsToLinks(repo, []byte(`#testtag`))), ShouldEqual, `#testtag`)
		})

		Convey("Should not render any hashtags of a markdown file", func() {
			markdown_content := []byte(`<h1>Acts 1</h1>

<p>#author-luke</p>

<p>This is some text where no hashtags should be linked since this is not a ubn repo.
#v12
#kingdomofgod
#da-god</p>
`)
			html_content := markdown.Render(markdown_content, "content/01.md", repo.ComposeMetas())
			converted_hashtags := ConvertHashtagsToLinks(repo, html_content)
			So(string(converted_hashtags), ShouldEqual, `<h1>Acts 1</h1>

<p>#author-luke</p>

<p>This is some text where no hashtags should be linked since this is not a ubn repo.
#v12
#kingdomofgod
#da-god</p>
`)
		})
	})
}
