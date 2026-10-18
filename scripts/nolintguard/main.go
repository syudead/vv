// nolintguard rejects Go lint suppression comments, including in tests, because golangci-lint cannot enforce this policy itself.
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Reject suppression directives regardless of leading whitespace or case.
var directive = regexp.MustCompile(`(?i)^\s*nolint\b`)

// Skip dependencies and directories without Go sources.
var skippedDirs = map[string]bool{
	".git":         true,
	".local":       true,
	"node_modules": true,
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	found, err := findDirectives(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nolintguard: %v\n", err)
		os.Exit(2)
	}
	for _, f := range found {
		fmt.Fprintln(os.Stderr, f)
	}
	if len(found) > 0 {
		fmt.Fprintf(os.Stderr, "nolintguard: %d //nolint directive(s) found. Fix the code instead of disabling the linter.\n", len(found))
		os.Exit(1)
	}
}

// Inspect parsed comments only; directive-like text in strings has no effect.
func findDirectives(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, group := range file.Comments {
			for _, c := range group.List {
				text := strings.TrimPrefix(strings.TrimPrefix(c.Text, "//"), "/*")
				if directive.MatchString(text) {
					found = append(found, fmt.Sprintf("%s: %s", fset.Position(c.Slash), c.Text))
				}
			}
		}
		return nil
	})
	return found, err
}
