// Package safeio opens files in folders the agent can write to without being
// led elsewhere: never through a link or a junction in place of the folder,
// never into a FIFO or a device that would hang or flood the reader, never
// into a file that has a second name outside the folder (a hard link).
package safeio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

var (
	// ErrNotRegular: what is there is not a regular file (a folder, a
	// FIFO, a device, a link…).
	ErrNotRegular = errors.New("not a regular file")
	// ErrLinked: the file has more than one name (a hard link), and the
	// other one may be outside the folder.
	ErrLinked = errors.New("has more than one name (a hard link)")
	// ErrNotRealDir: the folder is a link, a junction or not a folder.
	ErrNotRealDir = errors.New("not a real folder (a link, a junction or a file)")
)

// OpenRegular opens path for reading only if it is a regular file, and the
// same one Lstat saw. The agent can plant a FIFO, a device or a link to one
// (/dev/zero) where ADBuddy reads: that, or a file swapped in after the
// Lstat, is an error that wraps ErrNotRegular, never a read that hangs. On
// Unix the open itself does not block (ReadFlags), so a FIFO swapped in is
// not waited on.
func OpenRegular(path string) (*os.File, fs.FileInfo, error) {
	lst, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !lst.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	f, err := os.OpenFile(path, ReadFlags, 0)
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
		return nil, nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	return f, st, nil
}

// OpenRoot opens dir as an os.Root only if dir itself is a real folder.
// os.OpenRoot follows a link or a junction in place of dir (only what is
// below it is kept inside); this does not: it checks dir with Lstat first
// and that the root it opened is that same folder.
func OpenRoot(dir string) (*os.Root, error) {
	lst, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if lst.Mode().Type() != fs.ModeDir {
		return nil, fmt.Errorf("%s: %w", dir, ErrNotRealDir)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	st, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	if !os.SameFile(lst, st) {
		root.Close()
		return nil, fmt.Errorf("%s: %w", dir, ErrNotRealDir)
	}
	return root, nil
}

// OpenRegularIn opens name in root for reading only if it is a regular file
// with a single name. The open does not block (ReadFlags).
func OpenRegularIn(root *os.Root, name string) (*os.File, error) {
	lst, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !lst.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", name, ErrNotRegular)
	}
	f, err := root.OpenFile(name, ReadFlags, 0)
	if err != nil {
		return nil, err
	}
	if err := checkOpened(f, name); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// OpenAppendIn opens name in root to read and append, creating it with perm
// when it is missing, only if it is a regular file with a single name. The
// open does not block (AppendFlags).
func OpenAppendIn(root *os.Root, name string, perm os.FileMode) (*os.File, error) {
	lst, err := root.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	case !lst.Mode().IsRegular():
		return nil, fmt.Errorf("%s: %w", name, ErrNotRegular)
	}
	f, err := root.OpenFile(name, AppendFlags, perm)
	if err != nil {
		return nil, err
	}
	if err := checkOpened(f, name); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// checkOpened checks what was opened, whatever the Lstat before it saw: a
// regular file with a single name.
func checkOpened(f *os.File, name string) error {
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%s: %w", name, ErrNotRegular)
	}
	n, err := linkCount(f, st)
	if err != nil {
		return err
	}
	if n > 1 {
		return fmt.Errorf("%s: %w", name, ErrLinked)
	}
	return nil
}
