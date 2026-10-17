package main

import (
	"bytes"
	"testing"
)

func TestCommentOnlyChanges(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		equal               bool
	}{
		{"ordinary comment", "-- old reference\nselect 1;\n", "-- current design\nselect 1;\n", true},
		{"deleted comment", "select 1;\n  -- old reference\n", "select 1;\n", true},
		{"SQL", "select 1;\n", "select 2;\n", false},
		{"goose directive", "-- +goose Up\nselect 1;\n", "-- +goose Down\nselect 1;\n", false},
		{"deleted directive", "-- +goose StatementBegin\n", "", false},
		{"multiline string", "select 'text\n-- old\n';\n", "select 'text\n-- new\n';\n", false},
		{"escaped quote", "select 'it''s\n-- old\n';\n", "select 'it''s\n-- new\n';\n", false},
		{"quoted identifier", "select \"name\n-- old\n\";\n", "select \"name\n-- new\n\";\n", false},
		{"bracket identifier", "select [name\n-- old\n];\n", "select [name\n-- new\n];\n", false},
		{"backtick identifier", "select `name\n-- old\n`;\n", "select `name\n-- new\n`;\n", false},
		{"block comment", "/* text\n-- old\n*/\n", "/* text\n-- new\n*/\n", false},
		{"inline comment", "select 1; -- old\n", "select 1; -- new\n", false},
		{"quote in comment", "select 1; -- ' /*\n-- old\n", "select 1; -- ' /*\n-- new\n", true},
		{"after block", "/* x */\n-- old\nselect 1;\n", "/* x */\n-- new\nselect 1;\n", true},
		{"after string", "select 'x';\n-- old\n", "select 'x';\n-- new\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := bytes.Equal(withoutCommentLines([]byte(tc.before)), withoutCommentLines([]byte(tc.after)))
			if got != tc.equal {
				t.Fatalf("equal = %v, want %v", got, tc.equal)
			}
		})
	}
}
