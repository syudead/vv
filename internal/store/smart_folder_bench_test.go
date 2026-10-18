package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Measure third-level pages against the one-second budget; root pages are a reference measurement.
func BenchmarkSmartFolderReads(b *testing.B) {
	db, cat := smartFolderBenchDB(b)
	ctx := context.Background()
	thirty := 30 * 60
	root := benchSmartFolder(b, db, "猫", domain.SmartRule{Tags: []int64{cat}}, 0)
	child := benchSmartFolder(b, db, "未視聴", domain.SmartRule{Watch: domain.WatchUnwatched}, root)
	third := benchSmartFolder(b, db, "短い", domain.SmartRule{MaxDuration: &thirty}, child)
	for _, read := range []struct {
		name string
		id   int64
	}{{"third level", third}, {"root with a child", root}} {
		b.Run(read.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := db.SmartFolders().SmartFolderContents(ctx, domain.AudienceOwner, read.id, "", 0, ""); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Measure library and 24-card reads against a one-second budget using 20,000 videos and 50 varied-rule folders.
func BenchmarkLibraryWithSmartFolders(b *testing.B) {
	db, cat := smartFolderBenchDB(b)
	ctx := context.Background()
	read := func(b *testing.B) {
		b.Helper()
		for b.Loop() {
			if _, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{}); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.Run("no folders", read)
	ids := make([]int64, 0, 50)
	for i := range 50 {
		bound := (i%10 + 1) * 6 * 60
		days := i + 1
		rules := []domain.SmartRule{
			{Tags: []int64{cat}},
			{Watch: domain.WatchUnwatched},
			{MaxDuration: &bound},
			{AddedWithin: &days},
			{Q: fmt.Sprintf("series-%02d", i%40)},
		}
		ids = append(ids, benchSmartFolder(b, db, fmt.Sprintf("folder-%02d", i), rules[i%len(rules)], 0))
	}
	b.Run("50 root folders", read)
	b.Run("cards of 24 folders", func(b *testing.B) {
		for b.Loop() {
			cards, err := db.SmartFolders().SmartFolderCards(ctx, domain.AudienceOwner, ids[:24])
			if err != nil {
				b.Fatal(err)
			}
			if len(cards) != 24 {
				b.Fatalf("cards = %d, want 24", len(cards))
			}
		}
	})
}

// Use 20,000 videos in 4,000 automatic groups, tagging half and recording progress for every third video.
func smartFolderBenchDB(b *testing.B) (*DB, int64) {
	b.Helper()
	db, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := Migrate(ctx, db); err != nil {
		b.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values (?, 1, 1, 1)`, fixturePath("/media")); err != nil {
		b.Fatal(err)
	}
	cat, err := db.Tags().CreateTag(ctx, "猫")
	if err != nil {
		b.Fatal(err)
	}
	tx, err := db.sql.Begin()
	if err != nil {
		b.Fatal(err)
	}
	const videos = 20000
	for i := range videos {
		path := fmt.Sprintf(fixturePath("/media/series-%02d/season-%03d/episode-%d.mp4"), i%40, (i/40)%10, i)
		key := fmt.Sprintf("key-%d", i)
		res, err := tx.Exec(`insert into videos (added_at, indexed_at, content_key, duration_ms, playable, probe_state, thumbnail_state)
			values (?, 1, ?, ?, 1, 'done', 'pending')`, i, key, (i%120)*60_000)
		if err != nil {
			b.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			b.Fatal(err)
		}
		if _, err := tx.Exec(`insert into video_locations (video_id, path, version, title, size_bytes, mtime, created_at, updated_at)
			values (?, ?, 1, ?, 1, 1, 1, 1)`, id, path, path); err != nil {
			b.Fatal(err)
		}
		if i%2 == 0 {
			if _, err := tx.Exec(`insert into video_tags (content_key, tag_id, created_at) values (?, ?, 1)`, key, cat.ID); err != nil {
				b.Fatal(err)
			}
		}
		if i%3 == 0 {
			if _, err := tx.Exec(`insert into playback_progress (content_key, position_ms, duration_ms, completed, updated_at)
				values (?, 1000, 60000, ?, ?)`, key, i%2, i); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		b.Fatal(err)
	}
	return db, cat.ID
}

func benchSmartFolder(b *testing.B, db *DB, name string, rule domain.SmartRule, parent int64) int64 {
	b.Helper()
	folder, err := db.SmartFolders().CreateSmartFolder(context.Background(),
		domain.SmartFolderCreate{Name: name, Rule: rule, ParentID: parent})
	if err != nil {
		b.Fatal(err)
	}
	return folder.ID
}
