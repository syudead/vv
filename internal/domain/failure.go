package domain

import "errors"

// ProbeErrorCode は解析の失敗理由のコードである。videos.probe_error_code に保存し、
// API の Video.probeErrorCode に出る。値の綴りは api/openapi.yaml の ProbeErrorCode と
// 同じにする（specs/023-english-i18n/data-model.md §1）。
type ProbeErrorCode string

const (
	// ProbeErrorFileUnavailable は解析の前後でファイルを確かめられない、または通常
	// ファイルでないことを表す。
	ProbeErrorFileUnavailable ProbeErrorCode = "file_unavailable"
	// ProbeErrorProbeUnavailable は ffprobe を起動できないことを表す。
	ProbeErrorProbeUnavailable ProbeErrorCode = "probe_unavailable"
	// ProbeErrorProbeFailed は ffprobe が失敗したことを表す。壊れた・対応しない
	// ファイル、時間切れを含む。
	ProbeErrorProbeFailed ProbeErrorCode = "probe_failed"
	// ProbeErrorInvalidMetadata は ffprobe の出力を解釈できない、尺が読めない・不正で
	// あることを表す。
	ProbeErrorInvalidMetadata ProbeErrorCode = "invalid_metadata"
	// ProbeErrorInternal は上のどれにも分類できない失敗である。
	ProbeErrorInternal ProbeErrorCode = "internal"
)

// ProbeFailure は解析の失敗に理由のコードを付けて包む。失敗を作る adapter
// （internal/media）が包み、受け取る側は ProbeErrorCodeOf で取り出す。
type ProbeFailure struct {
	Code ProbeErrorCode
	Err  error
}

// NewProbeFailure は err を code で包む。
func NewProbeFailure(code ProbeErrorCode, err error) error {
	return &ProbeFailure{Code: code, Err: err}
}

func (e *ProbeFailure) Error() string { return e.Err.Error() }

func (e *ProbeFailure) Unwrap() error { return e.Err }

// ProbeErrorCodeOf は err の鎖にある ProbeFailure のコードを返す。無ければ
// ProbeErrorInternal である。
func ProbeErrorCodeOf(err error) ProbeErrorCode {
	var failure *ProbeFailure
	if errors.As(err, &failure) && failure.Code != "" {
		return failure.Code
	}
	return ProbeErrorInternal
}

// ScanErrorCode は取り込み（走査）の失敗理由のコードである。scans.error_code に
// 保存し、API の Scan.errorCode に出る。値の綴りは api/openapi.yaml の ScanErrorCode と
// 同じにする（specs/023-english-i18n/data-model.md §2）。
type ScanErrorCode string

const (
	// ScanErrorMediaFolderUnreadable はメディアフォルダを読めないことを表す。
	ScanErrorMediaFolderUnreadable ScanErrorCode = "media_folder_unreadable"
	// ScanErrorMediaFolderNotDirectory はメディアフォルダがディレクトリでない、または
	// シンボリックリンクであることを表す。
	ScanErrorMediaFolderNotDirectory ScanErrorCode = "media_folder_not_directory"
	// ScanErrorLocationUnreadable は走査の途中、または取り込み後の確認で読めない
	// 場所があったことを表す。
	ScanErrorLocationUnreadable ScanErrorCode = "location_unreadable"
	// ScanErrorInterrupted は停止の指示で打ち切った、または取り込みの途中で
	// アプリケーションが止まったことを表す。
	ScanErrorInterrupted ScanErrorCode = "interrupted"
	// ScanErrorInternal は上のどれにも分類できない失敗である。
	ScanErrorInternal ScanErrorCode = "internal"
)

// ScanFailure は取り込みの失敗に理由のコードと、理由が結び付く場所を付けて包む。
// 失敗を作る adapter（internal/scanner）が包み、受け取る側は ScanFailureOf で取り出す。
type ScanFailure struct {
	Code ScanErrorCode
	// Path は理由が結び付く絶対パス（メディアフォルダ、または読めなかった場所）。
	// 特定の場所に結び付かなければ空である。
	Path string
	Err  error
}

// NewScanFailure は err を code と path で包む。
func NewScanFailure(code ScanErrorCode, path string, err error) error {
	return &ScanFailure{Code: code, Path: path, Err: err}
}

func (e *ScanFailure) Error() string { return e.Err.Error() }

func (e *ScanFailure) Unwrap() error { return e.Err }

// ScanFailureOf は err の鎖にある ScanFailure のコードと場所を返す。無ければ
// ScanErrorInternal と空の場所である。
func ScanFailureOf(err error) (ScanErrorCode, string) {
	var failure *ScanFailure
	if errors.As(err, &failure) && failure.Code != "" {
		return failure.Code, failure.Path
	}
	return ScanErrorInternal, ""
}
