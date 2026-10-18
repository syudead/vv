package domain

import "testing"

func TestImportActivityProgress(t *testing.T) {
	for _, tc := range []struct {
		name string
		jobs []ImportJobState
		want *ActivityImportProgress
	}{
		{"nothing pending", []ImportJobState{{JobProbe, JobDone}, {JobThumbnail, JobFailed}}, nil},
		{"running stage first", []ImportJobState{
			{JobProbe, JobDone}, {JobThumbnail, JobQueued}, {JobSeekThumbnail, JobRunning},
		}, &ActivityImportProgress{Stage: JobSeekThumbnail, Done: 1, Total: ImportStageCount}},
		{"first waiting stage in stage order", []ImportJobState{
			{JobFingerprint, JobQueued}, {JobPreview, JobQueued}, {JobProbe, JobDone},
		}, &ActivityImportProgress{Stage: JobPreview, Done: 1, Total: ImportStageCount}},
		{"a stage done twice counts once", []ImportJobState{
			{JobProbe, JobDone}, {JobProbe, JobDone}, {JobThumbnail, JobRunning},
		}, &ActivityImportProgress{Stage: JobThumbnail, Done: 1, Total: ImportStageCount}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ImportActivityProgress(tc.jobs)
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Errorf("progress = %+v, want %+v", got, tc.want)
			}
		})
	}
}
