package domain

import (
	"testing"
	"time"
)

// 状態は上から順に最初に当てはまるものになる（specs/024-import-progress/contracts/scan-api.md §2）。
func TestImportTallyStatusOrder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tally ImportTally
		want  ImportStatus
	}{
		{
			name:  "走査の失敗はほかより優先する",
			tally: ImportTally{State: ScanFailed, ScanTotal: 0, Videos: 3, SettledVideos: 1, FailedIssues: 2},
			want:  ImportFailed,
		},
		{
			name:  "走査の失敗は、対象がすべて済んでいても failed",
			tally: ImportTally{State: ScanFailed, Videos: 2, SettledVideos: 2},
			want:  ImportFailed,
		},
		{
			name:  "走査中で対象を数え終えていなければ finding",
			tally: ImportTally{State: ScanRunning, Videos: 4, SettledVideos: 1},
			want:  ImportFinding,
		},
		{
			name:  "走査中で数え終えていれば running",
			tally: ImportTally{State: ScanRunning, ScanTotal: 10, ScanCompleted: 10, Videos: 10, SettledVideos: 10},
			want:  ImportRunning,
		},
		{
			name:  "走査が閉じても済んでいない対象があれば running",
			tally: ImportTally{State: ScanDone, ScanTotal: 2, ScanCompleted: 2, Videos: 2, SettledVideos: 1, FailedIssues: 1},
			want:  ImportRunning,
		},
		{
			name:  "閉じて済み、失敗の問題があれば partial",
			tally: ImportTally{State: ScanDone, ScanTotal: 2, ScanCompleted: 2, Videos: 2, SettledVideos: 2, FailedIssues: 1},
			want:  ImportPartial,
		},
		{
			name:  "閉じて済み、失敗の問題が無ければ done",
			tally: ImportTally{State: ScanDone, ScanTotal: 2, ScanCompleted: 2, Videos: 2, SettledVideos: 2},
			want:  ImportDone,
		},
		{
			name:  "変化の無い取り込みは done",
			tally: ImportTally{State: ScanDone},
			want:  ImportDone,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tally.Status(); got != tc.want {
				t.Fatalf("Status() = %q, want %q", got, tc.want)
			}
		})
	}
}

// 分母は集合の本数、登録できなかったファイルの問題の数、まだ登録を試していない
// ファイルの数の和で、済みは集合のうち済みの本数と登録できなかったファイルの
// 問題の数の和である（research.md R-5）。
func TestImportTallyCounts(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		tally                ImportTally
		wantTotal, wantSettl int
	}{
		{
			name: "走査中は、まだ登録を試していないファイルを分母に足す",
			tally: ImportTally{State: ScanRunning, ScanTotal: 10, ScanCompleted: 3, ScanFailed: 1,
				Videos: 3, SettledVideos: 1, UnregisteredIssues: 1},
			wantTotal: 3 + 1 + 6, wantSettl: 1 + 1,
		},
		{
			name: "走査が閉じたら、試されなかったファイルは数えない",
			tally: ImportTally{State: ScanFailed, ScanTotal: 10, ScanCompleted: 3,
				Videos: 3, SettledVideos: 3},
			wantTotal: 3, wantSettl: 3,
		},
		{
			name:      "持ち越した動画だけの取り込み",
			tally:     ImportTally{State: ScanDone, Videos: 4, SettledVideos: 2},
			wantTotal: 4, wantSettl: 2,
		},
		{
			name:      "済みは分母を超えない",
			tally:     ImportTally{State: ScanDone, Videos: 2, SettledVideos: 5},
			wantTotal: 2, wantSettl: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tally.VideosTotal(); got != tc.wantTotal {
				t.Errorf("VideosTotal() = %d, want %d", got, tc.wantTotal)
			}
			if got := tc.tally.VideosSettled(); got != tc.wantSettl {
				t.Errorf("VideosSettled() = %d, want %d", got, tc.wantSettl)
			}
		})
	}
}

// finding のあいだは本数を示さず、完了の時刻は done・partial のときだけ返す。
func TestImportTallyProgress(t *testing.T) {
	settledAt := time.Unix(1_700_000_000, 0)

	finding := ImportTally{State: ScanRunning, Videos: 2}.Progress(time.Time{})
	if finding.Status != ImportFinding || finding.Counted || finding.Total != 0 {
		t.Fatalf("finding = %+v, want 本数なし", finding)
	}

	running := ImportTally{State: ScanDone, Videos: 2, SettledVideos: 1}.Progress(settledAt)
	if running.Status != ImportRunning || !running.Counted || running.Total != 2 || running.Settled != 1 ||
		!running.SettledAt.IsZero() {
		t.Fatalf("running = %+v, want 2本のうち1本・時刻なし", running)
	}

	done := ImportTally{State: ScanDone, Videos: 2, SettledVideos: 2}.Progress(settledAt)
	if done.Status != ImportDone || !done.SettledAt.Equal(settledAt) {
		t.Fatalf("done = %+v, want 時刻つき", done)
	}

	partial := ImportTally{State: ScanDone, FailedIssues: 1}.Progress(settledAt)
	if partial.Status != ImportPartial || !partial.SettledAt.Equal(settledAt) {
		t.Fatalf("partial = %+v, want 時刻つき", partial)
	}

	failed := ImportTally{State: ScanFailed}.Progress(settledAt)
	if failed.Status != ImportFailed || !failed.SettledAt.IsZero() || !failed.Counted {
		t.Fatalf("failed = %+v, want 本数つき・時刻なし", failed)
	}
}
