//go:build unix

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// killGrace は SIGTERM のあと SIGKILL へ移るまでの猶予である。Go サーバーは
// 処理中の要求を cmd/mdm の shutdownGrace（10 秒）まで待ってから終わるので、
// それより短くすると正常な停止の途中で殺すことになる。猶予は
// アプリ < killGrace < drainTimeout の順に外側ほど長くする。
// この順序は scripts/dev の試験が守る。
const killGrace = 15 * time.Second

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
	time.AfterFunc(killGrace, func() { _ = syscall.Kill(group, syscall.SIGKILL) })
}
