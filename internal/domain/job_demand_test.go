package domain

import "testing"

// The slots leave CPU for streaming and stop growing at the ceiling on a machine with many cores.
func TestJobCPUSlotsKeepCPUForStreamingUpToTheCeiling(t *testing.T) {
	for _, tc := range []struct{ numCPU, cpu int }{{0, 1}, {1, 1}, {4, 2}, {12, 4}, {24, 4}, {64, 4}} {
		if got := JobCPUSlots(tc.numCPU); got != tc.cpu {
			t.Errorf("JobCPUSlots(%d) = %d, want %d", tc.numCPU, got, tc.cpu)
		}
	}
}

// Every scheduled kind has an explicit cap, so no kind can fill every slot.
func TestJobConcurrencyCapsEveryScheduledKind(t *testing.T) {
	for _, kind := range ScheduledJobKinds {
		if _, ok := jobConcurrency[kind]; !ok {
			t.Errorf("%s has no concurrency cap", kind)
		}
	}
	for _, tc := range []struct {
		kind JobKind
		cap  int
	}{{JobSeekThumbnail, 1}, {JobProbe, 2}, {JobKind("unknown"), 1}} {
		if got := JobConcurrency(tc.kind); got != tc.cap {
			t.Errorf("JobConcurrency(%s) = %d, want %d", tc.kind, got, tc.cap)
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
