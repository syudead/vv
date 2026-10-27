package media

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestProbeRunFailureRepeatsOnlyForInvalidData(t *testing.T) {
	ctx := context.Background()
	timedOut, cancel := context.WithTimeout(ctx, 0)
	defer cancel()
	<-timedOut.Done()
	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		repeats bool
	}{
		{"invalid data", ctx, &exec.ExitError{Stderr: []byte("/media/a.mp4: Invalid data found when processing input\n")}, true},
		{"I/O error", ctx, &exec.ExitError{Stderr: []byte("/media/a.mp4: Input/output error\n")}, false},
		{"timeout", timedOut, context.DeadlineExceeded, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := probeRunFailure(tt.ctx, "/media/a.mp4", tt.err)
			if got := errors.Is(err, domain.ErrFailureRepeats); got != tt.repeats {
				t.Errorf("repeats = %v, want %v (%v)", got, tt.repeats, err)
			}
			if code := domain.ProbeErrorCodeOf(err); code != domain.ProbeErrorProbeFailed {
				t.Errorf("code = %q, want probe_failed", code)
			}
		})
	}
}

func TestParseImportProbeFailureRepeats(t *testing.T) {
	_, err := parseImportProbe("/media/a.mp4", []byte(`{"format": {"duration": "N/A"}}`))
	if !errors.Is(err, domain.ErrFailureRepeats) || domain.ProbeErrorCodeOf(err) != domain.ProbeErrorInvalidMetadata {
		t.Fatalf("err = %v (code %q), want a repeating invalid_metadata failure", err, domain.ProbeErrorCodeOf(err))
	}
}

func TestFFmpegExitFailureRepeatsOnlyForInvalidData(t *testing.T) {
	cause := errors.New("ffmpeg failed")
	if err := exitFailure(cause, []byte("[mov,mp4] moov atom not found\nInvalid data found when processing input\n")); !errors.Is(err, domain.ErrFailureRepeats) || err.Error() != cause.Error() {
		t.Errorf("invalid data = %v, want the same message marked repeating", err)
	}
	if err := exitFailure(cause, []byte("Error reading header: Input/output error\n")); errors.Is(err, domain.ErrFailureRepeats) {
		t.Errorf("I/O error = %v, want a plain failure", err)
	}
}
