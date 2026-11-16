package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/flamego/flamego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/git"
)

func TestCompareRangeRoute(t *testing.T) {
	f := flamego.New()
	f.Get("/compare/"+compareRangeRoute, func(c flamego.Context) string {
		return c.Param("before") + " " + c.Param("after")
	})

	sha1Before, sha1After := strings.Repeat("a", 40), strings.Repeat("b", 40)
	sha256Before, sha256After := strings.Repeat("c", 64), strings.Repeat("d", 64)
	tests := []struct {
		name     string
		path     string
		wantCode int
		wantBody string
	}{
		{name: "SHA-1", path: sha1Before + "..." + sha1After, wantCode: http.StatusOK, wantBody: sha1Before + " " + sha1After},
		{name: "SHA-256", path: sha256Before + "..." + sha256After, wantCode: http.StatusOK, wantBody: sha256Before + " " + sha256After},
		{name: "abbreviated", path: sha1Before[:39] + "..." + sha1After, wantCode: http.StatusNotFound},
		{name: "between the two lengths", path: strings.Repeat("e", 50) + "..." + sha1After, wantCode: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp := httptest.NewRecorder()
			f.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/compare/"+test.path, nil))
			assert.Equal(t, test.wantCode, resp.Code)
			if test.wantBody != "" {
				assert.Equal(t, test.wantBody, resp.Body.String())
			}
		})
	}
}

func TestCompareRange_ListCommits(t *testing.T) {
	// The root commit and compareMaxCommits+1 commits after it, so that the
	// whole history is two commits longer than the list.
	var stream strings.Builder
	for i := 1; i <= compareMaxCommits+2; i++ {
		fmt.Fprintf(&stream, "commit refs/heads/main\nmark :%d\ncommitter T <t@example.com> %d +0000\ndata 3\nc%02d\n", i, 1700000000+i, i%100)
		if i > 1 {
			fmt.Fprintf(&stream, "from :%d\n", i-1)
		}
		stream.WriteString("\n")
	}
	path := t.TempDir()
	for _, args := range [][]string{{"init", "--bare", "-q", path}, {"-C", path, "fast-import", "--quiet"}} {
		cmd := exec.Command("git", args...)
		cmd.Stdin = strings.NewReader(stream.String())
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	ctx := context.Background()
	gitRepo, err := git.Open(path)
	require.NoError(t, err)
	history, err := gitRepo.RevList(ctx, []string{"main"})
	require.NoError(t, err)
	require.Len(t, history, compareMaxCommits+2)
	tip, root, second := history[0].ID.String(), history[len(history)-1].ID.String(), history[len(history)-2].ID.String()

	tests := []struct {
		name      string
		before    string
		wantTotal int64
		wantLen   int
	}{
		{name: "longer than the list", before: root, wantTotal: compareMaxCommits + 1, wantLen: compareMaxCommits},
		{name: "as long as the list", before: second, wantTotal: compareMaxCommits, wantLen: compareMaxCommits},
		{name: "short", before: history[3].ID.String(), wantTotal: 3, wantLen: 3},
		{name: "push of the default branch", before: git.ObjectFormatSHA1.EmptyID(), wantTotal: compareMaxCommits + 2, wantLen: compareMaxCommits},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r, err := resolveCompareRange(ctx, gitRepo, "main", test.before, tip)
			require.NoError(t, err)
			commits, total, err := r.listCommits(ctx, gitRepo)
			require.NoError(t, err)
			assert.Equal(t, test.wantTotal, total)
			require.Len(t, commits, test.wantLen)
			assert.Equal(t, tip, commits[0].ID.String())
			assert.Equal(t, history[test.wantLen-1].ID.String(), commits[test.wantLen-1].ID.String())
		})
	}
}
