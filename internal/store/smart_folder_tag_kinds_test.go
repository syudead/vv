package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func smartFolderVideoIDs(t *testing.T, db *DB, id int64) []int64 {
	t.Helper()
	_, items := smartFolderPages(t, db, domain.AudienceOwner, id, domain.MaxLimit)
	ids := []int64{}
	for _, item := range items {
		if item.Video != nil {
			ids = append(ids, item.Video.ID)
		}
	}
	slices.Sort(ids)
	return ids
}

func TestSmartFolderMatchesTagKindAndWatchState(t *testing.T) {
	f := tagKindListingFixture(t)
	ctx := context.Background()
	folder := newSmartFolder(t, f.db, "未視聴のジャンル",
		domain.SmartRule{TagKind: f.genre.ID, Watch: domain.WatchUnwatched}, domain.SortTitleAsc, 0)

	if got, want := smartFolderVideoIDs(t, f.db, folder), sortedIDs(f.folder, f.manual, f.both); !slices.Equal(got, want) {
		t.Fatalf("中身 = %v, want %v", got, want)
	}
	cards, err := f.db.SmartFolders().SmartFolderCards(ctx, domain.AudienceOwner, []int64{folder})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].VideoCount != 3 {
		t.Fatalf("カード = %+v, want 3 本", cards)
	}

	if _, err := f.db.Playback().SaveProgress(ctx, "k-manual", domain.Progress{PositionMs: 9000, Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := smartFolderVideoIDs(t, f.db, folder), sortedIDs(f.folder, f.both); !slices.Equal(got, want) {
		t.Fatalf("見終えたあとの中身 = %v, want %v", got, want)
	}
}

func TestDeletingATagKindMakesTheSmartFolderMatchNothing(t *testing.T) {
	f := tagKindListingFixture(t)
	ctx := context.Background()
	rule := domain.SmartRule{TagKind: f.genre.ID}
	folder := newSmartFolder(t, f.db, "ジャンル", rule, domain.SortTitleAsc, 0)
	other := newSmartFolder(t, f.db, "すべて", domain.SmartRule{}, domain.SortTitleAsc, 0)
	recorder := &eventRecorder{}
	f.db.PublishTo(recorder)

	if err := f.db.Tags().DeleteTagKind(ctx, f.genre.ID); err != nil {
		t.Fatal(err)
	}

	if got := smartFolderVideoIDs(t, f.db, folder); len(got) != 0 {
		t.Errorf("中身 = %v, want 空", got)
	}
	page, err := f.db.SmartFolders().SmartFolderContents(ctx, domain.AudienceOwner, folder, "", domain.MaxLimit, domain.VideoQuery{})
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SmartRuleRef{{Kind: domain.SmartRefTagKind, ID: f.genre.ID}}
	if !reflect.DeepEqual(page.Missing, want) {
		t.Errorf("Missing = %+v, want %+v", page.Missing, want)
	}
	if !reflect.DeepEqual(page.Folder.Rule, rule) {
		t.Errorf("条件 = %+v, want %+v（書き換えない）", page.Folder.Rule, rule)
	}
	cards, err := f.db.SmartFolders().SmartFolderCards(ctx, domain.AudienceOwner, []int64{folder})
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || cards[0].VideoCount != 0 {
		t.Errorf("カード = %+v, want 0 本", cards)
	}
	notices := []domain.SmartFoldersChanged{{IDs: sortedIDs(folder, other)}}
	if got := smartFolderNotices(recorder); !reflect.DeepEqual(got, notices) {
		t.Errorf("notices = %+v, want %+v", got, notices)
	}
}

func TestDeletingAMissingTagKindPublishesNothing(t *testing.T) {
	f := tagKindListingFixture(t)
	newSmartFolder(t, f.db, "すべて", domain.SmartRule{}, domain.SortTitleAsc, 0)
	recorder := &eventRecorder{}
	f.db.PublishTo(recorder)

	if err := f.db.Tags().DeleteTagKind(context.Background(), f.genre.ID+1000); !errors.Is(err, domain.ErrTagKindNotFound) {
		t.Fatalf("error = %v, want ErrTagKindNotFound", err)
	}
	if got := smartFolderNotices(recorder); len(got) != 0 {
		t.Errorf("notices = %+v, want none", got)
	}
}

func TestSettingATagKindNamesEverySmartFolder(t *testing.T) {
	f := tagKindListingFixture(t)
	ctx := context.Background()
	folder := newSmartFolder(t, f.db, "ジャンル", domain.SmartRule{TagKind: f.genre.ID}, domain.SortTitleAsc, 0)
	recorder := &eventRecorder{}
	f.db.PublishTo(recorder)

	for _, req := range []domain.TagBatchRequest{
		{Action: domain.TagBatchClearKind},
		{Action: domain.TagBatchSetKind, KindID: f.work.ID},
	} {
		if _, err := f.db.Tags().BatchTags(ctx, req, []int64{f.action.ID}); err != nil {
			t.Fatal(err)
		}
	}

	want := []domain.SmartFoldersChanged{{IDs: []int64{folder}}, {IDs: []int64{folder}}}
	if got := smartFolderNotices(recorder); !reflect.DeepEqual(got, want) {
		t.Errorf("notices = %+v, want %+v", got, want)
	}
	if got, want := smartFolderVideoIDs(t, f.db, folder), []int64{f.folder}; !slices.Equal(got, want) {
		t.Errorf("Action を作品にしたあとの中身 = %v, want %v", got, want)
	}
}
