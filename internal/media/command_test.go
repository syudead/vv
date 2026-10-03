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
