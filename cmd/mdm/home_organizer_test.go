package main

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/store"
)

// ホームの整理役を本物の保存層とつないで確かめる（specs/057-home/research.md R-2、R-8、R-9、R-10、
// 親 Issue の受け入れ条件 5・7）。app の単体試験は保存層を差し替えるので、案の適用が owner-kept の棚と
// 分野の棚を残し、extend がほかの棚の名前を変えないことはここで見る。

type homeOrganizerFixture struct {
	ctx       context.Context
	db        *store.DB
	root      domain.MediaFolder
	organizer *app.HomeOrganizer
	videos    int
}

func newHomeOrganizerFixture(t *testing.T) *homeOrganizerFixture {
	t.Helper()
	ctx := context.Background()
	db, err := store.OpenMigratedForTest(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	root, err := db.Settings().AddMediaFolder(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// 取り込みの待ち行列は、動画を入れると積まれる仕事を処理しないので渡さない（空とみなす）。
	organizer := app.NewHomeOrganizer(app.HomeOrganizerOptions{
		Store: db.Home(), Settle: time.Millisecond, Poll: time.Millisecond,
	})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		organizer.Run(runCtx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return &homeOrganizerFixture{ctx: ctx, db: db, root: root, organizer: organizer}
}

// addTagged は n 本の動画を入れ、どれにも tagID を付ける。tagID が 0 なら付けない。
func (f *homeOrganizerFixture) addTagged(t *testing.T, n int, tagID int64) []int64 {
	t.Helper()
	ids := make([]int64, n)
	for i := range ids {
		f.videos++
		name := fmt.Sprintf("v%03d", f.videos)
		got, err := f.db.ScanIndex().UpsertVideo(f.ctx, domain.VideoFile{
			Path: filepath.Join(f.root.Path, name+".mp4"), Title: name, ContentKey: "key-" + name,
			SizeBytes: 1, MTime: time.Unix(1, 0), Container: "mp4",
		})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = got.ID
	}
	if tagID != 0 {
		if _, _, err := f.db.Tags().AttachTagByID(f.ctx, ids, tagID); err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

func (f *homeOrganizerFixture) tag(t *testing.T, name string) int64 {
	t.Helper()
	created, err := f.db.Tags().CreateTag(f.ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	return created.ID
}

// shelves はホームの棚を並びの順に "id:name:owner_kept" で返す。
func (f *homeOrganizerFixture) shelves(t *testing.T) []string {
	t.Helper()
	shelves, err := f.db.Home().Shelves(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(shelves))
	for i, shelf := range shelves {
		out[i] = fmt.Sprintf("%d:%s:%t", shelf.ID, shelf.Name, shelf.OwnerKept)
	}
	return out
}

// waitRun は最後の実行が run になり、走っている実行も待ちも無くなるまで待つ。
func (f *homeOrganizerFixture) waitRun(t *testing.T, run domain.HomeOrganizeRun) domain.HomeOrganizeLast {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		state, err := f.organizer.State(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !state.Running && state.Pending == "" && state.Last != nil && state.Last.Run == run {
			return *state.Last
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no %s finished", run)
	return domain.HomeOrganizeLast{}
}

// organized は、Cats を 5 本、Few を 4 本、Owned を Few の 1 本に付け、Owned の所有者の棚と分野 anime の棚を
// 置いて rebuild を走らせる。棚にない動画は Few の残りの 3 本である。
func organized(t *testing.T) (*homeOrganizerFixture, []string) {
	t.Helper()
	f := newHomeOrganizerFixture(t)
	f.addTagged(t, 5, f.tag(t, "Cats"))
	few := f.addTagged(t, 4, f.tag(t, "Few"))
	owned := f.tag(t, "Owned")
	if _, _, err := f.db.Tags().AttachTagByID(f.ctx, few[:1], owned); err != nil {
		t.Fatal(err)
	}
	mine, err := f.db.SmartFolders().CreateSmartFolder(f.ctx, domain.SmartFolderCreate{
		Name: "Mine", Rule: domain.SmartRule{Tags: []int64{owned}}, Sort: domain.SortTitleAsc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Home().SetShelves(f.ctx, []int64{mine.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewOnboarding(f.db.Home(), nil).PickInterests(f.ctx, []string{"anime"}); err != nil {
		t.Fatal(err)
	}
	before := f.shelves(t)

	if _, err := f.organizer.Reorganize(f.ctx); err != nil {
		t.Fatal(err)
	}
	last := f.waitRun(t, domain.HomeRebuild)
	if last.Outcome != domain.HomeOrganizeDone || last.Added != 1 || last.Removed != 0 {
		t.Errorf("rebuild = %+v, want one shelf added", last)
	}
	return f, before
}

// rebuild は 5 本のタグに棚を作り、4 本のタグには作らない。owner-kept の棚と分野の棚はそのまま上に残る
// （受け入れ条件 7）。
func TestHomeOrganizerRebuildKeepsOwnerAndInterestShelves(t *testing.T) {
	f, before := organized(t)
	after := f.shelves(t)
	if len(before) != 2 || len(after) != 3 || !slices.Equal(after[:2], before) {
		t.Fatalf("shelves = %v, want %v followed by Cats", after, before)
	}
	if !strings.HasSuffix(after[2], ":Cats:false") {
		t.Errorf("organizer shelf = %q, want Cats", after[2])
	}
}

// 棚にない動画が 20 本でスキャンが終わると extend が新しいタグの棚を足し、ほかの棚の名前を変えない
// （受け入れ条件 5）。19 本では何も走らない。
func TestHomeOrganizerExtendsAfterScan(t *testing.T) {
	for _, c := range []struct {
		dogs int
		want bool
	}{{17, true}, {16, false}} {
		f, _ := organized(t)
		shelved := f.shelves(t)
		f.addTagged(t, c.dogs, f.tag(t, "Dogs"))
		f.organizer.ScanFinished(domain.ScanOriginWatch, domain.ScanDone)
		if !c.want {
			time.Sleep(100 * time.Millisecond)
			if state, err := f.organizer.State(f.ctx); err != nil || state.Last.Run != domain.HomeRebuild || state.Pending != "" {
				t.Errorf("19 unshelved: state = %+v %v, want no run after the rebuild", state, err)
			}
			if got := f.shelves(t); !slices.Equal(got, shelved) {
				t.Errorf("19 unshelved: shelves = %v, want %v", got, shelved)
			}
			continue
		}
		last := f.waitRun(t, domain.HomeExtend)
		if last.Added != 1 {
			t.Errorf("extend = %+v, want one shelf added", last)
		}
		got := f.shelves(t)
		if len(got) != 4 || !slices.Equal(got[:3], shelved) {
			t.Fatalf("shelves = %v, want %v followed by Dogs", got, shelved)
		}
		if !strings.HasSuffix(got[3], ":Dogs:false") {
			t.Errorf("new shelf = %q, want Dogs", got[3])
		}
	}
}
