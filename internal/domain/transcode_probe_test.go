package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTranscodeProbeUsable(t *testing.T) {
	probe := TranscodeProbe{
		FormatName: "mov,mp4,m4a,3gp,3g2,mj2",
		Video: TranscodeVideo{
			Index: 0, CodecName: "h264", Profile: "High", Level: 41, PixelFormat: "yuv420p", BitsPerRawSample: 8,
			Width: 1920, Height: 1080, SampleAspectNum: 1, SampleAspectDen: 1, Rotation: 90, FPS: 29.97, RealFPS: 29.97,
		},
		Audio: &TranscodeAudio{Index: 1, CodecName: "aac", Profile: "LC", SampleRate: 48000, Channels: 2},
	}
	encoded, err := json.Marshal(probe)
	if err != nil {
		t.Fatal(err)
	}
	opened := FileStamp{SizeBytes: 4096, ModTimeNs: 1_700_000_000_000_000_001}
	valid := StoredTranscodeProbe{Version: TranscodeProbeVersion, Source: opened, Probe: string(encoded)}

	got, ok := TranscodeProbeUsable(&valid, opened)
	if !ok || !reflect.DeepEqual(got, probe) {
		t.Fatalf("usable = %v, got %+v", ok, got)
	}

	tests := []struct {
		name   string
		stored *StoredTranscodeProbe
		opened FileStamp
	}{
		{"row missing", nil, opened},
		{"older version", &StoredTranscodeProbe{Version: TranscodeProbeVersion - 1, Source: opened, Probe: string(encoded)}, opened},
		{"newer version", &StoredTranscodeProbe{Version: TranscodeProbeVersion + 1, Source: opened, Probe: string(encoded)}, opened},
		{"broken JSON", &StoredTranscodeProbe{Version: TranscodeProbeVersion, Source: opened, Probe: `{"Video":`}, opened},
		{"wrong JSON type", &StoredTranscodeProbe{Version: TranscodeProbeVersion, Source: opened, Probe: `{"Video":"h264"}`}, opened},
		{"null JSON", &StoredTranscodeProbe{Version: TranscodeProbeVersion, Source: opened, Probe: `null`}, opened},
		{"empty object", &StoredTranscodeProbe{Version: TranscodeProbeVersion, Source: opened, Probe: `{}`}, opened},
		{"size differs", &valid, FileStamp{SizeBytes: opened.SizeBytes + 1, ModTimeNs: opened.ModTimeNs}},
		{"mtime differs", &valid, FileStamp{SizeBytes: opened.SizeBytes, ModTimeNs: opened.ModTimeNs + 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := TranscodeProbeUsable(tc.stored, tc.opened); ok {
				t.Fatalf("usable with %+v", got)
			}
		})
	}
}
