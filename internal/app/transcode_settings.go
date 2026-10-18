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

// Per-encoder startup check deadline.
const encoderCheckTimeout = 10 * time.Second

// Store raw strings; domain.ParseEncoderChoice owns interpretation.
type TranscodeSettingsStore interface {
	TranscodeEncoderChoice(ctx context.Context) (value string, found bool, err error)
	SaveTranscodeEncoderChoice(ctx context.Context, value string) error
}

// Trial encodes return available or unavailable with a reason and honor context deadlines.
type EncoderChecker interface {
	CheckEncoder(ctx context.Context, encoder domain.VideoEncoder) domain.EncoderAvailability
}

type TranscodeSettingsOptions struct {
	Store   TranscodeSettingsStore
	Checker EncoderChecker
	GOOS    string
	// Zero uses encoderCheckTimeout.
	CheckTimeout time.Duration
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// Keep capability checks in memory only; persisted choice and current availability determine the
// effective encoder.
type TranscodeSettings struct {
	store        TranscodeSettingsStore
	checker      EncoderChecker
	checkTimeout time.Duration
	logger       *slog.Logger

	// Serialize saving and in-memory selection to prevent divergence.
	selectMu sync.Mutex

	mu           sync.Mutex
	choice       domain.EncoderChoice
	availability []domain.EncoderAvailability
	started      bool
	done         chan struct{}
}

// Before StartChecks completes, hardware remains checking and software is effective.
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

// StartChecks runs once asynchronously and closes Done after recording all results.
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
		t.logCheckDetails(encoding)
		close(t.done)
	}()
}

func (t *TranscodeSettings) Done() <-chan struct{} {
	return t.done
}

// Do not wait past the deadline even if a checker ignores cancellation.
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
		failed := unavailable(encoder, reason)
		failed.Detail = got.Detail
		return failed
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

func (t *TranscodeSettings) Current() domain.TranscodeEncoding {
	t.mu.Lock()
	defer t.mu.Unlock()
	return domain.NewTranscodeEncoding(t.choice, t.availability)
}

// Select always accepts software/auto, rejecting unavailable or still-checking hardware without
// changing storage.
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

// Log FFmpeg stderr details without exposing them through the API.
func (t *TranscodeSettings) logCheckDetails(encoding domain.TranscodeEncoding) {
	for _, a := range encoding.Encoders {
		if a.State != domain.EncoderUnavailable || a.Detail == "" {
			continue
		}
		t.logger.Warn("transcode video encoder check failed",
			slog.String("encoder", string(a.Encoder)),
			slog.String("reason", string(a.Reason)),
			slog.String("detail", a.Detail),
		)
	}
}

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
