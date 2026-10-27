package mediafs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/syudead/vv/internal/domain"
)

// sysfsRoot is where sysfs is mounted; tests pass diskOfDevice a fake directory instead.
const sysfsRoot = "/sys"

// ResolveDisk returns the whole disk a media folder is stored on and whether it is rotational, from sysfs. A device
// sysfs does not list as a block device, such as Btrfs, ZFS, overlayfs or a network share, has its st_dev as key and
// an unknown type. An unreadable folder returns an error.
func (FS) ResolveDisk(root string) (domain.Disk, error) {
	dir, err := openReadableRoot(root)
	if err != nil {
		return domain.Disk{}, err
	}
	defer func() { _ = dir.Close() }()
	info, err := dir.Stat()
	if err != nil {
		return domain.Disk{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return domain.Disk{Key: "path:" + root}, nil
	}
	return diskOfDevice(sysfsRoot, uint64(stat.Dev)), nil
}

func diskOfDevice(sys string, dev uint64) domain.Disk {
	device := fmt.Sprintf("%d:%d", unix.Major(dev), unix.Minor(dev))
	unknown := domain.Disk{Key: "dev:" + device}
	block, err := filepath.EvalSymlinks(filepath.Join(sys, "dev", "block", device))
	if err != nil {
		return unknown
	}
	// A partition's directory sits inside its whole disk's, which holds the queue settings.
	disk := block
	if _, err := os.Stat(filepath.Join(block, "partition")); err == nil {
		disk = filepath.Dir(block)
	}
	number, err := os.ReadFile(filepath.Join(disk, "dev"))
	if err != nil {
		return unknown
	}
	key := "block:" + strings.TrimSpace(string(number))
	rotational, err := os.ReadFile(filepath.Join(disk, "queue", "rotational"))
	if err != nil {
		return domain.Disk{Key: key}
	}
	switch strings.TrimSpace(string(rotational)) {
	case "0":
		return domain.Disk{Key: key, Type: domain.DiskSolidState}
	case "1":
		return domain.Disk{Key: key, Type: domain.DiskRotational}
	default:
		return domain.Disk{Key: key}
	}
}
