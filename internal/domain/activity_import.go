package domain

import "slices"

// 取り込みの記録の進み具合。
// 1 本の動画の取り込みは 5 つの段階をまとめて 1 件の記録にする。

// ImportJobState は import の記録に属する仕事 1 件の段階と状態である。
type ImportJobState struct {
	Kind  JobKind
	State JobState
}

// ImportStageCount は import の記録がまとめる段階の数である（JobKinds の数）。
const ImportStageCount = 5

// ImportActivityProgress は running の import の記録の進み具合を、その動画の仕事から決める。Stage は
// 走っている仕事の段階で、無ければ待っている仕事のうち段階の順で最初のもの。Done は終わった仕事がある段階の
// 数で、Total を超えない。待つか走る仕事が無ければ nil を返す。
func ImportActivityProgress(jobs []ImportJobState) *ActivityImportProgress {
	stage, ok := firstImportStage(jobs, JobRunning)
	if !ok {
		stage, ok = firstImportStage(jobs, JobQueued)
	}
	if !ok {
		return nil
	}
	done := 0
	for _, kind := range JobKinds {
		if slices.Contains(jobs, ImportJobState{Kind: kind, State: JobDone}) {
			done++
		}
	}
	return &ActivityImportProgress{Stage: stage, Done: done, Total: ImportStageCount}
}

// firstImportStage は状態 state の仕事のうち、段階の順で最初の段階を返す。
func firstImportStage(jobs []ImportJobState, state JobState) (JobKind, bool) {
	for _, kind := range JobKinds {
		if slices.Contains(jobs, ImportJobState{Kind: kind, State: state}) {
			return kind, true
		}
	}
	return "", false
}
