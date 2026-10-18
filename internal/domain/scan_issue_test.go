package domain

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

func TestScanIssueKindSeverity(t *testing.T) {
	for _, kind := range scanIssueKinds {
		want := IssueFailed
		if kind == IssueThumbnailFirstFrame || kind == IssueSeekThumbnailFullDecode {
			want = IssueSubstituted
		}
		if got := kind.Severity(); got != want {
			t.Errorf("%s: 重さ = %s, want %s", kind, got, want)
		}
	}
	for _, job := range JobKinds {
		kind, ok := FailedIssueKind(job)
		if !ok || kind.Severity() != IssueFailed || kind.FromScan() {
			t.Errorf("%s: 失敗の種類 = %q, %v", job, kind, ok)
		}
	}
}

func TestSubstitutedIssueKind(t *testing.T) {
	want := map[JobKind]ScanIssueKind{
		JobThumbnail: IssueThumbnailFirstFrame, JobSeekThumbnail: IssueSeekThumbnailFullDecode,
	}
	for _, job := range JobKinds {
		kind, ok := SubstitutedIssueKind(job)
		if kind != want[job] || ok != (want[job] != "") {
			t.Errorf("%s: 代用の種類 = %q, %v, want %q", job, kind, ok, want[job])
		}
		if ok && kind.Severity() != IssueSubstituted {
			t.Errorf("%s: 重さ = %s", kind, kind.Severity())
		}
	}
	if SubstitutionOf(true) != SubstitutionUsed || SubstitutionOf(false) != SubstitutionNone {
		t.Error("SubstitutionOf が生成の値を写さない")
	}
}

func TestScanIssuesGroupsFiltersAndSorts(t *testing.T) {
	media := filepath.FromSlash("/media")
	roots := []MediaFolder{{ID: 1, Path: media}}
	in := func(rel string) string { return filepath.Join(media, filepath.FromSlash(rel)) }
	issues := ScanIssues(roots, []ScanIssueRecord{
		{VideoID: 1, Path: in("b/z.mp4"), Kinds: []ScanIssueKind{IssueThumbnailFirstFrame}, InImport: true},
		{VideoID: 2, Path: in("a/z.mp4"), Kinds: []ScanIssueKind{IssueThumbnailFirstFrame, IssueProbeFailed}, InImport: true},
		{Path: in("y.mp4"), Kinds: []ScanIssueKind{IssueUnreadable}},
		{Path: filepath.FromSlash("/elsewhere/x.mp4"), Kinds: []ScanIssueKind{IssueUnreadable}},
		{VideoID: 3, Path: "", Kinds: []ScanIssueKind{IssueProbeFailed}},
		{VideoID: 4, Path: in("a/w.mp4"), Kinds: []ScanIssueKind{IssueSeekThumbnailFullDecode}, InImport: true},
	})

	var got []string
	for _, issue := range issues {
		got = append(got, string(issue.Severity)+" "+issue.Folder.Path+"/"+issue.FileName)
	}
	want := []string{"failed /y.mp4", "failed a/z.mp4", "substituted a/w.mp4", "substituted b/z.mp4"}
	if !slices.Equal(got, want) {
		t.Fatalf("並び = %v, want %v", got, want)
	}
	if !slices.Equal(issues[1].Kinds, []ScanIssueKind{IssueProbeFailed, IssueThumbnailFirstFrame}) {
		t.Errorf("種類 = %v, want 重い順", issues[1].Kinds)
	}
	if issues[1].RootName != "media" {
		t.Errorf("登録フォルダの表示名 = %q", issues[1].RootName)
	}
	if !issues[0].Unregistered || issues[1].Unregistered {
		t.Errorf("未登録 = %v, %v", issues[0].Unregistered, issues[1].Unregistered)
	}
	if got := CountScanIssues(issues); got != (ScanIssueCounts{Failed: 2, Substituted: 2, Unregistered: 1}) {
		t.Errorf("本数 = %+v", got)
	}
}

func TestScanIssueFromScanInImportIsNotUnregistered(t *testing.T) {
	media := filepath.FromSlash("/media")
	issues := ScanIssues([]MediaFolder{{ID: 1, Path: media}}, []ScanIssueRecord{
		{VideoID: 1, Path: filepath.Join(media, "a.mp4"), Kinds: []ScanIssueKind{IssueRegisterFailed}, InImport: true},
		{VideoID: 2, Path: filepath.Join(media, "b.mp4"), Kinds: []ScanIssueKind{IssueRegisterFailed}},
	})
	if got := CountScanIssues(issues); got.Unregistered != 1 || got.Failed != 2 {
		t.Fatalf("本数 = %+v, want 失敗2・未登録1", got)
	}
}

func TestPageScanIssues(t *testing.T) {
	media := filepath.FromSlash("/media")
	records := []ScanIssueRecord{
		{Path: filepath.Join(media, "b", "x.mp4"), Kinds: []ScanIssueKind{IssueUnreadable}},
		{Path: filepath.Join(media, "b", "a.mp4"), Kinds: []ScanIssueKind{IssueThumbnailFirstFrame}},
		{Path: filepath.Join(media, "z.mp4"), Kinds: []ScanIssueKind{IssueUnreadable}},
		{Path: filepath.Join(media, "a", "x.mp4"), Kinds: []ScanIssueKind{IssueUnreadable}},
		{Path: filepath.Join(media, "a", "a.mp4"), Kinds: []ScanIssueKind{IssueThumbnailFirstFrame}},
	}
	issues := ScanIssues([]MediaFolder{{ID: 1, Path: media}}, records)
	want := []string{"a/x.mp4", "b/x.mp4", "z.mp4", "a/a.mp4", "b/a.mp4"}
	for _, limit := range []int{1, 2} {
		var paths []string
		cursor := ""
		for pages := 0; ; pages++ {
			if pages >= len(records) {
				t.Fatal("cursor did not finish")
			}
			page, next, err := PageScanIssues(issues, cursor, limit)
			if err != nil {
				t.Fatal(err)
			}
			if len(page) > limit || (next != "" && len(page) != limit) {
				t.Fatalf("page has %d items, limit %d, next %q", len(page), limit, next)
			}
			for _, issue := range page {
				rel, err := filepath.Rel(media, issue.Path)
				if err != nil {
					t.Fatal(err)
				}
				paths = append(paths, filepath.ToSlash(rel))
			}
			if next == "" {
				break
			}
			cursor = next
		}
		if !slices.Equal(paths, want) {
			t.Fatalf("limit %d: paths = %v, want %v", limit, paths, want)
		}
	}

	_, next, err := PageScanIssues(issues, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	page, _, err := PageScanIssues(slices.Delete(slices.Clone(issues), 1, 2), next, 2)
	if err != nil || len(page) == 0 || page[0].FileName != "z.mp4" {
		t.Fatalf("消えた件のあと = %+v, %v, want z.mp4 から", page, err)
	}

	for _, bad := range []string{"%%%", "bm90IGpzb24", "eyJzIjo1fQ"} {
		if _, _, err := PageScanIssues(issues, bad, 2); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("cursor=%q: err = %v, want ErrInvalidCursor", bad, err)
		}
	}
}
