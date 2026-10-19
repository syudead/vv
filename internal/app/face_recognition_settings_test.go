package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeFaceSettings struct{ value domain.FaceSettings }

func (f *fakeFaceSettings) FaceSettings(context.Context) (domain.FaceSettings, error) {
	return f.value, nil
}

func (f *fakeFaceSettings) SaveFaceSettings(_ context.Context, s domain.FaceSettings) error {
	f.value = s
	return nil
}

type fakeFaceQueue struct{}

func (fakeFaceQueue) Counts(context.Context) (domain.QueueCounts, error) {
	return domain.QueueCounts{}, nil
}

// fakeFaceRuntime records the calls that would download.
type fakeFaceRuntime struct {
	state  domain.FaceRuntimeState
	starts []domain.FaceRecognizer
	stops  int
}

func (*fakeFaceRuntime) Recognizers() []domain.FaceRecognizerDownload { return nil }

func (r *fakeFaceRuntime) Status(domain.FaceRecognizer) domain.FaceRuntimeStatus {
	return domain.FaceRuntimeStatus{State: r.state}
}

func (r *fakeFaceRuntime) Start(rec domain.FaceRecognizer) error {
	r.starts = append(r.starts, rec)
	return nil
}

func (r *fakeFaceRuntime) Stop() { r.stops++ }

func newFaceSettingsFixture(enabled bool) (*FaceRecognitionSettings, *fakeFaceSettings, *fakeFaceRuntime) {
	store := &fakeFaceSettings{value: domain.FaceSettings{Enabled: enabled, Recognizer: domain.FaceRecognizerSFaceInt8}}
	rt := &fakeFaceRuntime{state: domain.FaceRuntimeAbsent}
	return NewFaceRecognitionSettings(FaceRecognitionSettingsOptions{Settings: store, Queue: fakeFaceQueue{}, Runtime: rt}), store, rt
}

func TestFaceRecognitionOffNeverStartsADownload(t *testing.T) {
	ctx := context.Background()
	f, _, rt := newFaceSettingsFixture(false)

	if err := f.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.RetryDownload(ctx); !errors.Is(err, domain.ErrFaceRecognitionOff) {
		t.Fatalf("RetryDownload error = %v, want ErrFaceRecognitionOff", err)
	}
	if _, err := f.SaveSettings(ctx, domain.FaceSettings{Recognizer: domain.FaceRecognizerArcFaceR50}); err != nil {
		t.Fatal(err)
	}
	if len(rt.starts) != 0 || rt.stops != 1 {
		t.Fatalf("starts = %v, stops = %d; want none and the save's stop", rt.starts, rt.stops)
	}
}

func TestFaceRecognitionDownloadsTheChosenRecognizerWhileOn(t *testing.T) {
	ctx := context.Background()
	f, store, rt := newFaceSettingsFixture(true)

	if err := f.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.SaveSettings(ctx, domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerArcFaceR50}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.SaveSettings(ctx, domain.FaceSettings{Enabled: true, Recognizer: "other"}); !errors.Is(err, domain.ErrInvalidFaceSettings) {
		t.Fatalf("SaveSettings error = %v, want ErrInvalidFaceSettings", err)
	}
	want := []domain.FaceRecognizer{domain.FaceRecognizerSFaceInt8, domain.FaceRecognizerArcFaceR50}
	if !slices.Equal(rt.starts, want) || store.value.Recognizer != domain.FaceRecognizerArcFaceR50 {
		t.Fatalf("starts = %v, saved %+v; want %v and arcface_r50 kept", rt.starts, store.value, want)
	}
}

// A file that fails its hash at startup stays failed: only the owner's download replaces it.
func TestResumeLeavesAFailedRuntimeFailed(t *testing.T) {
	f, _, rt := newFaceSettingsFixture(true)
	rt.state = domain.FaceRuntimeFailed

	if err := f.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rt.starts) != 0 {
		t.Fatalf("starts = %v, want none", rt.starts)
	}
}

type fakeFaceAnalysisControl struct{ changes []domain.FaceSettings }

func (c *fakeFaceAnalysisControl) SettingsChanged(s domain.FaceSettings) {
	c.changes = append(c.changes, s)
}

// Turning the feature off reaches the analysis, which stops the running video.
func TestSavingFaceSettingsOffTellsTheAnalysis(t *testing.T) {
	control := &fakeFaceAnalysisControl{}
	f := NewFaceRecognitionSettings(FaceRecognitionSettingsOptions{
		Settings: &fakeFaceSettings{value: domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerSFaceInt8}},
		Queue:    fakeFaceQueue{}, Runtime: &fakeFaceRuntime{}, Analysis: control,
	})

	off := domain.FaceSettings{Recognizer: domain.FaceRecognizerSFaceInt8}
	if _, err := f.SaveSettings(context.Background(), off); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(control.changes, []domain.FaceSettings{off}) {
		t.Fatalf("analysis told %+v, want the saved settings %+v once", control.changes, off)
	}
}
