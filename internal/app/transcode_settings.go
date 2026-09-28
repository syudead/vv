package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// encoderCheckTimeout は起動時の確認の、エンコーダーごとの上限時間である
// （specs/025-hardware-encoding/research.md R-2）。
const encoderCheckTimeout = 10 * time.Second

// TranscodeSettingsStore はライブ変換の映像エンコード方式の保存先である。値は解釈せずに
// 文字列で読み書きし、解釈は domain.ParseEncoderChoice が行う。
type TranscodeSettingsStore interface {
	TranscodeEncoderChoice(ctx context.Context) (value string, found bool, err error)
	SaveTranscodeEncoderChoice(ctx context.Context, value string) error
}

// EncoderChecker は 1 つのハードウェアの方式が使えるかを、短い実エンコードで確かめる。
// 結果の State は available か unavailable で、unavailable には Reason を付ける。
// ctx の期限で打ち切る。internal/media の EncoderCheck がこれを満たす。
type EncoderChecker interface {
	CheckEncoder(ctx context.Context, encoder domain.VideoEncoder) domain.EncoderAvailability
}

// TranscodeSettingsOptions はライブ変換の方式の設定に必要な依存である。
type TranscodeSettingsOptions struct {
	Store   TranscodeSettingsStore
	Checker EncoderChecker
	// GOOS は確認対象を決める OS（runtime.GOOS の値）である。
	GOOS string
	// CheckTimeout はエンコーダーごとの上限時間で、0 なら encoderCheckTimeout。
	CheckTimeout time.Duration
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// TranscodeSettings はライブ変換の映像エンコード方式の選択と、起動時の確認の結果を
// メモリに持ち、実際に使う方式を決める（research.md R-3）。確認の結果は保存しない。
type TranscodeSettings struct {
	store        TranscodeSettingsStore
	checker      EncoderChecker
	checkTimeout time.Duration
	logger       *slog.Logger

	// selectMu は Select を 1 つずつにし、保存値とメモリの選択を食い違わせない。
	selectMu sync.Mutex

	mu           sync.Mutex
	choice       domain.EncoderChoice
	availability []domain.EncoderAvailability
	started      bool
	done         chan struct{}
}

// NewTranscodeSettings は保存値を読み込み、確認を始める前の状態を返す。確認は
// StartChecks で始める。それまでの確認対象は確認中で、実際に使う方式は software である。
func NewTranscodeSettings(ctx context.Context, opts TranscodeSettingsOptions) (*TranscodeSettings, error) {
	value, found, err := opts.Store.TranscodeEncoderChoice(ctx)
	if err != nil {
		return nil, err
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := opts.CheckTimeout
	if timeout <= 0 {
		timeout = encoderCheckTimeout
	}
	choice := domain.ParseEncoderChoice(value, found)
	if found && string(choice) != value {
		logger.Warn("unknown transcode video encoder is stored; using software",
			slog.String("stored", value))
	}
	return &TranscodeSettings{
		store:        opts.Store,
		checker:      opts.Checker,
		checkTimeout: timeout,
		logger:       logger,
		choice:       choice,
		availability: domain.InitialEncoderAvailability(opts.GOOS),
		done:         make(chan struct{}),
	}, nil
}

// StartChecks は確認対象のエンコーダーの確認を並行して始め、待たずに戻る。2 度目からは
// 何もしない。すべて終わると結果を反映してログに記録し、Done を閉じる。
func (t *TranscodeSettings) StartChecks(ctx context.Context) {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return
	}
	t.started = true
	var targets []domain.VideoEncoder
	for _, a := range t.availability {
		if a.State == domain.EncoderChecking {
			targets = append(targets, a.Encoder)
		}
	}
	t.mu.Unlock()

	var wg sync.WaitGroup
	for _, encoder := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t.setAvailability(t.check(ctx, encoder))
		}()
	}
	go func() {
		wg.Wait()
		encoding := t.Current()
		t.logEncoding("transcode video encoder checks finished", encoding)
		close(t.done)
	}()
}

// Done は起動時の確認がすべて終わると閉じる。
func (t *TranscodeSettings) Done() <-chan struct{} {
	return t.done
}

// check は 1 つのエンコーダーを上限時間つきで確かめる。checker が上限時間を過ぎても
// 戻らないときは待たずに timed_out とする。
func (t *TranscodeSettings) check(ctx context.Context, encoder domain.VideoEncoder) domain.EncoderAvailability {
	checkCtx, cancel := context.WithTimeout(ctx, t.checkTimeout)
	defer cancel()
	result := make(chan domain.EncoderAvailability, 1)
	go func() { result <- t.checker.CheckEncoder(checkCtx, encoder) }()
	var got domain.EncoderAvailability
	select {
	case got = <-result:
	case <-checkCtx.Done():
	}
	if err := checkCtx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return unavailable(encoder, domain.EncoderReasonTimedOut)
		}
		return unavailable(encoder, domain.EncoderReasonCheckFailed)
	}
	switch got.State {
	case domain.EncoderAvailable:
		return domain.EncoderAvailability{Encoder: encoder, State: domain.EncoderAvailable}
	case domain.EncoderUnavailable:
		reason := got.Reason
		if reason == "" {
			reason = domain.EncoderReasonCheckFailed
		}
		return unavailable(encoder, reason)
	default:
		return unavailable(encoder, domain.EncoderReasonCheckFailed)
	}
}

func unavailable(encoder domain.VideoEncoder, reason domain.EncoderUnavailableReason) domain.EncoderAvailability {
	return domain.EncoderAvailability{Encoder: encoder, State: domain.EncoderUnavailable, Reason: reason}
}

func (t *TranscodeSettings) setAvailability(result domain.EncoderAvailability) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range t.availability {
		if t.availability[i].Encoder == result.Encoder {
			t.availability[i] = result
		}
	}
}

// Current は今の選択・実際に使う方式・理由・確認中か・各エンコーダーの結果を返す。
func (t *TranscodeSettings) Current() domain.TranscodeEncoding {
	t.mu.Lock()
	defer t.mu.Unlock()
	return domain.NewTranscodeEncoding(t.choice, t.availability)
}

// Select は選択を保存して状態を更新し、更新後の状態を返す。software と auto は常に
// 受け付ける。使えない（確認中を含む）ハードウェアの方式には domain.ErrEncoderUnavailable
// を返し、保存値を変えない。
func (t *TranscodeSettings) Select(ctx context.Context, choice domain.EncoderChoice) (domain.TranscodeEncoding, error) {
	if !choice.Valid() {
		return domain.TranscodeEncoding{}, fmt.Errorf("unknown video encoder choice %q", choice)
	}
	t.selectMu.Lock()
	defer t.selectMu.Unlock()
	if encoder, ok := choice.HardwareEncoder(); ok && !t.isAvailable(encoder) {
		return domain.TranscodeEncoding{}, domain.ErrEncoderUnavailable
	}
	if err := t.store.SaveTranscodeEncoderChoice(ctx, string(choice)); err != nil {
		return domain.TranscodeEncoding{}, err
	}
	t.mu.Lock()
	t.choice = choice
	encoding := domain.NewTranscodeEncoding(t.choice, t.availability)
	t.mu.Unlock()
	t.logEncoding("transcode video encoder selected", encoding)
	return encoding, nil
}

func (t *TranscodeSettings) isAvailable(encoder domain.VideoEncoder) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, a := range t.availability {
		if a.Encoder == encoder {
			return a.State == domain.EncoderAvailable
		}
	}
	return false
}

// logEncoding は選択・実際の方式・理由を記録する（親 Issue #370 要件 9）。
func (t *TranscodeSettings) logEncoding(msg string, encoding domain.TranscodeEncoding) {
	encoders := make([]string, 0, len(encoding.Encoders))
	for _, a := range encoding.Encoders {
		entry := string(a.Encoder) + "=" + string(a.State)
		if a.Reason != "" {
			entry += "(" + string(a.Reason) + ")"
		}
		encoders = append(encoders, entry)
	}
	attrs := []any{
		slog.String("choice", string(encoding.Choice)),
		slog.String("effective", string(encoding.Effective)),
	}
	if encoding.FallbackReason != "" {
		attrs = append(attrs, slog.String("fallbackReason", string(encoding.FallbackReason)))
	}
	attrs = append(attrs, slog.Bool("checking", encoding.Checking), slog.String("encoders", strings.Join(encoders, " ")))
	t.logger.Info(msg, attrs...)
}
