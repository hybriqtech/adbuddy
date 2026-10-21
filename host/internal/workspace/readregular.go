package workspace

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/hybriqtech/adbuddy/host/internal/safeio"
)

// maxRead bounds what Init reads of a file the agent can write: a settings
// file, a file it compares before rewriting, a marker.
const maxRead = 1 << 20

var (
	errNotRegular = safeio.ErrNotRegular
	errTooLarge   = errors.New("larger than 1 MiB")
)

// openRegular opens path for reading only if it is a regular file, and the
// same one Lstat saw (safeio.OpenRegular): a FIFO, a device or a link to one
// is an error that wraps errNotRegular, never a read that hangs.
func openRegular(path string) (*os.File, fs.FileInfo, error) { return safeio.OpenRegular(path) }

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
