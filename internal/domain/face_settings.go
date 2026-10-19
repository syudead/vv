package domain

import (
	"errors"
	"fmt"
)

// The face runtime and its settings are described in docs/design-docs/face-recognition.md.

var (
	// ErrInvalidFaceSettings rejects an unknown recognizer.
	ErrInvalidFaceSettings = errors.New("invalid face recognition settings")
	// ErrFaceRecognitionOff refuses work that needs the feature on.
	ErrFaceRecognitionOff = errors.New("face recognition is turned off")
	// ErrFaceRecognitionUnsupported reports that no face runtime is published for this platform.
	ErrFaceRecognitionUnsupported = errors.New("no face runtime is published for this platform")
)

// FaceSettings are the owner's choices; the generation is kept by the store.
type FaceSettings struct {
	Enabled    bool
	Recognizer FaceRecognizer
}

// DefaultFaceSettings is what an owner who never saved the settings has.
func DefaultFaceSettings() FaceSettings {
	return FaceSettings{Recognizer: DefaultFaceRecognizer}
}

// ValidateFaceSettings accepts only a recognizer in the table.
func ValidateFaceSettings(s FaceSettings) (FaceSettings, error) {
	if !s.Recognizer.Valid() {
		return FaceSettings{}, fmt.Errorf("%w: unknown recognizer %q", ErrInvalidFaceSettings, s.Recognizer)
	}
	return s, nil
}

// FaceRuntimeState is what the files under the data folder and the running download say about the runtime.
type FaceRuntimeState string

const (
	FaceRuntimeUnsupported FaceRuntimeState = "unsupported"
	FaceRuntimeAbsent      FaceRuntimeState = "absent"
	FaceRuntimeDownloading FaceRuntimeState = "downloading"
	FaceRuntimeReady       FaceRuntimeState = "ready"
	FaceRuntimeFailed      FaceRuntimeState = "failed"
)

// FaceRuntimeFailure says why the runtime is failed.
type FaceRuntimeFailure string

const (
	FaceRuntimeNetwork      FaceRuntimeFailure = "network"
	FaceRuntimeHashMismatch FaceRuntimeFailure = "hash_mismatch"
	FaceRuntimeDisk         FaceRuntimeFailure = "disk"
	FaceRuntimeHelperStart  FaceRuntimeFailure = "helper_start"
)

// FaceRuntimeStatus is the runtime and model files of one recognizer.
type FaceRuntimeStatus struct {
	State FaceRuntimeState
	// BytesDone and BytesTotal are set while downloading.
	BytesDone, BytesTotal int64
	// Reason is set when failed.
	Reason FaceRuntimeFailure
}

// FaceRecognizerDownload describes what choosing a recognizer downloads.
type FaceRecognizerDownload struct {
	Recognizer FaceRecognizer
	// DownloadBytes is the recognizer model's size.
	DownloadBytes int64
	// License is an SPDX id, or "non-commercial".
	License       string
	NonCommercial bool
}

// QueueCounts are the rows of a per-content-key queue by state.
type QueueCounts struct {
	Queued  int
	Running int
	Done    int
	Failed  int
	// LastError is the latest failure reason in English, or empty.
	LastError string
}

// FaceRecognitionStatus is everything the face recognition settings show.
type FaceRecognitionStatus struct {
	Settings    FaceSettings
	Runtime     FaceRuntimeStatus
	Recognizers []FaceRecognizerDownload
	Counts      QueueCounts
}
