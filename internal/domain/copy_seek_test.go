package domain

import "testing"

func TestCopySeekWithinAllowance(t *testing.T) {
	tests := []struct {
		name                  string
		requestedMs, actualMs int64
		want                  bool
	}{
		{"same position", 27000, 27000, true},
		{"previous keyframe", 27000, 25000, true},
		{"exactly the allowance", 20000, 5000, true},
		{"beyond the allowance", 20001, 5000, false},
		{"first keyframe after the request", 500, 1000, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CopySeekWithinAllowance(tc.requestedMs, tc.actualMs); got != tc.want {
				t.Errorf("CopySeekWithinAllowance(%d, %d) = %v, want %v", tc.requestedMs, tc.actualMs, got, tc.want)
			}
		})
	}
}
