package store

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 適用したサムネイルの画像（specs/045-comfyui-thumbnails/data-model.md、research.md R-6・R-10）。

func applyThumbnailImage(t *testing.T, db *DB, id int64, key string, image []byte) domain.Video {
	t.Helper()
	video, err := db.Ingest().ApplyThumbnailImage(context.Background(), id, key, image)
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

func editedAtOf(t *testing.T, db *DB, key string) int64 {
	t.Helper()
	var editedAt int64
	if err := db.sql.QueryRow(`select coalesce((select edited_at from video_edits where content_key = ?), 0)`, key).
		Scan(&editedAt); err != nil {
		t.Fatal(err)
	}
	return editedAt
}

// 適用は行を書き、その内容の動画を done にして改版番号を返し、更新日時を進める。確定後に
// VideoOverrideChanged を発行する。適用し直すと改版番号が増える。
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

// 動画が無いか、別の内容を持つなら content_changed で、何も書かない。
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
			_, err := db.Ingest().ApplyThumbnailImage(context.Background(), tc.id, tc.key, []byte("jpeg"))
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

// 位置は画像の適用中も残り、位置を設定すると画像の行が消える。
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
	if moved.ThumbnailImageRevision != 0 || thumbnailImageOf(t, db, "key-alpha") != nil {
		t.Fatalf("位置の設定で画像の行が消えない: revision %d", moved.ThumbnailImageRevision)
	}
	// 同じ位置の指定し直しでも、画像を外したので更新日時は進む。
	if editedAtOf(t, db, "key-alpha") == 1 {
		t.Fatal("画像を外したのに更新日時が進まない")
	}
}

// 取り消しは行を消して位置を残し、done にする。画像が無くても成功し、更新日時は進めない。
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
	if reverted.ThumbnailImageRevision != 0 || thumbnailImageOf(t, db, "key-alpha") != nil {
		t.Fatalf("行が残った: revision %d", reverted.ThumbnailImageRevision)
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
	if editedAtOf(t, db, "key-alpha") != 1 {
		t.Fatal("何も外さない取り消しで更新日時が進んだ")
	}
	if _, err := db.Ingest().RevertThumbnailImage(context.Background(), 9999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("無い動画: err = %v", err)
	}
}

// 画像は内容に結ぶので、動画の行を消して同じ内容を取り込み直しても残り、新しい行に改版番号が
// 付く。フォルダのサムネイルにも改版番号が載る。
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
