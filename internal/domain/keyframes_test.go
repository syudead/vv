package domain

import "testing"

func TestKeyframeProbeSparseFor(t *testing.T) {
	source := FileStamp{SizeBytes: 12, ModTimeNs: 34}
	for _, tc := range []struct {
		name  string
		probe KeyframeProbe
		want  bool
	}{
		{"sparse", KeyframeProbe{KeyframeProbeVersion, source, KeyframesSparse}, true},
		{"short", KeyframeProbe{KeyframeProbeVersion, source, KeyframesShort}, false},
		{"unknown", KeyframeProbe{KeyframeProbeVersion, source, KeyframesUnknown}, false},
		{"missing", KeyframeProbe{}, false},
		{"old version", KeyframeProbe{0, source, KeyframesSparse}, false},
		{"size changed", KeyframeProbe{KeyframeProbeVersion, FileStamp{13, 34}, KeyframesSparse}, false},
		{"mtime changed", KeyframeProbe{KeyframeProbeVersion, FileStamp{12, 35}, KeyframesSparse}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.probe.SparseFor(source); got != tc.want {
				t.Fatalf("SparseFor = %v", got)
			}
		})
	}
}
