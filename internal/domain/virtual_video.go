package domain

import (
	"errors"
	"strings"
)

// A virtual video is a library entry with no file; its user key is virtual:<id> (docs/design-docs/catalog-organization.md).

var (
	ErrInvalidVirtualTitle = errors.New("a virtual video needs a title")
	ErrNotVirtual          = errors.New("the video has a file")
	// ErrVirtualVideo rejects a virtual video in an operation that needs a file, such as a version bundle.
	ErrVirtualVideo = errors.New("the video has no file")
)

// VirtualKeyPrefix starts every virtual video's user key, which is also the content key of its projected row.
const VirtualKeyPrefix = "virtual:"

// IsVirtualKey reports whether a content or user key belongs to a virtual video.
func IsVirtualKey(key string) bool {
	return strings.HasPrefix(key, VirtualKeyPrefix)
}

// Virtual reports whether the video is a virtual video, which has no file.
func (v Video) Virtual() bool {
	return IsVirtualKey(v.ContentKey)
}

// NormalizeVirtualTitle trims the title, rejects one that is empty after trimming, and applies the display-name rules.
func NormalizeVirtualTitle(title string) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", ErrInvalidVirtualTitle
	}
	name, clear, err := NormalizeDisplayName(title)
	if err != nil {
		return "", err
	}
	if clear {
		return "", ErrInvalidVirtualTitle
	}
	return name, nil
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
