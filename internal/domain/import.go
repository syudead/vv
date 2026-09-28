package domain

import "time"

// ImportStatus は直近の取り込みの、利用者に見せる状態である。値は
// api/openapi.yaml の Scan.status に対応する（specs/024-import-progress/contracts/scan-api.md §2）。
type ImportStatus string

const (
	// ImportFinding は走査がまだ対象のファイルを数え終えていない。本数を示さない。
	ImportFinding ImportStatus = "finding"
	// ImportRunning は走査中か、済んでいない対象の動画がある。
	ImportRunning ImportStatus = "running"
	// ImportDone は走査が閉じ、対象がすべて済み、失敗の問題が無い。
	ImportDone ImportStatus = "done"
	// ImportPartial は走査が閉じ、対象がすべて済んだが、失敗の問題がある。
	ImportPartial ImportStatus = "partial"
	// ImportFailed は走査そのものが失敗した。ほかより優先する。
	ImportFailed ImportStatus = "failed"
)

// ImportTally は取り込みの状態と本数を決める入力である
// （specs/024-import-progress/research.md R-4・R-5）。保存の手段は知らない。
type ImportTally struct {
	// State は走査そのものの状態である。
	State ScanState
	// ScanTotal・ScanCompleted・ScanFailed は走査が数えた対象のファイルの数と、
	// そのうち登録を終えた数・登録できなかった数である（scans の total・completed・failed）。
	ScanTotal     int
	ScanCompleted int
	ScanFailed    int
	// Videos は直近の走査の対象の動画の本数、SettledVideos はそのうち残りの仕事が
	// 無い本数である。
	Videos        int
	SettledVideos int
	// UnregisteredIssues は登録できなかったファイルの問題の数である。
	UnregisteredIssues int
	// FailedIssues は重さが失敗の問題の数である。
	FailedIssues int
}

// Enumerated は走査が対象のファイルを数え終えたかを返す。走査は全ルートを
// 列挙してから対象の数を記録するので、実行中に数が 0 のままなら列挙中である。
func (t ImportTally) Enumerated() bool {
	return t.State != ScanRunning || t.ScanTotal > 0
}

// pendingFiles は、走査が数えた対象のうち、まだ登録を試していないファイルの数である。
// 走査が閉じたあとは、試されないまま残ったファイルは対象に数えない。
func (t ImportTally) pendingFiles() int {
	if t.State != ScanRunning {
		return 0
	}
	return max(0, t.ScanTotal-t.ScanCompleted-t.ScanFailed)
}

// VideosTotal は進み具合の分母である。対象の動画の本数、登録できなかったファイルの
// 問題の数、まだ登録を試していないファイルの数の和にする。
func (t ImportTally) VideosTotal() int {
	return max(0, t.Videos) + max(0, t.UnregisteredIssues) + t.pendingFiles()
}

// VideosSettled は済みの本数である。対象のうち残りの仕事が無い本数と、登録できな
// かったファイルの問題の数（失敗として終わっている）の和にする。分母を超えない。
func (t ImportTally) VideosSettled() int {
	settled := min(max(0, t.SettledVideos), max(0, t.Videos)) + max(0, t.UnregisteredIssues)
	return min(settled, t.VideosTotal())
}

// Status は利用者に見せる状態を決める。上から順に最初に当てはまるものにする。
//
//  1. failed: 走査そのものが失敗した
//  2. finding: 走査中で、対象をまだ数え終えていない
//  3. running: 走査中か、済んでいない対象がある
//  4. partial: 失敗の問題がある
//  5. done: それ以外
func (t ImportTally) Status() ImportStatus {
	switch {
	case t.State == ScanFailed:
		return ImportFailed
	case !t.Enumerated():
		return ImportFinding
	case t.State == ScanRunning || t.VideosSettled() < t.VideosTotal():
		return ImportRunning
	case t.FailedIssues > 0:
		return ImportPartial
	default:
		return ImportDone
	}
}

// ImportProgress は直近の取り込みの、利用者から見た状態である。
type ImportProgress struct {
	Status ImportStatus
	// Counted は本数（Total・Settled）を示すかである。finding のあいだは示さない。
	Counted bool
	Total   int
	Settled int
	// SettledAt は対象がすべて済んだ時刻である。done・partial のときだけ入る。
	SettledAt time.Time
}

// Progress は状態と本数を組み立てる。settledAt は保存された完了の時刻で、
// done・partial のときだけ返す。
func (t ImportTally) Progress(settledAt time.Time) ImportProgress {
	status := t.Status()
	out := ImportProgress{Status: status}
	if status != ImportFinding {
		out.Counted = true
		out.Total = t.VideosTotal()
		out.Settled = t.VideosSettled()
	}
	if status == ImportDone || status == ImportPartial {
		out.SettledAt = settledAt
	}
	return out
}
