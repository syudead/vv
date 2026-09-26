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

	if s.files == nil {
		s.internalError(w, "メディアファイルの読み出しが設定されていません", nil)
		return
	}
	file, info, _, ok := s.openMediaFile(r, video)
	if !ok {
		s.notFound(w, "この動画の実体を開けません")
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
		// できればコピーする（plan.md Structural Decision 7）。
		Normalize:       video.Playable,
		StartupDeadline: time.Now().Add(transcodeStartupTimeout),
	}
	started, err := s.transcoder.Start(r.Context(), request)
	if err != nil {
		if errors.Is(r.Context().Err(), context.Canceled) {
			return
		}
		if errors.Is(err, domain.ErrUnprocessableMedia) {
			s.logger.Info("動画をライブ変換できません", slog.Int64("video", video.ID), slog.Any("error", err))
			s.writeError(w, http.StatusConflict, codeConflict, "この動画をライブ変換できません")
			return
		}
		s.internalError(w, "ライブ変換を開始できませんでした", err)
		return
	}
	// 実際の開始位置はまだ記録だけする（プレイヤーへの報告は次の実装単位）。
	s.logger.Debug("ライブ変換を開始しました",
		slog.Int64("video", video.ID), slog.Int64("requested_ms", startMs), slog.Int64("start_ms", started.StartMs))
	stream, wait, stop := started.Stream, started.Wait, started.Stop
	defer func() { _ = stream.Close() }()
	first, err := awaitInitialTranscodeData(r.Context(), stream, wait, stop)
	if err != nil {
		if errors.Is(r.Context().Err(), context.Canceled) {
			return
		}
		s.internalError(w, "ライブ変換が初期データを生成できませんでした", err)
		return
	}
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
		s.logger.Warn("ライブ変換streamが途中で終了しました",
			slog.Int64("video", video.ID), slog.Any("copy_error", copyErr), slog.Any("process_error", waitErr))
	}
}

// usableTranscodeProbe は保存済みの解析情報のうち、開いたファイルの変換に使って
// よいものを返す。無い・合わない・読めないときは nil で、変換がその場で解析する。
func (s *server) usableTranscodeProbe(ctx context.Context, videoID int64, source domain.FileStamp) *domain.TranscodeProbe {
	stored, err := s.videos.TranscodeProbe(ctx, videoID)
	if err != nil {
		s.logger.Warn("保存済みの解析情報を読めません。その場で解析します",
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
		s.logger.Warn("ライブ変換用の解析情報を保存できません",
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
