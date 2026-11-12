package git

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepository_Grep_Simple(t *testing.T) {
	ctx := context.Background()
	want := []*GrepResult{
		{
			Tree:   "HEAD",
			Path:   "src/Main.groovy",
			Line:   7,
			Column: 5,
			Text:   "int programmingPoints = 10",
		}, {
			Tree:   "HEAD",
			Path:   "src/Main.groovy",
			Line:   10,
			Column: 33,
			Text:   `println "${name} has at least ${programmingPoints} programming points."`,
		}, {
			Tree:   "HEAD",
			Path:   "src/Main.groovy",
			Line:   11,
			Column: 12,
			Text:   `println "${programmingPoints} squared is ${square(programmingPoints)}"`,
		}, {
			Tree:   "HEAD",
			Path:   "src/Main.groovy",
			Line:   12,
			Column: 12,
			Text:   `println "${programmingPoints} divided by 2 bonus points is ${divide(programmingPoints, 2)}"`,
		}, {
			Tree:   "HEAD",
			Path:   "src/Main.groovy",
			Line:   13,
			Column: 12,
			Text:   `println "${programmingPoints} minus 7 bonus points is ${subtract(programmingPoints, 7)}"`,
		}, {
			Tree:   "HEAD",
			Path:   "src/Main.groovy",
			Line:   14,
			Column: 12,
			Text:   `println "${programmingPoints} plus 3 bonus points is ${sum(programmingPoints, 3)}"`,
		},
	}
	got := testrepo.Grep(ctx, "programmingPoints")
	assert.Equal(t, want, got)
}

func TestRepository_Grep_IgnoreCase(t *testing.T) {
	ctx := context.Background()
	want := []*GrepResult{
		{
			Tree:   "HEAD",
			Path:   "README.txt",
			Line:   9,
			Column: 36,
			Text:   "* git@github.com:matthewmccullough/hellogitworld.git",
		}, {
			Tree:   "HEAD",
			Path:   "README.txt",
			Line:   10,
			Column: 38,
			Text:   "* git://github.com/matthewmccullough/hellogitworld.git",
		}, {
			Tree:   "HEAD",
			Path:   "README.txt",
			Line:   11,
			Column: 58,
			Text:   "* https://matthewmccullough@github.com/matthewmccullough/hellogitworld.git",
		}, {
			Tree:   "HEAD",
			Path:   "src/Main.groovy",
			Line:   9,
			Column: 10,
			Text:   `println "Hello ${name}"`,
		}, {
			Tree:   "HEAD",
			Path:   "src/main/java/com/github/App.java",
			Line:   4,
			Column: 4,
			Text:   " * Hello again",
		}, {
			Tree:   "HEAD",
			Path:   "src/main/java/com/github/App.java",
			Line:   5,
			Column: 4,
			Text:   " * Hello world!",
		}, {
			Tree:   "HEAD",
			Path:   "src/main/java/com/github/App.java",
			Line:   6,
			Column: 4,
			Text:   " * Hello",
		}, {
			Tree:   "HEAD",
			Path:   "src/main/java/com/github/App.java",
			Line:   13,
			Column: 30,
			Text:   `        System.out.println( "Hello World!" );`,
		},
	}
	got := testrepo.Grep(ctx, "Hello", GrepOptions{IgnoreCase: true})
	assert.Equal(t, want, got)
}

func TestRepository_Grep_ExtendedRegexp(t *testing.T) {
	ctx := context.Background()
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping testing on macOS")
		return
	}
	want := []*GrepResult{
		{
			Tree:   "HEAD",
			Path:   "src/main/java/com/github/App.java",
			Line:   13,
			Column: 30,
			Text:   `        System.out.println( "Hello World!" );`,
		},
	}
	got := testrepo.Grep(ctx, `Hello\sW\w+`, GrepOptions{ExtendedRegexp: true})
	assert.Equal(t, want, got)
}

func TestRepository_Grep_WordRegexp(t *testing.T) {
	ctx := context.Background()
	want := []*GrepResult{
		{
			Tree:   "HEAD",
			Path:   "src/main/java/com/github/App.java",
			Line:   5,
			Column: 10,
			Text:   ` * Hello world!`,
		},
	}
	got := testrepo.Grep(ctx, "world", GrepOptions{WordRegexp: true})
	assert.Equal(t, want, got)
}
