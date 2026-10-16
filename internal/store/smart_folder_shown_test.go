package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// まとめて確かめるフォルダの再生順が空でないか（smartFoldersHaveItems）は、どのフォルダも
// ライブラリの項目の問い合わせ（smartFolderHasItems）と同じ答えになる。視聴状態の混ざったグループ
// show と再生できないグループ pair の、丸ごと当たるときと一部だけ当たるときを、所有者とゲストで
// 確かめる。
func TestSmartFoldersHaveItemsAsTheItemQuery(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	setPublic(t, db, true, ids[fixturePath("/media/show/ep10.mp4")], ids[fixturePath("/media/show/ep1.mp4")],
		ids[fixturePath("/media/pair/p1.mp4")], ids[fixturePath("/media/pair/p2.mp4")], ids[fixturePath("/media/solo.mp4")])
	favoriteVideos(t, db, true, ids[fixturePath("/media/show/ep2.mp4")])
	short := 2
	rules := []domain.SmartRule{
		{Watch: domain.WatchUnwatched}, {Watch: domain.WatchInProgress}, {Watch: domain.WatchWatched},
		{Playable: true}, {Playable: true, Watch: domain.WatchInProgress}, {Q: "ep"}, {Q: "ep", Watch: domain.WatchInProgress},
		{Q: "ep", Watch: domain.WatchUnwatched}, {Q: "p", Playable: true}, {Q: "pair", Playable: true}, {Q: "show", Watch: domain.WatchWatched},
		{MaxDuration: &short}, {Favorite: true}, {Q: "no such video"},
	}
	var folders []int64
	for i, rule := range rules {
		folders = append(folders, newSmartFolder(t, db, fmt.Sprintf("f%02d", i), rule, domain.SortTitleAsc, 0))
	}
	parent := newSmartFolder(t, db, "親", domain.SmartRule{Watch: domain.WatchInProgress}, domain.SortTitleAsc, 0)
	folders = append(folders, parent, newSmartFolder(t, db, "子", domain.SmartRule{Q: "show"}, domain.SortTitleAsc, parent))

	for _, audience := range []domain.Audience{domain.AudienceOwner, domain.AudienceGuest} {
		tx, err := db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		targets := make([]smartFolderTarget, len(folders))
		for i, id := range folders {
			folder, err := readSmartFolder(ctx, tx, id)
			if err != nil {
				t.Fatal(err)
			}
			rules, err := smartFolderPathRules(ctx, tx, id)
			if err != nil {
				t.Fatal(err)
			}
			if targets[i], err = readSmartFolderTarget(ctx, tx, audience, folder, rules, time.Now().Unix()); err != nil {
				t.Fatal(err)
			}
		}
		got, err := smartFoldersHaveItems(ctx, tx, audience, targets)
		if err != nil {
			t.Fatal(err)
		}
		nonEmpty := 0
		for i, target := range targets {
			want, err := smartFolderHasItems(ctx, tx, target.spec)
			if err != nil {
				t.Fatal(err)
			}
			if got[i] != want {
				t.Errorf("%s の %s = %v, want %v", audience, target.summary.Name, got[i], want)
			}
			if want {
				nonEmpty++
			}
		}
		if nonEmpty == 0 || nonEmpty == len(targets) {
			t.Errorf("%s の空でないフォルダ = %d / %d（どちらの答えも確かめる）", audience, nonEmpty, len(targets))
		}
		_ = tx.Rollback()
	}
}
