package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func tagNamesOfKey(t *testing.T, db *DB, key string) []string {
	t.Helper()
	rows, err := db.sql.Query(`select tn.name from video_tags vt join tag_names tn on tn.tag_id = vt.tag_id and tn.canonical = 1
		where vt.content_key = ? order by tn.name`, key)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func attachTags(t *testing.T, db *DB, id int64, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, _, err := db.Tags().AttachTagByName(context.Background(), []int64{id}, name); err != nil {
			t.Fatal(err)
		}
	}
}

// Each row of the merge rules: the file video keeps its title, public flag, code and set dates, and the virtual
// video's tags, favorite, credits, external ids, missing dates, description and links join it.
func TestMergeVirtualVideoMovesUserData(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	fileID := upsertAll(t, db, listingFile(fixturePath("/media/clip.mp4"), "clip", "key-clip-0001", 1))[fixturePath("/media/clip.mp4")]
	virtual := createVirtual(t, db, "Moonlight", fixedTime)
	shared, fileOnly, virtualOnly := createPerson(t, db, "A"), createPerson(t, db, "B"), createPerson(t, db, "C")

	attachTags(t, db, fileID, "piano")
	attachTags(t, db, virtual.ID, "piano", "night")
	if _, err := db.Favorites().SetFavorites(ctx, domain.FavoriteChange{VideoIDs: []int64{virtual.ID}, Favorite: true}); err != nil {
		t.Fatal(err)
	}
	setPublic(t, db, true, virtual.ID)
	if _, err := db.WorkInfo().Set(ctx, fileID, domain.WorkInfo{
		Code: "F-1", Description: "file notes", URLs: []string{"https://example.com/a", "https://example.com/b"},
		Dates:       []domain.WorkDate{{Kind: domain.DateRelease, Date: "2020"}},
		People:      []domain.WorkPerson{{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: shared.ID}}, {Role: domain.PersonPerformer, Person: domain.CreditRef{ID: fileOnly.ID}}},
		ExternalIDs: []domain.ExternalID{{Source: "site", Value: "1"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.WorkInfo().Set(ctx, virtual.ID, domain.WorkInfo{
		Code: "V-1", Description: "virtual notes", URLs: []string{"https://example.com/b", "https://example.com/c"},
		Dates:       []domain.WorkDate{{Kind: domain.DateRelease, Date: "2001"}, {Kind: domain.DateRecorded, Date: "2002"}},
		People:      []domain.WorkPerson{{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: virtualOnly.ID}}, {Role: domain.PersonPerformer, Person: domain.CreditRef{ID: shared.ID}}},
		ExternalIDs: []domain.ExternalID{{Source: "site", Value: "2"}},
	}); err != nil {
		t.Fatal(err)
	}
	both, alone := createPlaylist(t, db, "both"), createPlaylist(t, db, "alone")
	addVideos(t, db, both, virtual.ID, fileID)
	addVideos(t, db, alone, virtual.ID)
	fileItem, virtualItem := playlistItemIDs(t, db, both)[1], playlistItemIDs(t, db, alone)[0]
	recorder := &releasedRecorder{}
	db.PublishTo(recorder)

	merged, err := db.VirtualVideos().Merge(ctx, virtual.ID, virtual.UserKey, fileID, "key-clip-000")
	if err != nil {
		t.Fatal(err)
	}

	if merged.ID != fileID || merged.Title != "clip" || merged.Public || !merged.Favorite {
		t.Errorf("まとめた先 = id %d title %q public %v favorite %v, want clip、非公開、お気に入り", merged.ID, merged.Title, merged.Public, merged.Favorite)
	}
	if got := tagNamesOfKey(t, db, "key-clip-0001"); !slices.Equal(got, []string{"night", "piano"}) {
		t.Errorf("タグ = %v, want 和集合 [night piano]", got)
	}
	want := domain.WorkInfo{
		Code: "F-1", Description: "file notes\n\nvirtual notes",
		URLs:  []string{"https://example.com/a", "https://example.com/b", "https://example.com/c"},
		Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: "2020"}, {Kind: domain.DateRecorded, Date: "2002"}},
		People: []domain.WorkPerson{{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: shared.ID}},
			{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: fileOnly.ID}}, {Role: domain.PersonPerformer, Person: domain.CreditRef{ID: virtualOnly.ID}}},
		ExternalIDs: []domain.ExternalID{{Source: "site", Value: "1"}, {Source: "site", Value: "2"}},
	}
	if merged.WorkInfo == nil || !merged.WorkInfo.Equal(want) {
		t.Errorf("作品情報 = %+v, want %+v", merged.WorkInfo, want)
	}
	if got := playlistItemIDs(t, db, both); !slices.Equal(got, []int64{fileItem}) {
		t.Errorf("両方あったプレイリスト = %v, want ファイルのある動画の項目 [%d]", got, fileItem)
	}
	if got := playlistItemIDs(t, db, alone); !slices.Equal(got, []int64{virtualItem}) {
		t.Errorf("バーチャルだけのプレイリスト = %v, want 付け替えた項目 [%d]", got, virtualItem)
	}
	var key string
	if err := db.sql.QueryRowContext(ctx, `select content_key from playlist_items where id = ?`, virtualItem).Scan(&key); err != nil || key != "key-clip-0001" {
		t.Errorf("付け替えた項目のキー = %q, err = %v", key, err)
	}
	if got, _ := listTitles(t, db, owner, domain.VideoQuery{}); !slices.Equal(got, []string{"clip"}) {
		t.Errorf("まとめた後の一覧 = %v, want [clip]", got)
	}
	if keys := recorder.take(); !slices.Equal(keys, []string{virtual.UserKey}) {
		t.Errorf("参照の無くなったキー = %v, want [%s]", keys, virtual.UserKey)
	}
	if _, err := db.VirtualVideos().Merge(ctx, virtual.ID, virtual.UserKey, fileID, "key-clip-000"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("二度目のまとめ: err = %v, want domain.ErrNotFound", err)
	}
}

// A bundled file video shares its user data through the bundle key, so the merge lands there.
func TestMergeVirtualVideoIntoBundle(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1), listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 2))
	a, b := ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")]
	versions, err := db.Versions().Bundle(ctx, []int64{a, b}, a)
	if err != nil {
		t.Fatal(err)
	}
	virtual := createVirtual(t, db, "Moonlight", fixedTime)
	attachTags(t, db, virtual.ID, "night")

	if _, err := db.VirtualVideos().Merge(ctx, virtual.ID, virtual.UserKey, b, "key-b"); err != nil {
		t.Fatal(err)
	}
	bundleKey := versions.Items[0].UserKey
	if got := tagNamesOfKey(t, db, bundleKey); !slices.Equal(got, []string{"night"}) {
		t.Errorf("束のキーのタグ = %v, want [night]", got)
	}
	if got := tagNamesOfKey(t, db, "key-b"); len(got) != 0 {
		t.Errorf("メンバーのキーにタグが入った: %v", got)
	}
}

// A rejected merge leaves both videos as they were.
func TestMergeVirtualVideoRejections(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	fileID := upsertAll(t, db, listingFile(fixturePath("/media/clip.mp4"), "clip", "key-clip", 1))[fixturePath("/media/clip.mp4")]
	virtual := createVirtual(t, db, "Moonlight", fixedTime)
	other := createVirtual(t, db, "Other", fixedTime)
	fileInfo := domain.WorkInfo{Description: strings.Repeat("あ", 6_000)}
	if _, err := db.WorkInfo().Set(ctx, fileID, fileInfo); err != nil {
		t.Fatal(err)
	}
	if _, err := db.WorkInfo().Set(ctx, virtual.ID, domain.WorkInfo{Description: strings.Repeat("い", 3_999)}); err != nil {
		t.Fatal(err)
	}
	attachTags(t, db, virtual.ID, "night")

	for _, tc := range []struct {
		name                   string
		virtualID, fileID      int64
		version, targetVersion string
		want                   error
	}{
		{"合わせた説明が上限を超える", virtual.ID, fileID, virtual.UserKey, "key-clip", domain.ErrMergedDescriptionTooLong},
		{"バーチャルな動画の版が古い", virtual.ID, fileID, virtual.UserKey + "0", "key-clip", domain.ErrContentChanged},
		{"ファイルのある動画の版が古い", virtual.ID, fileID, virtual.UserKey, "key-other", domain.ErrContentChanged},
		{"まとめ元にファイルがある", fileID, fileID, "key-clip", "key-clip", domain.ErrNotVirtual},
		{"まとめ先がバーチャル", virtual.ID, other.ID, virtual.UserKey, other.UserKey, domain.ErrVirtualVideo},
		{"まとめ先が無い", virtual.ID, fileID + 1000, virtual.UserKey, "key-clip", domain.ErrNotFound},
	} {
		if _, err := db.VirtualVideos().Merge(ctx, tc.virtualID, tc.version, tc.fileID, tc.targetVersion); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}

	if got, _ := listTitles(t, db, owner, domain.VideoQuery{}); !slices.Equal(got, []string{"Moonlight", "Other", "clip"}) {
		t.Errorf("拒んだ後の一覧 = %v", got)
	}
	if got := tagNamesOfKey(t, db, virtual.UserKey); !slices.Equal(got, []string{"night"}) {
		t.Errorf("バーチャルな動画のタグ = %v, want [night]", got)
	}
	if got := tagNamesOfKey(t, db, "key-clip"); len(got) != 0 {
		t.Errorf("ファイルのある動画にタグが入った: %v", got)
	}
	stored, err := db.WorkInfo().ByUserKeys(ctx, []string{"key-clip"})
	if err != nil || stored["key-clip"].Description != fileInfo.Description {
		t.Errorf("ファイルのある動画の説明が変わった: err = %v", err)
	}
}

// A tag the owner confirmed on the virtual video stays confirmed where the file video only had it as a suggestion.
func TestMergeVirtualVideoKeepsAConfirmedTag(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	fileID := upsertAll(t, db, listingFile(fixturePath("/media/clip.mp4"), "clip", "key-clip", 1))[fixturePath("/media/clip.mp4")]
	virtual := createVirtual(t, db, "Moonlight", fixedTime)
	attachTags(t, db, fileID, "piano")
	attachTags(t, db, virtual.ID, "piano")
	// Auto-tagging's suggestion, without running the classifier.
	if _, err := db.sql.ExecContext(ctx, `update video_tags set tentative = 1 where content_key = 'key-clip'`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.VirtualVideos().Merge(ctx, virtual.ID, virtual.UserKey, fileID, "key-clip"); err != nil {
		t.Fatal(err)
	}
	var tentative int
	if err := db.sql.QueryRowContext(ctx, `select tentative from video_tags where content_key = 'key-clip'`).Scan(&tentative); err != nil {
		t.Fatal(err)
	}
	if tentative != 0 {
		t.Errorf("まとめた先のタグ tentative = %d, want 確定 0", tentative)
	}
}

// A merge that leaves the work information as it was still tells the screen that the file video changed.
func TestMergeVirtualVideoNotifiesTheFileVideo(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	fileID := upsertAll(t, db, listingFile(fixturePath("/media/clip.mp4"), "clip", "key-clip", 1))[fixturePath("/media/clip.mp4")]
	virtual := createVirtual(t, db, "Moonlight", fixedTime)
	attachTags(t, db, virtual.ID, "night")
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if _, err := db.VirtualVideos().Merge(ctx, virtual.ID, virtual.UserKey, fileID, "key-clip"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(recorder.events, domain.Event(domain.VideoOverrideChanged{VideoID: fileID})) {
		t.Errorf("イベント = %+v, want ファイルのある動画 %d の変更", recorder.events, fileID)
	}
}
