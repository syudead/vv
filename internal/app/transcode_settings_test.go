package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeTranscodeSettingsStore は保存値を 1 つだけ持つ。
type fakeTranscodeSettingsStore struct {
	mu      sync.Mutex
	value   string
	found   bool
	saves   int
	saveErr error
}

func (f *fakeTranscodeSettingsStore) TranscodeEncoderChoice(context.Context) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value, f.found, nil
}

func (f *fakeTranscodeSettingsStore) SaveTranscodeEncoderChoice(_ context.Context, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.value, f.found = value, true
	f.saves++
	return nil
}

func (f *fakeTranscodeSettingsStore) stored() (string, bool, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value, f.found, f.saves
}

// fakeEncoderChecker は方式ごとに決めた結果を返す。release が nil でなければ、閉じるまで
// どの確認も戻らない。hang の方式は ctx が終わるまで戻らない。
type fakeEncoderChecker struct {
	results map[domain.VideoEncoder]domain.EncoderAvailability
	release chan struct{}
	hang    map[domain.VideoEncoder]bool
}

func (f *fakeEncoderChecker) CheckEncoder(ctx context.Context, encoder domain.VideoEncoder) domain.EncoderAvailability {
	if f.hang[encoder] {
		<-ctx.Done()
		return domain.EncoderAvailability{Encoder: encoder, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonCheckFailed}
	}
	if f.release != nil {
		<-f.release
	}
	return f.results[encoder]
}

func available(encoder domain.VideoEncoder) domain.EncoderAvailability {
	return domain.EncoderAvailability{Encoder: encoder, State: domain.EncoderAvailable}
}

func waitChecks(t *testing.T, s *TranscodeSettings) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("encoder checks did not finish")
	}
}

func TestTranscodeSettingsIsSoftwareWhileChecking(t *testing.T) {
	store := &fakeTranscodeSettingsStore{value: "nvenc", found: true}
	checker := &fakeEncoderChecker{
		results: map[domain.VideoEncoder]domain.EncoderAvailability{
			domain.VideoEncoderNVENC: available(domain.VideoEncoderNVENC),
			domain.VideoEncoderQSV:   available(domain.VideoEncoderQSV),
			domain.VideoEncoderVAAPI: available(domain.VideoEncoderVAAPI),
		},
		release: make(chan struct{}),
	}
	s, err := NewTranscodeSettings(context.Background(), TranscodeSettingsOptions{
		Store: store, Checker: checker, GOOS: "linux", Logger: discardLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s.StartChecks(context.Background())

	got := s.Current()
	if !got.Checking || got.Choice != domain.EncoderChoiceNVENC || got.Effective != domain.VideoEncoderSoftware ||
		got.FallbackReason != domain.EncoderFallbackChecking {
		t.Fatalf("while checking = %+v", got)
	}
	// 確認中のハードウェアの方式は選べず、保存値は変わらない。
	if _, err := s.Select(context.Background(), domain.EncoderChoiceQSV); !errors.Is(err, domain.ErrEncoderUnavailable) {
		t.Fatalf("Select(qsv) while checking = %v, want ErrEncoderUnavailable", err)
	}
	if value, _, saves := store.stored(); value != "nvenc" || saves != 0 {
		t.Fatalf("stored = %q after %d saves, want nvenc untouched", value, saves)
	}

	close(checker.release)
	waitChecks(t, s)
	got = s.Current()
	if got.Checking || got.Effective != domain.VideoEncoderNVENC || got.FallbackReason != "" {
		t.Fatalf("after checks = %+v", got)
	}
}

func TestTranscodeSettingsLogsResultWhenChecksFinish(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	store := &fakeTranscodeSettingsStore{value: "vaapi", found: true}
	checker := &fakeEncoderChecker{results: map[domain.VideoEncoder]domain.EncoderAvailability{
		domain.VideoEncoderNVENC: available(domain.VideoEncoderNVENC),
		domain.VideoEncoderQSV:   {Encoder: domain.VideoEncoderQSV, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonEncoderMissing},
		domain.VideoEncoderVAAPI: {Encoder: domain.VideoEncoderVAAPI, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonCheckFailed},
	}}
	s, err := NewTranscodeSettings(context.Background(), TranscodeSettingsOptions{
		Store: store, Checker: checker, GOOS: "linux", Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.StartChecks(context.Background())
	waitChecks(t, s)

	got := s.Current()
	want := []domain.EncoderAvailability{
		available(domain.VideoEncoderNVENC),
		{Encoder: domain.VideoEncoderQSV, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonEncoderMissing},
		{Encoder: domain.VideoEncoderVAAPI, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonCheckFailed},
		{Encoder: domain.VideoEncoderVideoToolbox, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonUnsupportedOS},
	}
	if len(got.Encoders) != len(want) {
		t.Fatalf("encoders = %+v", got.Encoders)
	}
	for i := range want {
		if got.Encoders[i] != want[i] {
			t.Fatalf("encoders[%d] = %+v, want %+v", i, got.Encoders[i], want[i])
		}
	}
	if got.Effective != domain.VideoEncoderSoftware || got.FallbackReason != domain.EncoderFallbackSelectedUnavailable {
		t.Fatalf("after checks = %+v", got)
	}
	// 使えない方式を選んでいても保存値は変えない（要件 8）。
	if value, _, saves := store.stored(); value != "vaapi" || saves != 0 {
		t.Fatalf("stored = %q after %d saves", value, saves)
	}
	line := buf.String()
	for _, part := range []string{"transcode video encoder checks finished", "choice=vaapi", "effective=software", "fallbackReason=selected_unavailable"} {
		if !strings.Contains(line, part) {
			t.Fatalf("log %q does not contain %q", line, part)
		}
	}
}

func TestTranscodeSettingsLogsCheckFailureDetail(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	store := &fakeTranscodeSettingsStore{}
	checker := &fakeEncoderChecker{results: map[domain.VideoEncoder]domain.EncoderAvailability{
		domain.VideoEncoderNVENC: {Encoder: domain.VideoEncoderNVENC, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonCheckFailed, Detail: "Cannot load libcuda.so.1"},
		domain.VideoEncoderQSV:   {Encoder: domain.VideoEncoderQSV, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonEncoderMissing},
		domain.VideoEncoderVAAPI: available(domain.VideoEncoderVAAPI),
	}}
	s, err := NewTranscodeSettings(context.Background(), TranscodeSettingsOptions{
		Store: store, Checker: checker, GOOS: "linux", Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.StartChecks(context.Background())
	waitChecks(t, s)

	if got := s.Current().Encoders[0]; got.Detail != "Cannot load libcuda.so.1" {
		t.Fatalf("nvenc = %+v, want the checker's detail kept", got)
	}
	log := buf.String()
	for _, part := range []string{"transcode video encoder check failed", "encoder=nvenc", "reason=check_failed", `detail="Cannot load libcuda.so.1"`} {
		if !strings.Contains(log, part) {
			t.Fatalf("log %q does not contain %q", log, part)
		}
	}
	if strings.Contains(log, "encoder=qsv") || strings.Contains(log, "encoder=vaapi") {
		t.Fatalf("log %q has a detail line for an encoder without detail", log)
	}
}

func TestTranscodeSettingsTimesOutOneCheck(t *testing.T) {
	store := &fakeTranscodeSettingsStore{found: false}
	checker := &fakeEncoderChecker{
		results: map[domain.VideoEncoder]domain.EncoderAvailability{
			domain.VideoEncoderNVENC: available(domain.VideoEncoderNVENC),
			domain.VideoEncoderVAAPI: available(domain.VideoEncoderVAAPI),
		},
		hang: map[domain.VideoEncoder]bool{domain.VideoEncoderQSV: true},
	}
	s, err := NewTranscodeSettings(context.Background(), TranscodeSettingsOptions{
		Store: store, Checker: checker, GOOS: "linux", Logger: discardLogger(), CheckTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.StartChecks(context.Background())
	waitChecks(t, s)

	got := s.Current()
	if got.Checking || got.Choice != domain.EncoderChoiceSoftware || got.Effective != domain.VideoEncoderSoftware {
		t.Fatalf("after checks = %+v", got)
	}
	states := map[domain.VideoEncoder]domain.EncoderAvailability{}
	for _, a := range got.Encoders {
		states[a.Encoder] = a
	}
	if states[domain.VideoEncoderNVENC].State != domain.EncoderAvailable || states[domain.VideoEncoderVAAPI].State != domain.EncoderAvailable {
		t.Fatalf("other checks did not report: %+v", got.Encoders)
	}
	if q := states[domain.VideoEncoderQSV]; q.State != domain.EncoderUnavailable || q.Reason != domain.EncoderReasonTimedOut {
		t.Fatalf("qsv = %+v, want unavailable/timed_out", q)
	}
}

func TestTranscodeSettingsSelect(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	store := &fakeTranscodeSettingsStore{}
	checker := &fakeEncoderChecker{results: map[domain.VideoEncoder]domain.EncoderAvailability{
		domain.VideoEncoderNVENC: {Encoder: domain.VideoEncoderNVENC, State: domain.EncoderUnavailable, Reason: domain.EncoderReasonCheckFailed},
		domain.VideoEncoderQSV:   available(domain.VideoEncoderQSV),
		domain.VideoEncoderVAAPI: available(domain.VideoEncoderVAAPI),
	}}
	s, err := NewTranscodeSettings(context.Background(), TranscodeSettingsOptions{
		Store: store, Checker: checker, GOOS: "linux", Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.StartChecks(context.Background())
	waitChecks(t, s)

	got, err := s.Select(context.Background(), domain.EncoderChoiceVAAPI)
	if err != nil {
		t.Fatal(err)
	}
	if got.Choice != domain.EncoderChoiceVAAPI || got.Effective != domain.VideoEncoderVAAPI {
		t.Fatalf("Select(vaapi) = %+v", got)
	}
	if !strings.Contains(buf.String(), "transcode video encoder selected") || !strings.Contains(buf.String(), "effective=vaapi") {
		t.Fatalf("log = %q", buf.String())
	}

	for _, choice := range []domain.EncoderChoice{domain.EncoderChoiceNVENC, domain.EncoderChoiceVideoToolbox} {
		if _, err := s.Select(context.Background(), choice); !errors.Is(err, domain.ErrEncoderUnavailable) {
			t.Fatalf("Select(%s) = %v, want ErrEncoderUnavailable", choice, err)
		}
	}
	if value, _, saves := store.stored(); value != "vaapi" || saves != 1 {
		t.Fatalf("stored = %q after %d saves, want vaapi once", value, saves)
	}
	if s.Current().Choice != domain.EncoderChoiceVAAPI {
		t.Fatalf("choice changed by a rejected Select: %+v", s.Current())
	}

	got, err = s.Select(context.Background(), domain.EncoderChoiceAuto)
	if err != nil {
		t.Fatal(err)
	}
	if got.Effective != domain.VideoEncoderQSV || got.FallbackReason != "" {
		t.Fatalf("Select(auto) = %+v", got)
	}
	if _, err := s.Select(context.Background(), domain.EncoderChoiceSoftware); err != nil {
		t.Fatal(err)
	}
	if value, _, _ := store.stored(); value != "software" {
		t.Fatalf("stored = %q, want software", value)
	}

	store.saveErr = errors.New("disk full")
	if _, err := s.Select(context.Background(), domain.EncoderChoiceAuto); err == nil {
		t.Fatal("Select with a failing store succeeded")
	}
	if s.Current().Choice != domain.EncoderChoiceSoftware {
		t.Fatalf("choice changed although saving failed: %+v", s.Current())
	}
}

func TestTranscodeSettingsTreatsUnknownStoredValueAsSoftware(t *testing.T) {
	store := &fakeTranscodeSettingsStore{value: "future_encoder", found: true}
	s, err := NewTranscodeSettings(context.Background(), TranscodeSettingsOptions{
		Store: store, Checker: &fakeEncoderChecker{}, GOOS: "plan9", Logger: discardLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s.StartChecks(context.Background())
	waitChecks(t, s)
	got := s.Current()
	if got.Choice != domain.EncoderChoiceSoftware || got.Effective != domain.VideoEncoderSoftware || got.Checking {
		t.Fatalf("current = %+v", got)
	}
	if value, _, _ := store.stored(); value != "future_encoder" {
		t.Fatalf("stored value was rewritten to %q", value)
	}
}
