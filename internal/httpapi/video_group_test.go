package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

func newVideoGroupFixture(t *testing.T) *libraryFixture {
	t.Helper()
	return newLibraryFixtureWith(t, withCatalog)
}

func withCatalog(options Options) Options {
	library, ok := options.Videos.(*store.LibraryStore)
	if !ok {
		panic("Videos が *store.LibraryStore ではない")
	}
	options.Catalog = app.NewCatalog(app.CatalogOptions{Index: library, Ingest: nil, Files: guestArtifactFiles{}})
	return options
}

func videoPath(id int64) string { return "/api/videos/" + strconv.FormatInt(id, 10) }

func relatedPath(id int64) string { return videoPath(id) + "/related" }

func videoIDs(videos []gen.Video) []int64 {
	ids := make([]int64, 0, len(videos))
	for _, video := range videos {
		ids = append(ids, video.Id)
	}
	return ids
}

func getJSON[T any](t *testing.T, f *libraryFixture, target string, cookies ...*http.Cookie) (T, map[string]json.RawMessage) {
	t.Helper()
	rec := f.env.get(target, cookies...)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", target, rec.Code, rec.Body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	return decode[T](t, rec), raw
}

func groupRefString(ref *gen.VideoGroupRef) string {
	if ref == nil {
		return "省略"
	}
	folder := "無し"
	if ref.Folder != nil {
		folder = fmt.Sprintf("%d:%s", ref.Folder.RootId, ref.Folder.Path)
	}
	return fmt.Sprintf("id=%d folder=%s name=%s %d/%d", ref.Id, folder, ref.Name, ref.Position, ref.Count)
}

func idPtrString(id *int64) string {
	if id == nil {
		return "省略"
	}
	return strconv.FormatInt(*id, 10)
}

func TestVideoGroupForOwner(t *testing.T) {
	f := newVideoGroupFixture(t)
	members := []int64{f.ids["ep1"], f.ids["ep2"], f.ids["ep10"]}
	show := f.groupID(t, "show")

	for index, id := range members {
		video, _ := getJSON[gen.Video](t, f, videoPath(id), f.owner)
		want := gen.VideoGroupRef{
			Id: show, Folder: &gen.VideoFolder{RootId: f.rootID, Path: "show"}, Name: "show", Position: index + 1, Count: 3,
		}
		if got := groupRefString(video.Group); got != groupRefString(&want) {
			t.Errorf("動画 %d の group = %s, want %s", id, got, groupRefString(&want))
		}

		related, _ := getJSON[gen.RelatedVideos](t, f, relatedPath(id), f.owner)
		if related.Group == nil {
			t.Fatalf("動画 %d の関連動画に group が無い", id)
		}
		if related.Group.Name != "show" || related.Group.Id != show {
			t.Errorf("関連動画の group = %+v", related.Group)
		}
		if got := videoIDs(related.Group.Items); !slices.Equal(got, members) {
			t.Errorf("group.items = %v, want %v", got, members)
		}
		if related.Group.Items[0].Progress == nil || len(related.Group.Items[0].Tags) == 0 {
			t.Errorf("group.items に再生位置かタグが無い: %+v", related.Group.Items[0])
		}
		var wantPrev, wantNext *int64
		if index > 0 {
			wantPrev = &members[index-1]
		}
		if index < len(members)-1 {
			wantNext = &members[index+1]
		}
		if idPtrString(related.PrevId) != idPtrString(wantPrev) || idPtrString(related.NextId) != idPtrString(wantNext) {
			t.Errorf("動画 %d の前後 = %s・%s, want %s・%s", id,
				idPtrString(related.PrevId), idPtrString(related.NextId), idPtrString(wantPrev), idPtrString(wantNext))
		}
		others := []int64{f.ids["solo"], f.ids["p2"], f.ids["p1"]}
		if got := videoIDs(related.Items); len(got) != 3 || !sameSet(got, others) {
			t.Errorf("動画 %d の関連動画 = %v, want %v の並べ替え（グループのメンバーを含まない）", id, got, others)
		}
	}
}

func sameSet(a, b []int64) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func TestVideoGroupAbsentForNonMember(t *testing.T) {
	f := newVideoGroupFixture(t)
	solo := f.ids["solo"]

	video, raw := getJSON[gen.Video](t, f, videoPath(solo), f.owner)
	if video.Group != nil {
		t.Errorf("group = %+v, want 省略", video.Group)
	}
	if _, ok := raw["group"]; ok {
		t.Error("JSON に group がある")
	}

	related, rawRelated := getJSON[gen.RelatedVideos](t, f, relatedPath(solo), f.owner)
	if _, ok := rawRelated["group"]; ok || related.Group != nil {
		t.Errorf("関連動画の group = %+v, want 省略", related.Group)
	}
	// solo has no title, tag, or fingerprint clues; same-day results sort newest first.
	want := []int64{f.ids["p2"], f.ids["p1"], f.ids["ep10"], f.ids["ep2"], f.ids["ep1"]}
	if got := videoIDs(related.Items); !slices.Equal(got, want) {
		t.Errorf("関連動画 = %v, want %v", got, want)
	}
	if idPtrString(related.NextId) != strconv.FormatInt(want[0], 10) || related.PrevId != nil {
		t.Errorf("前後 = %s・%s, want 省略・%d", idPtrString(related.PrevId), idPtrString(related.NextId), want[0])
	}
}

func TestVideoGroupForGuest(t *testing.T) {
	f := newVideoGroupFixture(t)
	public := []int64{f.ids["ep1"], f.ids["ep2"]}
	show := f.groupID(t, "show")

	for index, id := range public {
		video, _ := getJSON[gen.Video](t, f, videoPath(id))
		want := gen.VideoGroupRef{
			Id: show, Folder: &gen.VideoFolder{RootId: f.rootID, Path: "show"}, Name: "show", Position: index + 1, Count: 2,
		}
		if got := groupRefString(video.Group); got != groupRefString(&want) {
			t.Errorf("ゲストの動画 %d の group = %s, want %s", id, got, groupRefString(&want))
		}
	}

	related, _ := getJSON[gen.RelatedVideos](t, f, relatedPath(f.ids["ep2"]))
	if related.Group == nil {
		t.Fatal("ゲストの関連動画に group が無い")
	}
	if got := videoIDs(related.Group.Items); !slices.Equal(got, public) {
		t.Errorf("ゲストの group.items = %v, want %v", got, public)
	}
	for _, item := range related.Group.Items {
		if item.Progress != nil || len(item.Tags) != 0 || item.Location != nil {
			t.Errorf("ゲストの group.items に所有者のデータがある: %+v", item)
		}
	}
	if idPtrString(related.PrevId) != strconv.FormatInt(f.ids["ep1"], 10) || related.NextId != nil {
		t.Errorf("ゲストの ep2 の前後 = %s・%s, want ep1・省略", idPtrString(related.PrevId), idPtrString(related.NextId))
	}
	if got := videoIDs(related.Items); !sameSet(got, []int64{f.ids["p1"], f.ids["solo"]}) {
		t.Errorf("ゲストの関連動画 = %v, want p1 と solo", got)
	}

	p1 := f.ids["p1"]
	video, _ := getJSON[gen.Video](t, f, videoPath(p1))
	if video.Group == nil || video.Group.Count != 1 || video.Group.Position != 1 {
		t.Errorf("ゲストの p1 の group = %s, want 1/1", groupRefString(video.Group))
	}
	pair, _ := getJSON[gen.RelatedVideos](t, f, relatedPath(p1))
	if pair.Group == nil || !slices.Equal(videoIDs(pair.Group.Items), []int64{p1}) {
		t.Errorf("ゲストの p1 の関連動画の group = %+v, want p1 だけ", pair.Group)
	}
	if pair.NextId != nil || pair.PrevId != nil {
		t.Errorf("ゲストの p1 の前後 = %s・%s, want 省略・省略", idPtrString(pair.PrevId), idPtrString(pair.NextId))
	}

	owner, _ := getJSON[gen.Video](t, f, videoPath(p1), f.owner)
	if owner.Group == nil || owner.Group.Count != 2 || owner.Group.Position != 1 {
		t.Errorf("所有者の p1 の group = %+v", owner.Group)
	}
}

func TestVideoGroupLargerThanRelatedLimit(t *testing.T) {
	ctx := context.Background()
	mediaDir := t.TempDir()
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		return withCatalog(Options{Videos: db.Library(), Folders: db.Library(), Playback: db.Playback(), Tags: db.Tags()})
	})
	owner := env.setup()
	if _, err := env.db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var members, others []int64
	add := func(rel string, index int) int64 {
		path := filepath.Join(mediaDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		result, err := env.db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: path, Title: filepath.Base(rel), ContentKey: "key-" + rel, SizeBytes: 1,
			MTime: base, AddedAt: base.Add(time.Duration(index) * time.Minute), Container: "mp4",
		})
		if err != nil {
			t.Fatal(err)
		}
		return result.ID
	}
	const size = 25
	for i := range size {
		members = append(members, add(fmt.Sprintf("big/%02d.mp4", i+1), i))
	}
	for i := range 3 {
		others = append(others, add(fmt.Sprintf("else/%d/x.mp4", i), size+i))
	}
	if err := env.db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}

	rec := env.get(relatedPath(members[size-1]), owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	related := decode[gen.RelatedVideos](t, rec)
	if related.Group == nil || !slices.Equal(videoIDs(related.Group.Items), members) {
		t.Fatalf("group = %+v, want %d 本の全メンバー", related.Group, size)
	}
	if related.NextId != nil || related.PrevId == nil || *related.PrevId != members[size-2] {
		t.Errorf("最後のメンバーの前後 = %s・%s", idPtrString(related.PrevId), idPtrString(related.NextId))
	}
	if got := videoIDs(related.Items); !sameSet(got, others) {
		t.Errorf("関連動画 = %v, want グループの外の %v", got, others)
	}

	video := decode[gen.Video](t, env.get(videoPath(members[0]), owner))
	if video.Group == nil || video.Group.Position != 1 || video.Group.Count != size {
		t.Errorf("group = %+v", video.Group)
	}
}

func TestListVideoGroupMembers(t *testing.T) {
	f := newVideoGroupFixture(t)
	members := []int64{f.ids["ep1"], f.ids["ep2"], f.ids["ep10"]}

	related, _ := getJSON[gen.RelatedVideos](t, f, relatedPath(f.ids["ep2"]), f.owner)
	if related.Group == nil || related.Group.Offset != 0 || related.Group.Total != 3 {
		t.Fatalf("関連動画の group = %+v, want offset 0・total 3", related.Group)
	}

	page, _ := getJSON[gen.GroupMemberPage](t, f, videoPath(f.ids["ep2"])+"/group-members?offset=1&limit=5", f.owner)
	if page.Offset != 1 || page.Total != 3 || !slices.Equal(videoIDs(page.Items), members[1:]) {
		t.Errorf("page = offset %d・total %d・%v, want 1・3・%v", page.Offset, page.Total, videoIDs(page.Items), members[1:])
	}
	if page.Items[0].Progress == nil || len(page.Items[0].Tags) == 0 {
		t.Errorf("items に再生位置かタグが無い: %+v", page.Items[0])
	}
	past, _ := getJSON[gen.GroupMemberPage](t, f, videoPath(f.ids["ep2"])+"/group-members?offset=9", f.owner)
	if past.Offset != 3 || past.Total != 3 || len(past.Items) != 0 {
		t.Errorf("本数を超える offset = %+v, want 空の items", past)
	}

	guest, _ := getJSON[gen.GroupMemberPage](t, f, videoPath(f.ids["ep1"])+"/group-members")
	if guest.Total != 2 || !slices.Equal(videoIDs(guest.Items), members[:2]) {
		t.Errorf("ゲストの page = total %d・%v, want 2・%v", guest.Total, videoIDs(guest.Items), members[:2])
	}
	for _, item := range guest.Items {
		if item.Progress != nil || len(item.Tags) != 0 || item.Location != nil {
			t.Errorf("ゲストの items に所有者のデータがある: %+v", item)
		}
	}

	if rec := f.env.get(videoPath(f.ids["solo"])+"/group-members", f.owner); rec.Code != http.StatusNotFound {
		t.Errorf("メンバーでない動画: status = %d, want 404", rec.Code)
	}
	for _, query := range []string{"?offset=-1", "?limit=0", "?limit=201"} {
		if rec := f.env.get(videoPath(f.ids["ep1"])+"/group-members"+query, f.owner); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, rec.Code)
		}
	}
}

func TestRelatedVideosRankAndPage(t *testing.T) {
	f := newVideoGroupFixture(t)
	solo := f.ids["solo"]
	if _, _, err := f.env.db.Tags().AttachTagByID(context.Background(), []int64{solo}, f.manual); err != nil {
		t.Fatal(err)
	}

	first, _ := getJSON[gen.RelatedVideos](t, f, relatedPath(solo)+"?limit=2", f.owner)
	if len(first.Items) != 2 || first.Items[0].Id != f.ids["ep1"] || first.NextCursor == nil {
		t.Fatalf("最初のページ = %v・%v, want ep1 が先頭で続きあり", videoIDs(first.Items), first.NextCursor)
	}
	if idPtrString(first.NextId) != strconv.FormatInt(f.ids["ep1"], 10) {
		t.Errorf("nextId = %s, want ep1", idPtrString(first.NextId))
	}
	seen := videoIDs(first.Items)
	cursor := *first.NextCursor
	for range 5 {
		page, raw := getJSON[gen.RelatedVideos](t, f, relatedPath(solo)+"?limit=2&cursor="+cursor, f.owner)
		for _, key := range []string{"nextId", "prevId", "group"} {
			if _, ok := raw[key]; ok {
				t.Errorf("続きのページに %s がある", key)
			}
		}
		seen = append(seen, videoIDs(page.Items)...)
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	want := []int64{f.ids["ep1"], f.ids["ep2"], f.ids["ep10"], f.ids["p1"], f.ids["p2"]}
	if len(seen) != len(want) || !sameSet(seen, want) {
		t.Fatalf("読んだ動画 = %v, want 重複なく %v", seen, want)
	}
}
