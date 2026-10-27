//go:build !linux && !windows

package mediafs

import "github.com/syudead/vv/internal/domain"

// ResolveDisk gives each media folder its own disk of unknown type, with 1 read slot, where no disk lookup exists.
// An unreadable folder returns an error.
func (FS) ResolveDisk(root string) (domain.Disk, error) {
	dir, err := openReadableRoot(root)
	if err != nil {
		return domain.Disk{}, err
	}
	_ = dir.Close()
	return domain.Disk{Key: "path:" + root}, nil
}
