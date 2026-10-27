package mediafs

import (
	"golang.org/x/sys/windows"

	"github.com/syudead/vv/internal/domain"
)

// DiskUsage reports the space available to this process's user, as Explorer does.
func (FS) DiskUsage(path string) (domain.DiskUsage, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return domain.DiskUsage{}, err
	}
	var free, total uint64
	if err := windows.GetDiskFreeSpaceEx(name, &free, &total, nil); err != nil {
		return domain.DiskUsage{}, err
	}
	return domain.DiskUsage{
		FreeBytes:  int64(free),
		TotalBytes: int64(total),
	}, nil
}
