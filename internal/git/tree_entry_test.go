package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTreeEntry(t *testing.T) {
	id := MustIDFromString("0eedd79eba4394bbef888c804e899731644367fe")
	e := &TreeEntry{
		mode: EntrySymlink,
		typ:  ObjectTree,
		id:   id,
		name: "go.mod",
	}

	assert.False(t, e.IsTree())
	assert.False(t, e.IsBlob())
	assert.False(t, e.IsExec())
	assert.True(t, e.IsSymlink())
	assert.False(t, e.IsCommit())

	assert.Equal(t, ObjectTree, e.Type())
	assert.Equal(t, e.id, e.ID())
	assert.Equal(t, "go.mod", e.Name())
}

func TestTreeEntry_Size(t *testing.T) {
	ctx := context.Background()
	tree, err := testrepo.LsTree(ctx, "0eedd79eba4394bbef888c804e899731644367fe")
	require.NoError(t, err)

	es, err := tree.Entries(ctx)
	require.NoError(t, err)

	t.Run("blob", func(t *testing.T) {
		var entry *TreeEntry
		for _, e := range es {
			if e.Name() == "README.txt" {
				entry = e
				break
			}
		}
		require.NotNil(t, entry, "entry README.txt not found")
		assert.Equal(t, int64(795), entry.Size(ctx))
	})

	t.Run("tree returns zero", func(t *testing.T) {
		var entry *TreeEntry
		for _, e := range es {
			if e.IsTree() {
				entry = e
				break
			}
		}
		require.NotNil(t, entry, "tree entry not found")
		assert.Equal(t, int64(0), entry.Size(ctx))
	})
}

func TestEntries_Sort(t *testing.T) {
	ctx := context.Background()
	tree, err := testrepo.LsTree(ctx, "0eedd79eba4394bbef888c804e899731644367fe")
	if err != nil {
		t.Fatal(err)
	}

	es, err := tree.Entries(ctx)
	if err != nil {
		t.Fatal(err)
	}

	es.Sort()

	expEntries := []*TreeEntry{
		{
			mode: EntryTree,
			typ:  ObjectTree,
			id:   MustIDFromString("fcf7087e732bfe3c25328248a9bf8c3ccd85bed4"),
			name: "gogs",
		}, {
			mode: EntryTree,
			typ:  ObjectTree,
			id:   MustIDFromString("a41a5a6cfd2d5ec3c0c1101e7cc05c9dedc3e11d"),
			name: "img",
		}, {
			mode: EntryTree,
			typ:  ObjectTree,
			id:   MustIDFromString("aaa0af6b82db99c660b169962524e2201ac7079c"),
			name: "resources",
		}, {
			mode: EntryTree,
			typ:  ObjectTree,
			id:   MustIDFromString("007cb92318c7bd3b56908ea8c2e54370245562f8"),
			name: "src",
		}, {
			mode: EntryBlob,
			typ:  ObjectBlob,
			id:   MustIDFromString("021a721a61a1de65865542c405796d1eb985f784"),
			name: ".DS_Store",
		}, {
			mode: EntryBlob,
			typ:  ObjectBlob,
			id:   MustIDFromString("412eeda78dc9de1186c2e0e1526764af82ab3431"),
			name: ".gitattributes",
		}, {
			mode: EntryBlob,
			typ:  ObjectBlob,
			id:   MustIDFromString("7c820833a9ad5fbfc96efd533d55f5edc65dc977"),
			name: ".gitignore",
		}, {
			mode: EntryBlob,
			typ:  ObjectBlob,
			id:   MustIDFromString("6abde17f49a6d43df40366e57d8964fee0dfda11"),
			name: ".gitmodules",
		}, {
			mode: EntryBlob,
			typ:  ObjectBlob,
			id:   MustIDFromString("17eccd68b7cafa718d53c8b4db666194646e2bd9"),
			name: ".travis.yml",
		}, {
			mode: EntryBlob,
			typ:  ObjectBlob,
			id:   MustIDFromString("adfd6da3c0a3fb038393144becbf37f14f780087"),
			name: "README.txt",
		}, {
			mode: EntryBlob,
			typ:  ObjectBlob,
			id:   MustIDFromString("6058be211566308428ca6dcab3f08cf270cd9568"),
			name: "build.gradle",
		}, {
			mode: EntryBlob,
			typ:  ObjectBlob,
			id:   MustIDFromString("99975710477a65b89233b2d12bf60f7c0ffc1f5c"),
			name: "pom.xml",
		}, {
			mode: EntryExec,
			typ:  ObjectBlob,
			id:   MustIDFromString("fb4bd4ec9220ed4fe0d9526d1b77147490ce8842"),
			name: "run.sh",
		},
	}
	for i := range expEntries {
		assert.Equal(t, expEntries[i].Mode(), es[i].Mode(), "idx: %d", i)
		assert.Equal(t, expEntries[i].Type(), es[i].Type(), "idx: %d", i)
		assert.Equal(t, expEntries[i].ID().String(), es[i].ID().String(), "idx: %d", i)
		assert.Equal(t, expEntries[i].Name(), es[i].Name(), "idx: %d", i)
	}
}

func TestEntries_CommitsInfo(t *testing.T) {
	ctx := context.Background()
	tree, err := testrepo.LsTree(ctx, "cfc3b2993f74726356887a5ec093de50486dc617")
	if err != nil {
		t.Fatal(err)
	}

	c, err := testrepo.CatFileCommit(ctx, tree.id.String())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("general directory", func(t *testing.T) {
		es, err := tree.Entries(ctx)
		if err != nil {
			t.Fatal(err)
		}

		infos, err := es.CommitsInfo(ctx, c)
		if err != nil {
			t.Fatal(err)
		}

		expInfos := []*EntryCommitInfo{
			{
				Entry: &TreeEntry{
					name: ".DS_Store",
				},
				Commit: &Commit{
					ID: MustIDFromString("4eaa8d4b05e731e950e2eaf9e8b92f522303ab41"),
				},
			}, {
				Entry: &TreeEntry{
					name: ".gitattributes",
				},
				Commit: &Commit{
					ID: MustIDFromString("bf7a9a5ee025edee0e610bd7ba23c0704b53c6db"),
				},
			}, {
				Entry: &TreeEntry{
					name: ".gitignore",
				},
				Commit: &Commit{
					ID: MustIDFromString("d2280d000c84f1e595e4dec435ae6c1e6c245367"),
				},
			}, {
				Entry: &TreeEntry{
					name: ".gitmodules",
				},
				Commit: &Commit{
					ID: MustIDFromString("4e59b72440188e7c2578299fc28ea425fbe9aece"),
				},
			}, {
				Entry: &TreeEntry{
					name: ".travis.yml",
				},
				Commit: &Commit{
					ID: MustIDFromString("9805760644754c38d10a9f1522a54a4bdc00fa8a"),
				},
			}, {
				Entry: &TreeEntry{
					name: "README.txt",
				},
				Commit: &Commit{
					ID: MustIDFromString("a13dba1e469944772490909daa58c53ac8fa4b0d"),
				},
			}, {
				Entry: &TreeEntry{
					name: "build.gradle",
				},
				Commit: &Commit{
					ID: MustIDFromString("c59479302142d79e46f84d11438a41b39ba51a1f"),
				},
			}, {
				Entry: &TreeEntry{
					name: "gogs",
				},
				Commit: &Commit{
					ID: MustIDFromString("4e59b72440188e7c2578299fc28ea425fbe9aece"),
				},
			}, {
				Entry: &TreeEntry{
					name: "img",
				},
				Commit: &Commit{
					ID: MustIDFromString("4eaa8d4b05e731e950e2eaf9e8b92f522303ab41"),
				},
			}, {
				Entry: &TreeEntry{
					name: "pom.xml",
				},
				Commit: &Commit{
					ID: MustIDFromString("ef7bebf8bdb1919d947afe46ab4b2fb4278039b3"),
				},
			}, {
				Entry: &TreeEntry{
					name: "resources",
				},
				Commit: &Commit{
					ID: MustIDFromString("755fd577edcfd9209d0ac072eed3b022cbe4d39b"),
				},
			}, {
				Entry: &TreeEntry{
					name: "run.sh",
				},
				Commit: &Commit{
					ID: MustIDFromString("0eedd79eba4394bbef888c804e899731644367fe"),
				},
			}, {
				Entry: &TreeEntry{
					name: "sameSHAs",
				},
				Commit: &Commit{
					ID: MustIDFromString("cfc3b2993f74726356887a5ec093de50486dc617"),
				},
			}, {
				Entry: &TreeEntry{
					name: "src",
				},
				Commit: &Commit{
					ID: MustIDFromString("ebbbf773431ba07510251bb03f9525c7bab2b13a"),
				},
			},
		}
		for i := range expInfos {
			assert.Equal(t, expInfos[i].Entry.Name(), infos[i].Entry.Name(), "idx: %d", i)
			assert.Equal(t, expInfos[i].Commit.ID.String(), infos[i].Commit.ID.String(), "idx: %d", i)
		}
	})

	t.Run("directory with submodule", func(t *testing.T) {
		subtree, err := tree.Subtree(ctx, "gogs")
		if err != nil {
			t.Fatal(err)
		}

		es, err := subtree.Entries(ctx)
		if err != nil {
			t.Fatal(err)
		}

		infos, err := es.CommitsInfo(ctx, c, CommitsInfoOptions{
			Path: "gogs",
		})
		if err != nil {
			t.Fatal(err)
		}

		expInfos := []*EntryCommitInfo{
			{
				Entry: &TreeEntry{
					name: "docs-api",
				},
				Commit: &Commit{
					ID: MustIDFromString("4e59b72440188e7c2578299fc28ea425fbe9aece"),
				},
			},
		}
		for i := range expInfos {
			assert.Equal(t, expInfos[i].Entry.Name(), infos[i].Entry.Name(), "idx: %d", i)
			assert.Equal(t, expInfos[i].Commit.ID.String(), infos[i].Commit.ID.String(), "idx: %d", i)
		}
	})

	t.Run("directory with files that have the same SHA", func(t *testing.T) {
		subtree, err := tree.Subtree(ctx, "sameSHAs")
		if err != nil {
			t.Fatal(err)
		}

		es, err := subtree.Entries(ctx)
		if err != nil {
			t.Fatal(err)
		}

		infos, err := es.CommitsInfo(ctx, c, CommitsInfoOptions{
			Path: "sameSHAs",
		})
		if err != nil {
			t.Fatal(err)
		}

		expInfos := []*EntryCommitInfo{
			{
				Entry: &TreeEntry{
					name: "file1.txt",
				},
				Commit: &Commit{
					ID: MustIDFromString("cfc3b2993f74726356887a5ec093de50486dc617"),
				},
			}, {
				Entry: &TreeEntry{
					name: "file2.txt",
				},
				Commit: &Commit{
					ID: MustIDFromString("cfc3b2993f74726356887a5ec093de50486dc617"),
				},
			},
		}
		for i := range expInfos {
			assert.Equal(t, expInfos[i].Entry.Name(), infos[i].Entry.Name(), "idx: %d", i)
			assert.Equal(t, expInfos[i].Commit.ID.String(), infos[i].Commit.ID.String(), "idx: %d", i)
		}
	})
}
