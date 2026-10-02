package workspace

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// maxRead bounds what Init reads of a file the agent can write: a settings
// file, a file it compares before rewriting, a marker.
const maxRead = 1 << 20

var (
	errNotRegular = errors.New("not a regular file")
	errTooLarge   = errors.New("larger than 1 MiB")
)

// openRegular opens path for reading only if it is a regular file, and the
// same one Lstat saw. The agent can plant a FIFO, a device or a link to one
// (/dev/zero) where Init reads: that, or a file swapped in after the Lstat, is
// an error that wraps errNotRegular, never a read that hangs. On Unix the open
// itself does not block (openFlags), so a FIFO swapped in is not waited on.
func openRegular(path string) (*os.File, fs.FileInfo, error) {
	lst, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !lst.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s: %w", path, errNotRegular)
	}
	f, err := os.OpenFile(path, openFlags, 0)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !st.Mode().IsRegular() || !os.SameFile(lst, st) {
		f.Close()
		return nil, nil, fmt.Errorf("%s: %w", path, errNotRegular)
	}
	return f, st, nil
}

// readRegular reads the regular file at path (see openRegular) up to maxRead
// bytes. A longer one returns its first maxRead bytes and errTooLarge.
func readRegular(path string) ([]byte, error) {
	f, _, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRead+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRead {
		return data[:maxRead], fmt.Errorf("%s: %w", path, errTooLarge)
	}
	return data, nil
}
