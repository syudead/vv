package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 解析の失敗は、状況に応じた理由のコードで包まれる
// （specs/023-english-i18n/data-model.md §1）。
func TestProbeFailureCodes(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.mp4")
	if _, err := Probe(context.Background(), missing); domain.ProbeErrorCodeOf(err) != domain.ProbeErrorFileUnavailable {
		t.Errorf("missing file: code = %q, want file_unavailable (%v)", domain.ProbeErrorCodeOf(err), err)
	}
	if err := NewAssets().CheckSource(dir); domain.ProbeErrorCodeOf(err) != domain.ProbeErrorFileUnavailable {
		t.Errorf("directory: code = %q, want file_unavailable (%v)", domain.ProbeErrorCodeOf(err), err)
	}
	if err := NewAssets().CheckSource(missing); domain.ProbeErrorCodeOf(err) != domain.ProbeErrorFileUnavailable {
		t.Errorf("missing source: code = %q, want file_unavailable (%v)", domain.ProbeErrorCodeOf(err), err)
	}

	if _, err := exec.LookPath(probeCommand); err != nil {
		t.Skip("ffprobe is unavailable")
	}
	text := filepath.Join(dir, "not-a-video.mp4")
	if err := os.WriteFile(text, []byte("plain text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Probe(context.Background(), text); domain.ProbeErrorCodeOf(err) != domain.ProbeErrorProbeFailed {
		t.Errorf("non-video file: code = %q, want probe_failed (%v)", domain.ProbeErrorCodeOf(err), err)
	}
}
