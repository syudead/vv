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
	// Control whitespace is tested separately because it must be rejected before trimming.
	for _, input := range []string{"", " ", "　"} {
		if got := normalizeTagNameProblem(t, input); got != TagNameEmpty {
			t.Errorf("NormalizeTagName(%q) problem = %v, want TagNameEmpty", input, got)
		}
	}
}

func TestNormalizeTagNameRejectsControlCharactersBeforeTrimming(t *testing.T) {
	// Leading/trailing tabs and newlines must not silently normalize into an accepted name.
	for _, input := range []string{"旅行\n", "\t旅行", "旅行\r", "旅\n行", "a\tb"} {
		if got := normalizeTagNameProblem(t, input); got != TagNameControlCharacters {
			t.Errorf("NormalizeTagName(%q) problem = %v, want TagNameControlCharacters", input, got)
		}
	}
}

func TestNormalizeTagNameRejectsOverLongNames(t *testing.T) {
	ok := strings.Repeat("あ", TagNameMaxLength)
	if _, err := NormalizeTagName(ok); err != nil {
		t.Errorf("NormalizeTagName(100 符号位置) error = %v, want nil", err)
	}
	tooLong := strings.Repeat("あ", TagNameMaxLength+1)
	if got := normalizeTagNameProblem(t, tooLong); got != TagNameTooLong {
		t.Errorf("NormalizeTagName(101 符号位置) problem = %v, want TagNameTooLong", got)
	}
}

func normalizeTagNameProblem(t *testing.T, input string) TagNameProblem {
	t.Helper()
	_, err := NormalizeTagName(input)
	if !errors.Is(err, ErrInvalidTagName) {
		t.Fatalf("NormalizeTagName(%q) error = %v, want ErrInvalidTagName", input, err)
	}
	var invalid *InvalidTagNameError
	if !errors.As(err, &invalid) {
		t.Fatalf("NormalizeTagName(%q) error = %v, want *InvalidTagNameError", input, err)
	}
	if err.Error() == "" {
		t.Errorf("NormalizeTagName(%q) error message is empty", input)
	}
	return invalid.Problem
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
