package domain

import "testing"

// running の import の進み具合は、走っている段階（無ければ段階の順で最初の待っている段階）と終わった仕事の数で
// ある。待つか走る仕事が無ければ進み具合は無い。
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ImportActivityProgress(tc.jobs)
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Errorf("progress = %+v, want %+v", got, tc.want)
			}
		})
	}
}
