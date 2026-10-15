package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// smartFolderNotices は記録した変化のうち、スマートフォルダの知らせだけを返す。
func smartFolderNotices(recorder *eventRecorder) []domain.SmartFoldersChanged {
	var out []domain.SmartFoldersChanged
	for _, event := range recorder.events {
		if notice, ok := event.(domain.SmartFoldersChanged); ok {
			out = append(out, notice)
		}
	}
	return out
}

// tagNoticeFixture はタグ 1 つと動画 1 本を持つデータベースを返す。
func tagNoticeFixture(t *testing.T) (*DB, domain.Tag, int64) {
	t.Helper()
	db := migratedDB(t)
	tag, err := db.Tags().CreateTag(context.Background(), "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))
	return db, tag, ids[fixturePath("/media/a.mp4")]
}

// フォルダが 1 つあるとき、タグを付けると確定後にそのフォルダを名指す知らせが 1 つ出る
// （specs/052-smart-folders/research.md R-12）。
func TestAttachingATagNamesEverySmartFolderOnce(t *testing.T) {
	db, tag, videoID := tagNoticeFixture(t)
	folder := createSmartFolder(t, db, "旅", 0)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if _, _, err := db.Tags().AttachTagByID(context.Background(), []int64{videoID}, tag.ID); err != nil {
		t.Fatal(err)
	}

	want := []domain.SmartFoldersChanged{{IDs: []int64{folder.ID}}}
	if got := smartFolderNotices(recorder); !reflect.DeepEqual(got, want) {
		t.Fatalf("notices = %+v, want %+v", got, want)
	}
}

// フォルダが無ければ、一致が変わりうる書き込みでも何も出ない。
func TestNoSmartFolderPublishesNoNotice(t *testing.T) {
	db, tag, videoID := tagNoticeFixture(t)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	ctx := context.Background()

	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{videoID}, tag.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playback().SaveProgress(ctx, "key-a", domain.Progress{PositionMs: 10, Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
	if got := smartFolderNotices(recorder); len(got) != 0 {
		t.Fatalf("notices = %+v, want none", got)
	}
}

// 視聴状態の変わらない進捗の保存は何も出さず、動画を見終える保存は出す。
func TestProgressSaveNamesFoldersOnlyWhenWatchStateChanges(t *testing.T) {
	db, _, _ := tagNoticeFixture(t)
	folder := createSmartFolder(t, db, "旅", 0)
	ctx := context.Background()
	if _, err := db.Playback().SaveProgress(ctx, "key-a", domain.Progress{PositionMs: 1000}, nil); err != nil {
		t.Fatal(err)
	}
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if _, err := db.Playback().SaveProgress(ctx, "key-a", domain.Progress{PositionMs: 5000}, nil); err != nil {
		t.Fatal(err)
	}
	if got := smartFolderNotices(recorder); len(got) != 0 {
		t.Fatalf("見ている途中の保存の notices = %+v, want none", got)
	}

	if _, err := db.Playback().SaveProgress(ctx, "key-a", domain.Progress{PositionMs: 9000, Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
	want := []domain.SmartFoldersChanged{{IDs: []int64{folder.ID}}}
	if got := smartFolderNotices(recorder); !reflect.DeepEqual(got, want) {
		t.Fatalf("見終える保存の notices = %+v, want %+v", got, want)
	}
}

// ロールバックした書き込みは何も出さない。
func TestRolledBackMatchChangePublishesNothing(t *testing.T) {
	db, _, videoID := tagNoticeFixture(t)
	createSmartFolder(t, db, "旅", 0)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	// 無いタグを付ける書き込みは途中で断って巻き戻す。
	if _, _, err := db.Tags().AttachTagByID(context.Background(), []int64{videoID}, 999999); !errors.Is(err, domain.ErrTagNotFound) {
		t.Fatalf("AttachTagByID error = %v, want domain.ErrTagNotFound", err)
	}

	// 確定の前に取り消した取引も発行しない。
	ctx, cancel := context.WithCancel(context.Background())
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `delete from video_tags`); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := db.commitMatchChange(ctx, tx); err == nil {
		t.Fatal("取り消した取引が確定した")
	}

	if len(recorder.events) != 0 {
		t.Fatalf("ロールバックした書き込みから発行した: %+v", recorder.events)
	}
}

// フォルダの編集は、そのフォルダと子孫と、移動の前後の親を名指す。
func TestSmartFolderEditsNameTheFolderItsDescendantsAndParents(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	root := createSmartFolder(t, db, "根", 0)
	other := createSmartFolder(t, db, "別", 0)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	child := createSmartFolder(t, db, "子", root.ID)
	grandchild := createSmartFolder(t, db, "孫", child.ID)
	if _, err := db.SmartFolders().ChangeSmartFolder(ctx, child.ID,
		domain.SmartFolderChange{ParentSet: true, ParentID: other.ID}); err != nil {
		t.Fatal(err)
	}
	if err := db.SmartFolders().DeleteSmartFolder(ctx, other.ID); err != nil {
		t.Fatal(err)
	}

	want := []domain.SmartFoldersChanged{
		{IDs: []int64{root.ID, child.ID}},
		{IDs: []int64{child.ID, grandchild.ID}},
		{IDs: []int64{root.ID, other.ID, child.ID, grandchild.ID}},
		{IDs: []int64{other.ID, child.ID, grandchild.ID}},
	}
	if got := smartFolderNotices(recorder); !reflect.DeepEqual(got, want) {
		t.Fatalf("notices = %+v, want %+v", got, want)
	}
}

// グループの変化はすべてのフォルダを名指す知らせを伴い、知らせは 1 つにまとまる。
func TestGroupChangeNamesEverySmartFolder(t *testing.T) {
	db, _, videoID := tagNoticeFixture(t)
	first := createSmartFolder(t, db, "一", 0)
	second := createSmartFolder(t, db, "二", 0)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if _, err := db.Groups().CreateGroup(context.Background(), "まとめ", domain.GroupAdd{VideoIDs: []int64{videoID}}); err != nil {
		t.Fatal(err)
	}

	want := []domain.SmartFoldersChanged{{IDs: []int64{first.ID, second.ID}}}
	if got := smartFolderNotices(recorder); !reflect.DeepEqual(got, want) {
		t.Fatalf("notices = %+v, want %+v", got, want)
	}
}

// 解析の結果を書くと、後継の引き継ぎが無くても長さと再生可否が変わるので、すべてのフォルダを名指す。
// 走査を閉じるときの知らせは解析より前に出るので、ここで知らせないと長さで絞るフォルダが古いまま残る。
func TestProbeResultNamesEverySmartFolder(t *testing.T) {
	db, videoID := jobsFixture(t)
	folder := createSmartFolder(t, db, "長い", 0)
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	probe := domain.Probe{DurationMs: 600_000, VideoCodec: "h264"}

	written, err := db.Ingest().ApplyProbeForJob(ctx, job, probe, domain.Playability{Playable: true})
	if err != nil || !written {
		t.Fatalf("ApplyProbeForJob = %v, %v, want true, nil", written, err)
	}
	if err := db.Ingest().ApplyProbe(ctx, videoID, probe, domain.Playability{Playable: true}); err != nil {
		t.Fatal(err)
	}

	once := domain.SmartFoldersChanged{IDs: []int64{folder.ID}}
	want := []domain.SmartFoldersChanged{once, once}
	if got := smartFolderNotices(recorder); !reflect.DeepEqual(got, want) {
		t.Fatalf("notices = %+v, want %+v", got, want)
	}
}

// 表示名は検索の対象なので、変えるとすべてのフォルダを名指す。同じ名前の書き直しは何も出さない。
func TestDisplayNameChangeNamesEverySmartFolder(t *testing.T) {
	db, _, videoID := tagNoticeFixture(t)
	folder := createSmartFolder(t, db, "題名", 0)
	ctx := context.Background()
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if _, err := db.WorkInfo().SetDetails(ctx, videoID, "新しい題名", domain.WorkInfo{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Overrides().SetDisplayName(ctx, videoID, "別の題名"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Overrides().SetDisplayNames(ctx, []domain.DisplayNameChange{
		{Video: domain.VideoRef{ID: videoID}, DisplayName: "三つ目の題名"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Overrides().SetDisplayName(ctx, videoID, "三つ目の題名"); err != nil {
		t.Fatal(err)
	}

	once := domain.SmartFoldersChanged{IDs: []int64{folder.ID}}
	want := []domain.SmartFoldersChanged{once, once, once}
	if got := smartFolderNotices(recorder); !reflect.DeepEqual(got, want) {
		t.Fatalf("notices = %+v, want %+v", got, want)
	}
}
