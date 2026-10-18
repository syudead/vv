package httpapi

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// IDs follow creation order; playback traverses nested groups depth-first.
func handMadeGroups(t *testing.T, f *libraryFixture) (g1, g2, parent int64) {
	t.Helper()
	ctx := context.Background()
	groups := f.env.db.Groups()
	create := func(name string, add domain.GroupAdd) int64 {
		group, err := groups.CreateGroup(ctx, name, add)
		if err != nil {
			t.Fatal(err)
		}
		return group.ID
	}
	g1 = create("G1", domain.GroupAdd{VideoIDs: []int64{f.ids["ep1"], f.ids["ep10"], f.ids["solo"], f.ids["ep2"]}})
	g2 = create("G2", domain.GroupAdd{VideoIDs: []int64{f.ids["solo"], f.ids["p1"]}})
	child := create("C", domain.GroupAdd{VideoIDs: []int64{f.ids["solo"], f.ids["p2"]}})
	parent = create("P", domain.GroupAdd{VideoIDs: []int64{f.ids["ep1"]}})
	for _, add := range []domain.GroupAdd{{GroupIDs: []int64{child}}, {VideoIDs: []int64{f.ids["p1"]}}} {
		if _, err := groups.AddGroupItems(ctx, parent, add); err != nil {
			t.Fatal(err)
		}
	}
	return g1, g2, parent
}

func inGroup(path string, group int64) string {
	return path + "?group=" + strconv.FormatInt(group, 10)
}

func wantInGroup(t *testing.T, f *libraryFixture, label, query string, id int64, prev, next *int64, ref gen.VideoGroupRef, cookies ...*http.Cookie) {
	t.Helper()
	related, _ := getJSON[gen.RelatedVideos](t, f, relatedPath(id)+query, cookies...)
	if idPtrString(related.PrevId) != idPtrString(prev) || idPtrString(related.NextId) != idPtrString(next) {
		t.Errorf("%s: 前後 = %s・%s, want %s・%s", label,
			idPtrString(related.PrevId), idPtrString(related.NextId), idPtrString(prev), idPtrString(next))
	}
	if related.Group == nil || related.Group.Id != ref.Id || related.Group.Total != ref.Count {
		t.Errorf("%s: 関連動画の group = %+v, want id %d・total %d", label, related.Group, ref.Id, ref.Count)
	}
	video, _ := getJSON[gen.Video](t, f, videoPath(id)+query, cookies...)
	if got := groupRefString(video.Group); got != groupRefString(&ref) {
		t.Errorf("%s: group = %s, want %s", label, got, groupRefString(&ref))
	}
}

func TestVideoGroupFollowsTheGroupItWasOpenedFrom(t *testing.T) {
	f := newVideoGroupFixture(t)
	g1, g2, _ := handMadeGroups(t, f)
	solo, ep2, ep10, p1 := f.ids["solo"], f.ids["ep2"], f.ids["ep10"], f.ids["p1"]
	inG1 := gen.VideoGroupRef{Id: g1, Name: "G1", Position: 3, Count: 4}

	wantInGroup(t, f, "group=G1", inGroup("", g1), solo, &ep10, &ep2, inG1, f.owner)
	wantInGroup(t, f, "group=G2", inGroup("", g2), solo, nil, &p1,
		gen.VideoGroupRef{Id: g2, Name: "G2", Position: 1, Count: 2}, f.owner)
	wantInGroup(t, f, "group 無し", "", solo, &ep10, &ep2, inG1, f.owner)
	wantInGroup(t, f, "動画を持たないグループ", inGroup("", f.groupID(t, "show")), solo, &ep10, &ep2, inG1, f.owner)
	wantInGroup(t, f, "無いグループ", inGroup("", 999999), solo, &ep10, &ep2, inG1, f.owner)

	page, _ := getJSON[gen.GroupMemberPage](t, f, inGroup(videoPath(solo)+"/group-members", g2), f.owner)
	if page.Total != 2 || !slices.Equal(videoIDs(page.Items), []int64{solo, p1}) {
		t.Errorf("group=G2 のメンバー = total %d・%v, want 2・[solo p1]", page.Total, videoIDs(page.Items))
	}
	related, _ := getJSON[gen.RelatedVideos](t, f, inGroup(relatedPath(solo), g2), f.owner)
	if got := videoIDs(related.Items); slices.Contains(got, p1) || !slices.Contains(got, ep2) {
		t.Errorf("group=G2 の関連動画 = %v, want p1 を除き ep2 を含む", got)
	}
}

func TestVideoGroupFollowsNestedPlayOrder(t *testing.T) {
	f := newVideoGroupFixture(t)
	_, _, parent := handMadeGroups(t, f)
	ep1, solo, p1, p2 := f.ids["ep1"], f.ids["solo"], f.ids["p1"], f.ids["p2"]
	query := inGroup("", parent)

	wantInGroup(t, f, "P の solo", query, solo, &ep1, &p2, gen.VideoGroupRef{Id: parent, Name: "P", Position: 2, Count: 4}, f.owner)
	wantInGroup(t, f, "P の p2", query, p2, &solo, &p1, gen.VideoGroupRef{Id: parent, Name: "P", Position: 3, Count: 4}, f.owner)

	page, _ := getJSON[gen.GroupMemberPage](t, f, inGroup(videoPath(p2)+"/group-members", parent)+"&offset=1&limit=2", f.owner)
	if page.Offset != 1 || page.Total != 4 || !slices.Equal(videoIDs(page.Items), []int64{solo, p2}) {
		t.Errorf("P のメンバーのページ = offset %d・total %d・%v, want 1・4・[solo p2]", page.Offset, page.Total, videoIDs(page.Items))
	}
}

func TestVideoGroupContextForGuestCountsPublicVideos(t *testing.T) {
	f := newVideoGroupFixture(t)
	g1, _, _ := handMadeGroups(t, f)
	solo, ep1, ep2 := f.ids["solo"], f.ids["ep1"], f.ids["ep2"]

	wantInGroup(t, f, "ゲストの G1", inGroup("", g1), solo, &ep1, &ep2, gen.VideoGroupRef{Id: g1, Name: "G1", Position: 2, Count: 3})
	page, _ := getJSON[gen.GroupMemberPage](t, f, inGroup(videoPath(solo)+"/group-members", g1))
	if page.Total != 3 || !slices.Equal(videoIDs(page.Items), []int64{ep1, solo, ep2}) {
		t.Errorf("ゲストの G1 のメンバー = total %d・%v, want 3・[ep1 solo ep2]", page.Total, videoIDs(page.Items))
	}
}
