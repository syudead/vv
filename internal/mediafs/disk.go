package mediafs

import (
	"errors"
	"io"
	"os"
)

// openReadableRoot opens a media folder and lists one entry, so an unplugged disk or a share that stopped answering
// counts as unreadable even where the folder can still be stat'ed.
func openReadableRoot(root string) (*os.File, error) {
	dir, err := os.Open(root)
	if err != nil {
		return nil, err
	}
	if _, err := dir.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		_ = dir.Close()
		return nil, err
	}
	return dir, nil
}
