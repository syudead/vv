package domain

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TagNameMaxLength はタグ名（元の名前・シノニムとも）の上限（符号位置数）で
// ある。検索語の上限（domain.SearchKeyMaxRunes 相当）とそろえ、利用者が覚える
// 上限を1つにする（data-model.md §2）。
const TagNameMaxLength = 100

// ErrInvalidTagName はタグ名として受け付けられない入力を表す。API では
// invalid_request（400）になる（data-model.md §2）。
//
// NormalizeTagName はこれを直接は返さず、具体的な理由（空・制御文字・長さ超過）を
// 運ぶ *invalidTagNameError を返す。errors.Is(err, ErrInvalidTagName) は真のまま
// になるが、err.Error() は利用者にそのまま出せる理由を持つ（親 Issue 要件 4、
// 受け入れ条件 6）。
var ErrInvalidTagName = errors.New("タグ名が使えません")

// invalidTagNameError はタグ名の規則違反の具体的な理由を運ぶ。
type invalidTagNameError struct {
	reason string
}

func (e *invalidTagNameError) Error() string { return e.reason }

func (e *invalidTagNameError) Unwrap() error { return ErrInvalidTagName }

func invalidTagName(reason string) error {
	return &invalidTagNameError{reason: reason}
}

// ErrTagNotFound は指定したタグがもう無いことを表す。API では tag_not_found
// （404）になる。
var ErrTagNotFound = errors.New("タグが見つかりません")

// ErrTagNameTaken はその名前が既に別のタグの元の名前かシノニムであることを
// 表す。API では tag_name_taken（409）になる。errors.As で *TagNameConflict を
// 取り出すと、その名前を持つタグが分かる。
var ErrTagNameTaken = errors.New("その名前は既に使われています")

// ErrTagMergeRequired は、シノニムにしようとした名前が既存のタグの元の名前で、
// その統合の承諾が無い（無効な承諾を含む）ことを表す。API では
// tag_merge_required（409）になる。errors.As で *TagMergeRequired を取り出すと、
// 統合の承諾が要るタグが分かる。
var ErrTagMergeRequired = errors.New("統合の承諾が必要です")

// TagRef は動画に付いたタグ1件である。Name は常に元の名前
// （contracts/tags-api.md §1 の TagRef）。
type TagRef struct {
	ID   int64
	Name string
}

// Tag は管理画面と候補に出す1件である（contracts/tags-api.md §1 の Tag）。
type Tag struct {
	ID   int64
	Name string
	// Synonyms は名前の自然順（SortTagNames）。
	Synonyms []string
	// VideoCount はいまライブラリにある動画の本数（data-model.md §5）。
	VideoCount int
}

// TagSummaryItem は選んだ動画のタグの要約1件である
// （contracts/tags-api.md §4 の summary）。
type TagSummaryItem struct {
	Tag TagRef
	// Count は選んだ動画のうちこのタグが付いている本数。Count が
	// TagSummary.Total より小さいタグが「一部にだけ付いている」。
	Count int
}

// TagSummary は選んだ動画のタグの要約である（contracts/tags-api.md §4 の
// summary）。
type TagSummary struct {
	// Total は選んだ動画のうちいまライブラリにある動画の数。
	Total int
	// Items は1本以上に付いているタグだけで、名前の自然順
	// （SortTagSummaryItems）。
	Items []TagSummaryItem
}

// TagNameConflict はその名前を既に持つタグを運ぶ。errors.Is(err,
// ErrTagNameTaken) が真になる。
type TagNameConflict struct {
	// Tag はその名前を持つタグ（元の名前としてでもシノニムとしてでも）。
	Tag TagRef
}

func (e *TagNameConflict) Error() string {
	return fmt.Sprintf("タグ「%s」の名前です", e.Tag.Name)
}

func (e *TagNameConflict) Unwrap() error { return ErrTagNameTaken }

// TagMergeRequired は、統合の承諾が要るタグ（シノニムにしようとした名前を
// いま元の名前として持つタグ）を運ぶ。errors.Is(err, ErrTagMergeRequired) が
// 真になる。
type TagMergeRequired struct {
	Tag TagRef
}

func (e *TagMergeRequired) Error() string {
	return fmt.Sprintf("タグ「%s」への統合の承諾が要ります", e.Tag.Name)
}

func (e *TagMergeRequired) Unwrap() error { return ErrTagMergeRequired }

// NormalizeTagName は受け取った入力をタグ名に整える（data-model.md §2）。
// 手順の順番に意味がある。
//
//  1. 入力のどこかに制御文字（一般カテゴリ Cc。改行・タブ・CR を含む）が
//     あれば誤り。前後の空白を取り除く前に調べる。改行・タブ・CR は
//     Unicode の White_Space でもあるので、先に手順2を掛けると
//     "旅行\n" や "\t旅行" の前後の制御文字が黙って取り除かれ、拒むべき
//     入力が別の名前として作られてしまう。
//  2. 前後の Unicode White_Space を取り除く。内側の空白は残す。
//  3. 空になったら誤り。
//  4. TagNameMaxLength 符号位置を超えたら誤り。
//
// Unicode の正規化・大文字小文字の畳み込みはしない。Anime と anime は
// 別の名前である。
func NormalizeTagName(input string) (string, error) {
	for _, r := range input {
		if unicode.Is(unicode.Cc, r) {
			return "", invalidTagName("タグ名に制御文字は使えません")
		}
	}
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if trimmed == "" {
		return "", invalidTagName("タグ名を入力してください")
	}
	if utf8.RuneCountInString(trimmed) > TagNameMaxLength {
		return "", invalidTagName(fmt.Sprintf("タグ名は%d文字以内にしてください", TagNameMaxLength))
	}
	return trimmed, nil
}

// SortTagNames はタグ名（シノニムの一覧など）を名前の自然順に並べる。
// 同順位は元の文字列で決着させる（folder.go の SortFolders と同じ規則）。
func SortTagNames(names []string) {
	slices.SortStableFunc(names, func(a, b string) int {
		if order := CompareNatural(a, b); order != 0 {
			return order
		}
		return strings.Compare(a, b)
	})
}

// SortTags はタグを Name の自然順に並べる。
func SortTags(tags []Tag) {
	slices.SortStableFunc(tags, func(a, b Tag) int {
		if order := CompareNatural(a.Name, b.Name); order != 0 {
			return order
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// SortTagRefs は動画に付いたタグ（TagRef）を Name の自然順に並べる。同順位は
// ID で決着させる（元の名前は tag_names の主キーで一意なので、実際には
// CompareNatural だけで決まる。ID の比較は同順位が起き得る呼び出し側の入力
// でも決定的な並びにするための保険である）（contracts/tags-api.md §1 の
// Video.tags）。
func SortTagRefs(refs []TagRef) {
	slices.SortStableFunc(refs, func(a, b TagRef) int {
		if order := CompareNatural(a.Name, b.Name); order != 0 {
			return order
		}
		if order := strings.Compare(a.Name, b.Name); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

// SortTagSummaryItems は要約を Tag.Name の自然順に並べる（同順位の決着は
// SortTagRefs と同じ）。
func SortTagSummaryItems(items []TagSummaryItem) {
	slices.SortStableFunc(items, func(a, b TagSummaryItem) int {
		if order := CompareNatural(a.Tag.Name, b.Tag.Name); order != 0 {
			return order
		}
		if order := strings.Compare(a.Tag.Name, b.Tag.Name); order != 0 {
			return order
		}
		return cmp.Compare(a.Tag.ID, b.Tag.ID)
	})
}
