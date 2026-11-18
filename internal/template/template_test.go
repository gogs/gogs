package template

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/markup"
)

func TestRenderCommitMessage(t *testing.T) {
	markup.NewSanitizer()

	tests := []struct {
		name   string
		format string
		want   string
	}{
		{
			name: "internal tracker",
			want: `<a href="/alice/repo/issues/1" rel="nofollow">#1</a> fix &lt;b&gt; &amp; more`,
		},
		{
			name:   "HTTP format",
			format: "https://tracker.example.com/{user}/{repo}/{index}",
			want:   `<a href="https://tracker.example.com/alice/repo/1" rel="nofollow">#1</a> fix &lt;b&gt; &amp; more`,
		},
		{
			name:   "javascript format",
			format: "javascript:alert(1)//{index}",
			want:   `#1 fix &lt;b&gt; &amp; more`,
		},
		{
			name:   "mixed case javascript format",
			format: "JaVaScRiPt:alert(1)//{index}",
			want:   `#1 fix &lt;b&gt; &amp; more`,
		},
		{
			name:   "data format",
			format: "data:text/html,{index}",
			want:   `#1 fix &lt;b&gt; &amp; more`,
		},
		{
			name:   "vbscript format",
			format: "vbscript:{index}",
			want:   `#1 fix &lt;b&gt; &amp; more`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			metas := map[string]string{"user": "alice", "repo": "repo"}
			if tc.format != "" {
				metas["format"] = tc.format
				metas["style"] = markup.IssueNameStyleNumeric
			}
			got := RenderCommitMessage(false, "#1 fix <b> & more\n\nbody", "/alice/repo", metas)
			assert.Equal(t, tc.want, got)
		})
	}
}
