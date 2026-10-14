package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// ドロップで「バージョンとしてまとめる」（specs/050-drag-organize/data-model.md「Operations」の Bundle）。

// bundleSnapshot は organizeSnapshot に、集まり・メンバー・タグ・お気に入り・候補の行を足す。
func bundleSnapshot(t *testing.T, db *DB) []string {
	t.Helper()
	out := organizeSnapshot(t, db)
	for _, query := range []string{
		`select id || '|' || user_key || '|' || representative_key from video_bundles order by id`,
		`select content_key || '|' || bundle_id from video_bundle_members order by content_key`,
		`select content_key || '|' || tag_id from video_tags order by content_key, tag_id`,
		`select content_key from video_favorites order by content_key`,
		`select key_a || '|' || key_b from video_version_candidates order by key_a, key_b`,
	} {
		rows, err := queryStringsForTest(db, query)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, rows...)
		out = append(out, "--")
	}
	return out
}

func organizeBundle(t *testing.T, db *DB, target int64, videoIDs ...int64) []byte {
	t.Helper()
	image, err := db.Organize().Bundle(context.Background(), videoIDs, target)
	if err != nil {
		t.Fatalf("Bundle(%v, %d) error = %v", videoIDs, target, err)
	}
	return image
}

// A を B に落とすと、B を代表にした集まりができ、B の値が集まりの鍵へ写る（受け入れ条件 3）。
// 確定後に 2 本の VideoBundleChanged を知らせる。
func TestOrganizeBundleMakesTargetRepresentative(t *testing.T) {
	db, ids := versionFixture(t)
	attachNamedTag(t, db, "X", ids["a"])
	attachNamedTag(t, db, "Y", ids["b"])
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	organizeBundle(t, db, ids["b"], ids["a"])

	b := ownerVideo(t, db, ids["b"])
	if b.Versions == nil || b.Versions.Count != 2 || b.Versions.RepresentativeID != ids["b"] {
		t.Fatalf("B の Versions = %+v", b.Versions)
	}
	if a := ownerVideo(t, db, ids["a"]); a.UserKey != b.UserKey {
		t.Fatalf("UserKey = %q / %q", a.UserKey, b.UserKey)
	}
	if got := manualTagNames(t, db, b.UserKey); !slices.Equal(got, []string{"Y"}) {
		t.Errorf("集まりのタグ = %v, want [Y]", got)
	}
	want := domain.VideoBundleChanged{VideoIDs: []int64{ids["a"], ids["b"]}}
	if len(recorder.events) == 0 || !eventsEqual(recorder.events[0], want) {
		t.Errorf("events = %+v, want first %+v", recorder.events, want)
	}
}

// 既にまとまっている A を B に落とすと、A の集まりの全メンバーごと B を代表にした集まりになり、
// 吸い込んだ集まりの行は消える（Edge Case）。
func TestOrganizeBundleBringsWholeBundle(t *testing.T) {
	db, ids := versionFixture(t)
	bundle(t, db, ids["a"], ids["a"], ids["c"])

	organizeBundle(t, db, ids["b"], ids["a"])

	if bundleCount(t, db) != 1 {
		t.Fatalf("集まりの数 = %d, want 1", bundleCount(t, db))
	}
	b := ownerVideo(t, db, ids["b"])
	if b.Versions == nil || b.Versions.Count != 3 || b.Versions.RepresentativeID != ids["b"] {
		t.Fatalf("B の Versions = %+v", b.Versions)
	}
	if c := ownerVideo(t, db, ids["c"]); c.UserKey != b.UserKey {
		t.Errorf("C の UserKey = %q, want %q", c.UserKey, b.UserKey)
	}
}

// 取り消すと、新しい集まりと写した利用者データが消え、吸い込んだ集まりが id ごと戻り、グループと
// プレイリストの項目と刈り取った候補が元の鍵で戻る。
func TestOrganizeBundleUndoRestoresEverything(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	bundle(t, db, ids["a"], ids["a"], ids["c"])
	attachNamedTag(t, db, "X", ids["a"])
	attachNamedTag(t, db, "Y", ids["b"])
	if _, err := db.Favorites().SetFavorites(ctx, domain.FavoriteChange{VideoIDs: []int64{ids["b"]}, Favorite: true}); err != nil {
		t.Fatal(err)
	}
	group := createGroup(t, db, "Mix", domain.GroupAdd{VideoIDs: []int64{ids["d"], ids["a"], ids["b"]}})
	playlist := createPlaylist(t, db, "List")
	if _, err := db.Playlists().AddPlaylistItems(ctx, playlist, domain.PlaylistAdd{VideoIDs: []int64{ids["b"], ids["d"], ids["c"]}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into video_version_candidates (key_a, key_b, distance, created_at) values
		('key-a', 'key-b', 1, 1), ('key-b', 'key-d', 2, 1)`); err != nil {
		t.Fatal(err)
	}
	before := bundleSnapshot(t, db)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	image := organizeBundle(t, db, ids["b"], ids["a"])
	if got := bundleSnapshot(t, db); slices.Equal(got, before) {
		t.Fatal("束ねても行が変わらない")
	}
	if got, err := queryStringsForTest(db, `select key_a || '|' || key_b from video_version_candidates order by key_a`); err != nil {
		t.Fatal(err)
	} else if !slices.Equal(got, []string{"key-b|key-d"}) {
		t.Errorf("束ねたあとの候補 = %v, want [key-b|key-d]", got)
	}
	newKey := ownerVideo(t, db, ids["b"]).UserKey
	recorder.events = nil

	undoImageForTest(t, db, image)
	assertSnapshot := func(label string) {
		t.Helper()
		if got := bundleSnapshot(t, db); !slices.Equal(got, before) {
			t.Fatalf("%s:\n%v\nwant\n%v", label, got, before)
		}
	}
	assertSnapshot("取り消しのあと")
	if got := manualTagNames(t, db, newKey); len(got) != 0 {
		t.Errorf("新しい鍵のタグ = %v, want なし", got)
	}
	if b := ownerVideo(t, db, ids["b"]); b.Versions != nil {
		t.Errorf("取り消しのあとの B の Versions = %+v", b.Versions)
	}
	var sawBundle, sawGroup bool
	for _, event := range recorder.events {
		switch e := event.(type) {
		case domain.VideoBundleChanged:
			sawBundle = slices.Equal(e.VideoIDs, []int64{ids["a"], ids["b"], ids["c"]})
		case domain.GroupsChanged:
			sawGroup = sawGroup || slices.Contains(e.GroupIDs, group)
		}
	}
	if !sawBundle || !sawGroup {
		t.Errorf("取り消しの events = %+v", recorder.events)
	}
	if err := db.Organize().Undo(ctx, image); !errors.Is(err, domain.ErrUndoConflict) {
		t.Errorf("2 回目の取り消し error = %v, want ErrUndoConflict", err)
	}
	assertSnapshot("2 回目の取り消しのあと")
}

// 束ねたあとで集まりにタグを付けると、取り消しは ErrUndoConflict で何も書かない。
func TestOrganizeBundleUndoConflictsAfterChange(t *testing.T) {
	db, ids := versionFixture(t)
	image := organizeBundle(t, db, ids["b"], ids["a"])
	attachNamedTag(t, db, "Later", ids["a"])
	after := bundleSnapshot(t, db)
	if err := db.Organize().Undo(context.Background(), image); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo error = %v, want ErrUndoConflict", err)
	}
	if got := bundleSnapshot(t, db); !slices.Equal(got, after) {
		t.Fatalf("拒まれた取り消しが書いた:\n%v\nwant\n%v", got, after)
	}
}

// 落とした先だけ・無い動画・展開して 20 本を超える集まりは誤りで、何も書かない。
func TestOrganizeBundleRefusals(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	var files []domain.VideoFile
	for i := range domain.MaxOrganizeBundleVideos + 1 {
		files = append(files, listingFile(fixturePath(fmt.Sprintf("/media/v%02d.mp4", i)), fmt.Sprintf("v%02d", i), fmt.Sprintf("key-%02d", i), i))
	}
	paths := upsertAll(t, db, files...)
	ids := make([]int64, 0, len(files))
	for _, file := range files {
		ids = append(ids, paths[file.Path])
	}
	bundle(t, db, ids[0], ids[:domain.MaxOrganizeBundleVideos]...)
	before := bundleSnapshot(t, db)

	for _, tc := range []struct {
		label    string
		target   int64
		videoIDs []int64
		want     error
	}{
		{"落とした先だけ", ids[20], []int64{ids[20]}, domain.ErrTooFewVersions},
		{"無い動画", ids[20], []int64{999999}, domain.ErrGroupTargetNotFound},
		{"無い落とした先", 999999, []int64{ids[20]}, domain.ErrGroupTargetNotFound},
		{"展開して 21 本", ids[20], []int64{ids[3]}, domain.ErrTooManyVersions},
	} {
		if _, err := db.Organize().Bundle(ctx, tc.videoIDs, tc.target); !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", tc.label, err, tc.want)
		}
	}
	if got := bundleSnapshot(t, db); !slices.Equal(got, before) {
		t.Fatalf("拒まれた束ねが書いた:\n%v\nwant\n%v", got, before)
	}
}
