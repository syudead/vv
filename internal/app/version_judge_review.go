package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/syudead/vv/internal/domain"
)

// VersionJudgeReviewStore is what the owner's review of automatic judgements reads and changes.
type VersionJudgeReviewStore interface {
	RejudgeCandidate(ctx context.Context, videoIDs [2]int64) error
	JudgementEvidence(ctx context.Context, entryID int64) ([2]string, *domain.VersionJudgeEvidence, error)
}

type versionJudgeWaker interface {
	Enabled(ctx context.Context) (bool, error)
	Wake()
}

// VersionJudgeReview lets the owner send a pair back to the classifier and see the frames it was shown; see
// docs/design-docs/duplicate-judging.md.
type VersionJudgeReview struct {
	store  VersionJudgeReviewStore
	frames VersionFrameSource
	judge  versionJudgeWaker
}

func NewVersionJudgeReview(
	store VersionJudgeReviewStore, frames VersionFrameSource, judge versionJudgeWaker,
) *VersionJudgeReview {
	return &VersionJudgeReview{store: store, frames: frames, judge: judge}
}

// Rejudge returns ErrVersionJudgingOff while judging is off, since nothing would claim the pair.
func (r *VersionJudgeReview) Rejudge(ctx context.Context, videoIDs [2]int64) error {
	on, err := r.judge.Enabled(ctx)
	if err != nil {
		return err
	}
	if !on {
		return domain.ErrVersionJudgingOff
	}
	if err := r.store.RejudgeCandidate(ctx, videoIDs); err != nil {
		return err
	}
	r.judge.Wake()
	return nil
}

// JudgementFrame returns the index-th image the classifier was shown, A on the left; a missing entry, frame or seek
// preview returns ErrNotFound.
func (r *VersionJudgeReview) JudgementFrame(ctx context.Context, entryID int64, index int) ([]byte, error) {
	keys, evidence, err := r.store.JudgementEvidence(ctx, entryID)
	if err != nil {
		return nil, err
	}
	if evidence == nil || index < 0 || index >= len(evidence.Frames) {
		return nil, fmt.Errorf("judgement frame %d of entry %d: %w", index, entryID, domain.ErrNotFound)
	}
	images, err := r.frames.SeekFramePairs(keys[0], keys[1], evidence.FramePairs()[index:index+1])
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("judgement frame %d of entry %d: %w", index, entryID, domain.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return images[0], nil
}
