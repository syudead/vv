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

// ErrInvalidVirtualLink rejects a link that is not an http or https URL of at most WorkInfoLimits.URLMaxLength.
var ErrInvalidVirtualLink = errors.New("the link must be an http or https URL")

// NormalizeVirtualLink trims a link a virtual video is created from and checks it as a stored work URL is checked.
func NormalizeVirtualLink(raw string) (string, error) {
	link, err := normalizeWorkInfoText("url", raw, WorkInfoLimits.URLMaxLength, false)
	if err != nil || !isWorkURL(link) {
		return "", ErrInvalidVirtualLink
	}
	return link, nil
}

// LinkTitle is the title of a virtual video created from link without one: the link itself, cut to the
// display-name limit, until a fetch or the owner replaces it.
func LinkTitle(link string) string {
	return TruncateRunes(link, DisplayNameMaxLength)
}

// LinkFetchState is the progress of the fetch started when a virtual video was created from a link.
type LinkFetchState string

const (
	LinkFetchRunning LinkFetchState = "running"
	LinkFetchFailed  LinkFetchState = "failed"
	LinkFetchDone    LinkFetchState = "done"
)

// LinkFetch is a virtual video's link fetch; Failure tells the owner why a failed one failed.
type LinkFetch struct {
	State   LinkFetchState
	Failure string
	// Plugin names the plugin that ran, empty when none did.
	Plugin string
}
