package domain

import "testing"

// 仕事の種類ごとに今の処理の種類があり、知らない種類には無い。
func TestActivityKindOf(t *testing.T) {
	want := map[JobKind]ScanActivityKind{
		JobProbe: ActivityProbe, JobThumbnail: ActivityThumbnail,
		JobSeekThumbnail: ActivitySeekThumbnail, JobPreview: ActivityPreview,
		JobFingerprint: ActivityFingerprint,
	}
	for _, kind := range JobKinds {
		got, ok := ActivityKindOf(kind)
		if !ok || got != want[kind] {
			t.Errorf("ActivityKindOf(%s) = %q, %v, want %q", kind, got, ok, want[kind])
		}
	}
	if _, ok := ActivityKindOf("unknown"); ok {
		t.Error("知らない種類に今の処理の種類がある")
	}
}
