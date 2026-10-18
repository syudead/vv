// migrationcomments filters changed migrations to those with executable changes.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: migrationcomments <base>")
		os.Exit(2)
	}
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		path := scan.Text()
		before, err := exec.Command("git", "show", os.Args[1]+":"+path).Output()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(withoutCommentLines(before), withoutCommentLines(after)) {
			fmt.Println(path)
		}
	}
	if err := scan.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Only ordinary line comments may change. Preserve SQL bytes, inline line endings,
// blank lines, goose directives, block comments and quoted values.
func withoutCommentLines(sql []byte) []byte {
	var out []byte
	var quote byte
	block := false
	for _, line := range bytes.SplitAfter(sql, []byte("\n")) {
		var comment int
		quote, block, comment = nextState(line, quote, block)
		if comment >= 0 && !strings.Contains(strings.ToLower(string(line[comment:])), "+goose") {
			prefix := line[:comment]
			if len(bytes.TrimSpace(prefix)) == 0 {
				continue
			}
			out = append(out, prefix...)
			if bytes.HasSuffix(line, []byte("\r\n")) {
				out = append(out, '\r', '\n')
			} else if bytes.HasSuffix(line, []byte("\n")) {
				out = append(out, '\n')
			}
			continue
		}
		out = append(out, line...)
	}
	return out
}

func nextState(line []byte, quote byte, block bool) (byte, bool, int) {
	for i := 0; i < len(line); i++ {
		c := line[i]
		var next byte
		if i+1 < len(line) {
			next = line[i+1]
		}
		switch {
		case block:
			if c == '*' && next == '/' {
				block = false
				i++
			}
		case quote != 0:
			if c == quote {
				if next == quote && quote != ']' {
					i++
				} else {
					quote = 0
				}
			}
		case c == '-' && next == '-':
			return quote, block, i
		case c == '/' && next == '*':
			block = true
			i++
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '[':
			quote = ']'
		}
	}
	return quote, block, -1
}
