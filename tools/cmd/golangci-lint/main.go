// Command golangci-lint is the pinned golangci-lint with this repository's analyzers compiled in, the
// manual way of golangci-lint's module plugin system (docs/how-to/development.md#lint-rules).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/golangci/golangci-lint/v2/pkg/commands"
	"github.com/golangci/golangci-lint/v2/pkg/exitcodes"

	_ "github.com/syudead/vv/tools/lint"
)

const upstream = "github.com/golangci/golangci-lint/v2"

func main() {
	if err := commands.Execute(buildInfo()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "The command is terminated due to an error: %v\n", err)
		os.Exit(exitcodes.Failure)
	}
}

// golangci-lint salts its analysis cache with the version, so the version carries a digest of this
// executable: a changed analyzer must not reuse the results of the previous build.
func buildInfo() commands.BuildInfo {
	info := commands.BuildInfo{Version: "unknown", Commit: "?", Date: "(unknown)", GoVersion: runtime.Version()}
	if build, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range build.Deps {
			if dep.Path == upstream {
				info.Version = strings.TrimPrefix(dep.Version, "v")
			}
		}
	}
	if digest, err := executableDigest(); err == nil {
		info.Version += "+vv." + digest
	}
	return info
}

func executableDigest() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil))[:12], nil
}
