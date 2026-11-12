package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTree_TreeEntry(t *testing.T) {
	ctx := context.Background()
	tree, err := testrepo.LsTree(ctx, "master")
	if err != nil {
		t.Fatal(err)
	}

	e, err := tree.TreeEntry(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, tree.id, e.ID())
	assert.Equal(t, ObjectTree, e.Type())
	assert.True(t, e.IsTree())
}

func TestTree_Blob(t *testing.T) {
	ctx := context.Background()
	tree, err := testrepo.LsTree(ctx, "d58e3ef9f123eea6857161c79275ee22b228f659")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("not a blob", func(t *testing.T) {
		_, err := tree.Blob(ctx, "src")
		assert.Equal(t, ErrNotBlob, err)
	})

	t.Run("get a blob", func(t *testing.T) {
		b, err := tree.Blob(ctx, "README.txt")
		if err != nil {
			t.Fatal(err)
		}

		assert.True(t, b.IsBlob())
	})

	t.Run("get an executable as blob", func(t *testing.T) {
		b, err := tree.Blob(ctx, "run.sh")
		if err != nil {
			t.Fatal(err)
		}

		assert.True(t, b.IsExec())
	})
}
