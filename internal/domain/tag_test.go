package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeTagNameTrimsSurroundingWhitespace(t *testing.T) {
	got, err := NormalizeTagName("  旅行 先  ")
	if err != nil {
		t.Fatalf("NormalizeTagName() error = %v", err)
	}
	if want := "旅行 先"; got != want {
		t.Errorf("NormalizeTagName() = %q, want %q (内側の空白は残す)", got, want)
	}
}

func TestNormalizeTagNameRejectsBlank(t *testing.T) {
	for _, input := range []string{"", " ", "　", "\t \n"} {
		if _, err := NormalizeTagName(input); !errors.Is(err, ErrInvalidTagName) {
			t.Errorf("NormalizeTagName(%q) error = %v, want ErrInvalidTagName", input, err)
		}
	}
}

func TestNormalizeTagNameRejectsControlCharactersBeforeTrimming(t *testing.T) {
	// 前後に付いた改行・タブは、White_Space として黙って取り除かれてはならない。
	// 取り除いてしまうと "旅行\n" と "旅行" が同じ名前になる。
	for _, input := range []string{"旅行\n", "\t旅行", "旅行\r", "旅\n行", "a\tb"} {
		if _, err := NormalizeTagName(input); !errors.Is(err, ErrInvalidTagName) {
			t.Errorf("NormalizeTagName(%q) error = %v, want ErrInvalidTagName", input, err)
		}
	}
}

func TestNormalizeTagNameRejectsOverLongNames(t *testing.T) {
	ok := strings.Repeat("あ", TagNameMaxLength)
	if _, err := NormalizeTagName(ok); err != nil {
		t.Errorf("NormalizeTagName(100 符号位置) error = %v, want nil", err)
	}
	tooLong := strings.Repeat("あ", TagNameMaxLength+1)
	if _, err := NormalizeTagName(tooLong); !errors.Is(err, ErrInvalidTagName) {
		t.Errorf("NormalizeTagName(101 符号位置) error = %v, want ErrInvalidTagName", err)
	}
}

func TestNormalizeTagNameDoesNotFoldCase(t *testing.T) {
	got, err := NormalizeTagName("Anime")
	if err != nil {
		t.Fatalf("NormalizeTagName() error = %v", err)
	}
	if got != "Anime" {
		t.Errorf("NormalizeTagName(%q) = %q, want unchanged", "Anime", got)
	}
	lower, err := NormalizeTagName("anime")
	if err != nil {
		t.Fatalf("NormalizeTagName() error = %v", err)
	}
	if got == lower {
		t.Errorf("Anime と anime が同じ名前に畳まれている")
	}
}

func TestTagNameConflictUnwrapsToErrTagNameTaken(t *testing.T) {
	err := error(&TagNameConflict{Tag: TagRef{ID: 1, Name: "Anime"}})
	if !errors.Is(err, ErrTagNameTaken) {
		t.Errorf("errors.Is(err, ErrTagNameTaken) = false, want true")
	}
	var conflict *TagNameConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("errors.As(err, &conflict) = false, want true")
	}
	if conflict.Tag.Name != "Anime" {
		t.Errorf("conflict.Tag.Name = %q, want %q", conflict.Tag.Name, "Anime")
	}
}

func TestTagMergeRequiredUnwrapsToErrTagMergeRequired(t *testing.T) {
	err := error(&TagMergeRequired{Tag: TagRef{ID: 2, Name: "anime"}})
	if !errors.Is(err, ErrTagMergeRequired) {
		t.Errorf("errors.Is(err, ErrTagMergeRequired) = false, want true")
	}
	var required *TagMergeRequired
	if !errors.As(err, &required) {
		t.Fatalf("errors.As(err, &required) = false, want true")
	}
	if required.Tag.ID != 2 {
		t.Errorf("required.Tag.ID = %d, want 2", required.Tag.ID)
	}
}

func TestSortTagNamesUsesNaturalOrder(t *testing.T) {
	names := []string{"tag10", "tag2", "tag1"}
	SortTagNames(names)
	want := []string{"tag1", "tag2", "tag10"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("SortTagNames() = %v, want %v", names, want)
			break
		}
	}
}
