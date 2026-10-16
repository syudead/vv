package domain

import "time"

// MediaBroadcast は TS の録画の放送局名と番組名である（specs/056-media-info/data-model.md
// 「Stored value」）。局名は SDT の service name、番組名は SelectBroadcastProgram で選んだ
// EIT の event name である。
type MediaBroadcast struct {
	Station string `json:",omitempty"`
	Program string `json:",omitempty"`
}

// BroadcastEvent は EIT present/following の event 1 つである。Start が零値なら開始は
// 未定義、Duration が負なら長さは未定義である（EIT の全ビット 1）。
type BroadcastEvent struct {
	Name     string
	Start    time.Time
	Duration time.Duration
}

// NewMediaBroadcast は局名と番組名を整えて MediaBroadcast にする。どちらも空なら nil。
func NewMediaBroadcast(station, program string) *MediaBroadcast {
	broadcast := MediaBroadcast{Station: CleanMediaText(station), Program: CleanMediaText(program)}
	if broadcast == (MediaBroadcast{}) {
		return nil
	}
	return &broadcast
}

// SelectBroadcastProgram は TS の録画の番組名を選ぶ（specs/056-media-info/data-model.md
// 「Rules」、research.md R-4）。録画の範囲は TOT/TDT の時刻 recordedAt から尺 durationMs
// までで、present と following のうち、その範囲と最も長く重なる event の名前を返す。
// 同じ長さなら following を採る。長さが未定義の present は following の開始で終わり、
// 長さが未定義の following は録画の終わりまで続くものとする。時刻が零値か、どちらも
// 重ならなければ空（項目なし）である。
func SelectBroadcastProgram(recordedAt time.Time, durationMs int64, present, following *BroadcastEvent) string {
	if recordedAt.IsZero() || durationMs <= 0 {
		return ""
	}
	recordingEnd := recordedAt.Add(time.Duration(durationMs) * time.Millisecond)
	var presentEnd time.Time
	if following != nil {
		presentEnd = following.Start
	}
	presentOverlap := eventOverlap(present, presentEnd, recordedAt, recordingEnd)
	followingOverlap := eventOverlap(following, recordingEnd, recordedAt, recordingEnd)
	switch {
	case followingOverlap > 0 && followingOverlap >= presentOverlap:
		return CleanMediaText(following.Name)
	case presentOverlap > 0:
		return CleanMediaText(present.Name)
	default:
		return ""
	}
}

// eventOverlap は event と録画の範囲 [from, to) が重なる長さを返す。event の長さが未定義なら
// undefinedEnd を終わりとし、それも零値なら重なりは無い。開始が未定義でも重なりは無い。
func eventOverlap(event *BroadcastEvent, undefinedEnd, from, to time.Time) time.Duration {
	if event == nil || event.Start.IsZero() {
		return 0
	}
	end := undefinedEnd
	if event.Duration >= 0 {
		end = event.Start.Add(event.Duration)
	}
	if end.IsZero() {
		return 0
	}
	start := event.Start
	if start.Before(from) {
		start = from
	}
	if end.After(to) {
		end = to
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start)
}
