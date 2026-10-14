package httpapi

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// POST /api/organize/bundle（specs/050-drag-organize/contracts/screen-api.md）を、本物の保存層と
// internal/app の Organizer をつないで確かめる。

// versionsOf は所有者として動画 id の集まりを読み、代表と本数を返す。
func (f *libraryFixture) versionsOf(t *testing.T, id int64) (int64, int) {
	t.Helper()
	rec := f.groupRequest(http.MethodGet, fmt.Sprintf("/api/videos/%d/versions", id), "", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("集まりの読み出し: status = %d: %s", rec.Code, rec.Body)
	}
	versions := decode[gen.VideoVersions](t, rec)
	return versions.RepresentativeId, len(versions.Items)
}

// 動画を動画に落とすと 200 で、落とした先を代表にした集まりができ、取り消しは 204 で集まりを解く。
func TestOrganizeBundleAndUndo(t *testing.T) {
	f := newLibraryFixture(t)
	rec := f.organize("bundle", map[string]any{"videoIds": []int64{f.ids["p1"]}, "target": f.ids["p2"]}, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("束ね: status = %d: %s", rec.Code, rec.Body)
	}
	result := decode[gen.OrganizeResult](t, rec)
	if result.UndoToken == "" || result.GroupId != nil {
		t.Fatalf("束ねの応答 = %+v", result)
	}
	if representative, count := f.versionsOf(t, f.ids["p1"]); representative != f.ids["p2"] || count != 2 {
		t.Fatalf("束ねたあと: 代表 %d・%d 本", representative, count)
	}
	f.undone(t, "束ねの取り消し", result.UndoToken)
	if representative, count := f.versionsOf(t, f.ids["p1"]); representative != f.ids["p1"] || count != 1 {
		t.Fatalf("取り消したあと: 代表 %d・%d 本", representative, count)
	}
}

// 本文の誤りと落とした先だけは 400 invalid_request、21 本以上は 400 too_many_videos、無い動画は 404
// group_target_not_found で、どれも何も束ねない。ゲストは 401 である。
func TestOrganizeBundleErrors(t *testing.T) {
	f := newLibraryFixture(t)
	tooMany := make([]int64, 21)
	for i := range tooMany {
		tooMany[i] = int64(1000 + i)
	}
	tooManyVideos := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos, limit: 20}
	for _, tc := range []struct {
		label string
		body  map[string]any
		want  wantError
	}{
		{"videoIds なし", map[string]any{"target": f.ids["p2"]}, wantGroupInvalid},
		{"target なし", map[string]any{"videoIds": []int64{f.ids["p1"]}}, wantGroupInvalid},
		{"target が 0", map[string]any{"videoIds": []int64{f.ids["p1"]}, "target": 0}, wantGroupInvalid},
		{"空の videoIds", map[string]any{"videoIds": []int64{}, "target": f.ids["p2"]}, wantGroupInvalid},
		{"落とした先だけ", map[string]any{"videoIds": []int64{f.ids["p2"]}, "target": f.ids["p2"]}, wantGroupInvalid},
		{"知らない欄", map[string]any{"videoIds": []int64{f.ids["p1"]}, "target": f.ids["p2"], "from": 1}, wantGroupInvalid},
		{"21 本", map[string]any{"videoIds": tooMany, "target": f.ids["p2"]}, tooManyVideos},
		{"無い動画", map[string]any{"videoIds": []int64{999999}, "target": f.ids["p2"]}, wantGroupTargetNotFound},
	} {
		assertGroupError(t, tc.label, f.organize("bundle", tc.body, f.owner), tc.want)
	}
	assertUnauthenticated(t, "bundle", f.organize("bundle", map[string]any{"videoIds": []int64{f.ids["p1"]}, "target": f.ids["p2"]}))
	if _, count := f.versionsOf(t, f.ids["p2"]); count != 1 {
		t.Fatalf("拒まれた束ねのあと: %d 本", count)
	}
}
