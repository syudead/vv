package media

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// RequiredCommands are checked before startup accepts requests.
var RequiredCommands = []string{"ffprobe", "ffmpeg"}

var ErrMissingCommands = errors.New("required external commands were not found")

// Include installation guidance so missing-command errors are actionable.
const installHint = "Run with Docker (task up) or install ffmpeg" +
	" (alpine: apk add ffmpeg / Debian, Ubuntu: apt-get install ffmpeg / macOS: brew install ffmpeg)"

// Preflight reports all missing commands together with installation instructions.
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

	return fmt.Errorf("%w: %s. %s",
		ErrMissingCommands, strings.Join(missing, ", "), installHint)
}
