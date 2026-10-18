package domain

import (
	"errors"
	"fmt"
	"time"
)

// Tag-kind names use NormalizeTagName and a unique namespace separate from tag names; a tag has at most one kind.

var ErrTagKindNotFound = errors.New("tag kind not found")

var ErrTagKindNameTaken = errors.New("tag kind name is already in use")

type TagKindRef struct {
	ID   int64
	Name string
}

type TagKind struct {
	ID   int64
	Name string
	// TagCount includes tentative tags.
	TagCount int
	// CreatedAt uses Unix seconds.
	CreatedAt time.Time
}

type TagKindNameConflict struct {
	Kind TagKindRef
}

func (e *TagKindNameConflict) Error() string {
	return fmt.Sprintf("a tag kind named %q already exists", e.Kind.Name)
}

func (e *TagKindNameConflict) Unwrap() error { return ErrTagKindNameTaken }
