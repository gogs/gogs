package git

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlob(t *testing.T) {
	ctx := context.Background()
	expOutput := `This is a sample project students can use during Matthew's Git class.

Here is an addition by me

We can have a bit of fun with this repo, knowing that we can always reset it to a known good state.  We can apply labels, and branch, then add new code and merge it in to the master branch.

As a quick reminder, this came from one of three locations in either SSH, Git, or HTTPS format:

* git@github.com:matthewmccullough/hellogitworld.git
* git://github.com/matthewmccullough/hellogitworld.git
* https://matthewmccullough@github.com/matthewmccullough/hellogitworld.git

We can, as an example effort, even modify this README and change it as if it were source code for the purposes of the class.

This demo also includes an image with changes on a branch for examination of image diff on GitHub.
`

	blob := &Blob{
		TreeEntry: &TreeEntry{
			mode: EntryBlob,
			typ:  ObjectBlob,
			id:   MustIDFromString("adfd6da3c0a3fb038393144becbf37f14f780087"), // Blob ID of "README.txt" file
			parent: &Tree{
				repo: testrepo,
			},
		},
	}

	t.Run("get data all at once", func(t *testing.T) {
		p, err := blob.Bytes(ctx)
		assert.Nil(t, err)
		assert.Equal(t, expOutput, string(p))
	})

	t.Run("get data with pipe", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		err := blob.Pipe(ctx, stdout)
		assert.Nil(t, err)
		assert.Equal(t, expOutput, stdout.String())
	})
}
