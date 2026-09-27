package domain

import (
	"errors"
	"fmt"
	"testing"
)

// 包まれた理由のコードは、さらに包まれても取り出せる。包まれていなければ internal。
func TestFailureCodesOf(t *testing.T) {
	probe := fmt.Errorf("job: %w", NewProbeFailure(ProbeErrorInvalidMetadata, errors.New("bad duration")))
	if got := ProbeErrorCodeOf(probe); got != ProbeErrorInvalidMetadata {
		t.Errorf("ProbeErrorCodeOf = %q, want invalid_metadata", got)
	}
	if probe.Error() != "job: bad duration" {
		t.Errorf("Error() = %q", probe.Error())
	}
	if got := ProbeErrorCodeOf(errors.New("plain")); got != ProbeErrorInternal {
		t.Errorf("ProbeErrorCodeOf(plain) = %q, want internal", got)
	}

	scan := fmt.Errorf("scan: %w", NewScanFailure(ScanErrorLocationUnreadable, "/media/a", errors.New("denied")))
	if code, path := ScanFailureOf(scan); code != ScanErrorLocationUnreadable || path != "/media/a" {
		t.Errorf("ScanFailureOf = %q (%q)", code, path)
	}
	if code, path := ScanFailureOf(errors.New("plain")); code != ScanErrorInternal || path != "" {
		t.Errorf("ScanFailureOf(plain) = %q (%q), want internal", code, path)
	}
}
