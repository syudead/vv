package store

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 所有者の操作の記録。

// ownerEntry は所有者の記録 1 件の、比べる欄である。
type ownerEntry struct {
	kind   domain.ActivityKind
	key    string
	keyB   string
	label  string
	detail string
}

// ownerEntries は decider owner の記録を古い順に返す。どれも done/completed か、work_info_fetch の失敗は
// done/failed であることを確かめる。
func ownerEntries(t *testing.T, db *DB) []ownerEntry {
	t.Helper()
	page := listActivity(t, db, domain.ActivityQuery{Decider: domain.ActivityByOwner}, "", 1000)
	var out []ownerEntry
	for _, entry := range slices.Backward(page.Items) {
		if entry.Kind == domain.ActivityUndo {
			continue
		}
		if entry.Status != domain.ActivityDone ||
			(entry.Outcome != domain.ActivityCompleted && entry.Outcome != domain.ActivityOutcomeFailed) {
			t.Errorf("所有者の記録 %+v, want done/completed", entry)
		}
		out = append(out, ownerEntry{entry.Kind, entry.Subject.Key, entry.Subject.KeyB, entry.Subject.Label, entry.Detail})
	}
	return out
}

// expectOwnerEntries は前回から増えた所有者の記録が want であることを確かめ、今の件数を返す。
func expectOwnerEntries(t *testing.T, db *DB, since int, want ...ownerEntry) int {
	t.Helper()
	got := ownerEntries(t, db)
	if len(got) < since || !slices.Equal(got[since:], want) {
		t.Fatalf("増えた所有者の記録 = %+v, want %+v", got[min(since, len(got)):], want)
	}
	return len(got)
}

// 束ねる・代表にする・版から外すは動画ごとに 1 件、「違う動画」は組の記録を 1 件書く。
func TestOwnerVersionOperationsWriteEntries(t *testing.T) {
	ctx := context.Background()
	db, ids := versionFixture(t)

	bundle(t, db, ids["a"], ids["a"], ids["b"], ids["c"])
	n := expectOwnerEntries(t, db, 0,
		ownerEntry{domain.ActivityBundle, "key-a", "", "a", ""},
		ownerEntry{domain.ActivityBundle, "key-b", "", "b", ""},
		ownerEntry{domain.ActivityBundle, "key-c", "", "c", ""})

	if _, err := db.Versions().MakeRepresentative(ctx, ids["b"]); err != nil {
		t.Fatal(err)
	}
	n = expectOwnerEntries(t, db, n, ownerEntry{domain.ActivityMakeRepresentative, "key-b", "", "b", ""})

	if _, err := db.Versions().Unbundle(ctx, ids["c"]); err != nil {
		t.Fatal(err)
	}
	n = expectOwnerEntries(t, db, n, ownerEntry{domain.ActivityUnbundle, "key-c", "", "c", ""})

	if err := db.Versions().Dismiss(ctx, [2]int64{ids["d"], ids["c"]}); err != nil {
		t.Fatal(err)
	}
	expectOwnerEntries(t, db, n, ownerEntry{domain.ActivityDismiss, "key-c", "key-d", "c", ""})
	page := listActivity(t, db, domain.ActivityQuery{Features: []domain.ActivityFeature{domain.ActivityFeatureVersions}}, "", 10)
	if dismiss := page.Items[0]; dismiss.Subject.Kind != domain.ActivitySubjectPair || dismiss.Subject.LabelB != "d" {
		t.Errorf("「違う動画」の記録 = %+v, want 組 (c, d)", dismiss.Subject)
	}
}

// 画面からタグを付ける・外すは、実際に変わった動画とタグごとに 1 件書く。集まりの鍵は代表を対象にする。
func TestOwnerTagOperationsWriteOneEntryPerVideo(t *testing.T) {
	ctx := context.Background()
	db, ids := versionFixture(t)
	bundle(t, db, ids["c"], ids["c"], ids["d"])
	n := len(ownerEntries(t, db))

	// 3 本に付けると 3 件。もう付いている動画は数えない。
	cat := attachNamedTag(t, db, "cat", ids["a"], ids["b"], ids["d"])
	n = expectOwnerEntries(t, db, n,
		ownerEntry{domain.ActivityTagAttach, "key-a", "", "a", "cat"},
		ownerEntry{domain.ActivityTagAttach, "key-b", "", "b", "cat"},
		ownerEntry{domain.ActivityTagAttach, "key-c", "", "c", "cat"})
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids["a"]}, cat); err != nil {
		t.Fatal(err)
	}
	n = expectOwnerEntries(t, db, n)

	if _, _, err := db.Tags().DetachTag(ctx, []int64{ids["a"], ids["b"]}, cat); err != nil {
		t.Fatal(err)
	}
	expectOwnerEntries(t, db, n,
		ownerEntry{domain.ActivityTagDetach, "key-a", "", "a", "cat"},
		ownerEntry{domain.ActivityTagDetach, "key-b", "", "b", "cat"})
}

// グループをタグにすると、タグが実際に付いた動画ごとに tag_attach を書き、確定後に ActivityChanged を知らせる。
func TestTagGroupWritesOneEntryPerTaggedVideo(t *testing.T) {
	ctx := context.Background()
	db, ids := versionFixture(t)
	attachNamedTag(t, db, "cat", ids["a"])
	group := createGroup(t, db, "cat", domain.GroupAdd{VideoIDs: []int64{ids["a"], ids["b"]}})
	n := len(ownerEntries(t, db))
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if _, err := db.Groups().TagGroup(ctx, group); err != nil {
		t.Fatal(err)
	}
	expectOwnerEntries(t, db, n, ownerEntry{domain.ActivityTagAttach, "key-b", "", "b", "cat"})
	if !slices.Contains(recorder.events, domain.Event(domain.ActivityChanged{})) {
		t.Errorf("events = %+v, want ActivityChanged を含む", recorder.events)
	}
}

// 外部 API /api/v1 のタグの操作も所有者の記録を、動画とタグごとに書く。
func TestExternalVideoTagsWriteOwnerEntries(t *testing.T) {
	ctx := context.Background()
	db, _ := tentativeFixture(t)
	if _, err := db.Tags().ApplyVideoTags(ctx, bothVideos, domain.VideoTagsAdd, []string{"x", "y"}, false); err != nil {
		t.Fatal(err)
	}
	n := expectOwnerEntries(t, db, 0,
		ownerEntry{domain.ActivityTagAttach, "key-a", "", "a", "x"},
		ownerEntry{domain.ActivityTagAttach, "key-a", "", "a", "y"},
		ownerEntry{domain.ActivityTagAttach, "key-b", "", "b", "x"},
		ownerEntry{domain.ActivityTagAttach, "key-b", "", "b", "y"})

	// 置き換えは足したタグと外したタグを両方書く。
	if _, err := db.Tags().ApplyVideoTags(ctx, bothVideos[:1], domain.VideoTagsReplace, []string{"y", "z"}, false); err != nil {
		t.Fatal(err)
	}
	n = expectOwnerEntries(t, db, n,
		ownerEntry{domain.ActivityTagDetach, "key-a", "", "a", "x"},
		ownerEntry{domain.ActivityTagAttach, "key-a", "", "a", "z"})

	if _, err := db.Tags().ApplyVideoTags(ctx, bothVideos, domain.VideoTagsRemove, []string{"y", "none"}, false); err != nil {
		t.Fatal(err)
	}
	expectOwnerEntries(t, db, n,
		ownerEntry{domain.ActivityTagDetach, "key-a", "", "a", "y"},
		ownerEntry{domain.ActivityTagDetach, "key-b", "", "b", "y"})
}

// 外部 API の add で提案のまま付いているタグを確定するのは付与なので、tag_attach を書き、更新日時を進める。
// 確定したタグがもう付いている動画は書かない。
func TestExternalVideoTagsRecordConfirmedSuggestion(t *testing.T) {
	ctx := context.Background()
	db, ids := tentativeFixture(t)
	if _, err := db.Tags().ApplyVideoTags(ctx, bothVideos, domain.VideoTagsAdd, []string{"x"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`update video_tags set tentative = 1 where content_key = 'key-a'`); err != nil {
		t.Fatal(err)
	}
	n := len(ownerEntries(t, db))
	resetEditedAt(t, db)
	start := time.Now()
	if _, err := db.Tags().ApplyVideoTags(ctx, bothVideos, domain.VideoTagsAdd, []string{"x"}, false); err != nil {
		t.Fatal(err)
	}
	expectOwnerEntries(t, db, n, ownerEntry{domain.ActivityTagAttach, "key-a", "", "a", "x"})
	assertEditedSince(t, db, ids[fixturePath("/media/a.mp4")], start)
	assertEditedAt(t, db, ids[fixturePath("/media/b.mp4")], staleEditedAt)
	var tentative int
	if err := db.sql.QueryRow(`select tentative from video_tags where content_key = 'key-a'`).Scan(&tentative); err != nil ||
		tentative != 0 {
		t.Errorf("key-a の x の tentative = %d, %v, want 0（確定した）", tentative, err)
	}
}

// 提案のタグを残す・却下するは、その動画とタグの記録を 1 件書く。
func TestKeepAndRejectSuggestedTagWriteEntries(t *testing.T) {
	ctx := context.Background()
	db, ids := itemsFixture(t)
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")
	cat, dog := createTestTag(t, db, "cat"), createTestTag(t, db, "dog")
	finishSolo(t, db, ids, domain.AutoTagSelection{Tentative: []int64{cat.ID, dog.ID}})
	n := len(ownerEntries(t, db))

	if err := db.AutoTags().ConfirmTentative(ctx, solo, cat.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoTags().RejectTentative(ctx, solo, dog.ID); err != nil {
		t.Fatal(err)
	}
	label := ownerVideo(t, db, solo).Title
	expectOwnerEntries(t, db, n,
		ownerEntry{domain.ActivityTagKeep, soloKey, "", label, "cat"},
		ownerEntry{domain.ActivityTagReject, soloKey, "", label, "dog"})
}

// 作品情報の取得は 1 回ごとに、失敗ならプラグインの文を持つ done/failed で書き、適用は動画の記録を 1 件書く。
func TestWorkInfoFetchAndApplyWriteEntries(t *testing.T) {
	ctx := context.Background()
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	plugin := approvedPlugin(t, db, "example", "d1")

	if err := db.Overrides().RecordWorkInfoFetch(ctx, alpha, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Overrides().RecordWorkInfoFetch(ctx, alpha, "no such work"); err != nil {
		t.Fatal(err)
	}
	title := "Fetched"
	if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
		VideoID: alpha, PluginID: plugin.ID, Digest: "d1", Title: &title, Tags: []string{"drama"},
	}); err != nil {
		t.Fatal(err)
	}
	// 適用のタグは作品情報の適用の一部で、タグの記録は書かない。表示名は書いた時点のもの。
	expectOwnerEntries(t, db, 0,
		ownerEntry{domain.ActivityWorkInfoFetch, "key-alpha", "", "alpha", ""},
		ownerEntry{domain.ActivityWorkInfoFetch, "key-alpha", "", "alpha", "no such work"},
		ownerEntry{domain.ActivityWorkInfoApply, "key-alpha", "", "Fetched", ""})
	page := listActivity(t, db, domain.ActivityQuery{Features: []domain.ActivityFeature{domain.ActivityFeatureWorkInfo}}, "", 10)
	failed := page.Items[1]
	if failed.Outcome != domain.ActivityOutcomeFailed || failed.RetryOffered() {
		t.Errorf("失敗した取得 = %s/%s（やり直し %v）, want done/failed でやり直し無し", failed.Status, failed.Outcome,
			failed.RetryOffered())
	}
}

// ドラッグで束ねた 3 本は 3 件の bundle を書き、その取り消しは 3 本に organize_undo を書く。グループの並べ替えと
// プレイリストの並べ替えは何も書かない。
func TestOrganizeBundleUndoAndReordersWriteEntries(t *testing.T) {
	ctx := context.Background()
	db, ids := versionFixture(t)
	image, err := db.Organize().Bundle(ctx, []int64{ids["b"], ids["c"]}, ids["a"])
	if err != nil {
		t.Fatal(err)
	}
	n := expectOwnerEntries(t, db, 0,
		ownerEntry{domain.ActivityBundle, "key-a", "", "a", ""},
		ownerEntry{domain.ActivityBundle, "key-b", "", "b", ""},
		ownerEntry{domain.ActivityBundle, "key-c", "", "c", ""})
	if err := db.Organize().Undo(ctx, image); err != nil {
		t.Fatal(err)
	}
	n = expectOwnerEntries(t, db, n,
		ownerEntry{domain.ActivityOrganizeUndo, "key-a", "", "a", ""},
		ownerEntry{domain.ActivityOrganizeUndo, "key-b", "", "b", ""},
		ownerEntry{domain.ActivityOrganizeUndo, "key-c", "", "c", ""})

	playlist := createPlaylist(t, db, "list")
	addVideos(t, db, playlist, ids["a"], ids["b"])
	_, items, _, err := db.Playlists().Playlist(ctx, playlist)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Playlists().MovePlaylistItem(ctx, playlist, items[0].ID, &items[1].ID); err != nil {
		t.Fatal(err)
	}
	expectOwnerEntries(t, db, n)
	if all := listActivity(t, db, domain.ActivityQuery{}, "", 100); len(all.Items) != n {
		t.Errorf("記録 = %d 件, want プレイリストの操作で増えない %d 件", len(all.Items), n)
	}
}

// メディアフォルダの登録を外すと、ライブラリから外れた動画の数を持つ記録を 1 件書く。登録外の所在だけが
// 残る動画は数え、別の登録フォルダに所在が残る動画は数えない。
func TestMediaFolderRemovalWritesOneEntry(t *testing.T) {
	ctx := context.Background()
	db, ids := versionFixture(t)
	folders, err := db.ScanIndex().ListMediaFolders(ctx)
	if err != nil || len(folders) != 1 {
		t.Fatalf("folders = %+v, %v", folders, err)
	}
	// a は登録外の所在も持つので行は残るが、ライブラリからは外れる。b は別の登録フォルダにも所在があるので残る。
	if _, err := db.Settings().AddMediaFolder(ctx, fixturePath("/other")); err != nil {
		t.Fatal(err)
	}
	for video, path := range map[string]string{"a": fixturePath("/legacy/a.mp4"), "b": fixturePath("/other/b.mp4")} {
		if _, err := db.sql.Exec(`insert into video_locations (video_id, path, version, title, size_bytes, mtime, created_at, updated_at)
			values (?, ?, 1, ?, 1, 1, 1, 1)`, ids[video], path, path); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Settings().DeleteMediaFolder(ctx, folders[0].ID, folders[0].Version); err != nil {
		t.Fatal(err)
	}
	path := fixturePath("/media")
	expectOwnerEntries(t, db, 0, ownerEntry{domain.ActivityMediaFolderRemove, path, "", path, strconv.Itoa(3)})
	page := listActivity(t, db, domain.ActivityQuery{}, "", 10)
	if entry := page.Items[0]; entry.Subject.Kind != domain.ActivitySubjectFolder ||
		entry.Feature != domain.ActivityFeatureMediaFolders {
		t.Errorf("記録 = %+v, want media_folders のフォルダの記録", entry)
	}
}
