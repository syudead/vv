package domain

import (
	"errors"
	"testing"
)

func TestActivityKindFeature(t *testing.T) {
	for kind, want := range map[ActivityKind]ActivityFeature{
		ActivityImport:            ActivityFeatureImport,
		ActivityVersionJudgement:  ActivityFeatureDuplicateCheck,
		ActivityWorkInfoApply:     ActivityFeatureWorkInfo,
		ActivityTagReject:         ActivityFeatureTags,
		ActivityDismiss:           ActivityFeatureVersions,
		ActivityMediaFolderRemove: ActivityFeatureMediaFolders,
		ActivityOrganizeUndo:      ActivityFeatureOrganize,
	} {
		if got, ok := kind.Feature(); !ok || got != want {
			t.Errorf("%s.Feature() = %q, %v; want %q", kind, got, ok, want)
		}
	}
	if _, ok := ActivityUndo.Feature(); ok {
		t.Error("undo に種類だけで決まる機能がある")
	}
}

func TestParseActivityFilters(t *testing.T) {
	if _, err := ParseActivityStatus("attention"); err != nil {
		t.Errorf("attention: %v", err)
	}
	if _, err := ParseActivityFeature("duplicate_check"); err != nil {
		t.Errorf("duplicate_check: %v", err)
	}
	if _, err := ParseActivityDecider("ai"); err != nil {
		t.Errorf("ai: %v", err)
	}
	for _, err := range []error{
		func() error { _, err := ParseActivityStatus("failed"); return err }(),
		func() error { _, err := ParseActivityFeature("undo"); return err }(),
		func() error { _, err := ParseActivityDecider("model"); return err }(),
	} {
		if !errors.Is(err, ErrInvalidActivityFilter) {
			t.Errorf("知らない値の誤り = %v, want ErrInvalidActivityFilter", err)
		}
	}
}

func TestImportActivityState(t *testing.T) {
	for _, tc := range []struct {
		name string
		jobs []JobState
		want ActivityState
	}{
		{"all queued", []JobState{JobQueued}, ActivityState{Status: ActivityRunning, Reason: ActivityWaiting}},
		{"claimed", []JobState{JobRunning}, ActivityState{Status: ActivityRunning, Reason: ActivityProcessing}},
		{"between stages", []JobState{JobDone, JobQueued}, ActivityState{Status: ActivityRunning, Reason: ActivityProcessing}},
		{"pending after a failure", []JobState{JobFailed, JobQueued}, ActivityState{Status: ActivityRunning, Reason: ActivityProcessing}},
		{"gave up", []JobState{JobDone, JobFailed}, ActivityState{Status: ActivityAttention, Reason: ActivityFailed}},
		{"finished", []JobState{JobDone, JobDone}, ActivityState{Status: ActivityDone, Outcome: ActivityCompleted}},
	} {
		if got := ImportActivityState(tc.jobs); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestActivityRetryOffered(t *testing.T) {
	for _, tc := range []struct {
		entry ActivityEntry
		want  bool
	}{
		{ActivityEntry{Kind: ActivityImport, Status: ActivityAttention, Reason: ActivityFailed}, true},
		{ActivityEntry{Kind: ActivityScan, Status: ActivityAttention, Reason: ActivityInterrupted}, true},
		{ActivityEntry{Kind: ActivityVersionJudgement, Status: ActivityAttention, Reason: ActivityFailed}, true},
		{ActivityEntry{Kind: ActivityVersionJudgement, Status: ActivityAttention, Reason: ActivityUndecided}, false},
		{ActivityEntry{Kind: ActivityAutoTag, Status: ActivityAttention, Reason: ActivityNoConnection}, false},
		{ActivityEntry{Kind: ActivityImport, Status: ActivityDone, Outcome: ActivityCompleted}, false},
		{ActivityEntry{Kind: ActivityWorkInfoFetch, Status: ActivityAttention, Reason: ActivityFailed}, false},
	} {
		if got := tc.entry.RetryOffered(); got != tc.want {
			t.Errorf("%+v.RetryOffered() = %v, want %v", tc.entry, got, tc.want)
		}
	}
}

func TestActivityUndoOffered(t *testing.T) {
	same := ActivityEntry{Kind: ActivityVersionJudgement, Status: ActivityDone, Outcome: ActivitySame, Decider: ActivityByAI}
	byRule := same
	byRule.Decider, byRule.Outcome = ActivityByRule, ActivityDifferent
	undone := same
	undone.UndoneBy = 9
	superseded := same
	superseded.Outcome = ActivityDecidedByOwner
	owner := ActivityEntry{Kind: ActivityBundle, Status: ActivityDone, Outcome: ActivityCompleted, Decider: ActivityByOwner}
	tags := ActivityEntry{Kind: ActivityAutoTag, Status: ActivityDone, Decider: ActivityByAI,
		Tags: []ActivityEntryTag{{TagID: 1, UndoneBy: 4}, {TagID: 2}}}
	allUndone := tags
	allUndone.Tags = []ActivityEntryTag{{TagID: 1, UndoneBy: 4}}
	merge := ActivityEntry{Kind: ActivityFaceMerge, Status: ActivityDone, Outcome: ActivityCompleted, Decider: ActivityByRule}
	mergeUndone := merge
	mergeUndone.UndoneBy = 7
	for name, tc := range map[string]struct {
		entry ActivityEntry
		want  bool
	}{
		"same by ai":       {same, true},
		"different":        {byRule, true},
		"already undone":   {undone, false},
		"decided by owner": {superseded, false},
		"owner operation":  {owner, false},
		"tag left":         {tags, true},
		"every tag undone": {allUndone, false},
		"automatic merge":  {merge, true},
		"merge undone":     {mergeUndone, false},
	} {
		if got := tc.entry.UndoOffered(); got != tc.want {
			t.Errorf("%s: UndoOffered() = %v, want %v", name, got, tc.want)
		}
	}
	if tags.TagUndoOffered(tags.Tags[0]) || !tags.TagUndoOffered(tags.Tags[1]) {
		t.Error("TagUndoOffered は取り消していないタグだけを出す")
	}
}

func TestActivitySupersedes(t *testing.T) {
	subject := ActivitySubject{Kind: ActivitySubjectVideo, Key: "k", Label: "a.mp4"}
	newer := ActivityEntry{Kind: ActivityImport, Subject: subject, Status: ActivityRunning}
	older := ActivityEntry{Kind: ActivityImport, Subject: ActivitySubject{Kind: ActivitySubjectVideo, Key: "k", Label: "renamed.mp4"},
		Status: ActivityAttention}
	if !newer.Supersedes(older) {
		t.Error("同じ種類と対象の attention を閉じない")
	}
	for name, other := range map[string]ActivityEntry{
		"done":         {Kind: ActivityImport, Subject: subject, Status: ActivityDone},
		"other kind":   {Kind: ActivityAutoTag, Subject: subject, Status: ActivityAttention},
		"other key":    {Kind: ActivityImport, Subject: ActivitySubject{Kind: ActivitySubjectVideo, Key: "j"}, Status: ActivityAttention},
		"other pair b": {Kind: ActivityImport, Subject: ActivitySubject{Kind: ActivitySubjectVideo, Key: "k", KeyB: "m"}, Status: ActivityAttention},
	} {
		if newer.Supersedes(other) {
			t.Errorf("%s を閉じる", name)
		}
	}
	pair := ActivitySubject{Kind: ActivitySubjectPair, Key: "a", KeyB: "b"}
	judgement := ActivityEntry{Kind: ActivityVersionJudgement, Subject: pair, Status: ActivityRunning}
	if judgement.Supersedes(ActivityEntry{Kind: ActivityVersionJudgement, Subject: pair, Status: ActivityAttention}) {
		t.Error("version_judgement が組の記録を閉じる")
	}
}

func TestActivityCursorRoundTrip(t *testing.T) {
	id, err := DecodeActivityCursor(EncodeActivityCursor(42))
	if err != nil || id != 42 {
		t.Fatalf("round trip = %d, %v", id, err)
	}
	for _, cursor := range []string{"???", EncodeActivityCursor(0), "e30"} {
		if _, err := DecodeActivityCursor(cursor); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("%q: %v, want ErrInvalidCursor", cursor, err)
		}
	}
}
