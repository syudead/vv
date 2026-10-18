// Package devtools shares helpers for task commands run with go run; see Taskfile.yml.
package devtools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RepositoryRoot uses the source location so callers need not run from the repository root.
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

// GoToolPath resolves tools/go.mod tool directives without changing the caller working directory.
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

// NodeToolPath resolves Node tools isolated in tools/package.json.
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

// Run streams command output to the user.
func Run(dir string, name string, args ...string) error {
	return RunEnv(dir, nil, name, args...)
}

// RunEnv overlays additional environment variables on the parent environment.
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

// Fail prints an error and exits.
func Fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
