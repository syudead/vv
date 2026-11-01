package app

import (
	"context"
)

// MediaFiles lets a job open a stored location only through the media folders' access rules, which internal/mediafs
// owns; see ARCHITECTURE.md "One owner for opening files".
type MediaFiles struct {
	Folders  MediaFolderLister
	Resolver MediaFileResolver
}

// Resolve returns the path another process may open; a refused location wraps domain.ErrMediaFileUnavailable.
func (m MediaFiles) Resolve(ctx context.Context, path string) (string, error) {
	roots, err := mediaRoots(ctx, m.Folders)
	if err != nil {
		return "", err
	}
	return m.Resolver.ResolveMediaFile(roots, path)
}

func mediaRoots(ctx context.Context, folders MediaFolderLister) ([]string, error) {
	list, err := folders.ListMediaFolders(ctx)
	if err != nil {
		return nil, err
	}
	roots := make([]string, len(list))
	for i, folder := range list {
		roots[i] = folder.Path
	}
	return roots, nil
}
