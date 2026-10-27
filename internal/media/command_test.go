package media

import (
	"context"
	"slices"
	"testing"
)

func TestCommandKeepsNameAndArguments(t *testing.T) {
	cmd := command(context.Background(), transcodeCommand, "-hide_banner", "-version")
	if got := cmd.Args; !slices.Equal(got, []string{transcodeCommand, "-hide_banner", "-version"}) {
		t.Fatalf("Args = %q", got)
	}
}

// A grandchild that keeps the output pipes open must not block Wait after cancellation.
func TestCommandBoundsWaitForOutputPipes(t *testing.T) {
	cmd := command(context.Background(), transcodeCommand, "-version")
	if cmd.WaitDelay != commandStopDelay {
		t.Fatalf("WaitDelay = %v, want %v", cmd.WaitDelay, commandStopDelay)
	}
}
