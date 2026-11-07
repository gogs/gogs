package git

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepository_Diff(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		rev          string
		maxFiles     int
		maxFileLines int
		maxLineChars int
		opt          DiffOptions
		expDiff      *Diff
	}{
		{
			rev: "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			expDiff: &Diff{
				Files: []*DiffFile{
					{
						Name:         "fix.txt",
						Type:         DiffFileDelete,
						Index:        "0000000000000000000000000000000000000000",
						OldIndex:     "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391",
						Sections:     nil,
						numAdditions: 0,
						numDeletions: 0,
						oldName:      "fix.txt",
						isBinary:     false,
						isSubmodule:  false,
						isIncomplete: false,
						mode:         0100644,
						oldMode:      0100644,
					},
				},
				totalAdditions: 0,
				totalDeletions: 0,
				isIncomplete:   false,
			},
		},
		{
			rev: "755fd577edcfd9209d0ac072eed3b022cbe4d39b",
			expDiff: &Diff{
				Files: []*DiffFile{
					{
						Name:     "README.txt",
						Type:     DiffFileAdd,
						Index:    "1e24b564bf2298965d8037af42d3ae15ad7d225a",
						OldIndex: "0000000000000000000000000000000000000000",
						Sections: []*DiffSection{
							{
								Lines: []*DiffLine{
									{
										Type:    DiffLineSection,
										Content: "@@ -0,0 +1,11 @@",
									},
									{
										Type:      DiffLineAdd,
										Content:   "+This is a sample project students can use during Matthew's Git class.",
										LeftLine:  0,
										RightLine: 1,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+",
										LeftLine:  0,
										RightLine: 2,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+We can have a bit of fun with this repo, knowing that we can always reset it to a known good state.  We can apply labels, and branch, then add new code and merge it in to the master branch.",
										LeftLine:  0,
										RightLine: 3,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+",
										LeftLine:  0,
										RightLine: 4,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+As a quick reminder, this came from one of three locations in either SSH, Git, or HTTPS format:",
										LeftLine:  0,
										RightLine: 5,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+",
										LeftLine:  0,
										RightLine: 6,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+* git@github.com:matthewmccullough/hellogitworld.git",
										LeftLine:  0,
										RightLine: 7,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+* git://github.com/matthewmccullough/hellogitworld.git",
										LeftLine:  0,
										RightLine: 8,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+* https://matthewmccullough@github.com/matthewmccullough/hellogitworld.git",
										LeftLine:  0,
										RightLine: 9,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+",
										LeftLine:  0,
										RightLine: 10,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+We can, as an example effort, even modify this README and change it as if it were source code for the purposes of the class.",
										LeftLine:  0,
										RightLine: 11,
									},
								},
								numAdditions: 11,
							},
						},
						numAdditions: 11,
						numDeletions: 0,
						oldName:      "README.txt",
						isBinary:     false,
						isSubmodule:  false,
						isIncomplete: false,
						mode:         0100644,
						oldMode:      0100644,
					},
					{
						Name:     "resources/labels.properties",
						Type:     DiffFileAdd,
						Index:    "fbdcfef007c0c09061199e687087b18c3cf8e083",
						OldIndex: "0000000000000000000000000000000000000000",
						Sections: []*DiffSection{
							{
								Lines: []*DiffLine{
									{
										Type:    DiffLineSection,
										Content: "@@ -0,0 +1,4 @@",
									},
									{
										Type:      DiffLineAdd,
										Content:   "+app.title=Our App",
										LeftLine:  0,
										RightLine: 1,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+app.welcome=Welcome to the application",
										LeftLine:  0,
										RightLine: 2,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+app.goodbye=We hope you enjoyed using our application",
										LeftLine:  0,
										RightLine: 3,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+cli.usage=This application doesn't use a command line interface",
										LeftLine:  0,
										RightLine: 4,
									},
								},
								numAdditions: 4,
							},
						},
						numAdditions: 4,
						numDeletions: 0,
						oldName:      "resources/labels.properties",
						isBinary:     false,
						isSubmodule:  false,
						isIncomplete: false,
						mode:         0100644,
						oldMode:      0100644,
					},
					{
						Name:     "src/Main.groovy",
						Type:     DiffFileAdd,
						Index:    "51680791956b43effdb2f16bccd2b4752d66078f",
						OldIndex: "0000000000000000000000000000000000000000",
						Sections: []*DiffSection{
							{
								Lines: []*DiffLine{
									{
										Type:    DiffLineSection,
										Content: "@@ -0,0 +1,6 @@",
									},
									{
										Type:      DiffLineAdd,
										Content:   "+def name = \"Matthew\"",
										LeftLine:  0,
										RightLine: 1,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+",
										LeftLine:  0,
										RightLine: 2,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+println \"Hello ${name}\"",
										LeftLine:  0,
										RightLine: 3,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+",
										LeftLine:  0,
										RightLine: 4,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+int programmingPoints = 10",
										LeftLine:  0,
										RightLine: 5,
									},
									{
										Type:      DiffLineAdd,
										Content:   "+println \"${name} has at least ${programmingPoints} programming points.\"",
										LeftLine:  0,
										RightLine: 6,
									},
								},
								numAdditions: 6,
							},
						},
						numAdditions: 6,
						numDeletions: 0,
						oldName:      "src/Main.groovy",
						isBinary:     false,
						isSubmodule:  false,
						isIncomplete: false,
						mode:         0100644,
						oldMode:      0100644,
					},
				},
				totalAdditions: 21,
				totalDeletions: 0,
				isIncomplete:   false,
			},
		},
		{
			rev: "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			opt: DiffOptions{
				Base: "ef7bebf8bdb1919d947afe46ab4b2fb4278039b3",
			},
			expDiff: &Diff{
				Files: []*DiffFile{
					{
						Name:         "fix.txt",
						Type:         DiffFileDelete,
						Index:        "0000000000000000000000000000000000000000",
						OldIndex:     "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391",
						Sections:     nil,
						numAdditions: 0,
						numDeletions: 0,
						oldName:      "fix.txt",
						isBinary:     false,
						isSubmodule:  false,
						isIncomplete: false,
						mode:         0100644,
						oldMode:      0100644,
					},
				},
				totalAdditions: 0,
				totalDeletions: 0,
				isIncomplete:   false,
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			diff, err := testrepo.Diff(ctx, test.rev, test.maxFiles, test.maxFileLines, test.maxLineChars, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expDiff, diff)
		})
	}
}

func TestRepository_RawDiff(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid revision", func(t *testing.T) {
		err := testrepo.RawDiff(ctx, "bad_revision", "bad_diff_type", nil)
		assert.Equal(t, ErrRevisionNotExist, err)
	})

	t.Run("invalid diffType", func(t *testing.T) {
		err := testrepo.RawDiff(ctx, "978fb7f6388b49b532fbef8b856681cfa6fcaa0a", "bad_diff_type", nil)
		assert.Equal(t, errors.New("invalid diffType: bad_diff_type"), err)
	})

	tests := []struct {
		rev       string
		diffType  RawDiffFormat
		opt       RawDiffOptions
		expOutput string
	}{
		{
			rev:      "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			diffType: RawDiffNormal,
			expOutput: `diff --git a/fix.txt b/fix.txt
deleted file mode 100644
index e69de29bb2d1d6434b8b29ae775ad8c2e48c5391..0000000000000000000000000000000000000000
`,
		},
		{
			rev:      "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			diffType: RawDiffPatch,
			expOutput: `Date: Sun, 9 Feb 2020 17:22:24 +0800
Subject: [PATCH] Delete fix.txt

---
 fix.txt | 0
 1 file changed, 0 insertions(+), 0 deletions(-)
 delete mode 100644 fix.txt

diff --git a/fix.txt b/fix.txt
deleted file mode 100644
index e69de29bb2d1d6434b8b29ae775ad8c2e48c5391..0000000000000000000000000000000000000000
`,
		},
		{
			rev:      "755fd577edcfd9209d0ac072eed3b022cbe4d39b",
			diffType: RawDiffNormal,
			expOutput: `commit 755fd577edcfd9209d0ac072eed3b022cbe4d39b
Author: Matthew McCullough <matthewm@ambientideas.com>
Date:   Mon Nov 24 21:22:01 2008 -0700

    Addition of the README and basic Groovy source samples.
    
    - Addition of the README.txt file explaining what this repository is all about.
    - Addition of Groovy sample source.
    - Addition of sample resource Properties file.

diff --git a/README.txt b/README.txt
new file mode 100644
index 0000000000000000000000000000000000000000..1e24b564bf2298965d8037af42d3ae15ad7d225a
--- /dev/null
+++ b/README.txt
@@ -0,0 +1,11 @@
+This is a sample project students can use during Matthew's Git class.
+
+We can have a bit of fun with this repo, knowing that we can always reset it to a known good state.  We can apply labels, and branch, then add new code and merge it in to the master branch.
+
+As a quick reminder, this came from one of three locations in either SSH, Git, or HTTPS format:
+
+* git@github.com:matthewmccullough/hellogitworld.git
+* git://github.com/matthewmccullough/hellogitworld.git
+* https://matthewmccullough@github.com/matthewmccullough/hellogitworld.git
+
+We can, as an example effort, even modify this README and change it as if it were source code for the purposes of the class.
\ No newline at end of file
diff --git a/resources/labels.properties b/resources/labels.properties
new file mode 100644
index 0000000000000000000000000000000000000000..fbdcfef007c0c09061199e687087b18c3cf8e083
--- /dev/null
+++ b/resources/labels.properties
@@ -0,0 +1,4 @@
+app.title=Our App
+app.welcome=Welcome to the application
+app.goodbye=We hope you enjoyed using our application
+cli.usage=This application doesn't use a command line interface
diff --git a/src/Main.groovy b/src/Main.groovy
new file mode 100644
index 0000000000000000000000000000000000000000..51680791956b43effdb2f16bccd2b4752d66078f
--- /dev/null
+++ b/src/Main.groovy
@@ -0,0 +1,6 @@
+def name = "Matthew"
+
+println "Hello ${name}"
+
+int programmingPoints = 10
+println "${name} has at least ${programmingPoints} programming points."
\ No newline at end of file
`,
		},
		{
			rev:      "755fd577edcfd9209d0ac072eed3b022cbe4d39b",
			diffType: RawDiffPatch,
			expOutput: `Date: Mon, 24 Nov 2008 21:22:01 -0700
Subject: [PATCH] Addition of the README and basic Groovy source samples.

- Addition of the README.txt file explaining what this repository is all about.
- Addition of Groovy sample source.
- Addition of sample resource Properties file.
---
 README.txt                  | 11 +++++++++++
 resources/labels.properties |  4 ++++
 src/Main.groovy             |  6 ++++++
 3 files changed, 21 insertions(+)
 create mode 100644 README.txt
 create mode 100644 resources/labels.properties
 create mode 100644 src/Main.groovy

diff --git a/README.txt b/README.txt
new file mode 100644
index 0000000000000000000000000000000000000000..1e24b564bf2298965d8037af42d3ae15ad7d225a
--- /dev/null
+++ b/README.txt
@@ -0,0 +1,11 @@
+This is a sample project students can use during Matthew's Git class.
+
+We can have a bit of fun with this repo, knowing that we can always reset it to a known good state.  We can apply labels, and branch, then add new code and merge it in to the master branch.
+
+As a quick reminder, this came from one of three locations in either SSH, Git, or HTTPS format:
+
+* git@github.com:matthewmccullough/hellogitworld.git
+* git://github.com/matthewmccullough/hellogitworld.git
+* https://matthewmccullough@github.com/matthewmccullough/hellogitworld.git
+
+We can, as an example effort, even modify this README and change it as if it were source code for the purposes of the class.
\ No newline at end of file
diff --git a/resources/labels.properties b/resources/labels.properties
new file mode 100644
index 0000000000000000000000000000000000000000..fbdcfef007c0c09061199e687087b18c3cf8e083
--- /dev/null
+++ b/resources/labels.properties
@@ -0,0 +1,4 @@
+app.title=Our App
+app.welcome=Welcome to the application
+app.goodbye=We hope you enjoyed using our application
+cli.usage=This application doesn't use a command line interface
diff --git a/src/Main.groovy b/src/Main.groovy
new file mode 100644
index 0000000000000000000000000000000000000000..51680791956b43effdb2f16bccd2b4752d66078f
--- /dev/null
+++ b/src/Main.groovy
@@ -0,0 +1,6 @@
+def name = "Matthew"
+
+println "Hello ${name}"
+
+int programmingPoints = 10
+println "${name} has at least ${programmingPoints} programming points."
\ No newline at end of file
`,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			buf := new(bytes.Buffer)
			err := testrepo.RawDiff(ctx, test.rev, test.diffType, buf, test.opt)
			if err != nil {
				t.Fatal(err)
			}
			output := buf.String()

			// Only check the content after "Date:" line, which is deterministic.
			i := strings.Index(output, "Date:")
			if i > 0 && test.diffType == RawDiffPatch {
				output = output[i:]
			}

			assert.Equal(t, test.expOutput, output)
		})
	}
}

func TestRepository_DiffBinary(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		base      string
		head      string
		opt       DiffBinaryOptions
		expOutput string
	}{
		{
			base: "4eaa8d4b05e731e950e2eaf9e8b92f522303ab41",
			head: "4e59b72440188e7c2578299fc28ea425fbe9aece",
			expOutput: `diff --git a/.gitmodules b/.gitmodules
new file mode 100644
index 0000000000000000000000000000000000000000..6abde17f49a6d43df40366e57d8964fee0dfda11
--- /dev/null
+++ b/.gitmodules
@@ -0,0 +1,3 @@
+[submodule "gogs/docs-api"]
+	path = gogs/docs-api
+	url = https://github.com/gogs/docs-api.git
diff --git a/gogs/docs-api b/gogs/docs-api
new file mode 160000
index 0000000000000000000000000000000000000000..6b08f76a5313fa3d26859515b30aa17a5faa2807
--- /dev/null
+++ b/gogs/docs-api
@@ -0,0 +1 @@
+Subproject commit 6b08f76a5313fa3d26859515b30aa17a5faa2807
`,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			p, err := testrepo.DiffBinary(ctx, test.base, test.head, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expOutput, string(p))
		})
	}
}
