package workspace

import (
	"errors"
	"fmt"
	"io"

	"github.com/hybriqtech/adbuddy/host/internal/safeio"
)

// maxRead bounds what Init reads of a file the agent can write: a settings
// file, a file it compares before rewriting, a marker.
const maxRead = 1 << 20

var (
	errNotRegular = safeio.ErrNotRegular
	errTooLarge   = errors.New("larger than 1 MiB")
)

// readRegular reads the regular file at path (see safeio.OpenRegular) up to maxRead
// bytes. A longer one returns its first maxRead bytes and errTooLarge.
func readRegular(path string) ([]byte, error) {
	f, _, err := safeio.OpenRegular(path)
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
