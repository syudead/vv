package domain

import (
	"errors"
	"strings"
)

// A virtual video is a library entry with no file; its user key is virtual:<id> (docs/design-docs/catalog-organization.md).

var (
	ErrInvalidVirtualTitle = errors.New("a virtual video needs a title")
	ErrNotVirtual          = errors.New("the video has a file")
)

// NormalizeVirtualTitle trims the title and rejects one that is empty after trimming.
func NormalizeVirtualTitle(title string) (string, error) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return "", ErrInvalidVirtualTitle
	}
	return trimmed, nil
}

// VideoKind narrows a library listing to file videos or virtual videos; empty means all.
type VideoKind string

const (
	VideoKindAll     VideoKind = "all"
	VideoKindFile    VideoKind = "file"
	VideoKindVirtual VideoKind = "virtual"
)

// IncludesFiles reports whether the listing keeps videos that have a file.
func (k VideoKind) IncludesFiles() bool {
	return k != VideoKindVirtual
}

// IncludesVirtual reports whether the listing keeps virtual videos.
func (k VideoKind) IncludesVirtual() bool {
	return k != VideoKindFile
}
