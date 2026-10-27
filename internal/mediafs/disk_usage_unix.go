//go:build linux || darwin || freebsd || netbsd || openbsd

package mediafs

import (
	"golang.org/x/sys/unix"

	"github.com/syudead/vv/internal/domain"
)

// DiskUsage reports the space available to unprivileged users, as df does.
func (FS) DiskUsage(path string) (domain.DiskUsage, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return domain.DiskUsage{}, err
	}
	size := uint64(st.Bsize)
	return domain.DiskUsage{
		FreeBytes:  int64(uint64(st.Bavail) * size),
		TotalBytes: int64(uint64(st.Blocks) * size),
	}, nil
}
