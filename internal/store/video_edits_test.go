package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

// 動画の更新日時（specs/033-video-dates/data-model.md §1・§3・§4、research.md R-1〜R-3）。

// staleEditedAt は「進んでいない」ことを見分けるために更新日時へ書き戻す過去の値である。
var staleEditedAt = time.Unix(1_000, 0)

// editedAt は動画の更新日時を GetVideo で読む。
func editedAt(t *testing.T, db *DB, id int64) time.Time {
	t.Helper()
	return ownerVideo(t, db, id).EditedAt
}

// resetEditedAt は記録済みの更新日時をすべて staleEditedAt に戻す。
func resetEditedAt(t *testing.T, db *DB) {
	t.Helper()
	if _, err := db.sql.Exec(`update video_edits set edited_at = ?`, staleEditedAt.Unix()); err != nil {
		t.Fatal(err)
	}
}

// assertEditedSince は動画の更新日時が since 以後（操作の時刻）であることを確かめる。
func assertEditedSince(t *testing.T, db *DB, id int64, since time.Time) {
	t.Helper()
	if got := editedAt(t, db, id); got.Before(since.Truncate(time.Second)) || got.After(time.Now()) {
		t.Errorf("動画 %d の EditedAt = %v, want 操作の時刻（%v 以後）", id, got, since)
	}
}

// assertEditedAt は動画の更新日時が want であることを確かめる。
func assertEditedAt(t *testing.T, db *DB, id int64, want time.Time) {
	t.Helper()
	if got := editedAt(t, db, id); !got.Equal(want) {
		t.Errorf("動画 %d の EditedAt = %v, want %v", id, got, want)
	}
}

func editRowCount(t *testing.T, db *DB) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from video_edits`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// 編集していない動画の更新日時は追加日時で、取り込み（UpsertVideo）は行を作らない
// （受け入れ条件 3）。一覧の読み出しにも同じ値が載る。
func TestEditedAtDefaultsToAddedAt(t *testing.T) {
	db, ids := overrideFixture(t)
	for _, id := range ids {
		video := ownerVideo(t, db, id)
		if !video.EditedAt.Equal(video.AddedAt) {
			t.Errorf("動画 %d の EditedAt = %v, want AddedAt %v", id, video.EditedAt, video.AddedAt)
		}
	}
	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if !item.EditedAt.Equal(item.AddedAt) {
			t.Errorf("一覧の動画 %d の EditedAt = %v, want AddedAt %v", item.ID, item.EditedAt, item.AddedAt)
		}
	}
	if n := editRowCount(t, db); n != 0 {
		t.Errorf("video_edits = %d 行, want 0", n)
	}
}

// 表示名・代表サムネイルの位置・公開の設定・タグの付け外しは、値を変えたときだけ更新日時を
// 操作の時刻に進める（受け入れ条件 1、Edge Case「値が変わらなかった」）。
func TestEditsAdvanceEditedAtOnlyWhenChanged(t *testing.T) {
	ctx := context.Background()
	position := int64(5_000)
	other := int64(7_000)
	for name, c := range map[string]struct {
		// prepare は最初の変更、again は同じ値の書き直し、change は別の値への変更である。
		prepare, again, change func(t *testing.T, db *DB, id int64) error
	}{
		"表示名": {
			prepare: func(t *testing.T, db *DB, id int64) error {
				_, err := db.Overrides().SetDisplayName(ctx, id, "名前")
				return err
			},
			again: func(t *testing.T, db *DB, id int64) error {
				_, err := db.Overrides().SetDisplayName(ctx, id, " 名前 ")
				return err
			},
			change: func(t *testing.T, db *DB, id int64) error {
				_, err := db.Overrides().SetDisplayName(ctx, id, "")
				return err
			},
		},
		"代表サムネイルの位置": {
			prepare: func(t *testing.T, db *DB, id int64) error {
				_, err := db.Overrides().SetThumbnailPosition(ctx, id, &position)
				return err
			},
			again: func(t *testing.T, db *DB, id int64) error {
				same := position
				_, err := db.Overrides().SetThumbnailPosition(ctx, id, &same)
				return err
			},
			change: func(t *testing.T, db *DB, id int64) error {
				_, err := db.Overrides().SetThumbnailPosition(ctx, id, &other)
				return err
			},
		},
		"公開の設定": {
			prepare: func(t *testing.T, db *DB, id int64) error {
				_, err := db.Visibility().SetVideosPublic(ctx, []int64{id}, true)
				return err
			},
			again: func(t *testing.T, db *DB, id int64) error {
				_, err := db.Visibility().SetVideosPublic(ctx, []int64{id}, true)
				return err
			},
			change: func(t *testing.T, db *DB, id int64) error {
				_, err := db.Visibility().SetVideosPublic(ctx, []int64{id}, false)
				return err
			},
		},
		"タグ（画面）": {
			prepare: func(t *testing.T, db *DB, id int64) error {
				_, _, err := db.Tags().AttachTagByName(ctx, []int64{id}, "好き")
				return err
			},
			again: func(t *testing.T, db *DB, id int64) error {
				ref, _, err := db.Tags().AttachTagByName(ctx, []int64{id}, "好き")
				if err != nil {
					return err
				}
				_, _, err = db.Tags().AttachTagByID(ctx, []int64{id}, ref.ID)
				return err
			},
			change: func(t *testing.T, db *DB, id int64) error {
				ref, _, err := db.Tags().AttachTagByName(ctx, nil, "好き")
				if err != nil {
					return err
				}
				_, _, err = db.Tags().DetachTag(ctx, []int64{id}, ref.ID)
				return err
			},
		},
		"タグ（外部連携の一括）": {
			prepare: func(t *testing.T, db *DB, id int64) error {
				_, err := db.Tags().ApplyVideoTags(ctx, []domain.VideoRef{{ID: id}}, domain.VideoTagsAdd, []string{"x", "y"}, false)
				return err
			},
			again: func(t *testing.T, db *DB, id int64) error {
				refs := []domain.VideoRef{{ID: id}}
				if _, err := db.Tags().ApplyVideoTags(ctx, refs, domain.VideoTagsAdd, []string{"x"}, false); err != nil {
					return err
				}
				if _, err := db.Tags().ApplyVideoTags(ctx, refs, domain.VideoTagsReplace, []string{"x", "y"}, false); err != nil {
					return err
				}
				_, err := db.Tags().ApplyVideoTags(ctx, refs, domain.VideoTagsRemove, []string{"z"}, false)
				return err
			},
			change: func(t *testing.T, db *DB, id int64) error {
				_, err := db.Tags().ApplyVideoTags(ctx, []domain.VideoRef{{ID: id}}, domain.VideoTagsReplace, []string{"x"}, false)
				return err
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			db, ids := overrideFixture(t)
			id := ids[fixturePath("/media/alpha.mp4")]
			untouched := ids[fixturePath("/media/beta.mp4")]
			addedAt := ownerVideo(t, db, untouched).AddedAt

			start := time.Now()
			if err := c.prepare(t, db, id); err != nil {
				t.Fatal(err)
			}
			assertEditedSince(t, db, id, start)
			assertEditedAt(t, db, untouched, addedAt)

			resetEditedAt(t, db)
			if err := c.again(t, db, id); err != nil {
				t.Fatal(err)
			}
			assertEditedAt(t, db, id, staleEditedAt)

			start = time.Now()
			if err := c.change(t, db, id); err != nil {
				t.Fatal(err)
			}
			assertEditedSince(t, db, id, start)
			assertEditedAt(t, db, untouched, addedAt)
		})
	}
}

// SetDisplayNames の 1 回の一括は取引の初めと最後の名前を比べる。同じ動画を A→B→A と書いても
// 進めず、名前が変わった動画だけを進める（Edge Case）。
func TestSetDisplayNamesComparesTheFinalName(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	beta := ids[fixturePath("/media/beta.mp4")]
	setDisplayName(t, db, alpha, "A")
	resetEditedAt(t, db)
	betaAdded := ownerVideo(t, db, beta).AddedAt

	start := time.Now()
	if _, err := db.Overrides().SetDisplayNames(context.Background(), []domain.DisplayNameChange{
		{Video: domain.VideoRef{ID: alpha}, DisplayName: "B"},
		{Video: domain.VideoRef{ContentKey: "key-beta"}, DisplayName: "新しい"},
		{Video: domain.VideoRef{ID: alpha}, DisplayName: "A"},
	}); err != nil {
		t.Fatal(err)
	}
	assertEditedAt(t, db, alpha, staleEditedAt)
	assertEditedSince(t, db, beta, start)
	if got := editedAt(t, db, beta); got.Equal(betaAdded) {
		t.Errorf("名前を変えた動画の EditedAt が追加日時のまま")
	}
}

// 一括のタグ付けは、タグが実際に付いた動画だけを進める（Edge Case）。
func TestBulkTaggingAdvancesOnlyChangedVideos(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	beta := ids[fixturePath("/media/beta.mp4")]
	gamma := ids[fixturePath("/media/gamma.mp4")]
	tagID := attachNamedTag(t, db, "好き", alpha)
	if _, err := db.Tags().ApplyVideoTags(context.Background(),
		[]domain.VideoRef{{ID: beta}}, domain.VideoTagsAdd, []string{"別"}, false); err != nil {
		t.Fatal(err)
	}
	resetEditedAt(t, db)

	start := time.Now()
	if _, _, err := db.Tags().AttachTagByID(context.Background(), []int64{alpha, gamma}, tagID); err != nil {
		t.Fatal(err)
	}
	assertEditedAt(t, db, alpha, staleEditedAt)
	assertEditedSince(t, db, gamma, start)

	resetEditedAt(t, db)
	start = time.Now()
	if _, err := db.Tags().ApplyVideoTags(context.Background(),
		[]domain.VideoRef{{ID: alpha}, {ID: beta}}, domain.VideoTagsAdd, []string{"別"}, false); err != nil {
		t.Fatal(err)
	}
	assertEditedSince(t, db, alpha, start)
	assertEditedAt(t, db, beta, staleEditedAt)
}

// 名前での一括の置き換え・外すは、手で付けたタグが実際に変わる動画だけを進める（R-3）。
// 置き換えは足すだけ・外すだけのどちらで変わっても進め、同じ集合なら進めない。
func TestApplyVideoTagsReplaceAndRemoveAdvanceOnlyChanged(t *testing.T) {
	ctx := context.Background()
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	beta := ids[fixturePath("/media/beta.mp4")]
	gamma := ids[fixturePath("/media/gamma.mp4")]
	apply := func(action domain.VideoTagsAction, names []string, videos ...int64) {
		t.Helper()
		refs := make([]domain.VideoRef, 0, len(videos))
		for _, id := range videos {
			refs = append(refs, domain.VideoRef{ID: id})
		}
		if _, err := db.Tags().ApplyVideoTags(ctx, refs, action, names, false); err != nil {
			t.Fatal(err)
		}
	}
	apply(domain.VideoTagsAdd, []string{"好き"}, alpha)     // 同じ集合
	apply(domain.VideoTagsAdd, []string{"好き", "別"}, beta) // 集合に無いタグがある
	resetEditedAt(t, db)                                  // gamma はタグなし（足すだけで変わる）
	start := time.Now()
	apply(domain.VideoTagsReplace, []string{"好き"}, alpha, beta, gamma)
	assertEditedAt(t, db, alpha, staleEditedAt)
	assertEditedSince(t, db, beta, start)
	assertEditedSince(t, db, gamma, start)

	resetEditedAt(t, db)
	apply(domain.VideoTagsAdd, []string{"別"}, beta)
	resetEditedAt(t, db)
	start = time.Now()
	apply(domain.VideoTagsRemove, []string{"別", "無い名前"}, alpha, beta)
	assertEditedAt(t, db, alpha, staleEditedAt)
	assertEditedSince(t, db, beta, start)
}

// 変わる鍵を求める問い合わせは、鍵ごとに高々 1 行を返す。鍵 × タグの組の数だけ行を
// 返さないので、上限の一括操作でも書き込みの取引の中で受け取る行は鍵の数までである。
func TestChangedManualTagKeysReturnsOneRowPerKey(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	keys := make([]string, 0, 500)
	for i := range 500 {
		keys = append(keys, fmt.Sprintf("key-%d", i))
	}
	tagIDs := make([]int64, 0, 100)
	for i := range 100 {
		tagIDs = append(tagIDs, int64(i+1))
	}
	encodedKeys, _ := json.Marshal(keys)
	encodedTags, _ := json.Marshal(tagIDs)
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, action := range []domain.VideoTagsAction{domain.VideoTagsAdd, domain.VideoTagsReplace} {
		got, err := changedManualTagKeys(ctx, tx, string(encodedKeys), action, string(encodedTags))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(keys) || got[0] != keys[0] || got[len(got)-1] != keys[len(keys)-1] {
			t.Errorf("%s: %d 行, want 鍵ごとに 1 行・鍵の順の %d 行", action, len(got), len(keys))
		}
	}
	got, err := changedManualTagKeys(ctx, tx, string(encodedKeys), domain.VideoTagsRemove, string(encodedTags))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("remove: %d 行, want 0（どの鍵にもタグが無い）", len(got))
	}
}

// 集まりのメンバーへのタグ付け・公開の設定は、集まりの全メンバーを進める（R-1）。
func TestBundleEditsAdvanceEveryMember(t *testing.T) {
	db, ids := versionFixture(t)
	bundle(t, db, ids["a"], ids["a"], ids["b"])

	start := time.Now()
	attachNamedTag(t, db, "好き", ids["b"])
	for _, id := range []int64{ids["a"], ids["b"]} {
		assertEditedSince(t, db, id, start)
	}
	assertEditedAt(t, db, ids["c"], ownerVideo(t, db, ids["c"]).AddedAt)

	resetEditedAt(t, db)
	start = time.Now()
	setPublic(t, db, true, ids["a"])
	for _, id := range []int64{ids["a"], ids["b"]} {
		assertEditedSince(t, db, id, start)
	}
}

// 再生位置・取り込み・解析・タグ自体の操作・束ねる操作は更新日時を進めない（受け入れ条件 2、
// R-2）。
func TestNonEditingWritesKeepEditedAt(t *testing.T) {
	ctx := context.Background()
	db, ids := versionFixture(t)
	a := ids["a"]
	tagID := attachNamedTag(t, db, "好き", a, ids["b"])
	otherID := attachNamedTag(t, db, "別", ids["c"])
	userKey := ownerVideo(t, db, a).UserKey
	resetEditedAt(t, db)

	for _, step := range []struct {
		name  string
		write func() error
	}{
		{"再生位置", func() error {
			_, err := db.Playback().SaveProgress(ctx, userKey,
				domain.Progress{PositionMs: 1_000, DurationMs: 100_000})
			return err
		}},
		{"取り込み", func() error {
			file := listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1)
			file.SizeBytes++
			_, err := db.ScanIndex().UpsertVideo(ctx, file)
			return err
		}},
		{"解析", func() error {
			probe := domain.Probe{DurationMs: 100_000, VideoCodec: "h264", AudioCodec: "aac"}
			return db.Ingest().ApplyProbe(ctx, a, probe, domain.EvaluatePlayability("mp4", probe))
		}},
		{"タグの改名", func() error {
			_, err := db.Tags().RenameTag(ctx, tagID, "大好き")
			return err
		}},
		{"タグの統合", func() error {
			_, err := db.Tags().MergeTags(ctx, tagID, []int64{otherID})
			return err
		}},
		{"束ねる", func() error {
			_, err := db.Versions().Bundle(ctx, []int64{a, ids["d"]}, a)
			return err
		}},
		{"代表の変更", func() error {
			_, err := db.Versions().MakeRepresentative(ctx, ids["d"])
			return err
		}},
		{"外す", func() error {
			_, err := db.Versions().Unbundle(ctx, ids["d"])
			return err
		}},
		{"タグの削除", func() error {
			return db.Tags().DeleteTag(ctx, tagID)
		}},
	} {
		if err := step.write(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}
	for _, key := range []string{"a", "b", "c"} {
		assertEditedAt(t, db, ids[key], staleEditedAt)
	}
	assertEditedAt(t, db, ids["d"], ownerVideo(t, db, ids["d"]).AddedAt)
}

// 同じパスの中身の引き継ぎは、前の内容の更新日時を新しい内容へ付け替える（Edge Case）。
func TestSuccessionCarriesEditedAt(t *testing.T) {
	db, a := taggedVideoFixture(t)
	resetEditedAt(t, db)

	scan := startTestScan(t, db)
	b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
	finishTestScan(t, db, scan, domain.ScanDone)
	probeDuration(t, db, b, 100_000)

	assertEditedAt(t, db, b, staleEditedAt)
	var rows int
	if err := db.sql.QueryRow(`select count(*) from video_edits where content_key = 'key-a'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || a == b {
		t.Errorf("前の内容の行 = %d, a = %d, b = %d", rows, a, b)
	}
}

// 移行は空の video_edits を作り（既存の動画を埋め戻さない）、Down は表を落とす。
func TestVideoEditsMigration(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 26); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into videos(content_key, probe_state, thumbnail_state, preview_state,
		seek_thumbnail_state) values ('key-a', 'done', 'done', 'done', 'done')`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 27); err != nil {
		t.Fatal(err)
	}
	if n := editRowCount(t, db); n != 0 {
		t.Errorf("移行のあとの video_edits = %d 行, want 0", n)
	}
	if _, err := provider.DownTo(ctx, 26); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.sql.QueryRow(`select count(*) from sqlite_master where name = 'video_edits'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("Down のあとも video_edits がある")
	}
}
