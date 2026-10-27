package domain

import "testing"

func TestJobCapacityKeepsCPUForStreaming(t *testing.T) {
	for _, tc := range []struct{ numCPU, cpu int }{{1, 1}, {4, 2}, {16, 5}} {
		if got, want := JobCapacity(tc.numCPU), (JobResources{Read: SharedReadSlots, CPU: tc.cpu}); got != want {
			t.Errorf("JobCapacity(%d) = %+v, want %+v", tc.numCPU, got, want)
		}
	}
}
