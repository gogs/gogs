package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommit(t *testing.T) {
	ctx := context.Background()
	c, err := testrepo.CatFileCommit(ctx, "435ffceb7ba576c937e922766e37d4f7abdcc122")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("ID", func(t *testing.T) {
		assert.Equal(t, "435ffceb7ba576c937e922766e37d4f7abdcc122", c.ID.String())
	})

	t.Run("Summary", func(t *testing.T) {
		assert.Equal(t, "Merge pull request #35 from githubtraining/travis-yml-docker", c.Summary())
	})
}

func TestCommit_Parent(t *testing.T) {
	ctx := context.Background()
	c, err := testrepo.CatFileCommit(ctx, "435ffceb7ba576c937e922766e37d4f7abdcc122")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ParentsCount", func(t *testing.T) {
		assert.Equal(t, 2, c.ParentsCount())
	})

	t.Run("Parent", func(t *testing.T) {
		t.Run("no such parent", func(t *testing.T) {
			_, err := c.Parent(ctx, c.ParentsCount()+1)
			assert.Equal(t, ErrParentNotExist, err)
		})

		tests := []struct {
			n           int
			expParentID string
		}{
			{
				n:           0,
				expParentID: "a13dba1e469944772490909daa58c53ac8fa4b0d",
			},
			{
				n:           1,
				expParentID: "7c5ee6478d137417ae602140c615e33aed91887c",
			},
		}
		for _, test := range tests {
			t.Run("", func(t *testing.T) {
				p, err := c.Parent(ctx, test.n)
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, test.expParentID, p.ID.String())
			})
		}
	})
}

func TestCommit_CommitByPath(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		id          string
		opt         CommitByRevisionOptions
		expCommitID string
	}{
		{
			id: "2a52e96389d02209b451ae1ddf45d645b42d744c",
			opt: CommitByRevisionOptions{
				Path: "", // No path gets back to the commit itself
			},
			expCommitID: "2a52e96389d02209b451ae1ddf45d645b42d744c",
		},
		{
			id: "2a52e96389d02209b451ae1ddf45d645b42d744c",
			opt: CommitByRevisionOptions{
				Path: "resources/labels.properties",
			},
			expCommitID: "755fd577edcfd9209d0ac072eed3b022cbe4d39b",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			c, err := testrepo.CatFileCommit(ctx, test.id)
			if err != nil {
				t.Fatal(err)
			}

			cc, err := c.CommitByPath(ctx, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expCommitID, cc.ID.String())
		})
	}
}

// commitsToIDs returns a list of IDs for given commits.
func commitsToIDs(commits []*Commit) []string {
	ids := make([]string, len(commits))
	for i := range commits {
		ids[i] = commits[i].ID.String()
	}
	return ids
}

func TestCommit_CommitsByPage(t *testing.T) {
	ctx := context.Background()
	// There are at most 5 commits can be used for pagination before this commit.
	c, err := testrepo.CatFileCommit(ctx, "f5ed01959cffa4758ca0a49bf4c34b138d7eab0a")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		page         int
		size         int
		opt          CommitsByPageOptions
		expCommitIDs []string
	}{
		{
			page: 0,
			size: 2,
			expCommitIDs: []string{
				"f5ed01959cffa4758ca0a49bf4c34b138d7eab0a",
				"9cdb160ee4118035bf73c744e3bf72a1ba16484a",
			},
		},
		{
			page: 1,
			size: 2,
			expCommitIDs: []string{
				"f5ed01959cffa4758ca0a49bf4c34b138d7eab0a",
				"9cdb160ee4118035bf73c744e3bf72a1ba16484a",
			},
		},
		{
			page: 2,
			size: 2,
			expCommitIDs: []string{
				"dc64fe4ab8618a5be491a9fca46f1585585ea44e",
				"32c273781bab599b955ce7c59d92c39bedf35db0",
			},
		},
		{
			page: 3,
			size: 2,
			expCommitIDs: []string{
				"755fd577edcfd9209d0ac072eed3b022cbe4d39b",
			},
		},
		{
			page:         4,
			size:         2,
			expCommitIDs: []string{},
		},

		{
			page: 2,
			size: 2,
			opt: CommitsByPageOptions{
				Path: "src",
			},
			expCommitIDs: []string{
				"755fd577edcfd9209d0ac072eed3b022cbe4d39b",
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			commits, err := c.CommitsByPage(ctx, test.page, test.size, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expCommitIDs, commitsToIDs(commits))
		})
	}
}

func TestCommit_SearchCommits(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		id           string
		pattern      string
		opt          SearchCommitsOptions
		expCommitIDs []string
	}{
		{
			id:      "2a52e96389d02209b451ae1ddf45d645b42d744c",
			pattern: "",
			expCommitIDs: []string{
				"2a52e96389d02209b451ae1ddf45d645b42d744c",
				"57d0bf61e57cdacb309ebd1075257c6bd7e1da81",
				"cb2d322bee073327e058143329d200024bd6b4c6",
				"818f033c4ae7f26b2b29e904942fa79a5ccaadd0",
				"369adba006a1bbf25e957a8622d2b919c994d035",
				"2956e1d20897bf6ed509f6429d7f64bc4823fe33",
				"333fd9bc94084c3e07e092e2bc9c22bab4476439",
				"f5ed01959cffa4758ca0a49bf4c34b138d7eab0a",
				"9cdb160ee4118035bf73c744e3bf72a1ba16484a",
				"dc64fe4ab8618a5be491a9fca46f1585585ea44e",
				"32c273781bab599b955ce7c59d92c39bedf35db0",
				"755fd577edcfd9209d0ac072eed3b022cbe4d39b",
			},
		},
		{
			id:      "2a52e96389d02209b451ae1ddf45d645b42d744c",
			pattern: "",
			opt: SearchCommitsOptions{
				MaxCount: 3,
			},
			expCommitIDs: []string{
				"2a52e96389d02209b451ae1ddf45d645b42d744c",
				"57d0bf61e57cdacb309ebd1075257c6bd7e1da81",
				"cb2d322bee073327e058143329d200024bd6b4c6",
			},
		},

		{
			id:      "2a52e96389d02209b451ae1ddf45d645b42d744c",
			pattern: "feature",
			expCommitIDs: []string{
				"2a52e96389d02209b451ae1ddf45d645b42d744c",
				"cb2d322bee073327e058143329d200024bd6b4c6",
			},
		},
		{
			id:      "2a52e96389d02209b451ae1ddf45d645b42d744c",
			pattern: "feature",
			opt: SearchCommitsOptions{
				MaxCount: 1,
			},
			expCommitIDs: []string{
				"2a52e96389d02209b451ae1ddf45d645b42d744c",
			},
		},

		{
			id:      "2a52e96389d02209b451ae1ddf45d645b42d744c",
			pattern: "add.*",
			opt: SearchCommitsOptions{
				Path: "src",
			},
			expCommitIDs: []string{
				"cb2d322bee073327e058143329d200024bd6b4c6",
				"818f033c4ae7f26b2b29e904942fa79a5ccaadd0",
				"333fd9bc94084c3e07e092e2bc9c22bab4476439",
				"32c273781bab599b955ce7c59d92c39bedf35db0",
				"755fd577edcfd9209d0ac072eed3b022cbe4d39b",
			},
		},
		{
			id:      "2a52e96389d02209b451ae1ddf45d645b42d744c",
			pattern: "add.*",
			opt: SearchCommitsOptions{
				MaxCount: 2,
				Path:     "src",
			},
			expCommitIDs: []string{
				"cb2d322bee073327e058143329d200024bd6b4c6",
				"818f033c4ae7f26b2b29e904942fa79a5ccaadd0",
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			c, err := testrepo.CatFileCommit(ctx, test.id)
			if err != nil {
				t.Fatal(err)
			}

			commits, err := c.SearchCommits(ctx, test.pattern, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expCommitIDs, commitsToIDs(commits))
		})
	}
}

func TestCommit_ShowNameStatus(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		id        string
		opt       ShowNameStatusOptions
		expStatus *NameStatus
	}{
		{
			id: "755fd577edcfd9209d0ac072eed3b022cbe4d39b",
			expStatus: &NameStatus{
				Added: []string{
					"README.txt",
					"resources/labels.properties",
					"src/Main.groovy",
				},
			},
		},
		{
			id: "32c273781bab599b955ce7c59d92c39bedf35db0",
			expStatus: &NameStatus{
				Modified: []string{
					"src/Main.groovy",
				},
			},
		},
		{
			id: "dc64fe4ab8618a5be491a9fca46f1585585ea44e",
			expStatus: &NameStatus{
				Added: []string{
					"src/Square.groovy",
				},
				Modified: []string{
					"src/Main.groovy",
				},
			},
		},
		{
			id: "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			expStatus: &NameStatus{
				Removed: []string{
					"fix.txt",
				},
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			c, err := testrepo.CatFileCommit(ctx, test.id)
			if err != nil {
				t.Fatal(err)
			}

			status, err := c.ShowNameStatus(ctx, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expStatus, status)
		})
	}
}

func TestCommit_CommitsCount(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		id       string
		opt      RevListCountOptions
		expCount int64
	}{
		{
			id:       "755fd577edcfd9209d0ac072eed3b022cbe4d39b",
			expCount: 1,
		},
		{
			id:       "f5ed01959cffa4758ca0a49bf4c34b138d7eab0a",
			expCount: 5,
		},
		{
			id:       "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			expCount: 27,
		},

		{
			id: "7c5ee6478d137417ae602140c615e33aed91887c",
			opt: RevListCountOptions{
				Path: "README.txt",
			},
			expCount: 3,
		},
		{
			id: "7c5ee6478d137417ae602140c615e33aed91887c",
			opt: RevListCountOptions{
				Path: "resources",
			},
			expCount: 1,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			c, err := testrepo.CatFileCommit(ctx, test.id)
			if err != nil {
				t.Fatal(err)
			}

			count, err := c.CommitsCount(ctx, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expCount, count)
		})
	}
}

func TestCommit_FilesChangedAfter(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		id       string
		after    string
		opt      DiffNameOnlyOptions
		expFiles []string
	}{
		{
			id:       "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			after:    "ef7bebf8bdb1919d947afe46ab4b2fb4278039b3",
			expFiles: []string{"fix.txt"},
		},
		{
			id:       "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			after:    "45a30ea9afa413e226ca8614179c011d545ca883",
			expFiles: []string{"fix.txt", "pom.xml", "src/test/java/com/github/AppTest.java"},
		},

		{
			id:    "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			after: "45a30ea9afa413e226ca8614179c011d545ca883",
			opt: DiffNameOnlyOptions{
				Path: "src",
			},
			expFiles: []string{"src/test/java/com/github/AppTest.java"},
		},
		{
			id:    "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			after: "45a30ea9afa413e226ca8614179c011d545ca883",
			opt: DiffNameOnlyOptions{
				Path: "resources",
			},
			expFiles: []string{},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			c, err := testrepo.CatFileCommit(ctx, test.id)
			if err != nil {
				t.Fatal(err)
			}

			files, err := c.FilesChangedAfter(ctx, test.after, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expFiles, files)
		})
	}
}

func TestCommit_CommitsAfter(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		id           string
		after        string
		opt          RevListOptions
		expCommitIDs []string
	}{
		{
			id:    "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			after: "45a30ea9afa413e226ca8614179c011d545ca883",
			expCommitIDs: []string{
				"978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
				"ef7bebf8bdb1919d947afe46ab4b2fb4278039b3",
				"ebbbf773431ba07510251bb03f9525c7bab2b13a",
			},
		},
		{
			id:    "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			after: "45a30ea9afa413e226ca8614179c011d545ca883",
			opt: RevListOptions{
				Path: "src",
			},
			expCommitIDs: []string{
				"ebbbf773431ba07510251bb03f9525c7bab2b13a",
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			c, err := testrepo.CatFileCommit(ctx, test.id)
			if err != nil {
				t.Fatal(err)
			}

			commits, err := c.CommitsAfter(ctx, test.after, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expCommitIDs, commitsToIDs(commits))
		})
	}
}

func TestCommit_Ancestors(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		id           string
		opt          LogOptions
		expCommitIDs []string
	}{
		{
			id: "2a52e96389d02209b451ae1ddf45d645b42d744c",
			opt: LogOptions{
				MaxCount: 3,
			},
			expCommitIDs: []string{
				"57d0bf61e57cdacb309ebd1075257c6bd7e1da81",
				"cb2d322bee073327e058143329d200024bd6b4c6",
				"818f033c4ae7f26b2b29e904942fa79a5ccaadd0",
			},
		},
		{
			id:           "755fd577edcfd9209d0ac072eed3b022cbe4d39b",
			expCommitIDs: []string{},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			c, err := testrepo.CatFileCommit(ctx, test.id)
			if err != nil {
				t.Fatal(err)
			}

			commits, err := c.Ancestors(ctx, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expCommitIDs, commitsToIDs(commits))
		})
	}
}

func TestCommit_IsImageFile(t *testing.T) {
	ctx := context.Background()

	t.Run("not a blob", func(t *testing.T) {
		c, err := testrepo.CatFileCommit(ctx, "4e59b72440188e7c2578299fc28ea425fbe9aece")
		if err != nil {
			t.Fatal(err)
		}

		isImage, err := c.IsImageFile(ctx, "gogs/docs-api")
		if err != nil {
			t.Fatal(err)
		}
		assert.False(t, isImage)
	})

	tests := []struct {
		id     string
		name   string
		expVal bool
	}{
		{
			id:     "4eaa8d4b05e731e950e2eaf9e8b92f522303ab41",
			name:   "README.txt",
			expVal: false,
		},
		{
			id:     "4eaa8d4b05e731e950e2eaf9e8b92f522303ab41",
			name:   "img/sourcegraph.png",
			expVal: true,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			c, err := testrepo.CatFileCommit(ctx, test.id)
			if err != nil {
				t.Fatal(err)
			}

			isImage, err := c.IsImageFile(ctx, test.name)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expVal, isImage)
		})
	}
}

func TestCommit_IsImageFileByIndex(t *testing.T) {
	ctx := context.Background()

	t.Run("not a blob", func(t *testing.T) {
		c, err := testrepo.CatFileCommit(ctx, "4e59b72440188e7c2578299fc28ea425fbe9aece")
		if err != nil {
			t.Fatal(err)
		}

		isImage, err := c.IsImageFileByIndex(ctx, "fcf7087e732bfe3c25328248a9bf8c3ccd85bed4") // "gogs"
		if err != nil {
			t.Fatal(err)
		}
		assert.False(t, isImage)
	})

	tests := []struct {
		id     string
		index  string
		expVal bool
	}{
		{
			id:     "4eaa8d4b05e731e950e2eaf9e8b92f522303ab41",
			index:  "adfd6da3c0a3fb038393144becbf37f14f780087", // "README.txt"
			expVal: false,
		},
		{
			id:     "4eaa8d4b05e731e950e2eaf9e8b92f522303ab41",
			index:  "2ce918888b0fdd4736767360fc5e3e83daf47fce", // "img/sourcegraph.png"
			expVal: true,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			c, err := testrepo.CatFileCommit(ctx, test.id)
			if err != nil {
				t.Fatal(err)
			}

			isImage, err := c.IsImageFileByIndex(ctx, test.index)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expVal, isImage)
		})
	}
}
