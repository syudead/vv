package store

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

func applyThumbnailImage(t *testing.T, db *DB, id int64, key string, image []byte) domain.Video {
	t.Helper()
	video, err := db.Ingest().ApplyThumbnailImage(context.Background(), id, key, domain.AppliedThumbnailImage{
		Image: image, Source: append([]byte("source:"), image...), Origin: domain.ThumbnailImageGenerated,
	})
	if err != nil {
		t.Fatalf("画像を適用できない: %v", err)
	}
	return video
}

func thumbnailImageOf(t *testing.T, db *DB, key string) []byte {
	t.Helper()
	image, err := db.Ingest().ThumbnailImage(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return image
}

func thumbnailImageRows(t *testing.T, db *DB) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from thumbnail_images`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func editedAtOf(t *testing.T, db *DB, key string) int64 {
	t.Helper()
	var editedAt int64
	if err := db.sql.QueryRow(`select coalesce((select edited_at from video_edits where content_key = ?), 0)`, key).
		Scan(&editedAt); err != nil {
		t.Fatal(err)
	}
	return editedAt
}

func TestApplyThumbnailImageRecordsRow(t *testing.T) {
	db, ids := overrideFixture(t)
	id := ids[fixturePath("/media/alpha.mp4")]
	if _, err := db.sql.Exec(`update videos set thumbnail_state = 'failed' where id = ?`, id); err != nil {
		t.Fatal(err)
	}
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	first := applyThumbnailImage(t, db, id, "key-alpha", []byte("jpeg-1"))
	if first.ThumbnailImageRevision == 0 || first.ThumbnailState != domain.ThumbnailStateDone {
		t.Fatalf("返した動画 = revision %d state %s", first.ThumbnailImageRevision, first.ThumbnailState)
	}
	if got := thumbnailImageOf(t, db, "key-alpha"); !bytes.Equal(got, []byte("jpeg-1")) {
		t.Fatalf("行 = %q", got)
	}
	if editedAtOf(t, db, "key-alpha") == 0 {
		t.Fatal("更新日時が進まない")
	}
	if want := []domain.Event{domain.VideoOverrideChanged{VideoID: id}}; !slices.Equal(recorder.events, want) {
		t.Fatalf("events = %v, want %v", recorder.events, want)
	}

	second := applyThumbnailImage(t, db, id, "key-alpha", []byte("jpeg-2"))
	if second.ThumbnailImageRevision <= first.ThumbnailImageRevision {
		t.Fatalf("改版番号 %d → %d（増えるはず）", first.ThumbnailImageRevision, second.ThumbnailImageRevision)
	}
	if got := thumbnailImageOf(t, db, "key-alpha"); !bytes.Equal(got, []byte("jpeg-2")) {
		t.Fatalf("行 = %q", got)
	}
	stored, err := db.Library().GetVideo(context.Background(), domain.AudienceOwner, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThumbnailImageRevision != second.ThumbnailImageRevision {
		t.Fatalf("読み直した改版番号 = %d", stored.ThumbnailImageRevision)
	}
	other, err := db.Library().GetVideo(context.Background(), domain.AudienceOwner, ids[fixturePath("/media/beta.mp4")])
	if err != nil {
		t.Fatal(err)
	}
	if other.ThumbnailImageRevision != 0 {
		t.Fatalf("別の内容に改版番号が付いた: %d", other.ThumbnailImageRevision)
	}
}

func TestApplyThumbnailImageRecordsSourceAndOrigin(t *testing.T) {
	db, ids := overrideFixture(t)
	ctx := context.Background()
	id := ids[fixturePath("/media/alpha.mp4")]

	uploaded, err := db.Ingest().ApplyThumbnailImage(ctx, id, "key-alpha", domain.AppliedThumbnailImage{
		Image: []byte("jpeg-640"), Source: []byte("jpeg-2048"), Origin: domain.ThumbnailImageUploaded,
	})
	if err != nil {
		t.Fatal(err)
	}
	if uploaded.ThumbnailImageOrigin != domain.ThumbnailImageUploaded {
		t.Fatalf("返した動画の出どころ = %q", uploaded.ThumbnailImageOrigin)
	}
	source, err := db.Ingest().ThumbnailImageSource(ctx, "key-alpha")
	if err != nil || !bytes.Equal(source, []byte("jpeg-2048")) {
		t.Fatalf("元の画像 = %q, %v", source, err)
	}
	if got := thumbnailImageOf(t, db, "key-alpha"); !bytes.Equal(got, []byte("jpeg-640")) {
		t.Fatalf("行の画像 = %q", got)
	}

	applyThumbnailImage(t, db, id, "key-alpha", []byte("jpeg-2"))
	stored, err := db.Library().GetVideo(ctx, domain.AudienceOwner, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ThumbnailImageOrigin != domain.ThumbnailImageGenerated {
		t.Fatalf("読み直した出どころ = %q", stored.ThumbnailImageOrigin)
	}
	if source, _ := db.Ingest().ThumbnailImageSource(ctx, "key-alpha"); !bytes.Equal(source, []byte("source:jpeg-2")) {
		t.Fatalf("適用し直した元の画像 = %q", source)
	}
	other, err := db.Library().GetVideo(ctx, domain.AudienceOwner, ids[fixturePath("/media/beta.mp4")])
	if err != nil {
		t.Fatal(err)
	}
	if other.ThumbnailImageOrigin != "" {
		t.Fatalf("画像の無い動画の出どころ = %q", other.ThumbnailImageOrigin)
	}
	if source, err := db.Ingest().ThumbnailImageSource(ctx, "key-beta"); source != nil || err != nil {
		t.Fatalf("画像の無い内容の元の画像 = %q, %v", source, err)
	}
}

func TestApplyThumbnailImageRejectsIncompleteRow(t *testing.T) {
	db, ids := overrideFixture(t)
	id := ids[fixturePath("/media/alpha.mp4")]
	for name, image := range map[string]domain.AppliedThumbnailImage{
		"空の画像":    {Source: []byte("s"), Origin: domain.ThumbnailImageGenerated},
		"空の元の画像":  {Image: []byte("i"), Origin: domain.ThumbnailImageGenerated},
		"未知の出どころ": {Image: []byte("i"), Source: []byte("s"), Origin: "pasted"},
		"出どころが無い": {Image: []byte("i"), Source: []byte("s")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := db.Ingest().ApplyThumbnailImage(context.Background(), id, "key-alpha", image); err == nil {
				t.Fatal("誤りにならない")
			}
			if n := thumbnailImageRows(t, db); n != 0 {
				t.Fatalf("書いた: 行 %d", n)
			}
		})
	}
}

func TestApplyThumbnailImageContentChanged(t *testing.T) {
	db, ids := overrideFixture(t)
	id := ids[fixturePath("/media/alpha.mp4")]
	for _, tc := range []struct {
		name string
		id   int64
		key  string
	}{
		{name: "どの動画も持たない内容", id: id, key: "key-gone"},
		{name: "別の動画の内容", id: id, key: "key-beta"},
		{name: "動画が無い", id: 9999, key: "key-alpha"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.Ingest().ApplyThumbnailImage(context.Background(), tc.id, tc.key, domain.AppliedThumbnailImage{
				Image: []byte("jpeg"), Source: []byte("source"), Origin: domain.ThumbnailImageUploaded,
			})
			if !errors.Is(err, domain.ErrContentChanged) {
				t.Fatalf("err = %v, want ErrContentChanged", err)
			}
			var count int
			if err := db.sql.QueryRow(`select count(*) from thumbnail_images`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 || editedAtOf(t, db, tc.key) != 0 {
				t.Fatalf("書いた: 行 %d", count)
			}
		})
	}
}

func TestThumbnailPositionAndAppliedImage(t *testing.T) {
	db, ids := overrideFixture(t)
	id := ids[fixturePath("/media/alpha.mp4")]
	position := int64(1_500)
	setThumbnailPosition(t, db, id, &position)

	applied := applyThumbnailImage(t, db, id, "key-alpha", []byte("jpeg"))
	if applied.ThumbnailPositionMs == nil || *applied.ThumbnailPositionMs != position {
		t.Fatalf("適用で位置が消えた: %v", applied.ThumbnailPositionMs)
	}

	if _, err := db.sql.Exec(`update video_edits set edited_at = 1`); err != nil {
		t.Fatal(err)
	}
	moved := setThumbnailPosition(t, db, id, &position)
	if moved.ThumbnailImageRevision != 0 || moved.ThumbnailImageOrigin != "" || thumbnailImageRows(t, db) != 0 {
		t.Fatalf("位置の設定で画像の行（画像・元の画像・出どころ）が消えない: revision %d origin %q",
			moved.ThumbnailImageRevision, moved.ThumbnailImageOrigin)
	}
	// Removing an applied image is an edit even when the requested frame position stays unchanged.
	if editedAtOf(t, db, "key-alpha") == 1 {
		t.Fatal("画像を外したのに更新日時が進まない")
	}
}

func TestRevertThumbnailImage(t *testing.T) {
	db, ids := overrideFixture(t)
	id := ids[fixturePath("/media/alpha.mp4")]
	position := int64(2_000)
	setThumbnailPosition(t, db, id, &position)
	applyThumbnailImage(t, db, id, "key-alpha", []byte("jpeg"))
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	reverted, err := db.Ingest().RevertThumbnailImage(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if reverted.ThumbnailImageRevision != 0 || reverted.ThumbnailImageOrigin != "" || thumbnailImageRows(t, db) != 0 {
		t.Fatalf("行（画像・元の画像・出どころ）が残った: revision %d origin %q",
			reverted.ThumbnailImageRevision, reverted.ThumbnailImageOrigin)
	}
	if reverted.ThumbnailPositionMs == nil || *reverted.ThumbnailPositionMs != position {
		t.Fatalf("位置 = %v", reverted.ThumbnailPositionMs)
	}
	if want := []domain.Event{domain.VideoOverrideChanged{VideoID: id}}; !slices.Equal(recorder.events, want) {
		t.Fatalf("events = %v, want %v", recorder.events, want)
	}

	if _, err := db.sql.Exec(`update video_edits set edited_at = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().RevertThumbnailImage(context.Background(), id); err != nil {
		t.Fatalf("画像の無い取り消し: %v", err)
	}
	if editedAtOf(t, db, "key-alpha") == 1 {
		t.Fatal("画像の無い取り消しで更新日時が進まない")
	}
	if _, err := db.Ingest().RevertThumbnailImage(context.Background(), 9999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("無い動画: err = %v", err)
	}
}

func TestThumbnailImageSurvivesVideoDeletion(t *testing.T) {
	db, ids := overrideFixture(t)
	ctx := context.Background()
	path := fixturePath("/media/alpha.mp4")
	applied := applyThumbnailImage(t, db, ids[path], "key-alpha", []byte("jpeg"))

	if err := db.ScanIndex().DeleteVideos(ctx, []int64{ids[path]}); err != nil {
		t.Fatal(err)
	}
	again := upsertAll(t, db, listingFile(path, "alpha", "key-alpha", 1))
	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, again[path])
	if err != nil {
		t.Fatal(err)
	}
	if video.ThumbnailImageRevision != applied.ThumbnailImageRevision {
		t.Fatalf("改版番号 = %d, want %d", video.ThumbnailImageRevision, applied.ThumbnailImageRevision)
	}
	if got := thumbnailImageOf(t, db, "key-alpha"); !bytes.Equal(got, []byte("jpeg")) {
		t.Fatalf("行 = %q", got)
	}

	locations, err := db.Library().FolderLocations(ctx, domain.AudienceOwner, fixturePath("/media"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, location := range locations {
		if location.ContentKey == "key-alpha" {
			found = location.ThumbnailImageRevision == applied.ThumbnailImageRevision
		}
	}
	if !found {
		t.Fatalf("フォルダの所在に改版番号が載らない: %+v", locations)
	}
}

// Legacy images have NULL sources and generated origin; also exercise invariant rejection of empty sources and invalid origins.
func TestThumbnailImageSourcesMigration(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 40); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into thumbnail_images (content_key, image, revision, applied_at)
		values ('key-old', x'ffd8', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 41); err != nil {
		t.Fatal(err)
	}

	var origin string
	if err := db.sql.QueryRow(`select origin from thumbnail_images where content_key = 'key-old'`).Scan(&origin); err != nil {
		t.Fatal(err)
	}
	if origin != string(domain.ThumbnailImageGenerated) {
		t.Fatalf("移行前の行の出どころ = %q", origin)
	}
	if source, err := db.Ingest().ThumbnailImageSource(ctx, "key-old"); source != nil || err != nil {
		t.Fatalf("移行前の行の元の画像 = %q, %v", source, err)
	}
	if image := thumbnailImageOf(t, db, "key-old"); !bytes.Equal(image, []byte{0xff, 0xd8}) {
		t.Fatalf("移行前の行の画像 = %x", image)
	}
	if violations, err := thumbnailImageViolations(db); err != nil || len(violations) != 0 {
		t.Fatalf("移行前の行が不変条件に反する: %v, %v", violations, err)
	}

	if _, err := db.sql.Exec(`insert into thumbnail_images (content_key, image, source, origin, revision, applied_at)
		values ('key-empty', x'ffd8', x'', 'generated', 1, 1), ('key-odd', x'ffd8', x'ffd8', 'pasted', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	violations, err := thumbnailImageViolations(db)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"空の元の画像の行が 1 行ある", "出どころが未知の行が 1 行ある"}
	if !slices.Equal(violations, want) {
		t.Fatalf("不変条件の検査 = %v, want %v", violations, want)
	}
	if _, err := db.sql.Exec(`delete from thumbnail_images where content_key in ('key-empty', 'key-odd')`); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.DownTo(ctx, 40); err != nil {
		t.Fatal(err)
	}
	columns := tableColumns(t, db, "thumbnail_images")
	if columns["source"] || columns["origin"] {
		t.Fatalf("Down のあとも列が残った: %v", columns)
	}
	if image := thumbnailImageOf(t, db, "key-old"); !bytes.Equal(image, []byte{0xff, 0xd8}) {
		t.Fatalf("Down のあとの行の画像 = %x", image)
	}
}

func TestCurrentThumbnailSelection(t *testing.T) {
	db, ids := overrideFixture(t)
	ctx := context.Background()
	id := ids[fixturePath("/media/alpha.mp4")]
	read := func() domain.CurrentThumbnailSelection {
		t.Helper()
		selection, err := db.Overrides().CurrentThumbnailSelection(ctx, "key-alpha")
		if err != nil {
			t.Fatal(err)
		}
		return selection
	}

	if got := read(); got.Image != nil || got.PositionMs != nil {
		t.Fatalf("何も無い内容の選び方 = %+v", got)
	}
	position := int64(12_000)
	if _, err := db.Ingest().SetThumbnailPosition(ctx, id, &position); err != nil {
		t.Fatal(err)
	}
	if got := read(); got.Image != nil || got.PositionMs == nil || *got.PositionMs != position {
		t.Fatalf("位置だけの選び方 = %+v", got)
	}
	applyThumbnailImage(t, db, id, "key-alpha", []byte("jpeg-640"))
	if got := read(); !bytes.Equal(got.Image, []byte("source:jpeg-640")) || got.PositionMs == nil {
		t.Fatalf("画像を適用した選び方 = %+v", got)
	}
	if _, err := db.sql.Exec(`update thumbnail_images set source = null where content_key = 'key-alpha'`); err != nil {
		t.Fatal(err)
	}
	if got := read(); !bytes.Equal(got.Image, []byte("jpeg-640")) {
		t.Fatalf("元の画像を持たない行の選び方 = %+v", got)
	}
	if got, err := db.Overrides().CurrentThumbnailSelection(ctx, ""); err != nil || got.Image != nil || got.PositionMs != nil {
		t.Fatalf("内容の無い選び方 = %+v, %v", got, err)
	}
}
