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

// transcodeStartupTimeout はその場の解析と最初のデータまでの期限で、Transcoder.Start
// の中だけで使う。Start は最初のデータを持って戻るので、経路は同じ期限をかけ直さない。
// テストが短くできるよう変数にしている。
var transcodeStartupTimeout = 6 * time.Second

// transcodeProbeSaveTimeout はその場で解析した結果の保存にかける期限である。
// SQLite の書き込み待ち（busy_timeout 5 秒）より長く、配信の期限とは独立している。
const transcodeProbeSaveTimeout = 10 * time.Second

// TranscodeVideo streams one request-scoped fragmented MP4 process.
func (s *server) TranscodeVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.TranscodeVideoParams) {
	// ゲストとして処理する要求は、非公開にされたら打ち切れるよう台帳に載せる
	// （contracts/guest-api.md §6）。
	video, r, release, ok := s.lookupServedVideo(w, r, id)
	if !ok {
		return
	}
	defer release()
	// attempt 付きの要求は始めた時点で台帳に載せ、最初のデータを持って本文を書き始める
	// 前に実際の開始位置を記録する。記録の前に終われば、報告の経路は 404 を返す
	// （contracts/transcode-start-api.md §1）。
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

	// 比べるのは変換で実際に開いた所在の印である。同じ content_key の別の所在が
	// あっても、開いた所在で決める（specs/018-live-transcode-seek/data-model.md §3）。
	source := domain.FileStampOf(info)
	request := domain.LiveTranscodeRequest{
		Path:    file.Name(),
		Source:  source,
		Probe:   s.usableTranscodeProbe(r.Context(), video.ID, source),
		StartMs: startMs,
		// 直接再生から切り替えた変換だけエンコードを強いる。シークでは映像がコピー
		// できればコピーする。
		Normalize:       video.Playable,
		StartupDeadline: time.Now().Add(transcodeStartupTimeout),
		// 方式は要求ごとに今の設定から決める。変更は次に始まる要求から効き、配信中の変換は
		// 始めた方式のまま続く（specs/025-hardware-encoding/contracts/transcoding-settings-api.md §4）。
		VideoEncoder: s.effectiveVideoEncoder(),
		// 画質があれば映像を必ずエンコードするので、変換は startMs の位置そのものから始まる
		// （specs/027-playback-quality/contracts/transcode-quality-api.md §1）。
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
		// ハードウェアが最初のデータを出さずに終わり、同じ要求の中で software に切り替えた
		// （research.md R-6）。誤りには FFmpeg の標準エラーの末尾が入っている。
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
		// 保存は配信と並べて行い、書き込みの待ちで初期データの期限を使わない。
		// 解析は終わっているので、要求が取り消されても独立した期限の中で保存は済ませ、
		// 経路はその保存を待ってから戻る。
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

// transcodeQuality は quality の指定を解釈する。無ければ元の画質（空）である。知らない値、
// 動画の表示の短辺以上の画質、寸法の無い動画は 400 を書いて ok を偽にし、変換を始めない。
// 使えるかの規則は domain が持つ（specs/027-playback-quality/research.md R-3）。
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

// qualityLogValue はログに添える画質で、元の画質は "original" と書く。
func qualityLogValue(quality domain.TranscodeQuality) string {
	if quality == "" {
		return "original"
	}
	return string(quality)
}

// effectiveVideoEncoder は今の設定が使う方式を返す。設定が無ければ software である。
func (s *server) effectiveVideoEncoder() domain.VideoEncoder {
	if s.transcodeSettings == nil {
		return domain.VideoEncoderSoftware
	}
	return s.transcodeSettings.Current().Effective
}

// usableTranscodeProbe は保存済みの解析情報のうち、開いたファイルの変換に使って
// よいものを返す。無い・合わない・読めないときは nil で、変換がその場で解析する。
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

// saveTranscodeProbe はその場で解析した結果を保存する。保存できなくても変換は続け、
// 次の変換がまた解析する。
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

// readInitialTranscodeData は Start が読んでおいた最初のデータを取り出す。開始の期限は
// Start の中で済んでいるので、ここでは要求の取り消しだけで打ち切る。期限をかけ直すと、
// 期限の間際に始まった変換が、データを持っているのに期限切れで止められる。
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
