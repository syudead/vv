package domain

import (
	"errors"
	"fmt"
	"time"
)

// タグの種類。種類は名前だけを持ち
// （R-7）、タグは種類を 0 か 1 つ持つ。種類の名前は NormalizeTagName で整え、タグの名前とは
// 別の名前空間で一意にする。

// ErrTagKindNotFound は指定した種類がもう無いことを表す。API では not_found
// （404）/ tag_kind_not_found になる。
var ErrTagKindNotFound = errors.New("tag kind not found")

// ErrTagKindNameTaken はその名前が既に別の種類の名前であることを表す。API では
// conflict（409）/ tag_kind_name_taken になる。errors.As で *TagKindNameConflict を
// 取り出すと、その名前を持つ種類が分かる。
var ErrTagKindNameTaken = errors.New("tag kind name is already in use")

// TagKindRef はタグの種類の id と名前である。Tag・TagRef・VideoTag の Kind に載る
type TagKindRef struct {
	ID   int64
	Name string
}

// TagKind は種類の一覧の 1 件である。
type TagKind struct {
	ID   int64
	Name string
	// TagCount はこの種類のタグの数。仮のタグも数える。
	TagCount int
	// CreatedAt は tag_kinds.created_at（Unix 秒）。
	CreatedAt time.Time
}

// TagKindNameConflict はその名前を既に持つ種類を運ぶ。errors.Is(err,
// ErrTagKindNameTaken) が真になる。
type TagKindNameConflict struct {
	Kind TagKindRef
}

func (e *TagKindNameConflict) Error() string {
	return fmt.Sprintf("a tag kind named %q already exists", e.Kind.Name)
}

func (e *TagKindNameConflict) Unwrap() error { return ErrTagKindNameTaken }
