// Package devtools は scripts/ 配下の開発者コマンドが共有する土台である。
// 各コマンドは task から go run で呼ばれる。検査のためだけに別のランタイムを
// 足さないという決まりは
// specs/001-initial-setup/contracts/developer-commands.md にある。
package devtools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RepositoryRoot はこのソースの位置から版管理の根を求める。go run の作業
// ディレクトリに依存させないため、呼び出し側の cwd ではなくソース位置を使う。
func RepositoryRoot() (string, error) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("devtools のソース位置を解決できません")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("版管理の根 %s を確認できません: %w", root, err)
	}
	return root, nil
}

// GoToolPath は tools/go.mod の tool directive が固定する実行ファイルを解決する。
// 呼び出し側の作業ディレクトリは変えず、ツールだけ分離 module から選べる。
func GoToolPath(root, name string) (string, error) {
	cmd := exec.Command("go", "-C", "tools", "tool", "-n", name)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go tool %s を解決できません: %w", name, err)
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", fmt.Errorf("go tool %s の実行ファイルが空です", name)
	}
	return path, nil
}

// NodeToolPath は tools/package.json に分離した Node 製ツールを解決する。
func NodeToolPath(root, name string) (string, error) {
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	path := filepath.Join(root, "tools", "node_modules", ".bin", name)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("node tool %s を解決できません。task setup を実行してください: %w", name, err)
	}
	return path, nil
}

// Run は外部コマンドを実行し、出力をそのまま親へ流す。開発者コマンドの出力は
// 利用者が読むものなので、握り潰さずに素通しする。
func Run(dir string, name string, args ...string) error {
	return RunEnv(dir, nil, name, args...)
}

// RunEnv は環境変数を足して Run する。足した変数は親の環境に重ねる。
func RunEnv(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s の実行に失敗しました: %w", name, err)
	}
	return nil
}

// Fail はエラーを表示して終了する。各コマンドの main を短く保つための入口。
func Fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
