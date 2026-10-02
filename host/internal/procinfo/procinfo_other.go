//go:build !linux && !darwin

package procinfo

import "errors"

// Get: this system is not read here (Windows asks its job object instead).
func Get(int) (Info, error) { return Info{}, errors.ErrUnsupported }
