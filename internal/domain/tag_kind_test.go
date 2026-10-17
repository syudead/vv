package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// 名前の重複は包まれても ErrTagKindNameTaken として判定でき、文はその名前を挙げる
func TestTagKindNameConflictIsNameTaken(t *testing.T) {
	err := fmt.Errorf("create: %w", &TagKindNameConflict{Kind: TagKindRef{ID: 1, Name: "Genre"}})
	if !errors.Is(err, ErrTagKindNameTaken) {
		t.Fatalf("errors.Is(%v, ErrTagKindNameTaken) = false", err)
	}
	var conflict *TagKindNameConflict
	if !errors.As(err, &conflict) || conflict.Kind.ID != 1 {
		t.Fatalf("errors.As = %v, %+v", err, conflict)
	}
	if !strings.Contains(err.Error(), `"Genre"`) {
		t.Errorf("Error() = %q, want the kind's name", err.Error())
	}
}
