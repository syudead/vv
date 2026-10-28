package domain

import "testing"

func TestJobCPUSlotsKeepCPUForStreaming(t *testing.T) {
	for _, tc := range []struct{ numCPU, cpu int }{{1, 1}, {4, 2}, {16, 5}} {
		if got := JobCPUSlots(tc.numCPU); got != tc.cpu {
			t.Errorf("JobCPUSlots(%d) = %d, want %d", tc.numCPU, got, tc.cpu)
		}
	}
}

func TestDiskReadSlotsFollowTheDiskType(t *testing.T) {
	for _, tc := range []struct {
		disk  DiskType
		slots int
	}{{DiskUnknown, 1}, {DiskRotational, 1}, {DiskSolidState, 4}} {
		if got := (Disk{Type: tc.disk}).ReadSlots(); got != tc.slots {
			t.Errorf("%v disk read slots = %d, want %d", tc.disk, got, tc.slots)
		}
	}
}
