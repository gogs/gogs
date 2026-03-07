package iox

import (
	"io"
	"os"

	"github.com/cockroachdb/errors"
)

// CopyFile copies the file at src to dst, preserving file mode and
// modification time.
func CopyFile(src, dst string) error {
	si, err := os.Stat(src)
	if err != nil {
		return errors.Wrap(err, "stat source")
	}

	in, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "open source")
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return errors.Wrap(err, "create target")
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return errors.Wrap(err, "copy")
	}

	if err = out.Sync(); err != nil {
		return errors.Wrap(err, "sync target")
	}

	if err = os.Chmod(dst, si.Mode()); err != nil {
		return errors.Wrap(err, "chmod target")
	}

	if err = os.Chtimes(dst, si.ModTime(), si.ModTime()); err != nil {
		return errors.Wrap(err, "chtimes target")
	}

	return nil
}
