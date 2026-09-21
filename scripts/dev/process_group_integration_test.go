//go:build unix || windows

package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const processGroupHelper = "VV_PROCESS_GROUP_HELPER"

func TestProcessGroupStopsDescendantAfterRootExits(t *testing.T) {
	if mode := os.Getenv(processGroupHelper); mode != "" {
		runProcessGroupHelper(mode)
		return
	}

	dir := t.TempDir()
	addressFile := filepath.Join(dir, "address")
	cmd := exec.Command(os.Args[0], "-test.run=TestProcessGroupStopsDescendantAfterRootExits")
	cmd.Env = append(os.Environ(), processGroupHelper+"=root", "VV_ADDRESS_FILE="+addressFile)
	group, err := newProcessGroup(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { group.terminate(cmd) })
	if err := cmd.Start(); err != nil {
		group.terminate(cmd)
		t.Fatal(err)
	}
	if err := group.attach(cmd); err != nil {
		group.terminate(cmd)
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		group.terminate(cmd)
		t.Fatal(err)
	}

	address := waitForAddress(t, addressFile)
	if conn, err := net.DialTimeout("tcp", address, time.Second); err != nil {
		group.terminate(cmd)
		t.Fatalf("子プロセスが待ち受けていません: %v", err)
	} else {
		_ = conn.Close()
	}

	group.terminate(cmd)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("起動元の終了後も子プロセスが残っています")
}

func runProcessGroupHelper(mode string) {
	switch mode {
	case "root":
		child := exec.Command(os.Args[0], "-test.run=TestProcessGroupStopsDescendantAfterRootExits")
		child.Env = append(os.Environ(), processGroupHelper+"=child")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "child":
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(os.Getenv("VV_ADDRESS_FILE"), []byte(listener.Addr().String()), 0o600); err != nil {
			os.Exit(4)
		}
		for {
			conn, err := listener.Accept()
			if err != nil {
				os.Exit(0)
			}
			_ = conn.Close()
		}
	}
}

func waitForAddress(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil && len(content) > 0 {
			return string(content)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("子プロセスの待ち受けアドレスを取得できません")
	return ""
}
