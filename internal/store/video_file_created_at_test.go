package store

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

func upsertCreated(t *testing.T, db *DB, path, key string, mtime, created time.Time) int64 {
	t.Helper()
	got, err := db.ScanIndex().UpsertVideo(context.Background(), domain.VideoFile{
		Path: fixturePath(path), Title: "t", ContentKey: key, SizeBytes: 10,
		MTime: mtime, FileCreatedAt: created, Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	return got.ID
}

func locationCreatedAt(t *testing.T, db *DB, path string) sql.NullInt64 {
	t.Helper()
	var value sql.NullInt64
	if err := db.sql.QueryRow(`select file_created_at from video_locations where path = ?`, fixturePath(path)).
		Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestUpsertVideoStoresFileCreatedAt(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	mtime := fixedTime.Add(time.Hour)
	created := fixedTime.Add(-time.Hour)
	withID := upsertCreated(t, db, "/media/with.mp4", "key-with", mtime, created)
	withoutID := upsertCreated(t, db, "/media/without.mp4", "key-without", mtime, time.Time{})

	if got := locationCreatedAt(t, db, "/media/with.mp4"); !got.Valid || got.Int64 != created.Unix() {
		t.Errorf("file_created_at = %v, want %d", got, created.Unix())
	}
	if got := locationCreatedAt(t, db, "/media/without.mp4"); got.Valid {
		t.Errorf("file_created_at = %v, want null", got)
	}

	if got := ownerVideo(t, db, withID); !got.FileCreatedAt.Equal(created) {
		t.Errorf("GetVideo の FileCreatedAt = %v, want %v", got.FileCreatedAt, created)
	}
	if got := ownerVideo(t, db, withoutID); !got.FileCreatedAt.Equal(got.MTime) || !got.MTime.Equal(mtime) {
		t.Errorf("GetVideo の FileCreatedAt = %v, MTime = %v, want どちらも %v", got.FileCreatedAt, got.MTime, mtime)
	}

	page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]time.Time{withID: created, withoutID: mtime}
	if len(page.Items) != len(want) {
		t.Fatalf("ListVideos = %d 件, want %d 件", len(page.Items), len(want))
	}
	for _, item := range page.Items {
		if !item.FileCreatedAt.Equal(want[item.ID]) {
			t.Errorf("ListVideos の動画 %d の FileCreatedAt = %v, want %v", item.ID, item.FileCreatedAt, want[item.ID])
		}
	}

	locations, err := db.Library().VideoLocations(ctx, withID)
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 1 || !locations[0].FileCreatedAt.Equal(created) {
		t.Errorf("VideoLocations = %+v, want FileCreatedAt %v", locations, created)
	}
	locations, err = db.Library().VideoLocations(ctx, withoutID)
	if err != nil {
		t.Fatal(err)
	}
	if len(locations) != 1 || !locations[0].FileCreatedAt.IsZero() {
		t.Errorf("VideoLocations = %+v, want FileCreatedAt のゼロ値", locations)
	}

	indexed, err := db.ScanIndex().IndexedVideosByPath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := indexed[fixturePath("/media/with.mp4")].FileCreatedAt; !got.Equal(created) {
		t.Errorf("IndexedVideosByPath の FileCreatedAt = %v, want %v", got, created)
	}
	if got := indexed[fixturePath("/media/without.mp4")].FileCreatedAt; !got.IsZero() {
		t.Errorf("IndexedVideosByPath の FileCreatedAt = %v, want ゼロ値", got)
	}
}

func TestUpsertVideoRewritesFileCreatedAtOnChange(t *testing.T) {
	db := migratedDB(t)
	created := fixedTime.Add(-time.Hour)
	upsertCreated(t, db, "/media/a.mp4", "key-1", fixedTime, created)
	upsertCreated(t, db, "/media/a.mp4", "key-2", fixedTime.Add(time.Minute), time.Time{})
	if got := locationCreatedAt(t, db, "/media/a.mp4"); got.Valid {
		t.Errorf("file_created_at = %v, want null", got)
	}
	later := fixedTime.Add(-time.Minute)
	upsertCreated(t, db, "/media/a.mp4", "key-3", fixedTime.Add(2*time.Minute), later)
	if got := locationCreatedAt(t, db, "/media/a.mp4"); !got.Valid || got.Int64 != later.Unix() {
		t.Errorf("file_created_at = %v, want %d", got, later.Unix())
	}
}

func TestUpdateLocationCreatedAt(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	mtime := fixedTime.Add(time.Hour)
	id := upsertCreated(t, db, "/media/a.mp4", "key-a", mtime, time.Time{})
	if _, err := db.sql.Exec(`update video_locations set updated_at = 5 where path = ?`, fixturePath("/media/a.mp4")); err != nil {
		t.Fatal(err)
	}
	locations, err := db.Library().VideoLocations(ctx, id)
	if err != nil || len(locations) != 1 {
		t.Fatalf("VideoLocations = %+v, %v", locations, err)
	}
	before := locations[0]

	created := fixedTime.Add(-time.Hour)
	if err := db.ScanIndex().UpdateLocationCreatedAt(ctx, before.ID, created); err != nil {
		t.Fatal(err)
	}
	if got := ownerVideo(t, db, id).FileCreatedAt; !got.Equal(created) {
		t.Errorf("FileCreatedAt = %v, want %v", got, created)
	}

	if err := db.ScanIndex().UpdateLocationCreatedAt(ctx, before.ID, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if got := locationCreatedAt(t, db, "/media/a.mp4"); got.Valid {
		t.Errorf("file_created_at = %v, want null", got)
	}
	if got := ownerVideo(t, db, id).FileCreatedAt; !got.Equal(mtime) {
		t.Errorf("FileCreatedAt = %v, want mtime %v", got, mtime)
	}

	locations, err = db.Library().VideoLocations(ctx, id)
	if err != nil || len(locations) != 1 {
		t.Fatalf("VideoLocations = %+v, %v", locations, err)
	}
	if after := locations[0]; after.Version != before.Version || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("version・updated_at = %d・%v, want %d・%v（変わらない）", after.Version, after.UpdatedAt, before.Version, before.UpdatedAt)
	}
}

func TestVideoReadsCarryMTimeWithNanoseconds(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	precise := fixedTime.Add(123_456_789 * time.Nanosecond)
	id := upsertCreated(t, db, "/media/precise.mp4", "key-precise", precise, time.Time{})
	legacyID := upsertCreated(t, db, "/media/legacy.mp4", "key-legacy", precise, time.Time{})
	// Rows written before mtime_ns existed keep whole seconds only.
	if _, err := db.sql.Exec(`update video_locations set mtime_ns = null where path = ?`, fixturePath("/media/legacy.mp4")); err != nil {
		t.Fatal(err)
	}
	want := map[int64]time.Time{id: precise, legacyID: fixedTime}

	for videoID, at := range want {
		if got := ownerVideo(t, db, videoID).MTime; !got.Equal(at) {
			t.Errorf("GetVideo(%d) の MTime = %v, want %v", videoID, got, at)
		}
	}
	page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != len(want) {
		t.Fatalf("ListVideos = %d 件, want %d 件", len(page.Items), len(want))
	}
	for _, item := range page.Items {
		if !item.MTime.Equal(want[item.ID]) {
			t.Errorf("ListVideos の動画 %d の MTime = %v, want %v", item.ID, item.MTime, want[item.ID])
		}
	}
}

func TestMigrationLeavesExistingFileCreatedAtNull(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsy, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsy)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, 27); err != nil {
		t.Fatal(err)
	}
	res, err := db.sql.Exec(`insert into videos (content_key, added_at, updated_at) values ('key-old', 1, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	videoID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into video_locations (video_id, path, title, size_bytes, mtime, created_at, updated_at)
		values (?, '/media/old.mp4', 'old', 1, 1, 1, 1)`, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 28); err != nil {
		t.Fatal(err)
	}
	var value sql.NullInt64
	if err := db.sql.QueryRow(`select file_created_at from video_locations where path = '/media/old.mp4'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value.Valid {
		t.Errorf("file_created_at = %v, want null", value)
	}
}
