//go:build unix

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// shutdownGrace は SIGTERM のあと SIGKILL へ移るまでの猶予である。Go サーバーは
// 取り込み中のジョブを畳んでから終わるので、その時間を与える。
const shutdownGrace = 5 * time.Second

// isolateProcessGroup は開発サーバーを独立したプロセスグループの長にする。
// どちらのサーバーもさらに子を持つ（go run が起動するバイナリ、npm が起動する
// vite）ため、グループにしておかないと孫が残る。--strictPort の vite が
// 残ると、次の task dev が起動できない。
func isolateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateGroup はグループごと止める。まず SIGTERM を送り、猶予のあとに
// 残っていれば SIGKILL にする。
func terminateGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	group := -cmd.Process.Pid
	if err := syscall.Kill(group, syscall.SIGTERM); err != nil {
		_ = cmd.Process.Kill()
		return
	}
	time.AfterFunc(shutdownGrace, func() { _ = syscall.Kill(group, syscall.SIGKILL) })
}
