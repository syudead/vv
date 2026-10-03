package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/store"
)

// 中断した走査が残ったまま待ち受けを開けなかったとき、run は始め直した走査と
// ワーカーを止めてから戻る。止めずに戻ると、それらが閉じたデータベースへ書く。
func TestRunStopsResumedScanWhenPortIsInUse(t *testing.T) {
	stubMediaCommands(t)
	dataDir := t.TempDir()
	mediaDir := t.TempDir()
	for i := range 20 {
		name := filepath.Join(mediaDir, "video"+string(rune('a'+i))+".mp4")
		if err := os.WriteFile(name, []byte("not a video"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	ctx := context.Background()
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	// 前回の停止で running のまま残った走査。起動時に閉じて始め直す。
	if _, _, err := db.Scans().StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = occupied.Close() }()

	var logs bytes.Buffer
	err = run(runOptions{
		Config:     Config{Addr: occupied.Addr().String(), DataDir: dataDir, LogLevel: defaultLogLevel},
		LogOutput:  &logs,
		Listener:   newReopenableListener(),
		NotifyStop: notifySignals,
	})
	if stage := startupStageOf(err); err == nil || stage != stageListen {
		t.Fatalf("run = %v (stage %v); want a listen failure", err, stage)
	}
	if !strings.Contains(logs.String(), "resumed the interrupted scan") {
		t.Fatalf("中断した走査を始め直していない: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "stopped ingest and jobs") {
		t.Fatalf("走査とワーカーを止めずに戻った: %s", logs.String())
	}

	db, err = store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	scan, err := db.Scans().CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if scan.State == domain.ScanRunning {
		t.Fatalf("始め直した走査が running のまま残った: %+v", scan)
	}
}
