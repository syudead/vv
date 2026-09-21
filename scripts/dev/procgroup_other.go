//go:build !unix

package main

import "os/exec"

// Windows にはプロセスグループへ一括で合図する同等の仕組みがないので、
// 起動したプロセスだけを止める。孫が残ることがあり、その場合は次の
// task dev が --strictPort で失敗して気付ける。
func isolateProcessGroup(cmd *exec.Cmd) {}

func terminateGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
