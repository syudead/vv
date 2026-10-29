package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// JobPauser is a feature whose job kinds wait, queued and with their attempts kept, while the feature is off.
type JobPauser interface {
	PausedJobKinds(ctx context.Context) ([]domain.JobKind, error)
}

// PausedJobKinds collects the kinds every pauser holds back, for the scheduler to read before each round. See
// docs/design-docs/job-scheduling.md.
func PausedJobKinds(pausers ...JobPauser) func(context.Context) (map[domain.JobKind]bool, error) {
	return func(ctx context.Context) (map[domain.JobKind]bool, error) {
		paused := map[domain.JobKind]bool{}
		for _, pauser := range pausers {
			kinds, err := pauser.PausedJobKinds(ctx)
			if err != nil {
				return nil, err
			}
			for _, kind := range kinds {
				paused[kind] = true
			}
		}
		return paused, nil
	}
}
