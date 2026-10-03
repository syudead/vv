//go:build !(windows && desktop)

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

// main はサーバーとホスト側のコマンドの入口である。設定は環境変数（MDM_*）から読む。
// Windows デスクトップ版（desktop タグ）の入口は desktop_windows.go にある。
func main() {
	// 引数つきはホスト側のコマンド（mdm account …）で、サーバーは起動しない
	// （specs/016-single-account-auth/contracts/account-cli.md）。
	if len(os.Args) > 1 {
		os.Exit(runCommand(context.Background(), os.Args[1:], osAccountEnv()))
	}
	cfg, err := LoadConfig(os.Getenv)
	if err == nil {
		err = run(runOptions{
			Config:     cfg,
			LogOutput:  os.Stdout,
			Listener:   newReopenableListener(),
			NotifyStop: notifySignals,
		})
	}
	if err != nil {
		// 記録の設定前に失敗する場合もあるため、利用者向けの説明は標準エラーへ出す。
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// osAccountEnv は実際の標準入出力と環境変数を使う accountEnv を返す。
func osAccountEnv() accountEnv {
	env := accountEnv{Getenv: os.Getenv, Stdin: os.Stdin, Stderr: os.Stderr, Now: time.Now}
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		env.ReadHidden = func() ([]byte, error) { return term.ReadPassword(fd) }
	}
	return env
}
