package domain

import (
	"errors"
	"strings"
	"testing"
)

// 表示名はタグ名と同じ順で整え、空は解除、制御文字と上限超は理由つきの誤りにする
// （specs/029-video-overrides/research.md R-10）。
func TestNormalizeDisplayName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		clear   bool
		problem DisplayNameProblem
	}{
		{name: "そのまま", input: "旅行 2024 夏", want: "旅行 2024 夏"},
		{name: "前後の空白を除く", input: "　 旅行  ", want: "旅行"},
		{name: "空は解除", input: "", clear: true},
		{name: "空白だけは解除", input: " 　 ", clear: true},
		{name: "改行", input: "旅行\n", problem: DisplayNameControlCharacters},
		{name: "タブ", input: "\t旅行", problem: DisplayNameControlCharacters},
		{name: "上限ちょうど", input: strings.Repeat("あ", DisplayNameMaxLength), want: strings.Repeat("あ", DisplayNameMaxLength)},
		{name: "上限超", input: strings.Repeat("あ", DisplayNameMaxLength+1), problem: DisplayNameTooLong},
		{name: "前後の空白を除いてから長さを数える", input: " " + strings.Repeat("a", DisplayNameMaxLength) + " ", want: strings.Repeat("a", DisplayNameMaxLength)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, clear, err := NormalizeDisplayName(tc.input)
			if tc.problem != 0 {
				var invalid *InvalidDisplayNameError
				if !errors.As(err, &invalid) || invalid.Problem != tc.problem {
					t.Fatalf("err = %v, want problem %d", err, tc.problem)
				}
				if !errors.Is(err, ErrInvalidDisplayName) {
					t.Errorf("errors.Is(err, ErrInvalidDisplayName) = false")
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tc.want || clear != tc.clear {
				t.Errorf("= (%q, %v), want (%q, %v)", got, clear, tc.want, tc.clear)
			}
		})
	}
}

// 位置つきの誤りは、表示名の誤りとして判定でき、理由も取り出せる。
func TestDisplayNameAtErrorUnwraps(t *testing.T) {
	_, _, cause := NormalizeDisplayName("a\nb")
	err := error(&DisplayNameAtError{Index: 3, Err: cause})
	var invalid *InvalidDisplayNameError
	if !errors.Is(err, ErrInvalidDisplayName) || !errors.As(err, &invalid) {
		t.Fatalf("err = %v", err)
	}
}

// 位置は解析済みで尺の内側（0 以上、尺未満）だけを受け付け、解析前と尺の外を分けて返す
// （specs/029-video-overrides/research.md R-11）。
func TestCheckThumbnailPosition(t *testing.T) {
	duration := int64(60_000)
	zero := int64(0)
	probed := Video{ProbeState: ProbeStateDone, DurationMs: &duration}
	cases := []struct {
		name     string
		video    Video
		position int64
		want     error
	}{
		{name: "先頭", video: probed, position: 0},
		{name: "尺の直前", video: probed, position: 59_999},
		{name: "尺ちょうど", video: probed, position: 60_000, want: ErrThumbnailPositionOutOfRange},
		{name: "負", video: probed, position: -1, want: ErrThumbnailPositionOutOfRange},
		{name: "解析前", video: Video{ProbeState: ProbeStatePending, DurationMs: &duration}, position: 0,
			want: ErrDurationUnknown},
		{name: "解析失敗", video: Video{ProbeState: ProbeStateFailed}, position: 0, want: ErrDurationUnknown},
		{name: "尺が無い", video: Video{ProbeState: ProbeStateDone}, position: 0, want: ErrDurationUnknown},
		{name: "尺が 0", video: Video{ProbeState: ProbeStateDone, DurationMs: &zero}, position: 0,
			want: ErrDurationUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := CheckThumbnailPosition(tc.video, tc.position); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("CheckThumbnailPosition = %v, want %v", err, tc.want)
			}
		})
	}
}
