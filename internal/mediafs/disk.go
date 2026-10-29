package mediafs

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrMediaFolderUnmounted reports a media folder that lists no entry while the library holds videos under it: an
// unmounted mount point is a readable, empty directory on Linux and under a Windows volume mount point.
var ErrMediaFolderUnmounted = errors.New("the media folder is empty, so its disk is not mounted")

// openReadableRoot opens a media folder and lists one entry, so an unplugged disk or a share that stopped answering
// counts as unreadable even where the folder can still be stat'ed. populated reports that the library holds a
// location under the folder, so an empty folder is then unmounted rather than new.
func openReadableRoot(root string, populated bool) (*os.File, error) {
	dir, err := os.Open(root)
	if err != nil {
		return nil, err
	}
	_, err = dir.ReadDir(1)
	if errors.Is(err, io.EOF) {
		err = nil
		if populated {
			err = fmt.Errorf("%w: %s", ErrMediaFolderUnmounted, root)
		}
	}
	if err != nil {
		_ = dir.Close()
		return nil, err
	}
	return dir, nil
}
