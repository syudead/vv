package store

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/syudead/vv/internal/domain"
)

const mediaFolderSwitchesVersion = 94

type switchFixture struct {
	db           *DB
	onID, offID  int64
	onDir        string
	offDir       string
	shown, outID int64
}

// One video in each of two folders; both titles match "cat", and the second folder is switched off.
func switchedOffFolderFixture(t *testing.T) switchFixture {
	t.Helper()
	db, err := OpenMigratedForTest(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	f := switchFixture{db: db, onDir: fixturePath("/media/on"), offDir: fixturePath("/media/off")}
	for _, dir := range []string{f.onDir, f.offDir} {
		folder, err := db.Settings().AddMediaFolder(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if dir == f.onDir {
			f.onID = folder.ID
		} else {
			f.offID = folder.ID
		}
	}
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/on/cat-on.mp4"), "cat on", "key-on", 1),
		listingFile(fixturePath("/media/off/cat-off.mp4"), "cat off", "key-off", 2))
	f.shown, f.outID = ids[fixturePath("/media/on/cat-on.mp4")], ids[fixturePath("/media/off/cat-off.mp4")]

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	folder, err := db.Settings().SetMediaFolderEnabled(ctx, f.offID, false)
	if err != nil {
		t.Fatal(err)
	}
	if folder.Enabled || folder.Path != f.offDir || folder.Version != 1 {
		t.Fatalf("SetMediaFolderEnabled = %+v、パスと版を変えずにオフであるべき", folder)
	}
	if want := []domain.Event{domain.MediaFoldersChanged{Switch: true}}; !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("スイッチのイベント = %#v, want %#v", recorder.events, want)
	}
	db.PublishTo(nil)
	return f
}

func TestSwitchedOffFolderLeavesListsAndCounts(t *testing.T) {
	f := switchedOffFolderFixture(t)
	ctx := context.Background()
	db := f.db

	for _, q := range []domain.VideoQuery{{}, {Query: "cat"}} {
		if titles, total := listTitles(t, db, owner, q); !slices.Equal(titles, []string{"cat on"}) || total != 1 {
			t.Errorf("一覧 %+v = %v (total %d), want [cat on]", q, titles, total)
		}
	}
	if count, err := db.Library().CountVideos(ctx, owner, "cat"); err != nil || count != 1 {
		t.Errorf("CountVideos = %d, %v, want 1", count, err)
	}
	if found, err := db.Library().HasFolderLocations(ctx, owner, f.offDir); err != nil || found {
		t.Errorf("オフのフォルダの索引 = %v, %v, want 空", found, err)
	}
	if locations, err := db.Library().FolderLocations(ctx, owner, f.onDir); err != nil || len(locations) != 1 {
		t.Errorf("オンのフォルダの索引 = %+v, %v, want 1 件", locations, err)
	}

	insertHistory(t, db, "key-on", "cat on", 1)
	insertHistory(t, db, "key-off", "cat off", 2)
	insertHistory(t, db, "key-gone", "gone", 3)
	history := listHistory(t, db, owner, "", 10)
	var entries []string
	for _, entry := range history.Items {
		entries = append(entries, entry.Title)
	}
	slices.Sort(entries)
	if want := []string{"cat on", "gone"}; !slices.Equal(entries, want) {
		t.Errorf("視聴履歴 = %v, want %v (消えた動画の項目は残る)", entries, want)
	}

	tag, err := db.Tags().CreateTag(ctx, "tag")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{f.shown, f.outID}, tag.ID); err != nil {
		t.Fatal(err)
	}
	tags, err := db.Tags().ListTags(ctx, domain.TagListQuery{})
	if err != nil || len(tags.Items) != 1 || tags.Items[0].VideoCount != 1 {
		t.Errorf("タグの件数 = %+v, %v, want 1", tags.Items, err)
	}

	person := createPerson(t, db, "P")
	organization := createOrganization(t, db, "O", nil)
	for position, key := range []string{"key-on", "key-off"} {
		linkPeople(t, db, key, "performer", person.ID)
		execSQL(t, db, `insert into video_work_organizations (content_key, role, position, organization_id) values (?, 'maker', ?, ?)`,
			key, position, organization.ID)
	}
	for _, kind := range []domain.CreditKind{domain.CreditPerson, domain.CreditOrganization} {
		page, err := db.Credits().List(ctx, kind, domain.CreditQuery{Limit: 10})
		if err != nil || len(page.Items) != 1 || page.Items[0].VideoCount != 1 {
			t.Errorf("%s の件数 = %+v, %v, want 1", kind, page.Items, err)
		}
	}

	folder := createSmartFolder(t, db, "cat", 0)
	contents, err := db.SmartFolders().SmartFolderContents(ctx, owner, folder.ID, "", 10, domain.VideoQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(contents.Items) != 1 || contents.Items[0].Video == nil || contents.Items[0].Video.ID != f.shown {
		t.Errorf("スマートフォルダの一致 = %+v, want cat on だけ", contents.Items)
	}
}

// Stored group values follow the switches: a switch refreshes them on the next read.
func TestGroupCardsFollowTheSwitch(t *testing.T) {
	f := switchedOffFolderFixture(t)
	ctx := context.Background()
	group := createGroup(t, f.db, "both", domain.GroupAdd{VideoIDs: []int64{f.shown, f.outID}})
	members := func() []int64 {
		t.Helper()
		page, err := f.db.Library().ListLibrary(ctx, owner, domain.VideoQuery{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if item.Group != nil && item.Group.ID == group {
				return item.Group.Card.VideoIDs
			}
		}
		return nil
	}
	if got := members(); !slices.Equal(got, []int64{f.shown}) {
		t.Errorf("オフのときのグループのメンバー = %v, want [%d]", got, f.shown)
	}
	if _, err := f.db.Settings().SetMediaFolderEnabled(ctx, f.offID, true); err != nil {
		t.Fatal(err)
	}
	if got := members(); !slices.Equal(got, []int64{f.shown, f.outID}) {
		t.Errorf("オンに戻したときのグループのメンバー = %v, want [%d %d]", got, f.shown, f.outID)
	}
}

func TestSwitchedOffFolderKeepsReadsByIDWritesAndIngest(t *testing.T) {
	f := switchedOffFolderFixture(t)
	ctx := context.Background()
	db := f.db

	for id, want := range map[int64]bool{f.shown: false, f.outID: true} {
		video, err := db.Library().GetVideo(ctx, owner, id)
		if err != nil || video.FolderOff != want {
			t.Errorf("GetVideo(%d).FolderOff = %v, %v, want %v", id, video.FolderOff, err, want)
		}
	}
	if video, err := db.Overrides().SetDisplayName(ctx, f.outID, "renamed"); err != nil || video.DisplayName != "renamed" {
		t.Errorf("オフのフォルダの動画の上書き = %+v, %v", video, err)
	}

	if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, f.outID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	var claimed []int64
	for {
		job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
		if errors.Is(err, domain.ErrNoJob) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		claimed = append(claimed, job.VideoID)
	}
	if !slices.Contains(claimed, f.outID) {
		t.Errorf("取り出したジョブの動画 = %v、オフのフォルダの動画 %d を含むべき", claimed, f.outID)
	}

	folders, err := db.ScanIndex().ListMediaFolders(ctx)
	if err != nil || len(folders) != 2 {
		t.Fatalf("スキャンのフォルダ = %+v, %v, want 2 件", folders, err)
	}
	if under, err := db.ScanIndex().HasLocationsUnder(ctx, f.offID); err != nil || !under {
		t.Errorf("スキャンがオフのフォルダの場所を見ない: %v, %v", under, err)
	}
}

func TestExplicitExcludedFoldersReplaceTheSwitches(t *testing.T) {
	f := switchedOffFolderFixture(t)
	scoped := owner.WithExcludedFolders([]int64{f.onID})
	if titles, total := listTitles(t, f.db, scoped, domain.VideoQuery{}); !slices.Equal(titles, []string{"cat off"}) || total != 1 {
		t.Errorf("除外リストの一覧 = %v (total %d), want [cat off]", titles, total)
	}
	page, err := f.db.Library().ListLibrary(context.Background(), scoped, domain.VideoQuery{Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].Video == nil || page.Items[0].Video.ID != f.outID {
		t.Errorf("除外リストのライブラリ = %+v, %v, want cat off だけ", page.Items, err)
	}
}

func TestEveryFolderOffNamesTheEmptyLibrary(t *testing.T) {
	f := switchedOffFolderFixture(t)
	ctx := context.Background()
	if page, err := f.db.Library().ListVideos(ctx, owner, domain.VideoQuery{}); err != nil || page.EmptyReason != "" {
		t.Fatalf("オンのフォルダが残る一覧の EmptyReason = %q, %v, want なし", page.EmptyReason, err)
	}
	if _, err := f.db.Settings().SetMediaFolderEnabled(ctx, f.onID, false); err != nil {
		t.Fatal(err)
	}
	videos, err := f.db.Library().ListVideos(ctx, guest, domain.VideoQuery{})
	if err != nil || len(videos.Items) != 0 || videos.EmptyReason != domain.EmptyAllFoldersOff {
		t.Errorf("一覧 = %d 件 EmptyReason %q, %v, want all_folders_off", len(videos.Items), videos.EmptyReason, err)
	}
	library, err := f.db.Library().ListLibrary(ctx, owner, domain.VideoQuery{})
	if err != nil || len(library.Items) != 0 || library.EmptyReason != domain.EmptyAllFoldersOff {
		t.Errorf("ライブラリ = %d 件 EmptyReason %q, %v, want all_folders_off", len(library.Items), library.EmptyReason, err)
	}
}

func TestSetMediaFolderEnabledAnswersNotFound(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.Settings().SetMediaFolderEnabled(context.Background(), 999, false); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("存在しないフォルダ: err = %v, want ErrNotFound", err)
	}
}

func TestMediaFolderSwitchesMigrationTurnsExistingFoldersOn(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsys)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, mediaFolderSwitchesVersion-1); err != nil {
		t.Fatal(err)
	}
	execAll(t, db, `insert into media_folders (path, version, created_at, updated_at) values ('/a', 1, 1, 1), ('/b', 2, 1, 1)`)
	if _, err := provider.UpTo(ctx, mediaFolderSwitchesVersion); err != nil {
		t.Fatal(err)
	}
	if on := countRows(t, db, `select count(*) from media_folders where enabled = 1`); on != 2 {
		t.Errorf("オンのフォルダ = %d, want 2", on)
	}
}

// A playlist's group item is a read by id: its switched-off members stay, marked, and the count agrees.
func TestPlaylistGroupKeepsSwitchedOffMembers(t *testing.T) {
	f := switchedOffFolderFixture(t)
	group := createGroup(t, f.db, "off", domain.GroupAdd{VideoIDs: []int64{f.outID}})
	playlist := createPlaylist(t, f.db, "p")
	addToPlaylist(t, f.db, playlist, domain.PlaylistAdd{GroupIDs: []int64{group}})
	_, items := readPlaylist(t, f.db, playlist)
	if len(items) != 1 || items[0].Group == nil || len(items[0].Group.Members) != 1 {
		t.Fatalf("プレイリストの項目 = %+v, want オフのメンバーを持つグループ1件", items)
	}
	if member := items[0].Group.Members[0]; member.ID != f.outID || !member.FolderOff {
		t.Errorf("グループのメンバー = id %d FolderOff %v, want id %d FolderOff true", member.ID, member.FolderOff, f.outID)
	}
}

// Stored group members follow this node's switches; an explicit scope computes a group's items under its own list.
func TestExplicitExcludedFoldersReadGroupItems(t *testing.T) {
	f := switchedOffFolderFixture(t)
	child := createGroup(t, f.db, "child", domain.GroupAdd{VideoIDs: []int64{f.outID}})
	parent := createGroup(t, f.db, "parent", domain.GroupAdd{VideoIDs: []int64{f.shown, f.outID}, GroupIDs: []int64{child}})
	scoped := owner.WithExcludedFolders([]int64{f.onID})
	page, err := f.db.Library().GroupItems(context.Background(), scoped, parent, "", 10, domain.VideoQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var videos, groups []int64
	for _, item := range page.Items {
		if item.Video != nil {
			videos = append(videos, item.Video.ID)
		} else if item.Group != nil {
			groups = append(groups, item.Group.ID)
		}
	}
	if !slices.Equal(videos, []int64{f.outID}) || !slices.Equal(groups, []int64{child}) {
		t.Errorf("除外リストのグループ項目 = 動画 %v グループ %v, want 動画 [%d] グループ [%d]", videos, groups, f.outID, child)
	}
}

// A bundle whose representative's folder is off is listed through its on version, by its on version's names only.
func TestBundleWithSwitchedOffRepresentative(t *testing.T) {
	f := switchedOffFolderFixture(t)
	ctx := context.Background()
	self := upsertAll(t, f.db, listingFile(fixturePath("/media/on/self.mp4"), "self", "key-self", 3))[fixturePath("/media/on/self.mp4")]
	if _, err := f.db.Versions().Bundle(ctx, []int64{f.outID, f.shown}, f.outID); err != nil {
		t.Fatal(err)
	}
	if titles, _ := listTitles(t, f.db, owner, domain.VideoQuery{Query: "off"}); len(titles) != 0 {
		t.Errorf("オフの版の名前での検索 = %v, want 空", titles)
	}
	candidates, err := f.db.Library().RelatedCandidates(ctx, owner, self)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(candidates, func(c domain.RelatedCandidate) bool { return c.VideoID == f.shown }) {
		t.Fatalf("関連動画の候補 = %+v, want オンの版 %d を含む", candidates, f.shown)
	}
	videos, err := f.db.Library().VideosByIDs(ctx, owner, []int64{f.shown})
	if err != nil || len(videos) != 1 || videos[0].ID != f.shown {
		t.Errorf("候補の読み出し = %+v, %v, want オンの版 %d", videos, err, f.shown)
	}
}
