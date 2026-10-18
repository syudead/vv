//go:build unix

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/syudead/vv/scripts/devtools"
)

// Read the constant from cmd/mdm because package main cannot be imported.
var appShutdownGrace = regexp.MustCompile(`shutdownGrace = (\d+) \* time\.Second`)

// Keep outer shutdown deadlines longer than the application deadline.
func TestGracePeriodsAreOrdered(t *testing.T) {
	root, err := devtools.RepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(root, "cmd", "mdm", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	match := appShutdownGrace.FindSubmatch(source)
	if match == nil {
		t.Fatal("cmd/mdm/main.go から shutdownGrace を読めない。定義が変わったならこの試験も直すこと")
	}

	var seconds time.Duration
	for _, digit := range match[1] {
		seconds = seconds*10 + time.Duration(digit-'0')
	}
	appGrace := seconds * time.Second

	if killGrace <= appGrace {
		t.Errorf("killGrace（%s）がアプリの猶予（%s）以下である。正常な停止の途中で殺してしまう", killGrace, appGrace)
	}
	if drainTimeout <= killGrace {
		t.Errorf("drainTimeout（%s）が killGrace（%s）以下である。強制終了の前に待つのをやめてしまう", drainTimeout, killGrace)
	}
}
