package domain

import (
	"slices"
	"testing"
)

// 画質ごとの短辺と kbps は契約 §2 の表のとおりである。
func TestTranscodeQualityLimitsMatchContract(t *testing.T) {
	want := map[TranscodeQuality]TranscodeQualityLimits{
		"1080p": {ShortSide: 1080, VideoKbps: 5000, BufferKbps: 10000, AudioKbps: 128},
		"720p":  {ShortSide: 720, VideoKbps: 2500, BufferKbps: 5000, AudioKbps: 128},
		"480p":  {ShortSide: 480, VideoKbps: 1200, BufferKbps: 2400, AudioKbps: 96},
		"360p":  {ShortSide: 360, VideoKbps: 700, BufferKbps: 1400, AudioKbps: 64},
	}
	if len(TranscodeQualities) != len(want) {
		t.Fatalf("TranscodeQualities = %v", TranscodeQualities)
	}
	for _, quality := range TranscodeQualities {
		parsed, ok := ParseTranscodeQuality(string(quality))
		if !ok || parsed != quality {
			t.Errorf("ParseTranscodeQuality(%q) = %q, %v", quality, parsed, ok)
		}
		limits, ok := quality.Limits()
		if !ok || limits != want[quality] {
			t.Errorf("%s: Limits() = %+v, %v; want %+v", quality, limits, ok, want[quality])
		}
	}
}

func TestParseTranscodeQualityRejectsUnknown(t *testing.T) {
	for _, value := range []string{"", "original", "240p", "1080P", "480", " 480p"} {
		if quality, ok := ParseTranscodeQuality(value); ok {
			t.Errorf("ParseTranscodeQuality(%q) = %q, true", value, quality)
		}
		if _, ok := TranscodeQuality(value).Limits(); ok {
			t.Errorf("TranscodeQuality(%q).Limits() が解釈できた", value)
		}
	}
}

// 画質は、その短辺が動画の短辺より小さいときだけ使える（R-3）。
func TestTranscodeQualityAvailable(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
		want          []TranscodeQuality
	}{
		{"landscape 1080", 1920, 1080, []TranscodeQuality{"720p", "480p", "360p"}},
		{"portrait 1080", 1080, 1920, []TranscodeQuality{"720p", "480p", "360p"}},
		{"360", 640, 360, nil},
		{"4K", 3840, 2160, TranscodeQualities},
		{"no dimensions", 0, 0, nil},
		{"no height", 1920, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []TranscodeQuality
			for _, quality := range TranscodeQualities {
				if quality.Available(tc.width, tc.height) {
					got = append(got, quality)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("使える画質 = %v, want %v", got, tc.want)
			}
		})
	}
	if TranscodeQuality("240p").Available(1920, 1080) {
		t.Error("知らない画質が使える")
	}
}
