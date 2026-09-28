package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 外部連携 API の動画の読み出し（specs/026-external-api/contracts/external-api.md §2、
// research.md R-6）。

// readAllExternal は nextCursor が空になるまでたどり、動画の id を順に返す。
func readAllExternal(t *testing.T, db *DB, limit int) []int64 {
	t.Helper()
	var ids []int64
	cursor := ""
	for range 100 {
		page, err := db.Library().ListExternalVideos(context.Background(), domain.ExternalVideoQuery{Cursor: cursor, Limit: limit})
		if err != nil {
			t.Fatalf("一覧に失敗した (cursor=%q): %v", cursor, err)
		}
		if len(page.Items) > limit {
			t.Fatalf("limit=%d に %d 件", limit, len(page.Items))
		}
		for _, item := range page.Items {
			ids = append(ids, item.Video.ID)
		}
		if page.NextCursor == "" {
			return ids
		}
		cursor = page.NextCursor
	}
	t.Fatal("nextCursor が空にならない")
	return nil
}

func TestListExternalVideosPagesInAddedOrder(t *testing.T) {
	db := migratedDB(t)
	// 追加時刻の同じ動画を混ぜ、id で決着することも確かめる。取り込む順と追加時刻の順を変える。
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/e.mp4"), "e", "key-e", 5),
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1),
		listingFile(fixturePath("/media/c.mp4"), "c", "key-c", 3),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 3),
		listingFile(fixturePath("/media/d.mp4"), "d", "key-d", 3),
	)
	tie := []int64{ids[fixturePath("/media/c.mp4")], ids[fixturePath("/media/b.mp4")], ids[fixturePath("/media/d.mp4")]}
	slices.Sort(tie)
	want := append(append([]int64{ids[fixturePath("/media/a.mp4")]}, tie...), ids[fixturePath("/media/e.mp4")])

	for _, limit := range []int{1, 2, 4, 5, 200} {
		if got := readAllExternal(t, db, limit); !slices.Equal(got, want) {
			t.Errorf("limit=%d: %v, want %v", limit, got, want)
		}
	}

	// ちょうど limit 件のときも最後の応答の nextCursor は空（続きの無い要求を強いない）。
	page, err := db.Library().ListExternalVideos(context.Background(), domain.ExternalVideoQuery{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 5 || page.NextCursor != "" {
		t.Errorf("limit=5: %d 件, nextCursor=%q", len(page.Items), page.NextCursor)
	}
	// 既定の件数は 100。
	page, err = db.Library().ListExternalVideos(context.Background(), domain.ExternalVideoQuery{})
	if err != nil || len(page.Items) != 5 {
		t.Errorf("limit 無し: %d 件, err=%v", len(page.Items), err)
	}
}

func TestListExternalVideosRejectsInvalidCursors(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1))
	// 画面の一覧のカーソルは取り違えとして拒む。
	screen := listOrders[domain.SortAddedAsc]
	screenCursor, err := screen.encodeCursor(listSpec{sort: domain.SortAddedAsc}, int64(1), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, cursor := range []string{"!!", "bm90LWEtY3Vyc29y", screenCursor} {
		_, err := db.Library().ListExternalVideos(context.Background(), domain.ExternalVideoQuery{Cursor: cursor})
		if !errors.Is(err, domain.ErrInvalidCursor) {
			t.Errorf("cursor=%q: err = %v, want ErrInvalidCursor", cursor, err)
		}
	}
}

func TestListExternalVideosRereadIncludesNewVideos(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 2),
	)
	first := readAllExternal(t, db, 1)
	if len(first) != 2 {
		t.Fatalf("最初の読み通し: %v", first)
	}

	// スキャンで動画が増える（受け入れ条件 3）。追加時刻は既存より前でも、読み直せば含まれる。
	added := upsertAll(t, db, listingFile(fixturePath("/media/new.mp4"), "new", "key-new", 0))
	again := readAllExternal(t, db, 1)
	if !slices.Contains(again, added[fixturePath("/media/new.mp4")]) || len(again) != 3 {
		t.Errorf("読み直し: %v, 増えた動画 %d を含まない", again, added[fixturePath("/media/new.mp4")])
	}
}

func TestListExternalVideosContinuesAfterDeletion(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 2),
		listingFile(fixturePath("/media/c.mp4"), "c", "key-c", 3),
	)
	page, err := db.Library().ListExternalVideos(ctx, domain.ExternalVideoQuery{Limit: 1})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("1 ページ目: %+v, %v", page, err)
	}
	// カーソルの指す動画（a）と次の動画（b）を消しても、続きは失敗せずその時点の続きを返す。
	if err := db.ScanIndex().DeleteVideos(ctx, []int64{ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")]}); err != nil {
		t.Fatal(err)
	}
	next, err := db.Library().ListExternalVideos(ctx, domain.ExternalVideoQuery{Cursor: page.NextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("続きの要求が失敗した: %v", err)
	}
	if len(next.Items) != 1 || next.Items[0].Video.ID != ids[fixturePath("/media/c.mp4")] || next.NextCursor != "" {
		t.Errorf("続き: %+v", next)
	}
}

func TestExternalVideosOnlyReturnRegisteredLocations(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	registered := fixturePath("/media/a.mp4")
	outside := fixturePath("/other/a.mp4")
	ids := upsertAll(t, db,
		listingFile(registered, "a", "key-a", 1),
		listingFile(outside, "a", "key-a", 1),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 2),
	)
	id := ids[registered]
	if ids[outside] != id {
		t.Fatalf("同じ内容の所在が別の動画になった: %v", ids)
	}

	// 登録の下の所在があるあいだは、登録外の所在を返さない。
	got, err := db.Library().LookupExternalVideo(ctx, domain.VideoRef{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Locations, []string{registered}) {
		t.Errorf("所在 = %v, want [%s]", got.Locations, registered)
	}
	if _, err := db.Library().LookupExternalVideo(ctx, domain.VideoRef{Path: outside}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("登録外のパスで引けた: %v", err)
	}

	// 登録の下の所在をすべて失い、登録外の所在だけで残る。
	var locationID int64
	if err := db.sql.QueryRow(`select id from video_locations where path = ?`, registered).Scan(&locationID); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().DeleteVideoLocations(ctx, []int64{locationID}); err != nil {
		t.Fatal(err)
	}
	if list := readAllExternal(t, db, 10); slices.Contains(list, id) || len(list) != 1 {
		t.Errorf("登録外の所在だけの動画が一覧に出た: %v", list)
	}
	for _, ref := range []domain.VideoRef{{ID: id}, {ContentKey: "key-a"}, {Path: outside}} {
		if _, err := db.Library().LookupExternalVideo(ctx, ref); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%+v: err = %v, want ErrNotFound", ref, err)
		}
	}
}

func TestExternalVideosIncludePrivateVideosWithDetails(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	// NFD の綴り（か + 濁点）のパスを、ファイルシステムの綴りのまま保存する。
	nfd := fixturePath("/media/z/が.mp4")
	nfc := fixturePath("/media/z/が.mp4")
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/y/public.mp4"), "public", "key-public", 1),
		listingFile(nfd, "が", "key-private", 2),
		listingFile(fixturePath("/media/a/copy.mp4"), "copy", "key-private", 2),
	)
	publicID, privateID := ids[fixturePath("/media/y/public.mp4")], ids[nfd]
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{publicID}, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByName(ctx, []int64{privateID}, "猫"); err != nil {
		t.Fatal(err)
	}

	// 非公開の動画も返る（受け入れ条件 2）。
	if list := readAllExternal(t, db, 10); !slices.Equal(list, []int64{publicID, privateID}) {
		t.Errorf("一覧 = %v, want [%d %d]", list, publicID, privateID)
	}

	for _, ref := range []domain.VideoRef{{ID: privateID}, {ContentKey: "key-private"}, {Path: nfd}, {Path: fixturePath("/media/a/copy.mp4")}} {
		got, err := db.Library().LookupExternalVideo(ctx, ref)
		if err != nil {
			t.Fatalf("%+v: %v", ref, err)
		}
		if got.Video.ID != privateID || got.Video.ContentKey != "key-private" {
			t.Errorf("%+v: 動画 %d (%q)", ref, got.Video.ID, got.Video.ContentKey)
		}
		// 所在はパスの順で、代表（パスの最小）が先頭。
		wantLocations := []string{fixturePath("/media/a/copy.mp4"), nfd}
		if !slices.Equal(got.Locations, wantLocations) || got.Video.Path != wantLocations[0] {
			t.Errorf("%+v: 所在 = %v（代表 %q）, want %v", ref, got.Locations, got.Video.Path, wantLocations)
		}
		if len(got.Tags) != 1 || got.Tags[0].Name != "猫" || !got.Tags[0].Manual {
			t.Errorf("%+v: タグ = %+v", ref, got.Tags)
		}
	}

	// 無い指定と、正規化すれば一致するだけのパスは引けない。
	for _, ref := range []domain.VideoRef{{ID: 9999}, {ContentKey: "missing"}, {Path: nfc}, {Path: fixturePath("/media/none.mp4")}, {}} {
		if _, err := db.Library().LookupExternalVideo(ctx, ref); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%+v: err = %v, want ErrNotFound", ref, err)
		}
	}
}
