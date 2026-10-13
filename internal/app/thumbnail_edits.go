package app

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// サムネイルの編集のセッション（specs/045-comfyui-thumbnails/data-model.md の Edit session、
// research.md R-4・R-8・R-10・R-11、docs/design-docs/thumbnail-editing.md）。動画ごとに 1 つを
// メモリに持ち、画面が読むたびに提供元の状態を問い合わせる。イベントは出さない。

// thumbnailEditSweepInterval は、読まれないセッションと時間切れの実行を探す間隔である。
const thumbnailEditSweepInterval = 30 * time.Second

// thumbnailEditCloseTimeout は、止めるときに進んでいる実行を中止して回る時間の上限である。
const thumbnailEditCloseTimeout = 10 * time.Second

// thumbnailEditProviderTimeout は、状態の読み取り・中止・出力の取得の 1 回の上限である。
const thumbnailEditProviderTimeout = 30 * time.Second

// thumbnailEditCancelRetryLimit は、提供元が中止を受け付けなかった実行の中止を、片付けのたびに
// やり直し続ける時間の上限である。
const thumbnailEditCancelRetryLimit = time.Hour

// ComfyUISettingsReader は ComfyUI の接続先を読む。internal/store の *SettingsStore が満たす。
type ComfyUISettingsReader interface {
	ComfyUISettings(ctx context.Context) (domain.ComfyUISettings, error)
}

// ThumbnailFrameSource は動画のファイル path の positionMs の場面を、長辺を動画の長辺
// （上限 domain.ThumbnailFrameMaxSide）にした PNG で取り出す（research.md R-8）。
// internal/media の *Assets が満たす。
type ThumbnailFrameSource interface {
	FrameAt(ctx context.Context, path string, positionMs int64) (domain.ThumbnailFrame, error)
	// AutoThumbnailPositions は、位置を指定していない動画のサムネイルジョブがコマを取りに行く位置
	// （ミリ秒）を、尺 durationMs（0 は不明）に対して試す順に返す（自動の位置、次に先頭。
	// specs/046-thumbnail-image-upload/research.md R-4）。
	AutoThumbnailPositions(durationMs int64) []int64
}

// ThumbnailImageReader は内容の今の代表サムネイルの選び方（適用した画像の元の画像、元の画像を
// 持たない古い行では代表サムネイル、と指定した位置）を、同じ時点のものとして読む
// （specs/046-thumbnail-image-upload/research.md R-4）。internal/store の *OverrideStore が満たす。
type ThumbnailImageReader interface {
	CurrentThumbnailSelection(ctx context.Context, contentKey string) (domain.CurrentThumbnailSelection, error)
}

// ThumbnailImageApplier は出どころ origin の画像を動画のサムネイルとして適用する。*Ingest が満たす。
// 動画が消えたか別の内容を持てば domain.ErrContentChanged、画像を読めなければ
// domain.ErrThumbnailImageInvalid を返す。
type ThumbnailImageApplier interface {
	ApplyThumbnailImage(
		ctx context.Context, videoID int64, contentKey string, image []byte, origin domain.ThumbnailImageOrigin,
	) (domain.Video, error)
}

// ThumbnailEditsOptions は ThumbnailEdits を組み立てる部品である。
type ThumbnailEditsOptions struct {
	Settings ComfyUISettingsReader
	Editor   ImageEditor
	Frames   ThumbnailFrameSource
	Images   ThumbnailImageReader
	Applier  ThumbnailImageApplier
	Logger   *slog.Logger
	// Now は今の時刻を返す。nil なら time.Now。テストが進める。
	Now func() time.Time
}

// ThumbnailEdits は動画ごとに 1 つの編集のセッションをメモリに持つ。再起動ですべて消える。
type ThumbnailEdits struct {
	settings ComfyUISettingsReader
	editor   ImageEditor
	frames   ThumbnailFrameSource
	images   ThumbnailImageReader
	applier  ThumbnailImageApplier
	logger   *slog.Logger
	now      func() time.Time

	// mu は byID と byVideo を守る。セッションの mu を持ったまま取ってよいが、逆はしない。
	mu      sync.Mutex
	byID    map[string]*thumbnailEditSession
	byVideo map[int64]string

	// pendingMu は pending を守る。持ったまま他の錠を取らず、提供元にも問い合わせない。
	pendingMu sync.Mutex
	// pending は提供元が中止を受け付けなかった実行である。実行の id ごとに持ち、中止が済むまで
	// 片付けのたびにやり直す。セッションが終わっても残る。
	pending map[string]*thumbnailEditPendingCancel
}

// thumbnailEditPendingCancel は中止をやり直す実行である。
type thumbnailEditPendingCancel struct {
	videoID  int64
	job      domain.ImageEditJob
	state    domain.ThumbnailEditState
	failedAt time.Time
	// attempt は、ある呼び出しが中止をやり直している間だけ nil でなく、やり直しが終わると閉じる。
	// pendingMu が守る。
	attempt chan struct{}
}

// thumbnailEditSession は 1 つの編集のセッションである。mu が下の値を守る。
type thumbnailEditSession struct {
	mu         sync.Mutex
	id         string
	videoID    int64
	contentKey string
	frame      domain.ThumbnailFrame
	// source はセッションを何から始めたか、positionMs は frame がコマのときにそれを取った位置
	// （保存した元の画像なら nil）である。
	source     domain.ThumbnailEditSource
	positionMs *int64
	ended      bool
	lastReadAt time.Time
	run        *thumbnailEditRun
	result     []byte
}

// thumbnailEditRun はセッションの今の実行である。送るのに失敗した実行は job を持たず、failed である。
type thumbnailEditRun struct {
	job         domain.ImageEditJob
	submitted   bool
	state       domain.ThumbnailEditState
	submittedAt time.Time
	endedAt     time.Time
	progress    *domain.ImageEditProgress
	failure     *domain.ImageEditError
}

// inProgress は提供元で実行が進んでいるかを返す。
func (r *thumbnailEditRun) inProgress() bool {
	return r != nil && r.submitted && !r.state.Terminal()
}

// NewThumbnailEdits は編集のセッションの置き場を組み立てる。
func NewThumbnailEdits(opts ThumbnailEditsOptions) *ThumbnailEdits {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &ThumbnailEdits{
		settings: opts.Settings,
		editor:   opts.Editor,
		frames:   opts.Frames,
		images:   opts.Images,
		applier:  opts.Applier,
		logger:   logger,
		now:      now,
		byID:     map[string]*thumbnailEditSession{},
		byVideo:  map[int64]string{},
		pending:  map[string]*thumbnailEditPendingCancel{},
	}
}

// Start は動画 video の編集を、要求 start の画像から ready のセッションとして始める。画像は
// 始めた時点のものをセッションが持ち続ける（specs/046-thumbnail-image-upload/research.md R-4）。
// 同じ動画のセッションがあれば、その実行を提供元で中止してから終える（R-11）。接続先が保存されて
// いなければ domain.ErrThumbnailEditNotReady、内容を読めていない動画は domain.ErrNotFound、
// コマの位置は domain.CheckThumbnailPosition の誤り（位置が無ければ
// domain.ErrThumbnailPositionOutOfRange）、画像を用意できなければ
// domain.ErrThumbnailFrameUnavailable を返す。path は動画のファイルで、開けなかった動画では空で
// ある。空のパスでコマが要るとき（今のサムネイルが適用した画像でないとき）は
// domain.ErrMediaFileUnavailable を返す。
func (e *ThumbnailEdits) Start(
	ctx context.Context, video domain.Video, path string, start domain.ThumbnailEditStart,
) (domain.ThumbnailEdit, error) {
	conn, err := e.settings.ComfyUISettings(ctx)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	if !conn.Saved() {
		return domain.ThumbnailEdit{}, domain.ErrThumbnailEditNotReady
	}
	if video.ContentKey == "" {
		return domain.ThumbnailEdit{}, domain.ErrNotFound
	}
	image, err := e.startImage(ctx, video, path, start)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	id, err := newThumbnailEditID()
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	session := &thumbnailEditSession{
		id: id, videoID: video.ID, contentKey: video.ContentKey, frame: image.frame,
		source: start.Source, positionMs: image.positionMs, lastReadAt: e.now(),
	}
	view := e.view(session)
	e.replace(ctx, session)
	return view, nil
}

// thumbnailEditImage はセッションが始める画像と、それがコマならそれを取った位置である。
type thumbnailEditImage struct {
	frame      domain.ThumbnailFrame
	positionMs *int64
}

// startImage は要求 start が指す画像を用意する。
func (e *ThumbnailEdits) startImage(
	ctx context.Context, video domain.Video, path string, start domain.ThumbnailEditStart,
) (thumbnailEditImage, error) {
	switch start.Source {
	case domain.ThumbnailEditFromFrame:
		if start.PositionMs == nil {
			return thumbnailEditImage{}, domain.ErrThumbnailPositionOutOfRange
		}
		if err := domain.CheckThumbnailPosition(video, *start.PositionMs); err != nil {
			return thumbnailEditImage{}, err
		}
		return e.frameAt(ctx, path, *start.PositionMs)
	case domain.ThumbnailEditFromThumbnail:
		return e.currentThumbnail(ctx, video, path)
	default:
		return thumbnailEditImage{}, fmt.Errorf("unknown thumbnail edit source %q", start.Source)
	}
}

// currentThumbnail は動画 video の今のサムネイルの画像を用意する（research.md R-4）。画像を
// 適用していればその元の画像（無い古い行では代表サムネイル）を PNG にしたもの、なければ指定した
// 位置のコマ、それも無ければサムネイルジョブと同じ規則（自動の位置、次に先頭）のコマである。
// 画像と位置は video ではなく保存先から同じ時点のものとして読み、video を読んだあとの変更と
// 混ざらないようにする。
func (e *ThumbnailEdits) currentThumbnail(
	ctx context.Context, video domain.Video, path string,
) (thumbnailEditImage, error) {
	selection, err := e.images.CurrentThumbnailSelection(ctx, video.ContentKey)
	if err != nil {
		return thumbnailEditImage{}, err
	}
	if selection.Image != nil {
		frame, err := thumbnailEditFrameFromImage(selection.Image)
		if err != nil {
			return thumbnailEditImage{}, errors.Join(domain.ErrThumbnailFrameUnavailable, err)
		}
		return thumbnailEditImage{frame: frame}, nil
	}
	if path == "" {
		return thumbnailEditImage{}, domain.ErrMediaFileUnavailable
	}
	if selection.PositionMs != nil {
		return e.frameAt(ctx, path, *selection.PositionMs)
	}
	var durationMs int64
	if video.DurationMs != nil {
		durationMs = *video.DurationMs
	}
	var first error
	for _, positionMs := range e.frames.AutoThumbnailPositions(durationMs) {
		image, err := e.frameAt(ctx, path, positionMs)
		if err == nil {
			return image, nil
		}
		first = cmp.Or(first, err)
	}
	return thumbnailEditImage{}, cmp.Or(first, error(domain.ErrThumbnailFrameUnavailable))
}

// frameAt はファイル path の positionMs の場面のコマを取り出す。
func (e *ThumbnailEdits) frameAt(ctx context.Context, path string, positionMs int64) (thumbnailEditImage, error) {
	frame, err := e.frames.FrameAt(ctx, path, positionMs)
	if err != nil {
		return thumbnailEditImage{}, errors.Join(domain.ErrThumbnailFrameUnavailable, err)
	}
	return thumbnailEditImage{frame: frame, positionMs: &positionMs}, nil
}

// replace は同じ動画のセッションを終えてから session を置き場に載せる。前のセッションは、その錠を
// 取って end で終える（置き場から外すのは end だけなので、acquire は外れたセッションを受け付けない）。
// 前のセッションで進んでいた適用や実行の要求は、新しいセッションが見える前に終わる。錠を待つ間に
// 別の Start が載せたセッションも同じように終える。
func (e *ThumbnailEdits) replace(ctx context.Context, session *thumbnailEditSession) {
	for {
		e.mu.Lock()
		previous := e.byID[e.byVideo[session.videoID]]
		if previous == nil {
			e.byID[session.id] = session
			e.byVideo[session.videoID] = session.id
			e.mu.Unlock()
			return
		}
		e.mu.Unlock()
		previous.mu.Lock()
		if !previous.ended {
			e.end(ctx, previous)
		}
		previous.mu.Unlock()
	}
}

// Get はセッションの今の姿を返す。実行が進んでいれば提供元の状態を読み、成功していれば出力を
// 取る。読み取りはページの生存の知らせを兼ねる（R-4）。終わったセッションは
// domain.ErrThumbnailEditNotFound である。
func (e *ThumbnailEdits) Get(ctx context.Context, id string) (domain.ThumbnailEdit, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	defer session.mu.Unlock()
	e.refresh(ctx, session)
	return e.view(session), nil
}

// Frame はセッションのコマの PNG を返す。
func (e *ThumbnailEdits) Frame(ctx context.Context, id string) ([]byte, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return nil, err
	}
	defer session.mu.Unlock()
	return session.frame.PNG, nil
}

// Result は成功した実行の出力の画像を返す。無ければ domain.ErrThumbnailEditNoResult である。
func (e *ThumbnailEdits) Result(ctx context.Context, id string) ([]byte, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return nil, err
	}
	defer session.mu.Unlock()
	if !hasResult(session) {
		return nil, domain.ErrThumbnailEditNoResult
	}
	return session.result, nil
}

// StartRun は入力を確かめ、接続先の今の値をこの実行の写しにして提供元へ送る。進んでいる実行が
// あれば先に中止する（R-11）。この動画の前の実行の中止を提供元が受け付けていなければ、片付けが
// やり直している最中ならその終わりを待ってから、もう一度中止を試み、それでも済まなければ新しい実行を送らずに unreachable の failed にする。マスクと参考画像は何も上げる前に確かめ、合わなければ
// domain.ErrThumbnailEditMaskInvalid か domain.ErrThumbnailEditReferenceInvalid を返す。接続先が
// 消されていれば domain.ErrThumbnailEditNotReady である。提供元が受け付けなければセッションを
// 同じ理由の failed にし、*domain.ImageEditError を返す。
func (e *ThumbnailEdits) StartRun(ctx context.Context, id string, input domain.ThumbnailEditRunInput) (domain.ThumbnailEdit, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	defer session.mu.Unlock()
	if err := checkThumbnailEditMask(input.Mask, session.frame); err != nil {
		return domain.ThumbnailEdit{}, err
	}
	if err := checkThumbnailEditReference(input.Reference); err != nil {
		return domain.ThumbnailEdit{}, err
	}
	conn, err := e.settings.ComfyUISettings(ctx)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	if !conn.Saved() {
		return domain.ThumbnailEdit{}, domain.ErrThumbnailEditNotReady
	}
	if (session.run.inProgress() && !e.cancelRun(ctx, session)) || e.retryCancels(ctx, &session.videoID, true) > 0 {
		failure := &domain.ImageEditError{
			Reason: domain.ImageEditUnreachable, Message: "the previous run could not be cancelled at the provider",
		}
		now := e.now()
		session.result = nil
		session.run = &thumbnailEditRun{state: domain.ThumbnailEditFailed, submittedAt: now, endedAt: now, failure: failure}
		return domain.ThumbnailEdit{}, failure
	}
	session.result = nil

	job, err := e.editor.Start(ctx, conn, domain.ImageEditInput{
		Frame: session.frame.PNG, Mask: input.Mask, Reference: input.Reference, Instruction: input.Instruction,
	})
	now := e.now()
	if err != nil {
		failure := &domain.ImageEditError{Reason: domain.ImageEditRunFailed}
		if !errors.As(err, &failure) {
			e.logger.Warn("could not start a thumbnail edit run", slog.Any("error", err))
		}
		session.run = &thumbnailEditRun{state: domain.ThumbnailEditFailed, submittedAt: now, endedAt: now, failure: failure}
		return domain.ThumbnailEdit{}, err
	}
	session.run = &thumbnailEditRun{job: job, submitted: true, state: domain.ThumbnailEditQueued, submittedAt: now}
	return e.view(session), nil
}

// Cancel は進んでいる実行を提供元で中止し、cancelled にする。進んでいなければ今の姿を返す。
// サムネイルは変えない。
func (e *ThumbnailEdits) Cancel(ctx context.Context, id string) (domain.ThumbnailEdit, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	defer session.mu.Unlock()
	if session.run.inProgress() {
		e.cancelRun(ctx, session)
	}
	return e.view(session), nil
}

// Apply は成功した実行の画像をサムネイルとして適用し、セッションを終える（R-10）。succeeded で
// なければ domain.ErrThumbnailEditNoResult で、セッションは残る。動画が消えたか別の内容を持てば
// domain.ErrContentChanged でセッションを終え、画像を読めなければ domain.ErrThumbnailImageInvalid
// でセッションは残る。
func (e *ThumbnailEdits) Apply(ctx context.Context, id string) (domain.Video, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return domain.Video{}, err
	}
	defer session.mu.Unlock()
	if !hasResult(session) {
		return domain.Video{}, domain.ErrThumbnailEditNoResult
	}
	video, err := e.applier.ApplyThumbnailImage(
		ctx, session.videoID, session.contentKey, session.result, domain.ThumbnailImageGenerated)
	if errors.Is(err, domain.ErrContentChanged) {
		e.end(ctx, session)
		return domain.Video{}, err
	}
	if err != nil {
		return domain.Video{}, err
	}
	e.end(ctx, session)
	return video, nil
}

// Discard は進んでいる実行を提供元で中止し、セッションを捨てる。終わったセッションでも成功する。
func (e *ThumbnailEdits) Discard(ctx context.Context, id string) {
	e.mu.Lock()
	session := e.byID[id]
	e.mu.Unlock()
	if session == nil {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.ended {
		e.end(ctx, session)
	}
}

// Run は ctx が終わるまで、読まれないセッションと時間切れの実行を一定の間隔で片付ける。ctx が
// 終わったら、進んでいる実行を提供元で中止してすべてのセッションを終える。
func (e *ThumbnailEdits) Run(ctx context.Context) {
	ticker := time.NewTicker(thumbnailEditSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), thumbnailEditCloseTimeout)
			e.Close(closeCtx)
			cancel()
			return
		case <-ticker.C:
			e.Sweep(ctx)
		}
	}
}

// Sweep は 5 分読まれないセッションを、進んでいる実行を中止してから終え（R-4）、送ってから 10 分
// 終わらない実行を中止して timed_out で失敗させる（R-11）。提供元が中止を受け付けなかった実行の
// 中止もやり直す。
func (e *ThumbnailEdits) Sweep(ctx context.Context) {
	defer e.retryCancels(ctx, nil, false)
	for _, session := range e.sessions() {
		session.mu.Lock()
		switch {
		case session.ended:
		case e.idle(session):
			e.end(ctx, session)
		default:
			e.checkTimeLimit(ctx, session)
		}
		session.mu.Unlock()
	}
}

// Close は進んでいる実行を提供元で中止し、すべてのセッションを終える。提供元が中止を受け付けて
// いなかった実行の中止も最後に一度やり直す。
func (e *ThumbnailEdits) Close(ctx context.Context) {
	defer e.retryCancels(ctx, nil, false)
	for _, session := range e.sessions() {
		session.mu.Lock()
		if !session.ended {
			e.end(ctx, session)
		}
		session.mu.Unlock()
	}
}

// sessions は今あるセッションの写しを返す。
func (e *ThumbnailEdits) sessions() []*thumbnailEditSession {
	e.mu.Lock()
	defer e.mu.Unlock()
	sessions := make([]*thumbnailEditSession, 0, len(e.byID))
	for _, session := range e.byID {
		sessions = append(sessions, session)
	}
	return sessions
}

// acquire はセッション id の錠を取り、読んだ時刻を今にして返す。無いか、終わったか、5 分読まれて
// いなければ domain.ErrThumbnailEditNotFound を返す。読まれていなかったセッションはここで終える。
func (e *ThumbnailEdits) acquire(ctx context.Context, id string) (*thumbnailEditSession, error) {
	e.mu.Lock()
	session := e.byID[id]
	e.mu.Unlock()
	if session == nil {
		return nil, domain.ErrThumbnailEditNotFound
	}
	session.mu.Lock()
	if session.ended {
		session.mu.Unlock()
		return nil, domain.ErrThumbnailEditNotFound
	}
	if e.idle(session) {
		e.end(ctx, session)
		session.mu.Unlock()
		return nil, domain.ErrThumbnailEditNotFound
	}
	session.lastReadAt = e.now()
	return session, nil
}

// idle はセッションが 5 分読まれていないかを返す。
func (e *ThumbnailEdits) idle(session *thumbnailEditSession) bool {
	return e.now().Sub(session.lastReadAt) >= domain.ThumbnailEditIdleLimit
}

// end は進んでいる実行を提供元で中止し、セッションを置き場から外して終える。セッションの錠を
// 持って呼ぶ。
func (e *ThumbnailEdits) end(ctx context.Context, session *thumbnailEditSession) {
	if session.run.inProgress() {
		e.cancelRun(ctx, session)
	}
	session.ended = true
	session.frame = domain.ThumbnailFrame{}
	session.result = nil
	e.mu.Lock()
	if e.byID[session.id] == session {
		delete(e.byID, session.id)
	}
	if e.byVideo[session.videoID] == session.id {
		delete(e.byVideo, session.videoID)
	}
	e.mu.Unlock()
}

// cancelRun は進んでいる実行を提供元で中止し、cancelled にする。提供元が中止を受け付けなくても
// vv の側では中止したものとして扱うが、その実行は pending に残して片付けのたびに中止をやり直し、
// false を返す。
func (e *ThumbnailEdits) cancelRun(ctx context.Context, session *thumbnailEditSession) bool {
	run := session.run
	err := e.cancelJob(ctx, run.job, run.state)
	if err != nil {
		e.logger.Warn("could not cancel a thumbnail edit run; retrying on the next sweep",
			slog.Int64("video", session.videoID), slog.Any("error", err))
		e.pendingMu.Lock()
		e.pending[run.job.ID] = &thumbnailEditPendingCancel{
			videoID: session.videoID, job: run.job, state: run.state, failedAt: e.now(),
		}
		e.pendingMu.Unlock()
	}
	e.finish(run, domain.ThumbnailEditCancelled, nil)
	return err == nil
}

// cancelJob は実行 job を提供元で中止する。ページを離れた要求の取り消しに引きずられないよう、
// ctx の取り消しは引き継がず、期限（止めるときの猶予）だけを引き継ぐ。
func (e *ThumbnailEdits) cancelJob(ctx context.Context, job domain.ImageEditJob, state domain.ThumbnailEditState) error {
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), thumbnailEditProviderTimeout)
	defer cancel()
	if deadline, ok := ctx.Deadline(); ok {
		var stop context.CancelFunc
		cancelCtx, stop = context.WithDeadline(cancelCtx, deadline)
		defer stop()
	}
	return e.editor.Cancel(cancelCtx, job, state)
}

// retryCancels は提供元が中止を受け付けなかった実行の中止をやり直し、まだ済んでいない数を返す。
// videoID が nil でなければその動画の実行だけをやり直す。thumbnailEditCancelRetryLimit を過ぎた
// 実行は記録に残して諦める。別々の実行は同時にやり直し、1 つの実行のやり直しは一度に 1 つだけに
// する（Comfy Cloud の POST /interrupt はアカウントの実行中の仕事をすべて止めるので、遅れて届いた
// やり直しが、その後に送った新しい実行を止めうる）。ほかの呼び出しがやり直している実行は、wait なら
// その終わりを待ってから、まだ残っていれば自分でやり直し、wait でなければ飛ばす。待つ間に ctx が
// 終われば、その実行は済んでいないものとして数える。
func (e *ThumbnailEdits) retryCancels(ctx context.Context, videoID *int64, wait bool) int {
	tried := map[string]bool{}
	remaining := 0
	for {
		claimed, busy := e.claimCancels(videoID, tried)
		remaining += e.attemptCancels(ctx, claimed)
		if !wait || len(busy) == 0 {
			if len(claimed) == 0 {
				return remaining
			}
			continue
		}
		for _, done := range busy {
			select {
			case <-done:
			case <-ctx.Done():
				return remaining + len(busy)
			}
		}
	}
}

// claimCancels は、やり直す実行のうち、ほかの呼び出しがやり直しておらず tried に無いものを
// 自分のものにして返す。ほかの呼び出しがやり直している実行は、その終わりを知らせる channel で返す。
func (e *ThumbnailEdits) claimCancels(videoID *int64, tried map[string]bool) ([]*thumbnailEditPendingCancel, []chan struct{}) {
	e.pendingMu.Lock()
	defer e.pendingMu.Unlock()
	var claimed []*thumbnailEditPendingCancel
	var busy []chan struct{}
	for id, pending := range e.pending {
		switch {
		case videoID != nil && pending.videoID != *videoID:
		case pending.attempt != nil:
			busy = append(busy, pending.attempt)
		case !tried[id]:
			tried[id] = true
			pending.attempt = make(chan struct{})
			claimed = append(claimed, pending)
		}
	}
	return claimed, busy
}

// attemptCancels は claimed の実行の中止を同時にやり直し、まだ済んでいない数を返す。
func (e *ThumbnailEdits) attemptCancels(ctx context.Context, claimed []*thumbnailEditPendingCancel) int {
	var wg sync.WaitGroup
	var remaining atomic.Int64
	for _, pending := range claimed {
		wg.Go(func() {
			if !e.attemptCancel(ctx, pending) {
				remaining.Add(1)
			}
		})
	}
	wg.Wait()
	return int(remaining.Load())
}

// attemptCancel は claimCancels で自分のものにした実行の中止を一度やり直し、済んだか諦めたなら
// pending から外して true を返す。終わったら、待っている呼び出しに知らせる。
func (e *ThumbnailEdits) attemptCancel(ctx context.Context, pending *thumbnailEditPendingCancel) bool {
	err := e.cancelJob(ctx, pending.job, pending.state)
	resolved := true
	switch {
	case err == nil:
	case e.now().Sub(pending.failedAt) >= thumbnailEditCancelRetryLimit:
		e.logger.Warn("gave up cancelling a thumbnail edit run", slog.Int64("video", pending.videoID), slog.Any("error", err))
	default:
		e.logger.Debug("could not cancel a thumbnail edit run", slog.Int64("video", pending.videoID), slog.Any("error", err))
		resolved = false
	}
	e.pendingMu.Lock()
	if resolved {
		delete(e.pending, pending.job.ID)
	}
	close(pending.attempt)
	pending.attempt = nil
	e.pendingMu.Unlock()
	return resolved
}

// checkTimeLimit は送ってから 10 分たっても終わらない実行を中止し、timed_out で失敗させる。
// 時間切れなら true を返す。
func (e *ThumbnailEdits) checkTimeLimit(ctx context.Context, session *thumbnailEditSession) bool {
	run := session.run
	if !run.inProgress() || e.now().Sub(run.submittedAt) < domain.ThumbnailEditRunLimit {
		return false
	}
	e.cancelRun(ctx, session)
	e.finish(run, domain.ThumbnailEditFailed, &domain.ImageEditError{
		Reason: domain.ImageEditTimedOut, Message: "no result 10 minutes after the run was submitted",
	})
	return true
}

// refresh は進んでいる実行の状態を提供元から読み、成功していれば出力を取る。状態を読めないときは
// 前の状態のままにし、次の読み取りで読み直す。
func (e *ThumbnailEdits) refresh(ctx context.Context, session *thumbnailEditSession) {
	run := session.run
	if !run.inProgress() || e.checkTimeLimit(ctx, session) {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, thumbnailEditProviderTimeout)
	defer cancel()
	state, failure, err := e.editor.State(readCtx, run.job, run.state)
	if err != nil {
		e.logger.Debug("could not read a thumbnail edit run", slog.Int64("video", session.videoID), slog.Any("error", err))
		return
	}
	if progress := e.editor.Progress(run.job); progress != nil {
		run.progress = progress
	}
	switch state {
	case domain.ThumbnailEditSucceeded:
		e.fetchOutput(readCtx, session)
	case domain.ThumbnailEditFailed:
		if failure == nil {
			failure = &domain.ImageEditError{Reason: domain.ImageEditRunFailed}
		}
		e.finish(run, domain.ThumbnailEditFailed, failure)
	case domain.ThumbnailEditCancelled:
		e.finish(run, domain.ThumbnailEditCancelled, nil)
	default:
		run.state = state
	}
}

// fetchOutput は成功した実行の出力を取り、最後まで読める画像なら succeeded にする。届かなかった
// （unreachable か、理由の無い誤り）ときは前の状態のままにし、次の読み取りで取り直す。取り直しは
// 実行の 10 分の上限までである。出力が無いか読めない画像なら no_output、ほかの理由はその理由で
// failed にする。
func (e *ThumbnailEdits) fetchOutput(ctx context.Context, session *thumbnailEditSession) {
	run := session.run
	output, err := e.editor.Output(ctx, run.job)
	if err != nil {
		var failure *domain.ImageEditError
		if !errors.As(err, &failure) || failure.Reason == domain.ImageEditUnreachable {
			e.logger.Debug("could not fetch a thumbnail edit output", slog.Int64("video", session.videoID), slog.Any("error", err))
			return
		}
		e.finish(run, domain.ThumbnailEditFailed, failure)
		return
	}
	if err := checkThumbnailEditImage(output); err != nil {
		e.finish(run, domain.ThumbnailEditFailed, &domain.ImageEditError{
			Reason: domain.ImageEditNoOutput, Message: "the output image cannot be decoded",
		})
		return
	}
	session.result = output
	e.finish(run, domain.ThumbnailEditSucceeded, nil)
}

// finish は実行を終わった状態 state にする。
func (e *ThumbnailEdits) finish(run *thumbnailEditRun, state domain.ThumbnailEditState, failure *domain.ImageEditError) {
	run.state = state
	run.failure = failure
	run.endedAt = e.now()
}

// view はセッションの今の姿を返す。セッションの錠を持って呼ぶ。
func (e *ThumbnailEdits) view(session *thumbnailEditSession) domain.ThumbnailEdit {
	view := domain.ThumbnailEdit{
		ID: session.id, VideoID: session.videoID, State: domain.ThumbnailEditReady,
		Source: session.source, PositionMs: session.positionMs,
		FrameWidth: session.frame.Width, FrameHeight: session.frame.Height,
		HasResult: hasResult(session),
	}
	run := session.run
	if run == nil {
		return view
	}
	view.State = run.state
	end := e.now()
	if run.state.Terminal() {
		end = run.endedAt
	}
	elapsed := max(end.Sub(run.submittedAt), 0)
	view.Elapsed = &elapsed
	if run.progress != nil {
		progress := *run.progress
		view.Progress = &progress
	}
	if run.failure != nil {
		failure := *run.failure
		view.Error = &failure
	}
	return view
}

// hasResult はセッションに適用できる画像があるかを返す。
func hasResult(session *thumbnailEditSession) bool {
	return session.run != nil && session.run.state == domain.ThumbnailEditSucceeded && session.result != nil
}

// newThumbnailEditID は 128 ビットの乱数のセッションの識別子を返す。
func newThumbnailEditID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("cannot make a thumbnail edit id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
