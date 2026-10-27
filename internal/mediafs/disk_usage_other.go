//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !windows

package mediafs

import (
	"errors"

	"github.com/syudead/vv/internal/domain"
)

func (FS) DiskUsage(string) (domain.DiskUsage, error) {
	return domain.DiskUsage{}, errors.ErrUnsupported
}
