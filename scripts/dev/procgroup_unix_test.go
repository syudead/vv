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

// appShutdownGrace は cmd/mdm が処理中の要求を待つ猶予を読み出す。定数を
// import できないので、値そのものを読む。
var appShutdownGrace = regexp.MustCompile(`shutdownGrace = (\d+) \* time\.Second`)

// TestGracePeriodsAreOrdered は、停止の猶予が内側から順に長くなっていることを
// 確かめる。アプリが要求を捌き終える前に SIGKILL すると、task dev 越しの停止
// だけが中途半端に終わる。cmd/mdm 側の猶予が伸びたらここが落ちる。
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
