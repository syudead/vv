package media

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// RequiredCommands は起動前に存在を確認する外部コマンドである。
// Phase 0 の機能はこれらを使わないが、Phase 1 で必須になるため
// 「起動できた＝前提が揃っている」状態をここで保証する（FR-008 / research.md R-009）。
var RequiredCommands = []string{"ffprobe", "ffmpeg"}

// ErrMissingCommands は必要な外部コマンドが見つからなかったことを表す。
var ErrMissingCommands = errors.New("必要な外部コマンドが見つかりません")

// installHint は不足していた場合に提示する導入方法である。欠けているコマンド名だけを
// 出しても次の一手が分からないため、導入方法を必ず添える（SC-008）。
const installHint = "Docker で実行する（task up）か、ffmpeg を導入してください" +
	"（alpine: apk add ffmpeg / Debian・Ubuntu: apt-get install ffmpeg / macOS: brew install ffmpeg）"

// Preflight は RequiredCommands が実行パス上にあるかを確認する。
// 欠けている場合は、不足しているコマンド名と導入方法を含む誤りを返す。
func Preflight() error {
	var missing []string
	for _, name := range RequiredCommands {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s。%s",
		ErrMissingCommands, strings.Join(missing, ", "), installHint)
}
