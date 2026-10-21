package app

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeReviewStore struct {
	rejudged [][2]int64
	keys     [2]string
	evidence *domain.VersionJudgeEvidence
}

func (s *fakeReviewStore) RejudgeCandidate(_ context.Context, videoIDs [2]int64) error {
	s.rejudged = append(s.rejudged, videoIDs)
	return nil
}

func (s *fakeReviewStore) JudgementEvidence(context.Context, int64) ([2]string, *domain.VersionJudgeEvidence, error) {
	return s.keys, s.evidence, nil
}

type fakeJudgeWaker struct {
	enabled bool
	woken   int
}

func (w *fakeJudgeWaker) Enabled(context.Context) (bool, error) { return w.enabled, nil }

func (w *fakeJudgeWaker) Wake() { w.woken++ }

type missingFrames struct{}

func (missingFrames) SeekFramePairs(string, string, []domain.FramePair) ([][]byte, error) {
	return nil, fs.ErrNotExist
}

func TestVersionJudgeReviewRejudgeNeedsJudgingOn(t *testing.T) {
	store := &fakeReviewStore{}
	waker := &fakeJudgeWaker{}
	review := NewVersionJudgeReview(store, &fakeVersionFrames{}, waker)

	if err := review.Rejudge(context.Background(), [2]int64{1, 2}); !errors.Is(err, domain.ErrVersionJudgingOff) {
		t.Errorf("判定がオフ: Rejudge = %v, want ErrVersionJudgingOff", err)
	}
	if len(store.rejudged) != 0 || waker.woken != 0 {
		t.Errorf("オフなのに戻した: %v（起こした %d 回）", store.rejudged, waker.woken)
	}

	waker.enabled = true
	if err := review.Rejudge(context.Background(), [2]int64{1, 2}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.rejudged, [][2]int64{{1, 2}}) || waker.woken != 1 {
		t.Errorf("戻した組 = %v（起こした %d 回）, want [1 2] と 1 回", store.rejudged, waker.woken)
	}
}

func TestVersionJudgeReviewFrameCutsTheIndexedPair(t *testing.T) {
	store := &fakeReviewStore{keys: [2]string{"key-a", "key-b"}, evidence: &domain.VersionJudgeEvidence{
		Frames: []domain.VersionEvidenceFrame{{A: 1, B: 2, Distance: 20}, {A: 5, B: 6, Distance: 30}},
	}}
	frames := &fakeVersionFrames{}
	review := NewVersionJudgeReview(store, frames, &fakeJudgeWaker{})

	image, err := review.JudgementFrame(context.Background(), 7, 1)
	if err != nil || len(image) != 1 {
		t.Fatalf("JudgementFrame = %v, %v", image, err)
	}
	if frames.keys != [2]string{"key-a", "key-b"} || !reflect.DeepEqual(frames.pairs, []domain.FramePair{{A: 5, B: 6, Distance: 30}}) {
		t.Errorf("切り出した組 = %v %v, want key-a/key-b の 5/6", frames.keys, frames.pairs)
	}
	for name, index := range map[string]int{"範囲外": 2, "負": -1} {
		if _, err := review.JudgementFrame(context.Background(), 7, index); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: JudgementFrame = %v, want ErrNotFound", name, err)
		}
	}
	missing := NewVersionJudgeReview(store, missingFrames{}, &fakeJudgeWaker{})
	if _, err := missing.JudgementFrame(context.Background(), 7, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("シークプレビューなし: JudgementFrame = %v, want ErrNotFound", err)
	}
}
