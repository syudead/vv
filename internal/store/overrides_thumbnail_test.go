package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 代表サムネイルの位置（specs/029-video-overrides/data-model.md §1・§3、research.md R-4・R-6）。

func setThumbnailPosition(t *testing.T, db *DB, id int64, positionMs *int64) domain.Video {
	t.Helper()
	video, err := db.Ingest().SetThumbnailPosition(context.Background(), id, positionMs)
	if err != nil {
		t.Fatalf("位置を記録できない: %v", err)
	}
	return video
}

type overrideRow struct {
	displayName sql.NullString
	positionMs  sql.NullInt64
	revision    sql.NullInt64
}

func readOverrideRow(t *testing.T, db *DB, id int64) (overrideRow, bool) {
	t.Helper()
	var row overrideRow
	err := db.sql.QueryRow(`select ov.display_name, ov.thumbnail_position_ms, ov.thumbnail_revision
		from video_overrides ov join videos v on v.content_key = ov.content_key where v.id = ?`, id,
	).Scan(&row.displayName, &row.positionMs, &row.revision)
	if errors.Is(err, sql.ErrNoRows) {
		return overrideRow{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return row, true
}

// 記録の取引は位置と改版番号を書き、thumbnail_state を done にして、代表サムネイルの
// 代用と失敗の問題を消す。確定後に VideoOverrideChanged を1回発行し、問題を消したので
// 取り込みの画面に読み直させる ScanChanged も発行する。
func TestSetThumbnailPositionMarksDoneAndClearsSubstitution(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	id := importWithStages(t, db)
	completeStageJob(t, db, domain.JobThumbnail, domain.SubstitutionUsed)
	completeStageJob(t, db, domain.JobSeekThumbnail, domain.SubstitutionNone)
	runAllJobs(t, db)
	if _, issues, _ := importIssues(t, db); len(issues) != 1 {
		t.Fatalf("前提の問題 = %+v", issues)
	}
	if _, err := db.sql.Exec(`update videos set thumbnail_state = 'failed' where id = ?`, id); err != nil {
		t.Fatal(err)
	}
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	position := int64(1_500)
	video := setThumbnailPosition(t, db, id, &position)
	if video.ThumbnailState != domain.ThumbnailStateDone || video.ThumbnailPositionMs == nil ||
		*video.ThumbnailPositionMs != position || video.ThumbnailRevision == 0 {
		t.Fatalf("返した動画 = state %s position %v revision %d",
			video.ThumbnailState, video.ThumbnailPositionMs, video.ThumbnailRevision)
	}
	stored, err := db.Library().GetVideo(ctx, domain.AudienceOwner, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThumbnailState != domain.ThumbnailStateDone || stored.ThumbnailRevision != video.ThumbnailRevision {
		t.Fatalf("読み直した動画 = state %s revision %d", stored.ThumbnailState, stored.ThumbnailRevision)
	}
	if _, issues, _ := importIssues(t, db); len(issues) != 0 {
		t.Fatalf("代用の問題が残った: %+v", issues)
	}
	want := []domain.Event{domain.VideoOverrideChanged{VideoID: id}, domain.ScanChanged{}}
	if !slices.Equal(recorder.events, want) {
		t.Errorf("events = %v, want %v", recorder.events, want)
	}

	// 消す問題が無ければ、取り込みの知らせは出さない。
	recorder.events = nil
	setThumbnailPosition(t, db, id, &position)
	if want := []domain.Event{domain.VideoOverrideChanged{VideoID: id}}; !slices.Equal(recorder.events, want) {
		t.Errorf("問題の無い記録の events = %v, want %v", recorder.events, want)
	}
}

// 同じミリ秒の中で指定・解除（行が消える）・指定と続いても、改版番号は解除の前の値と
// 重ならない。重なると、知らせがまとまった画面は同じ URL を受け取り、古い画像を出し続ける。
func TestThumbnailRevisionAcrossClearWithinOneMillisecond(t *testing.T) {
	db, ids := overrideFixture(t)
	ctx := context.Background()
	id := ids[fixturePath("/media/alpha.mp4")]
	var key string
	if err := db.sql.QueryRow(`select content_key from videos where id = ?`, id).Scan(&key); err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1_700_000_000_000)
	position := int64(1_000)
	write := func(positionMs *int64) {
		t.Helper()
		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		if err := db.writeThumbnailPosition(ctx, tx, key, positionMs, now); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	write(&position)
	before, _ := readOverrideRow(t, db, id)
	write(nil)
	if _, ok := readOverrideRow(t, db, id); ok {
		t.Fatal("解除で行が消えていない")
	}
	write(&position)
	after, _ := readOverrideRow(t, db, id)
	if after.revision.Int64 <= before.revision.Int64 {
		t.Fatalf("改版番号 %d → %d（解除の前より大きいはず）", before.revision.Int64, after.revision.Int64)
	}
}

// 同じ位置を指定し直しても改版番号は増える。解除すると位置と改版番号が null になり、
// 表示名も無ければ行が消える。表示名があれば行は残る。
func TestSetThumbnailPositionRevisionAndClear(t *testing.T) {
	db, ids := overrideFixture(t)
	alphaID := ids[fixturePath("/media/alpha.mp4")]
	betaID := ids[fixturePath("/media/beta.mp4")]

	position := int64(0)
	first := setThumbnailPosition(t, db, alphaID, &position)
	second := setThumbnailPosition(t, db, alphaID, &position)
	if second.ThumbnailRevision <= first.ThumbnailRevision {
		t.Fatalf("改版番号 %d → %d（増えるはず）", first.ThumbnailRevision, second.ThumbnailRevision)
	}

	cleared := setThumbnailPosition(t, db, alphaID, nil)
	if cleared.ThumbnailPositionMs != nil || cleared.ThumbnailRevision != 0 ||
		cleared.ThumbnailState != domain.ThumbnailStateDone {
		t.Fatalf("解除後 = position %v revision %d state %s",
			cleared.ThumbnailPositionMs, cleared.ThumbnailRevision, cleared.ThumbnailState)
	}
	if _, ok := readOverrideRow(t, db, alphaID); ok {
		t.Fatal("表示名も位置も無い行が残った")
	}

	// 行を消して作り直しても、改版番号は前の値と重ならない。
	again := setThumbnailPosition(t, db, alphaID, &position)
	if again.ThumbnailRevision <= second.ThumbnailRevision {
		t.Fatalf("作り直した改版番号 %d（%d より大きいはず）", again.ThumbnailRevision, second.ThumbnailRevision)
	}

	setDisplayName(t, db, betaID, "名前")
	setThumbnailPosition(t, db, betaID, &position)
	setThumbnailPosition(t, db, betaID, nil)
	row, ok := readOverrideRow(t, db, betaID)
	if !ok || row.displayName.String != "名前" || row.positionMs.Valid || row.revision.Valid {
		t.Fatalf("表示名のある行 = %+v, %v", row, ok)
	}
	if overrideRowCount(t, db) != 2 {
		t.Fatalf("行の数 = %d, want 2", overrideRowCount(t, db))
	}
}

// 引けない動画は domain.ErrNotFound で、何も書かず発行しない。
func TestSetThumbnailPositionUnknownVideo(t *testing.T) {
	db, _ := overrideFixture(t)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	position := int64(0)
	if _, err := db.Overrides().SetThumbnailPosition(context.Background(), 9999, &position); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if overrideRowCount(t, db) != 0 || len(recorder.events) != 0 {
		t.Fatalf("行 %d・events %v", overrideRowCount(t, db), recorder.events)
	}
}

// ThumbnailSourceCurrent は、所在が今も動画の内容のものかを答える。走査が所在を別の内容へ
// 付け替えたら、元の内容が別の所在に残っていても偽にする。
func TestThumbnailSourceCurrentRejectsReassignedLocation(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b := fixturePath("/media/a.mp4"), fixturePath("/media/b.mp4")
	video, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(a, "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(b, "b", "key-a", 1024, 0)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{a, b} {
		current, err := db.Ingest().ThumbnailSourceCurrent(ctx, video.ID, "key-a", path)
		if err != nil || !current {
			t.Fatalf("%s: current = %v, %v; want true", path, current, err)
		}
	}

	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(a, "replacement", "key-b", 2048, time.Second)); err != nil {
		t.Fatal(err)
	}
	if current, err := db.Ingest().ThumbnailSourceCurrent(ctx, video.ID, "key-a", a); err != nil || current {
		t.Fatalf("付け替わった所在: current = %v, %v; want false", current, err)
	}
	if current, err := db.Ingest().ThumbnailSourceCurrent(ctx, video.ID, "key-a", b); err != nil || !current {
		t.Fatalf("残った所在: current = %v, %v; want true", current, err)
	}
}

// フォルダカードの所在にも位置と改版番号を載せ、サムネイルの URL の版を動画一覧と
// そろえる。解除すれば載らない。
func TestFolderLocationsCarryThumbnailRevision(t *testing.T) {
	db, ids := overrideFixture(t)
	alphaID := ids[fixturePath("/media/alpha.mp4")]
	ctx := context.Background()

	find := func() domain.FolderLocation {
		t.Helper()
		locations, err := db.Library().FolderLocations(ctx, domain.AudienceOwner, fixturePath("/media"))
		if err != nil {
			t.Fatal(err)
		}
		for _, location := range locations {
			if location.VideoID == alphaID {
				return location
			}
		}
		t.Fatalf("所在 %d が見つからない", alphaID)
		return domain.FolderLocation{}
	}

	if location := find(); location.ThumbnailPositionMs != nil || location.ThumbnailRevision != 0 {
		t.Fatalf("指定前 = %v, %d", location.ThumbnailPositionMs, location.ThumbnailRevision)
	}
	position := int64(0)
	video := setThumbnailPosition(t, db, alphaID, &position)
	location := find()
	if location.ThumbnailPositionMs == nil || *location.ThumbnailPositionMs != position ||
		location.ThumbnailRevision != video.ThumbnailRevision {
		t.Fatalf("指定後 = %v, %d（want %d, %d）",
			location.ThumbnailPositionMs, location.ThumbnailRevision, position, video.ThumbnailRevision)
	}
	setThumbnailPosition(t, db, alphaID, nil)
	if location := find(); location.ThumbnailPositionMs != nil || location.ThumbnailRevision != 0 {
		t.Fatalf("解除後 = %v, %d", location.ThumbnailPositionMs, location.ThumbnailRevision)
	}
}
