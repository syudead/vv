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

// 代用のある段階は代表サムネイルとシーク用サムネイルだけで、その種類は代用の重さである。
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

// 所在がどの登録フォルダにも含まれない件は除き、失敗を先に、同じ重さの中はファイル名、
// 次にフォルダの順に並べる。種類は重い順に並び、失敗を1つでも含めば失敗になる。
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

// 走査の失敗でも、対象の集合に入っている動画は登録できなかったファイルに数えない。
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
	var records []ScanIssueRecord
	for _, name := range []string{"a.mp4", "b.mp4", "c.mp4", "d.mp4", "e.mp4"} {
		records = append(records, ScanIssueRecord{Path: filepath.Join(media, name), Kinds: []ScanIssueKind{IssueUnreadable}})
	}
	issues := ScanIssues([]MediaFolder{{ID: 1, Path: media}}, records)

	var names []string
	cursor := ""
	for {
		page, next, err := PageScanIssues(issues, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, issue := range page {
			names = append(names, issue.FileName)
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if want := []string{"a.mp4", "b.mp4", "c.mp4", "d.mp4", "e.mp4"}; !slices.Equal(names, want) {
		t.Fatalf("辿った件 = %v, want %v", names, want)
	}

	// カーソルは最後に返した件の位置なので、その件が消えても次の件から続く。
	_, next, err := PageScanIssues(issues, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	page, _, err := PageScanIssues(slices.Delete(slices.Clone(issues), 1, 2), next, 2)
	if err != nil || len(page) == 0 || page[0].FileName != "c.mp4" {
		t.Fatalf("消えた件のあと = %+v, %v, want c.mp4 から", page, err)
	}

	for _, bad := range []string{"%%%", "bm90IGpzb24", "eyJzIjo1fQ"} {
		if _, _, err := PageScanIssues(issues, bad, 2); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("cursor=%q: err = %v, want ErrInvalidCursor", bad, err)
		}
	}
}
