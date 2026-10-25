package safeio

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// AtomicOpts says how WriteFileAtomic makes the file.
type AtomicOpts struct {
	// Pattern is the os.CreateTemp pattern of the temporary file, in the
	// folder of the target.
	Pattern string
	// Perm is the mode the temporary file gets (Chmod) before the rename; 0
	// leaves the 0600 of os.CreateTemp as it is.
	Perm fs.FileMode
	// KeepMode gives the file the permission bits the target has now (read
	// first: it must exist), by path and ignoring a failure. Perm is not used.
	KeepMode bool
	// TolerateChmod ignores a failed Perm Chmod on Windows.
	TolerateChmod bool
}

// WriteFileAtomic writes data to path through a new temporary file renamed
// over it, so it never writes through a link someone left at path. The
// temporary file is removed if anything fails.
func WriteFileAtomic(path string, data []byte, o AtomicOpts) error {
	var keep fs.FileMode
	if o.KeepMode {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		keep = info.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(path), o.Pattern)
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // after a successful rename it no longer exists
	if o.Perm != 0 && !o.KeepMode {
		if err := f.Chmod(o.Perm); err != nil && !(o.TolerateChmod && runtime.GOOS == "windows") {
			f.Close()
			return err
		}
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if o.KeepMode {
		_ = os.Chmod(tmp, keep)
	}
	return os.Rename(tmp, path)
}
