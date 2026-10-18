package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func versionFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	paths := upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 2),
		listingFile(fixturePath("/media/c.mp4"), "c", "key-c", 3),
		listingFile(fixturePath("/media/d.mp4"), "d", "key-d", 4),
	)
	ids := map[string]int64{}
	for _, name := range []string{"a", "b", "c", "d"} {
		ids[name] = paths[fixturePath("/media/"+name+".mp4")]
	}
	return db, ids
}

func attachNamedTag(t *testing.T, db *DB, name string, ids ...int64) int64 {
	t.Helper()
	ref, _, err := db.Tags().AttachTagByName(context.Background(), ids, name)
	if err != nil {
		t.Fatalf("タグ %s を付けられない: %v", name, err)
	}
	return ref.ID
}

func bundle(t *testing.T, db *DB, representative int64, ids ...int64) domain.VideoVersions {
	t.Helper()
	versions, err := db.Versions().Bundle(context.Background(), ids, representative)
	if err != nil {
		t.Fatalf("束ねられない: %v", err)
	}
	return versions
}

func ownerVideo(t *testing.T, db *DB, id int64) domain.Video {
	t.Helper()
	video, err := db.Library().GetVideo(context.Background(), domain.AudienceOwner, id)
	if err != nil {
		t.Fatalf("動画 %d を読めない: %v", id, err)
	}
	return video
}

func manualTagNames(t *testing.T, db *DB, key string) []string {
	t.Helper()
	tags, err := db.Tags().TagsByContentKeys(context.Background(), []string{key})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tag := range tags[key] {
		if tag.Manual {
			names = append(names, tag.Name)
		}
	}
	return names
}

func bundleCount(t *testing.T, db *DB) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from video_bundles`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func listedIDs(t *testing.T, db *DB, q domain.VideoQuery) []int64 {
	t.Helper()
	q.Limit = domain.MaxLimit
	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, q)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	slices.Sort(ids)
	return ids
}

func TestBundleKeysUserDataByTheBundle(t *testing.T) {
	db, ids := versionFixture(t)
	x := attachNamedTag(t, db, "X", ids["a"])
	y := attachNamedTag(t, db, "Y", ids["b"])

	versions := bundle(t, db, ids["a"], ids["a"], ids["b"])
	if versions.RepresentativeID != ids["a"] || len(versions.Items) != 2 ||
		versions.Items[0].ID != ids["a"] || versions.Items[1].ID != ids["b"] {
		t.Fatalf("versions = %+v", versions)
	}

	a, b := ownerVideo(t, db, ids["a"]), ownerVideo(t, db, ids["b"])
	if a.UserKey == a.ContentKey || a.UserKey != b.UserKey || len(a.UserKey) < len("bundle:") || a.UserKey[:7] != "bundle:" {
		t.Fatalf("UserKey = %q / %q", a.UserKey, b.UserKey)
	}
	if got := manualTagNames(t, db, a.UserKey); !slices.Equal(got, []string{"X"}) {
		t.Errorf("集まりのタグ = %v, want [X]", got)
	}
	if got := manualTagNames(t, db, b.ContentKey); !slices.Equal(got, []string{"Y"}) {
		t.Errorf("B の content_key のタグ = %v, want [Y]", got)
	}
	if b.Versions == nil || b.Versions.Count != 2 || b.Versions.RepresentativeID != ids["a"] {
		t.Errorf("B の Versions = %+v", b.Versions)
	}
	if got := listedIDs(t, db, domain.VideoQuery{TagIDs: []int64{x}}); !slices.Equal(got, []int64{ids["a"]}) {
		t.Errorf("タグ X で絞った動画 = %v", got)
	}
	if got := listedIDs(t, db, domain.VideoQuery{TagIDs: []int64{y}}); len(got) != 0 {
		t.Errorf("タグ Y で絞った動画 = %v, want なし", got)
	}
	if got, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit}); err != nil {
		t.Fatal(err)
	} else {
		for _, item := range got.Items {
			if item.ID == ids["b"] && item.UserKey != a.UserKey {
				t.Errorf("一覧の B の UserKey = %q", item.UserKey)
			}
			if item.ID == ids["a"] && (item.Versions == nil || item.Versions.Count != 2) {
				t.Errorf("一覧の A の Versions = %+v, want count 2", item.Versions)
			}
		}
	}

	summary, err := db.Tags().Summary(context.Background(), []int64{ids["a"], ids["b"]})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 1 || len(summary.Items) != 1 || summary.Items[0].Tag.ID != x || summary.Items[0].Count != 1 {
		t.Errorf("summary = %+v", summary)
	}
	if _, applied, err := db.Tags().AttachTagByName(context.Background(), []int64{ids["b"]}, "Z"); err != nil || applied != 1 {
		t.Fatalf("B への付与 = %d, %v", applied, err)
	}
	if got := manualTagNames(t, db, a.UserKey); !slices.Equal(got, []string{"X", "Z"}) {
		t.Errorf("B に付けたあとの集まりのタグ = %v, want [X Z]", got)
	}
}

func TestMakeRepresentativeKeepsBundleValues(t *testing.T) {
	db, ids := versionFixture(t)
	attachNamedTag(t, db, "X", ids["a"])
	attachNamedTag(t, db, "Y", ids["b"])
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	before := ownerVideo(t, db, ids["a"]).UserKey

	versions, err := db.Versions().MakeRepresentative(context.Background(), ids["b"])
	if err != nil {
		t.Fatal(err)
	}
	if versions.RepresentativeID != ids["b"] || versions.Items[0].ID != ids["b"] {
		t.Fatalf("versions = %+v", versions)
	}
	for _, id := range []int64{ids["a"], ids["b"]} {
		if got := ownerVideo(t, db, id).UserKey; got != before {
			t.Errorf("動画 %d の UserKey = %q, want %q", id, got, before)
		}
	}
	if got := manualTagNames(t, db, before); !slices.Equal(got, []string{"X"}) {
		t.Errorf("集まりのタグ = %v, want [X]", got)
	}
	if _, err := db.Versions().MakeRepresentative(context.Background(), ids["c"]); !errors.Is(err, domain.ErrNotBundled) {
		t.Errorf("束ねていない動画 = %v, want ErrNotBundled", err)
	}
}

func TestUnbundleRestoresOwnValues(t *testing.T) {
	db, ids := versionFixture(t)
	attachNamedTag(t, db, "X", ids["a"])
	attachNamedTag(t, db, "Y", ids["b"])
	bundle(t, db, ids["a"], ids["a"], ids["b"], ids["c"])
	bundleKey := ownerVideo(t, db, ids["a"]).UserKey

	b, err := db.Versions().Unbundle(context.Background(), ids["b"])
	if err != nil {
		t.Fatal(err)
	}
	if b.UserKey != b.ContentKey || b.Versions != nil {
		t.Fatalf("外した B の UserKey = %q, Versions = %+v", b.UserKey, b.Versions)
	}
	if got := manualTagNames(t, db, b.UserKey); !slices.Equal(got, []string{"Y"}) {
		t.Errorf("外した B のタグ = %v, want [Y]", got)
	}
	if got := manualTagNames(t, db, bundleKey); !slices.Equal(got, []string{"X"}) {
		t.Errorf("集まりのタグ = %v, want [X]", got)
	}

	if _, err := db.Versions().Unbundle(context.Background(), ids["a"]); err != nil {
		t.Fatal(err)
	}
	if bundleCount(t, db) != 0 {
		t.Errorf("集まりが残っている")
	}
	if _, err := db.Versions().Unbundle(context.Background(), ids["b"]); !errors.Is(err, domain.ErrNotBundled) {
		t.Errorf("束ねていない動画 = %v, want ErrNotBundled", err)
	}
}

func TestUnbundleRepresentativeChoosesNext(t *testing.T) {
	db, ids := versionFixture(t)
	bundle(t, db, ids["b"], ids["a"], ids["b"], ids["c"])
	if _, err := db.Versions().Unbundle(context.Background(), ids["b"]); err != nil {
		t.Fatal(err)
	}
	versions, err := db.Versions().Versions(context.Background(), domain.AudienceOwner, ids["c"])
	if err != nil {
		t.Fatal(err)
	}
	if versions.RepresentativeID != ids["a"] || len(versions.Items) != 2 {
		t.Errorf("versions = %+v, want 代表 a の 2 本", versions)
	}
}

func TestUnbundleDissolvesAndCopiesValues(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	attachNamedTag(t, db, "X", ids["a"])
	attachNamedTag(t, db, "Y", ids["b"])
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	bundleKey := ownerVideo(t, db, ids["a"]).UserKey
	if _, err := db.Playback().SaveProgress(ctx, bundleKey, domain.Progress{PositionMs: 4000, DurationMs: 10000}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{ids["a"]}, true); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Versions().Unbundle(ctx, ids["a"]); err != nil {
		t.Fatal(err)
	}
	if bundleCount(t, db) != 0 {
		t.Fatal("集まりが残っている")
	}
	b := ownerVideo(t, db, ids["b"])
	if b.UserKey != b.ContentKey || !b.Public {
		t.Errorf("残った B: UserKey %q, Public %v", b.UserKey, b.Public)
	}
	if got := manualTagNames(t, db, b.ContentKey); !slices.Equal(got, []string{"X"}) {
		t.Errorf("残った B のタグ = %v, want [X]（集まりの値で置き換える）", got)
	}
	if progress, err := db.Playback().Progress(ctx, b.ContentKey); err != nil || progress.PositionMs != 4000 {
		t.Errorf("残った B の再生位置 = %+v, %v", progress, err)
	}
	a := ownerVideo(t, db, ids["a"])
	if a.UserKey != a.ContentKey || a.Public {
		t.Errorf("外した A: UserKey %q, Public %v", a.UserKey, a.Public)
	}
}

func TestBundleMergesBundles(t *testing.T) {
	db, ids := versionFixture(t)
	attachNamedTag(t, db, "X", ids["a"])
	attachNamedTag(t, db, "Z", ids["c"])
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	bundle(t, db, ids["c"], ids["c"], ids["d"])
	firstKey := ownerVideo(t, db, ids["a"]).UserKey
	secondKey := ownerVideo(t, db, ids["c"]).UserKey

	versions := bundle(t, db, ids["c"], ids["a"], ids["c"])
	if len(versions.Items) != 4 || versions.RepresentativeID != ids["c"] {
		t.Fatalf("versions = %+v", versions)
	}
	if bundleCount(t, db) != 1 {
		t.Fatalf("集まりの数 = %d, want 1", bundleCount(t, db))
	}
	key := ownerVideo(t, db, ids["a"]).UserKey
	for _, name := range []string{"b", "c", "d"} {
		if got := ownerVideo(t, db, ids[name]).UserKey; got != key {
			t.Errorf("%s の UserKey = %q, want %q", name, got, key)
		}
	}
	if key == firstKey || key == secondKey {
		t.Errorf("新しい集まりの鍵 = %q が前の鍵と同じ", key)
	}
	if got := manualTagNames(t, db, key); !slices.Equal(got, []string{"Z"}) {
		t.Errorf("集まりのタグ = %v, want [Z]", got)
	}
	if got := manualTagNames(t, db, firstKey); !slices.Equal(got, []string{"X"}) {
		t.Errorf("吸収した集まりの鍵のタグ = %v, want [X]（消さない）", got)
	}
}

func TestSaveProgressOnBundleMember(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	b := ownerVideo(t, db, ids["b"])
	if _, err := db.Playback().SaveProgress(ctx, b.UserKey, domain.Progress{PositionMs: 3000, DurationMs: 10000}, nil); err != nil {
		t.Fatal(err)
	}
	a := ownerVideo(t, db, ids["a"])
	progress, err := db.Playback().ProgressByContentKeys(ctx, []string{a.UserKey})
	if err != nil {
		t.Fatal(err)
	}
	if progress[a.UserKey].PositionMs != 3000 {
		t.Errorf("A の再生位置 = %+v, want 3000", progress[a.UserKey])
	}
	if got := listedIDs(t, db, domain.VideoQuery{Watch: domain.WatchInProgress}); !slices.Equal(got, []int64{ids["a"]}) {
		t.Errorf("見かけの動画 = %v", got)
	}
}

func TestSetVideosPublicWritesBundleKey(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	bundle(t, db, ids["a"], ids["a"], ids["b"])

	keys, err := db.Visibility().SetVideosPublic(ctx, []int64{ids["b"]}, true)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"key-a", "key-b"}) {
		t.Errorf("返した鍵 = %v, want [key-a key-b]", keys)
	}
	bundleKey := ownerVideo(t, db, ids["a"]).UserKey
	var rows int
	if err := db.sql.QueryRow(`select count(*) from public_videos where content_key = ?`, bundleKey).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("集まりの鍵の公開の行 = %d, %v", rows, err)
	}
	for _, name := range []string{"a", "b"} {
		video, err := db.Library().GetVideo(ctx, domain.AudienceGuest, ids[name])
		if err != nil || !video.Public {
			t.Errorf("ゲストから見た %s = %+v, %v", name, video.Public, err)
		}
	}
	if _, err := db.Library().GetVideo(ctx, domain.AudienceGuest, ids["c"]); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("非公開の c = %v, want ErrNotFound", err)
	}
	guest, err := db.Versions().Versions(ctx, domain.AudienceGuest, ids["b"])
	if err != nil || len(guest.Items) != 2 {
		t.Errorf("ゲストのバージョン = %+v, %v", guest, err)
	}
}

func TestBundlePublishesOnceAndRejectsInvalidInput(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	for name, c := range map[string]struct {
		ids            []int64
		representative int64
		want           error
	}{
		"1 本":      {[]int64{ids["a"], ids["a"]}, ids["a"], domain.ErrTooFewVersions},
		"代表が含まれない": {[]int64{ids["a"], ids["b"]}, ids["c"], domain.ErrRepresentativeNotSelected},
		"無い動画":     {[]int64{ids["a"], 9999}, ids["a"], domain.ErrNotFound},
	} {
		if _, err := db.Versions().Bundle(ctx, c.ids, c.representative); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	if bundleCount(t, db) != 0 || len(recorder.events) != 0 {
		t.Fatalf("誤りで残した: 集まり %d, events %v", bundleCount(t, db), recorder.events)
	}

	bundle(t, db, ids["b"], ids["b"], ids["a"])
	bundled := domain.VideoBundleChanged{VideoIDs: []int64{ids["a"], ids["b"]}}
	want := []domain.Event{domain.ActivityChanged{}, bundled}
	if !slices.EqualFunc(recorder.events, want, eventsEqual) {
		t.Errorf("events = %v, want %v", recorder.events, want)
	}

	recorder.events = nil
	if _, err := db.Versions().Unbundle(ctx, ids["a"]); err != nil {
		t.Fatal(err)
	}
	want = []domain.Event{domain.ActivityChanged{}, bundled, domain.VersionCandidatesChanged{}}
	if !slices.EqualFunc(recorder.events, want, eventsEqual) {
		t.Errorf("解除の events = %v, want %v", recorder.events, want)
	}
}

func eventsEqual(a, b domain.Event) bool {
	x, okA := a.(domain.VideoBundleChanged)
	y, okB := b.(domain.VideoBundleChanged)
	if okA && okB {
		return slices.Equal(x.VideoIDs, y.VideoIDs)
	}
	return !okA && !okB && a == b
}

func TestExternalVideoTagsUseBundleKey(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	bundle(t, db, ids["a"], ids["a"], ids["b"])

	outcome, err := db.Tags().ApplyVideoTags(ctx, []domain.VideoRef{{ID: ids["b"]}}, domain.VideoTagsAdd, []string{"X"}, false)
	if err != nil {
		t.Fatal(err)
	}
	results := outcome.Items
	if len(results) != 1 || results[0].ContentKey != "key-b" || len(results[0].Tags) != 1 {
		t.Fatalf("results = %+v", results)
	}
	a := ownerVideo(t, db, ids["a"])
	if got := manualTagNames(t, db, a.UserKey); !slices.Equal(got, []string{"X"}) {
		t.Errorf("集まりのタグ = %v, want [X]", got)
	}
	if got := manualTagNames(t, db, "key-b"); len(got) != 0 {
		t.Errorf("B の content_key のタグ = %v, want なし", got)
	}
}
