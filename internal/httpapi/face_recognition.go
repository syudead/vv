package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// FaceRecognition is the face recognition settings and runtime download (docs/design-docs/face-recognition.md).
type FaceRecognition interface {
	Status(ctx context.Context) (domain.FaceRecognitionStatus, error)
	SaveSettings(ctx context.Context, settings domain.FaceSettings) (domain.FaceRecognitionStatus, error)
	RetryDownload(ctx context.Context) (domain.FaceRecognitionStatus, error)
}

// FaceAnalysis queues library videos for face analysis.
type FaceAnalysis interface {
	QueueLibrary(ctx context.Context, scope domain.FaceQueueScope) (int, error)
}

const faceRecognitionMissing = "Face recognition is not configured."

func (s *server) GetFaceRecognitionSettings(w http.ResponseWriter, r *http.Request) {
	if s.faceRecognition == nil {
		s.internalError(w, faceRecognitionMissing, nil)
		return
	}
	status, err := s.faceRecognition.Status(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the face recognition settings.", err)
		return
	}
	s.writeFaceRecognition(w, http.StatusOK, status)
}

func (s *server) UpdateFaceRecognitionSettings(w http.ResponseWriter, r *http.Request) {
	if s.faceRecognition == nil {
		s.internalError(w, faceRecognitionMissing, nil)
		return
	}
	var body struct {
		Enabled    *bool   `json:"enabled"`
		Recognizer *string `json:"recognizer"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Enabled == nil || body.Recognizer == nil {
		s.invalidRequest(w, "enabled and recognizer are required.")
		return
	}
	status, err := s.faceRecognition.SaveSettings(r.Context(), domain.FaceSettings{
		Enabled: *body.Enabled, Recognizer: domain.FaceRecognizer(*body.Recognizer),
	})
	if errors.Is(err, domain.ErrInvalidFaceSettings) {
		s.invalidRequest(w, err.Error())
		return
	}
	if err != nil {
		s.internalError(w, "Could not save the face recognition settings.", err)
		return
	}
	s.writeFaceRecognition(w, http.StatusOK, status)
}

func (s *server) DownloadFaceRuntime(w http.ResponseWriter, r *http.Request) {
	if s.faceRecognition == nil {
		s.internalError(w, faceRecognitionMissing, nil)
		return
	}
	status, err := s.faceRecognition.RetryDownload(r.Context())
	switch {
	case errors.Is(err, domain.ErrFaceRecognitionOff):
		s.conflictReason(w, gen.ErrorReasonFaceRecognitionOff, "Face recognition is turned off.")
	case errors.Is(err, domain.ErrFaceRecognitionUnsupported):
		s.conflictReason(w, gen.ErrorReasonFaceRecognitionUnsupported, "No face runtime is published for this platform.")
	case err != nil:
		s.internalError(w, "Could not start the face runtime download.", err)
	default:
		s.writeFaceRecognition(w, http.StatusAccepted, status)
	}
}

func (s *server) StartFaceRecognition(w http.ResponseWriter, r *http.Request) {
	if s.faceAnalysis == nil {
		s.internalError(w, "Face analysis is not configured.", nil)
		return
	}
	var body struct {
		Scope *string `json:"scope"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Scope == nil || !domain.FaceQueueScope(*body.Scope).Valid() {
		s.invalidRequest(w, "scope must be missing or all.")
		return
	}
	queued, err := s.faceAnalysis.QueueLibrary(r.Context(), domain.FaceQueueScope(*body.Scope))
	if errors.Is(err, domain.ErrFaceRecognitionOff) {
		s.conflictReason(w, gen.ErrorReasonFaceRecognitionOff, "Face recognition is turned off.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not queue videos for face analysis.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, gen.FaceRecognitionQueued{Queued: queued}, s.logger)
}

func (s *server) writeFaceRecognition(w http.ResponseWriter, code int, status domain.FaceRecognitionStatus) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, code, toAPIFaceRecognition(status), s.logger)
}

func toAPIFaceRecognition(status domain.FaceRecognitionStatus) gen.FaceRecognitionSettings {
	out := gen.FaceRecognitionSettings{
		Enabled:     status.Settings.Enabled,
		Recognizer:  gen.FaceRecognizer(status.Settings.Recognizer),
		Recognizers: make([]gen.FaceRecognizerOption, len(status.Recognizers)),
		Runtime:     toAPIFaceRuntime(status.Runtime),
		Counts: gen.FaceRecognitionCounts{
			Queued: status.Counts.Queued, Running: status.Counts.Running,
			Done: status.Counts.Done, Failed: status.Counts.Failed,
		},
	}
	for i, r := range status.Recognizers {
		out.Recognizers[i] = gen.FaceRecognizerOption{
			Id: gen.FaceRecognizer(r.Recognizer), DownloadBytes: r.DownloadBytes,
			License: r.License, NonCommercial: r.NonCommercial,
		}
	}
	if status.Counts.LastError != "" {
		lastFailure := status.Counts.LastError
		out.LastFailure = &lastFailure
	}
	return out
}

func toAPIFaceRuntime(rt domain.FaceRuntimeStatus) gen.FaceRuntime {
	out := gen.FaceRuntime{State: gen.FaceRuntimeState(rt.State)}
	switch rt.State {
	case domain.FaceRuntimeDownloading:
		done, total := rt.BytesDone, rt.BytesTotal
		out.BytesDone, out.BytesTotal = &done, &total
	case domain.FaceRuntimeFailed:
		reason := gen.FaceRuntimeReason(rt.Reason)
		out.Reason = &reason
	}
	return out
}
