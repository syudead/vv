package domain

import "testing"

func TestChooseVersionRepresentative(t *testing.T) {
	ptr := func(v int) *int { return &v }
	ms := func(v int64) *int64 { return &v }
	base := func(id int64) Video {
		return Video{ID: id, Playable: true, ProbeState: ProbeStateDone, AudioCodec: "aac", Width: ptr(1280),
			Height: ptr(720), SizeBytes: 1000, DurationMs: ms(100_000)}
	}
	hd := base(2)
	hd.Width, hd.Height = ptr(1920), ptr(1080)
	silent := base(2)
	silent.AudioCodec = ""
	unplayable := base(2)
	unplayable.Playable = false
	dense := base(2)
	dense.SizeBytes = 2000
	timed := func(videoMs, audioMs int64, bitRate int64) *MediaInfo {
		return &MediaInfo{
			Video:       &MediaVideo{BitRate: bitRate, MediaTiming: MediaTiming{DurationMs: ms(videoMs)}},
			AudioTracks: []MediaAudioTrack{{MediaTiming: MediaTiming{DurationMs: ms(audioMs)}}},
		}
	}
	for _, c := range []struct {
		name   string
		sides  [2]VersionSide
		want   int
		reason VersionRepresentativeReason
	}{
		{"持ち主が選んだ代表は画素の多い方より残る", [2]VersionSide{{Video: base(1), OwnerChosen: true}, {Video: hd}}, 0, VersionPreferResolution},
		{"再生できる方", [2]VersionSide{{Video: unplayable}, {Video: base(3)}}, 1, VersionPreferPlayable},
		{"音声がある方", [2]VersionSide{{Video: silent}, {Video: base(3)}}, 1, VersionPreferAudio},
		{"音声の長さが映像と合う方が画素の多い方より先", [2]VersionSide{
			{Video: hd, Media: timed(100_000, 90_000, 0)}, {Video: base(3), Media: timed(100_000, 99_500, 0)},
		}, 1, VersionPreferAudioSync},
		{"画素の多い方", [2]VersionSide{{Video: base(1)}, {Video: hd}}, 1, VersionPreferResolution},
		{"ファイルが大きくても映像のビットレートが低い方は負ける", [2]VersionSide{
			{Video: dense, Media: timed(100_000, 100_000, 2_000_000)}, {Video: base(3), Media: timed(100_000, 100_000, 3_000_000)},
		}, 1, VersionPreferBitrate},
		{"映像のビットレートが分からなければファイルの毎秒バイト", [2]VersionSide{{Video: base(1)}, {Video: dense}}, 1, VersionPreferBitrate},
		{"id の小さい方", [2]VersionSide{{Video: base(3)}, {Video: base(2)}}, 1, VersionPreferOlder},
	} {
		if got := ChooseVersionRepresentative(c.sides); got != c.want {
			t.Errorf("%s: 側 %d, want %d", c.name, got, c.want)
		}
		if _, reason := RecommendVersionRepresentative(c.sides); reason != c.reason {
			t.Errorf("%s: 理由 %q, want %q", c.name, reason, c.reason)
		}
	}
}

func TestAudioTiming(t *testing.T) {
	ms := func(v int64) *int64 { return &v }
	media := &MediaInfo{
		Video:       &MediaVideo{MediaTiming: MediaTiming{StartMs: ms(0), DurationMs: ms(600_000)}},
		AudioTracks: []MediaAudioTrack{{MediaTiming: MediaTiming{StartMs: ms(1200), DurationMs: ms(595_000)}}},
	}
	timing := AudioTiming(media)
	if timing.StartGapMs == nil || *timing.StartGapMs != 1200 || timing.LengthGapMs == nil || *timing.LengthGapMs != 5000 {
		t.Errorf("AudioTiming = %+v", timing)
	}
	if AudioLengthSuspect(media) {
		t.Error("10 分で 5 秒短いだけ（1% 以内）を疑った")
	}
	media.AudioTracks[0].DurationMs = ms(580_000)
	if !AudioLengthSuspect(media) {
		t.Error("10 分で 20 秒短いのを見逃した")
	}
	if AudioLengthSuspect(nil) || AudioLengthSuspect(&MediaInfo{Video: &MediaVideo{}}) {
		t.Error("情報が無いのに疑った")
	}
}
