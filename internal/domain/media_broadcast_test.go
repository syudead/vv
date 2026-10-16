package domain

import (
	"testing"
	"time"
)

// TS の録画の番組の選び方（specs/056-media-info/research.md R-4 の表）。

var broadcastZone = time.FixedZone("JST", 9*60*60)

func broadcastClock(hour, minute int) time.Time {
	return time.Date(2026, 10, 16, hour, minute, 0, 0, broadcastZone)
}

func broadcastEvent(name string, start time.Time, duration time.Duration) *BroadcastEvent {
	return &BroadcastEvent{Name: name, Start: start, Duration: duration}
}

func minutesMs(minutes int) int64 {
	return int64(minutes) * 60 * 1000
}

func TestSelectBroadcastProgram(t *testing.T) {
	news := broadcastEvent("ニュース", broadcastClock(20, 0), time.Hour)
	drama := broadcastEvent("ドラマ", broadcastClock(21, 0), 54*time.Minute)
	tests := []struct {
		name       string
		recordedAt time.Time
		durationMs int64
		present    *BroadcastEvent
		following  *BroadcastEvent
		want       string
	}{
		{
			name:       "番組の 5 分前に始まる録画は following の event",
			recordedAt: broadcastClock(20, 55), durationMs: minutesMs(61),
			present: news, following: drama, want: "ドラマ",
		},
		{
			name:       "番組の途中で始まる録画は present の event",
			recordedAt: broadcastClock(21, 10), durationMs: minutesMs(44),
			present:   drama,
			following: broadcastEvent("次の番組", broadcastClock(21, 54), time.Hour),
			want:      "ドラマ",
		},
		{
			name:       "長さが未定義の present は following の開始で終わる",
			recordedAt: broadcastClock(20, 30), durationMs: minutesMs(40),
			present:   broadcastEvent("ニュース", broadcastClock(20, 0), -1),
			following: drama,
			want:      "ニュース",
		},
		{
			name:       "長さが未定義の present と following が無ければ重なりは無い",
			recordedAt: broadcastClock(20, 30), durationMs: minutesMs(40),
			present: broadcastEvent("ニュース", broadcastClock(20, 0), -1),
			want:    "",
		},
		{
			name:       "同じ長さなら following",
			recordedAt: broadcastClock(20, 50), durationMs: minutesMs(20),
			present: news, following: drama, want: "ドラマ",
		},
		{
			name:       "時刻が無ければ Program は無い",
			recordedAt: time.Time{}, durationMs: minutesMs(61),
			present: news, following: drama, want: "",
		},
		{
			name:       "どちらとも重ならなければ Program は無い",
			recordedAt: broadcastClock(23, 0), durationMs: minutesMs(30),
			present: news, following: drama, want: "",
		},
		{
			name:       "長さが未定義の following は録画の終わりまで",
			recordedAt: broadcastClock(20, 50), durationMs: minutesMs(60),
			present:   news,
			following: broadcastEvent("特番", broadcastClock(21, 0), -1),
			want:      "特番",
		},
		{
			name:       "following が無くても present を選ぶ",
			recordedAt: broadcastClock(20, 10), durationMs: minutesMs(30),
			present: news, want: "ニュース",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectBroadcastProgram(tt.recordedAt, tt.durationMs, tt.present, tt.following)
			if got != tt.want {
				t.Errorf("SelectBroadcastProgram = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewMediaBroadcast(t *testing.T) {
	if got := NewMediaBroadcast(" 　", "\x07"); got != nil {
		t.Errorf("表示できる文字の無い名前は省く: %+v", got)
	}
	got := NewMediaBroadcast(" ＮＨＫ総合 ", "")
	if got == nil || got.Station != "ＮＨＫ総合" || got.Program != "" {
		t.Errorf("NewMediaBroadcast = %+v", got)
	}
}
