package web

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/repox"
)

func TestEmbedPermalinks(t *testing.T) {
	oldURL, oldRoot, oldMax := conf.Server.ExternalURL, conf.Repository.Root, conf.UI.MaxDisplayFileSize
	t.Cleanup(func() {
		conf.Server.ExternalURL, conf.Repository.Root, conf.UI.MaxDisplayFileSize = oldURL, oldRoot, oldMax
	})
	conf.Server.ExternalURL = "https://gogs.example.com/"
	conf.Repository.Root = t.TempDir()
	conf.UI.MaxDisplayFileSize = 1 << 20
	markup.NewSanitizer()

	var lines []string
	for i := 1; i <= 80; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	lines[2] = `if a < b && c > "d" {`
	content := strings.Join(lines, "\n") + "\n"
	long := strings.Repeat(strings.Repeat("x", 10*1024)+"\n", 3)
	stream := fmt.Sprintf("commit refs/heads/main\ncommitter T <t@example.com> 1700000000 +0000\ndata 1\nc\nM 100644 inline dir/a b.go\ndata %d\n%s\nM 100644 inline long.txt\ndata %d\n%s\n", len(content), content, len(long), long)
	repoPath := repox.RepositoryPath("alice", "demo")
	for _, args := range [][]string{{"init", "--bare", "-q", repoPath}, {"-C", repoPath, "fast-import", "--quiet"}} {
		cmd := exec.Command("git", args...)
		cmd.Stdin = strings.NewReader(stream)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "main").Output()
	require.NoError(t, err)
	commitID := strings.TrimSpace(string(out))

	link := "https://gogs.example.com/alice/demo/src/" + commitID + "/dir/a%20b.go"
	embed := func(first, last int, code string) string {
		return fmt.Sprintf(`<div data-permalink="%s#L%s" data-path="dir/a b.go" data-commit="%s" data-start="%d" data-end="%d"><pre><code>%s</code></pre></div>`+"\n",
			link, map[bool]string{true: fmt.Sprint(first), false: fmt.Sprintf("%d-L%d", first, last)}[first == last], commitID, first, last, code)
	}
	repoCtx := func(access database.AccessMode) *repoContext {
		return &repoContext{
			Owner:  &database.User{Name: "alice"},
			Repo:   &database.Repository{Name: "demo"},
			Viewer: &Viewer{access: access},
		}
	}
	renderAs := func(rc *repoContext, text string) string {
		return embedPermalinks(context.Background(), logx.NewNoopLogger(), rc, string(markup.Markdown(text, "/alice/demo", nil)))
	}
	render := func(text string) string {
		return renderAs(repoCtx(database.AccessModeRead), text)
	}

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "whole paragraph, escaped code",
			text: link + "#L2-L3",
			want: embed(2, 3, "line 2\nif a &lt; b &amp;&amp; c &gt; &#34;d&#34; {"),
		},
		{
			name: "line of a paragraph",
			text: "Look here:\n" + link + "#L5\nand here.",
			want: "<p>Look here:</p>\n" + embed(5, 5, "line 5") + "<p>and here.</p>\n",
		},
		{
			name: "two lines in a row",
			text: "See:\n" + link + "#L1\n" + link + "#L4",
			want: "<p>See:</p>\n" + embed(1, 1, "line 1") + embed(4, 4, "line 4"),
		},
		{
			name: "reversed range",
			text: link + "#L6-L5",
			want: strings.Replace(embed(5, 6, "line 5\nline 6"), "#L5-L6", "#L6-L5", 1),
		},
		{
			name: "range capped",
			text: link + "#L11-L80",
			want: strings.Replace(embed(11, 60, strings.Join(lines[10:60], "\n")), "#L11-L60", "#L11-L80", 1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, render(test.text))
		})
	}

	// Links that are not exactly a permalink to existing lines of this
	// repository stay links.
	for name, text := range map[string]string{
		"inline":          "See " + link + "#L5 for it.",
		"link text":       "[the code](" + link + "#L5)",
		"no lines":        link,
		"branch":          "https://gogs.example.com/alice/demo/src/main/dir/a%20b.go#L5",
		"short commit ID": "https://gogs.example.com/alice/demo/src/" + commitID[:10] + "/dir/a%20b.go#L5",
		"other repo":      "https://gogs.example.com/alice/other/src/" + commitID + "/dir/a%20b.go#L5",
		"other instance":  "https://other.example.com/alice/demo/src/" + commitID + "/dir/a%20b.go#L5",
		"past the end":    link + "#L80-L81",
		"missing file":    "https://gogs.example.com/alice/demo/src/" + commitID + "/nope.go#L1",
		"directory":       "https://gogs.example.com/alice/demo/src/" + commitID + "/dir#L1",
		"in a code block": "```\n" + link + "#L5\n```",
		"forged":          `<div data-permalink="` + link + `#L5" data-start="5" data-end="5"><pre><code>fake</code></pre></div>`,
	} {
		t.Run(name, func(t *testing.T) {
			got := render(text)
			assert.NotContains(t, got, "data-permalink")
		})
	}

	t.Run("viewer who cannot read the code", func(t *testing.T) {
		got := renderAs(repoCtx(database.AccessModeNone), link+"#L5")
		assert.NotContains(t, got, "data-permalink")
		assert.NotContains(t, got, "line 5")
	})

	t.Run("long lines are cut at whole lines", func(t *testing.T) {
		got := render("https://gogs.example.com/alice/demo/src/" + commitID + "/long.txt#L1-L3")
		assert.Contains(t, got, `data-start="1" data-end="1"`)
		assert.Contains(t, got, "<code>"+strings.Repeat("x", 10*1024)+"</code>")
	})

	t.Run("lookups per request are capped", func(t *testing.T) {
		// Links to lines that do not exist are looked up too, and the cap
		// holds over all the documents of the request.
		rc := repoCtx(database.AccessModeRead)
		missing := "https://gogs.example.com/alice/demo/src/" + commitID + "/nope.go#L1"
		renderAs(rc, strings.Repeat(missing+"\n\n", permalinkMaxLookups-2))
		assert.Equal(t, 2, strings.Count(renderAs(rc, strings.Repeat(link+"#L5\n\n", 5)), "data-permalink"))
		assert.NotContains(t, renderAs(rc, link+"#L5"), "data-permalink")
	})
}
