package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func codeEdit(t *testing.T, db *DB, id int64, code string) domain.VideoCodeEdit {
	t.Helper()
	return domain.VideoCodeEdit{VideoID: id, ContentVersion: domain.ContentVersion(ownerVideo(t, db, id).ContentKey), Code: code}
}

func TestCodeSummaryListsOneRowPerUserKey(t *testing.T) {
	db, ids := versionFixture(t)
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	setWorkInfo(t, db, ids["b"], domain.WorkInfo{Code: "BND-1"})
	virtual := createVirtual(t, db, "Virtual", fixedTime)

	rows, err := db.VideoEdits().CodeSummary(context.Background(), []int64{ids["b"], ids["c"], ids["a"], virtual.ID, 999_999})
	if err != nil {
		t.Fatal(err)
	}
	var got []int64
	var codes []string
	for _, row := range rows {
		got = append(got, row.Video.ID)
		codes = append(codes, row.Code)
	}
	if !slices.Equal(got, []int64{ids["a"], ids["c"], virtual.ID}) || !slices.Equal(codes, []string{"BND-1", "", ""}) {
		t.Fatalf("rows = %v with codes %q, want the bundle's representative a, c and the virtual video", got, codes)
	}
	if rows[0].Video.FileTitle != "a" {
		t.Errorf("bundle row fileTitle = %q, want the representative's a", rows[0].Video.FileTitle)
	}
	// The API drops a virtual video's fileTitle; the HTTP test checks the row goes through that conversion.
	if !rows[2].Video.Virtual() {
		t.Errorf("virtual row = %+v, want the virtual video", rows[2].Video)
	}
}

func TestApplyCodesReplacesOnlyTheCode(t *testing.T) {
	db, ids := versionFixture(t)
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	hanako := createPerson(t, db, "山田花子")
	kept := domain.WorkInfo{
		Code: "OLD", People: []domain.WorkPerson{performer(hanako.ID)},
		Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: "2020"}}, URLs: []string{"https://example.com/a"},
	}
	setWorkInfo(t, db, ids["a"], kept)
	setWorkInfo(t, db, ids["c"], domain.WorkInfo{Code: "SAME"})
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	applied, err := db.VideoEdits().ApplyCodes(context.Background(), []domain.VideoCodeEdit{
		codeEdit(t, db, ids["b"], " NEW "), codeEdit(t, db, ids["c"], "SAME"), codeEdit(t, db, ids["d"], "D-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Errorf("applied = %d, want 2 (the bundle and d; c already had its code)", applied)
	}
	kept.Code = "NEW"
	if info, _ := workInfoOf(t, db, ids["a"]); !info.Equal(kept) {
		t.Errorf("bundle work info = %+v, want %+v", info, kept)
	}
	if info, _ := workInfoOf(t, db, ids["d"]); info.Code != "D-1" {
		t.Errorf("d code = %q, want D-1", info.Code)
	}
	want := []int64{ids["a"], ids["b"], ids["d"]}
	slices.Sort(want)
	if len(recorder.events) != 1 {
		t.Fatalf("events = %v, want one VideoWorkInfoChanged", recorder.events)
	}
	changed, ok := recorder.events[0].(domain.VideoWorkInfoChanged)
	got := slices.Clone(changed.VideoIDs)
	slices.Sort(got)
	if !ok || !slices.Equal(got, want) {
		t.Errorf("event = %+v, want VideoWorkInfoChanged for %v", recorder.events[0], want)
	}
}

func TestApplyCodesWritesNothingOnAFailingItem(t *testing.T) {
	for _, tc := range []struct {
		name   string
		second func(t *testing.T, db *DB, ids map[string]int64) domain.VideoCodeEdit
		want   error
	}{
		{"missing video", func(*testing.T, *DB, map[string]int64) domain.VideoCodeEdit {
			return domain.VideoCodeEdit{VideoID: 999_999, ContentVersion: "x", Code: "X"}
		}, domain.ErrNotFound},
		{"reused video ID", func(t *testing.T, db *DB, _ map[string]int64) domain.VideoCodeEdit {
			t.Helper()
			deleted := createVirtual(t, db, "Deleted", fixedTime)
			if err := db.VirtualVideos().Delete(context.Background(), deleted.ID, deleted.UserKey); err != nil {
				t.Fatal(err)
			}
			if reused := createVirtual(t, db, "Unrelated", fixedTime); reused.ID != deleted.ID {
				t.Fatalf("new virtual video ID = %d; want the deleted ID %d reused", reused.ID, deleted.ID)
			}
			return domain.VideoCodeEdit{VideoID: deleted.ID, ContentVersion: deleted.UserKey, Code: "X"}
		}, domain.ErrContentChanged},
		{"two items on one key", func(t *testing.T, db *DB, ids map[string]int64) domain.VideoCodeEdit {
			t.Helper()
			return codeEdit(t, db, ids["b"], "X")
		}, domain.ErrDuplicateVideoCodeEdit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, ids := versionFixture(t)
			bundle(t, db, ids["a"], ids["a"], ids["b"])
			items := []domain.VideoCodeEdit{codeEdit(t, db, ids["a"], "A-1"), tc.second(t, db, ids)}
			before := workInfoRowCount(t, db)
			recorder := &eventRecorder{}
			db.PublishTo(recorder)

			_, err := db.VideoEdits().ApplyCodes(context.Background(), items)
			var at *domain.VideoCodeEditAtError
			if !errors.As(err, &at) || at.Index != 1 || !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v at item 1", err, tc.want)
			}
			if workInfoRowCount(t, db) != before || len(recorder.events) != 0 {
				t.Errorf("work-info rows = %d (was %d), events = %v; want nothing written", workInfoRowCount(t, db), before, recorder.events)
			}
		})
	}
}
