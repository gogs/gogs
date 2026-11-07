package git

import (
	"bytes"
	"context"
	"io"
)

// Blob is a blob object.
type Blob struct {
	*TreeEntry
}

// Bytes reads and returns the content of the blob all at once in bytes. This can
// be very slow and memory consuming for huge content.
func (b *Blob) Bytes(ctx context.Context) ([]byte, error) {
	stdout := new(bytes.Buffer)

	// Preallocate memory to save ~50% memory usage on big files.
	if size := b.Size(ctx); size > 0 && size < int64(^uint(0)>>1) {
		stdout.Grow(int(size))
	}

	if err := b.Pipe(ctx, stdout); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

// Pipe reads the content of the blob and pipes stdout to the supplied io.Writer.
func (b *Blob) Pipe(ctx context.Context, stdout io.Writer) error {
	return pipe(ctx, b.parent.repo.path, []string{"show", "--end-of-options", b.id.String()}, nil, stdout)
}
