package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Start owns the probe/first-byte deadline; handlers must not reapply it after Start returns buffered output.
var transcodeStartupTimeout = 6 * time.Second

// Probe persistence has an independent timeout longer than SQLite's five-second busy timeout.
const transcodeProbeSaveTimeout = 10 * time.Second

// TranscodeVideo streams one request-scoped fragmented MP4 process.
func (s *server) TranscodeVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.TranscodeVideoParams) {
	// Register guest transcodes for cancellation when visibility becomes private.
	video, r, release, ok := s.lookupServedVideo(w, r, id)
	if !ok {
		return
	}
	defer release()
	// Register attempts at request start and publish actual offsets before streaming; failure before publication yields 404 to offset readers.
	quality, ok := s.transcodeQuality(w, video, params.Quality)
	if !ok {
		return
	}
	resolveStart := func(int64) {}
	if params.Attempt != nil {
		if !validTranscodeAttempt(*params.Attempt) {
			s.invalidRequest(w, "Invalid attempt.")
			return
		}
		resolve, end := s.transcodeStarts.begin(transcodeStartKey{videoID: video.ID, attempt: *params.Attempt})
		defer end()
		resolveStart = resolve
	}
	if video.ProbeState != domain.ProbeStateDone || video.DurationMs == nil || *video.DurationMs <= 0 {
		s.conflictReason(w, reasonProbeInfoMissing, "This video lacks the media information needed for live transcoding.")
		return
	}

	startMs := int64(0)
	if params.StartMs != nil {
		startMs = *params.StartMs
	}
	if startMs < 0 || startMs >= *video.DurationMs {
		s.invalidRequest(w, "startMs must be within the video's duration.")
		return
	}
	if s.transcoder == nil {
		s.internalError(w, "Live transcoding is not configured.", nil)
		return
	}

	if s.files == nil {
		s.internalError(w, "Media file access is not configured.", nil)
		return
	}
	file, info, _, ok := s.openMediaFile(r, video)
	if !ok {
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
		return
	}
	defer func() { _ = file.Close() }()

	// Compare the stamp of the actually opened location, not another location with the same content key.
	source := domain.FileStampOf(info)
	request := domain.LiveTranscodeRequest{
		Path:            file.Name(),
		Source:          source,
		Probe:           s.usableTranscodeProbe(r.Context(), video.ID, source),
		StartMs:         startMs,
		SparseKeyframes: s.sparseKeyframes(r.Context(), video.ID, source),
		// Switching from direct playback forces encoding; seeking alone follows sparse-keyframe and stream-copy rules.
		Normalize:       video.Playable,
		StartupDeadline: time.Now().Add(transcodeStartupTimeout),
		// Encoder changes affect only future requests; running streams retain their selected encoder.
		VideoEncoder: s.effectiveVideoEncoder(),
		// A requested quality forces encoding and therefore starts exactly at startMs.
		Quality: quality,
	}
	started, err := s.transcoder.Start(r.Context(), request)
	if err != nil {
		if errors.Is(r.Context().Err(), context.Canceled) {
			return
		}
		if errors.Is(err, domain.ErrUnprocessableMedia) {
			s.logger.Info("video cannot be transcoded live", slog.Int64("video", video.ID), slog.Any("error", err))
			s.conflictReason(w, reasonTranscodeUnavailable, "This video cannot be transcoded live.")
			return
		}
		s.internalError(w, "Could not start live transcoding.", err)
		return
	}
	if started.HardwareFailure != nil {
		// Hardware fallback errors include FFmpeg's stderr tail.
		s.logger.Warn("hardware video encoder failed; live transcoding fell back to software",
			slog.Int64("video", video.ID),
			slog.String("encoder", string(request.VideoEncoder)),
			slog.Any("error", started.HardwareFailure))
	}
	s.logger.Debug("live transcoding started",
		slog.Int64("video", video.ID), slog.Int64("requested_ms", startMs), slog.Int64("start_ms", started.StartMs),
		slog.String("encoder", string(started.VideoEncoder)), slog.String("quality", qualityLogValue(quality)))
	stream, wait, stop := started.Stream, started.Wait, started.Stop
	defer func() { _ = stream.Close() }()
	first, err := awaitInitialTranscodeData(r.Context(), stream, wait, stop)
	if err != nil {
		if errors.Is(r.Context().Err(), context.Canceled) {
			return
		}
		s.internalError(w, "Live transcoding produced no initial data.", err)
		return
	}
	resolveStart(started.StartMs)
	if started.Probed != nil {
		// Persist probe data alongside streaming under an independent cancellation-free deadline; wait for persistence before returning.
		saved := make(chan struct{})
		saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(r.Context()), transcodeProbeSaveTimeout)
		go func(ctx context.Context, probe domain.TranscodeProbe) {
			defer close(saved)
			s.saveTranscodeProbe(ctx, video.ID, source, probe)
		}(saveCtx, *started.Probed)
		defer func() {
			<-saved
			cancelSave()
		}()
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", cacheNoStore)
	w.Header().Del("Accept-Ranges")
	w.Header().Del("Content-Length")
	w.WriteHeader(http.StatusOK)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	written, copyErr := w.Write(first)
	if copyErr == nil && written != len(first) {
		copyErr = io.ErrShortWrite
	}
	if copyErr == nil {
		_, copyErr = io.Copy(w, stream)
	}
	if copyErr != nil {
		stop()
	}
	waitErr := wait()
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	if copyErr != nil || waitErr != nil {
		s.logger.Warn("live transcoding stream ended early",
			slog.Int64("video", video.ID), slog.Any("copy_error", copyErr), slog.Any("process_error", waitErr))
	}
}

// Reject unknown/unavailable quality before starting a transcode; domain owns availability rules.
func (s *server) transcodeQuality(w http.ResponseWriter, video domain.Video, param *gen.TranscodeVideoParamsQuality) (domain.TranscodeQuality, bool) {
	if param == nil {
		return "", true
	}
	quality, known := domain.ParseTranscodeQuality(string(*param))
	if !known {
		s.invalidRequest(w, "quality must be one of 1080p, 720p, 480p or 360p.")
		return "", false
	}
	width, height := 0, 0
	if video.Width != nil {
		width = *video.Width
	}
	if video.Height != nil {
		height = *video.Height
	}
	if !quality.Available(width, height) {
		s.invalidRequest(w, "quality must be smaller than the video's resolution.")
		return "", false
	}
	return quality, true
}

func qualityLogValue(quality domain.TranscodeQuality) string {
	if quality == "" {
		return "original"
	}
	return string(quality)
}

func (s *server) effectiveVideoEncoder() domain.VideoEncoder {
	if s.transcodeSettings == nil {
		return domain.VideoEncoderSoftware
	}
	return s.transcodeSettings.Current().Effective
}

func (s *server) usableTranscodeProbe(ctx context.Context, videoID int64, source domain.FileStamp) *domain.TranscodeProbe {
	stored, err := s.videos.TranscodeProbe(ctx, videoID)
	if err != nil {
		s.logger.Warn("cannot read the stored media information; probing on the fly",
			slog.Int64("video", videoID), slog.Any("error", err))
		return nil
	}
	probe, ok := domain.TranscodeProbeUsable(stored, source)
	if !ok {
		return nil
	}
	return &probe
}

// Persistence failures do not interrupt transcoding; later requests probe again.
func (s *server) saveTranscodeProbe(ctx context.Context, videoID int64, source domain.FileStamp, probe domain.TranscodeProbe) {
	if s.transcodeProbes == nil {
		return
	}
	if err := s.transcodeProbes.SaveTranscodeProbe(ctx, videoID, source, probe); err != nil {
		s.logger.Warn("cannot save the media information for live transcoding",
			slog.Int64("video", videoID), slog.Any("error", err))
	}
}

type initialTranscodeRead struct {
	data []byte
	err  error
}

// Read Start's buffered output with request cancellation only; reapplying the startup deadline could discard a successful near-deadline start.
func readInitialTranscodeData(ctx context.Context, stream io.Reader) ([]byte, error) {
	result := make(chan initialTranscodeRead, 1)
	go func() {
		buffer := make([]byte, 32*1024)
		length, err := io.ReadAtLeast(stream, buffer, 1)
		result <- initialTranscodeRead{data: buffer[:length], err: err}
	}()

	select {
	case read := <-result:
		return read.data, read.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func awaitInitialTranscodeData(
	ctx context.Context,
	stream io.ReadCloser,
	wait func() error,
	stop func(),
) ([]byte, error) {
	data, readErr := readInitialTranscodeData(ctx, stream)
	if len(data) > 0 {
		return data, nil
	}
	stop()
	closeErr := stream.Close()
	waitErr := wait()
	return nil, errors.Join(readErr, closeErr, waitErr)
}

func (s *server) sparseKeyframes(ctx context.Context, videoID int64, source domain.FileStamp) bool {
	probe, err := s.videos.KeyframeSpacing(ctx, videoID)
	return err == nil && probe.SparseFor(source)
}
