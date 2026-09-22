package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const transcodeStartupTimeout = 4 * time.Second

// TranscodeVideo streams one request-scoped fragmented MP4 process.
func (s *server) TranscodeVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.TranscodeVideoParams) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if video.ProbeState != domain.ProbeStateDone || video.DurationMs == nil || *video.DurationMs <= 0 {
		s.writeError(w, http.StatusConflict, codeConflict, "この動画はライブ変換に必要な解析情報がありません")
		return
	}

	startMs := int64(0)
	if params.StartMs != nil {
		startMs = *params.StartMs
	}
	if startMs < 0 || startMs >= *video.DurationMs {
		s.invalidRequest(w, "startMsは動画の範囲内で指定してください")
		return
	}
	if s.transcoder == nil {
		s.internalError(w, "ライブ変換が設定されていません", nil)
		return
	}

	file, _, _, ok := s.openMediaFile(r, video)
	if !ok {
		s.notFound(w, "この動画の実体を開けません")
		return
	}
	defer func() { _ = file.Close() }()

	startupDeadline := time.Now().Add(transcodeStartupTimeout)
	stream, wait, stop, err := s.transcoder.Start(
		r.Context(), file.Name(), startMs, video.Playable || startMs > 0, startupDeadline,
	)
	if err != nil {
		if errors.Is(err, domain.ErrUnprocessableMedia) {
			s.logger.Info("動画をライブ変換できません", slog.Int64("video", video.ID), slog.Any("error", err))
			s.writeError(w, http.StatusConflict, codeConflict, "この動画をライブ変換できません")
			return
		}
		s.internalError(w, "ライブ変換を開始できませんでした", err)
		return
	}
	defer func() { _ = stream.Close() }()
	first, err := awaitInitialTranscodeData(r.Context(), stream, wait, stop, time.Until(startupDeadline))
	if err != nil {
		if errors.Is(r.Context().Err(), context.Canceled) {
			return
		}
		s.internalError(w, "ライブ変換が初期データを生成できませんでした", err)
		return
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
		s.logger.Warn("ライブ変換streamが途中で終了しました",
			slog.Int64("video", video.ID), slog.Any("copy_error", copyErr), slog.Any("process_error", waitErr))
	}
}

type initialTranscodeRead struct {
	data []byte
	err  error
}

func readInitialTranscodeData(ctx context.Context, stream io.Reader, timeout time.Duration) ([]byte, error) {
	result := make(chan initialTranscodeRead, 1)
	go func() {
		buffer := make([]byte, 32*1024)
		length, err := io.ReadAtLeast(stream, buffer, 1)
		result <- initialTranscodeRead{data: buffer[:length], err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case read := <-result:
		return read.data, read.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("初期データ待機が%sでタイムアウトしました", timeout)
	}
}

func awaitInitialTranscodeData(
	ctx context.Context,
	stream io.ReadCloser,
	wait func() error,
	stop func(),
	timeout time.Duration,
) ([]byte, error) {
	data, readErr := readInitialTranscodeData(ctx, stream, timeout)
	if len(data) > 0 {
		return data, nil
	}
	stop()
	closeErr := stream.Close()
	waitErr := wait()
	return nil, errors.Join(readErr, closeErr, waitErr)
}
