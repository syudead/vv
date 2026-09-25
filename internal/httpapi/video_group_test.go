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

// 動画と関連動画の応答のグループ（specs/017-folder-groups/contracts/folder-groups-api.md §3）を、
// 本物の認証・保存層・関連動画の組み立て（app.Catalog）をつないで確かめる。中身は
// newLibraryFixture と同じで、show は ep1・ep2（公開）と ep10（非公開）、pair は p1（公開）と
// p2（非公開）のグループである。
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

func idPtrString(id *int64) string {
	if id == nil {
		return "省略"
	}
	return strconv.FormatInt(*id, 10)
}

// 所有者: メンバーの動画に何本目かが入り、関連動画にはグループの全メンバーが並びの順で
// 入る。前後はグループの中の並びで、最初に前、最後に次が無い。関連動画の items には
// 同じグループのメンバーが無い。
func TestVideoGroupForOwner(t *testing.T) {
	f := newVideoGroupFixture(t)
	members := []int64{f.ids["ep1"], f.ids["ep2"], f.ids["ep10"]}

	for index, id := range members {
		video, _ := getJSON[gen.Video](t, f, videoPath(id), f.owner)
		want := gen.VideoGroupRef{
			Folder: gen.VideoFolder{RootId: f.rootID, Path: "show"}, Name: "show", Position: index + 1, Count: 3,
		}
		if video.Group == nil || *video.Group != want {
			t.Errorf("動画 %d の group = %+v, want %+v", id, video.Group, want)
		}

		related, _ := getJSON[gen.RelatedVideos](t, f, relatedPath(id), f.owner)
		if related.Group == nil {
			t.Fatalf("動画 %d の関連動画に group が無い", id)
		}
		if related.Group.Name != "show" || related.Group.Folder != (gen.VideoFolder{RootId: f.rootID, Path: "show"}) {
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

// 受け入れ条件 17: グループに属さない動画の応答は今と同じで、group が無く、関連動画は
// 同じフォルダと追加日時の近い動画から並ぶ（グループのメンバーも除かない）。
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
	// solo は登録フォルダ直下に1本だけで、追加日時は最後。差の小さい順に並ぶ。
	want := []int64{f.ids["p2"], f.ids["p1"], f.ids["ep10"], f.ids["ep2"], f.ids["ep1"]}
	if got := videoIDs(related.Items); !slices.Equal(got, want) {
		t.Errorf("関連動画 = %v, want %v", got, want)
	}
	if related.NextId != nil || related.PrevId != nil {
		t.Errorf("前後 = %s・%s, want 省略", idPtrString(related.PrevId), idPtrString(related.NextId))
	}
}

// ゲスト: group と前後は公開のメンバーだけで作る。公開のメンバーが1本のグループ（pair）は
// group を省き、グループに属さない動画と同じ応答にする（data-model.md §7）。
func TestVideoGroupForGuest(t *testing.T) {
	f := newVideoGroupFixture(t)
	public := []int64{f.ids["ep1"], f.ids["ep2"]}

	for index, id := range public {
		video, _ := getJSON[gen.Video](t, f, videoPath(id))
		want := gen.VideoGroupRef{
			Folder: gen.VideoFolder{RootId: f.rootID, Path: "show"}, Name: "show", Position: index + 1, Count: 2,
		}
		if video.Group == nil || *video.Group != want {
			t.Errorf("ゲストの動画 %d の group = %+v, want %+v", id, video.Group, want)
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

	// pair は公開のメンバーが p1 だけなので、ゲストにはグループが無い。
	p1 := f.ids["p1"]
	video, raw := getJSON[gen.Video](t, f, videoPath(p1))
	if _, ok := raw["group"]; ok || video.Group != nil {
		t.Errorf("ゲストの p1 の group = %+v, want 省略", video.Group)
	}
	pair, rawPair := getJSON[gen.RelatedVideos](t, f, relatedPath(p1))
	if _, ok := rawPair["group"]; ok || pair.Group != nil {
		t.Errorf("ゲストの p1 の関連動画の group = %+v, want 省略", pair.Group)
	}
	if pair.NextId != nil || pair.PrevId != nil {
		t.Errorf("ゲストの p1 の前後 = %s・%s, want 省略", idPtrString(pair.PrevId), idPtrString(pair.NextId))
	}
	// グループに属さない動画と同じく、追加日時の近い公開の動画から並ぶ（差が同じなら id の大きい方が先）。
	if got := videoIDs(pair.Items); !slices.Equal(got, []int64{f.ids["solo"], f.ids["ep2"], f.ids["ep1"]}) {
		t.Errorf("ゲストの p1 の関連動画 = %v", got)
	}

	// 所有者には pair が2本のグループである。
	owner, _ := getJSON[gen.Video](t, f, videoPath(p1), f.owner)
	if owner.Group == nil || owner.Group.Count != 2 || owner.Group.Position != 1 {
		t.Errorf("所有者の p1 の group = %+v", owner.Group)
	}
}

// Edge Case「大きなグループ」: group.items には上限を掛けず全メンバーが並び、関連動画は
// メンバーを除いたうえで残る。
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
	const size = domain.MaxRelatedVideos + 5
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
