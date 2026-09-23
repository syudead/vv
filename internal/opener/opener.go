package opener

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// ErrUnavailable は既定アプリを起動できない環境であることを表す。
var ErrUnavailable = errors.New("既定のアプリを起動できない環境です")

// Opener は OS の既定アプリでファイルを開く。
type Opener struct {
	// command は起動時に解決した実行ファイルのパス。空なら開けない。
	command string
	start   func(name string, args ...string) error
}

// New は稼働中の OS と環境変数から Opener を組み立てる。
func New() *Opener {
	return newOpener(runtime.GOOS, os.Getenv, exec.LookPath, startDetached)
}

// newOpener は判定に使う値を差し替えられる形で Opener を組み立てる。
//
// 使うコマンドは OS ごとに決まっている（Windows は explorer.exe、macOS は open、
// それ以外は xdg-open）。実行ファイルへ解決できたときだけ開けるとし、Linux 等では
// さらに DISPLAY か WAYLAND_DISPLAY があることを求める。これでコンテナや画面の
// 無いサーバーでは自動的に開けない扱いになる。
func newOpener(
	goos string,
	getenv func(string) string,
	lookPath func(string) (string, error),
	start func(name string, args ...string) error,
) *Opener {
	opener := &Opener{start: start}
	name := commandFor(goos)
	if name == "xdg-open" && getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == "" {
		return opener
	}
	resolved, err := lookPath(name)
	if err != nil {
		return opener
	}
	opener.command = resolved
	return opener
}

func commandFor(goos string) string {
	switch goos {
	case "windows":
		return "explorer.exe"
	case "darwin":
		return "open"
	default:
		return "xdg-open"
	}
}

// Available は既定アプリを起動できる環境かどうかを返す。
func (o *Opener) Available() bool {
	return o != nil && o.command != ""
}

// Command は起動時に解決した実行ファイルのパスを返す。開けない環境では空。
func (o *Opener) Command() string {
	if o == nil {
		return ""
	}
	return o.command
}

// Open は path を既定のアプリで開く子プロセスを起動する。アプリが開いたか
// どうかは確かめず、起動に成功した時点で戻る。
//
// path はシェルを通さず、引数としてそのまま渡す。
func (o *Opener) Open(path string) error {
	if !o.Available() {
		return ErrUnavailable
	}
	if err := o.start(o.command, path); err != nil {
		return fmt.Errorf("既定のアプリを起動できません: %w", err)
	}
	return nil
}

// startDetached は子プロセスを起動し、終了を待ち合わせて回収する。待ち合わせは
// 背後で行うので、呼び出し側は起動の成否だけを受け取る。
func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
