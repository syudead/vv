package domain

import "errors"

// ProbeErrorCode values must match api/openapi.yaml; they are persisted in videos.probe_error_code.
type ProbeErrorCode string

const (
	ProbeErrorFileUnavailable  ProbeErrorCode = "file_unavailable"
	ProbeErrorProbeUnavailable ProbeErrorCode = "probe_unavailable"
	// ProbeErrorProbeFailed includes corrupt/unsupported files and timeouts.
	ProbeErrorProbeFailed     ProbeErrorCode = "probe_failed"
	ProbeErrorInvalidMetadata ProbeErrorCode = "invalid_metadata"
	ProbeErrorInternal        ProbeErrorCode = "internal"
)

type ProbeFailure struct {
	Code ProbeErrorCode
	Err  error
}

func NewProbeFailure(code ProbeErrorCode, err error) error {
	return &ProbeFailure{Code: code, Err: err}
}

func (e *ProbeFailure) Error() string { return e.Err.Error() }

func (e *ProbeFailure) Unwrap() error { return e.Err }

// ProbeErrorCodeOf defaults to ProbeErrorInternal when no wrapped failure exists.
func ProbeErrorCodeOf(err error) ProbeErrorCode {
	var failure *ProbeFailure
	if errors.As(err, &failure) && failure.Code != "" {
		return failure.Code
	}
	return ProbeErrorInternal
}

// ScanErrorCode values must match api/openapi.yaml; they are persisted in scans.error_code.
type ScanErrorCode string

const (
	ScanErrorMediaFolderUnreadable ScanErrorCode = "media_folder_unreadable"
	// ScanErrorMediaFolderNotDirectory includes symlinks.
	ScanErrorMediaFolderNotDirectory ScanErrorCode = "media_folder_not_directory"
	ScanErrorLocationUnreadable      ScanErrorCode = "location_unreadable"
	// ScanErrorInterrupted includes requested stops and application shutdown during import.
	ScanErrorInterrupted ScanErrorCode = "interrupted"
	ScanErrorInternal    ScanErrorCode = "internal"
)

type ScanFailure struct {
	Code ScanErrorCode
	// Path is absolute, or empty when the failure has no specific location.
	Path string
	Err  error
}

func NewScanFailure(code ScanErrorCode, path string, err error) error {
	return &ScanFailure{Code: code, Path: path, Err: err}
}

func (e *ScanFailure) Error() string { return e.Err.Error() }

func (e *ScanFailure) Unwrap() error { return e.Err }

// ScanFailureOf defaults to ScanErrorInternal and an empty path.
func ScanFailureOf(err error) (ScanErrorCode, string) {
	var failure *ScanFailure
	if errors.As(err, &failure) && failure.Code != "" {
		return failure.Code, failure.Path
	}
	return ScanErrorInternal, ""
}
