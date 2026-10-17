package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// pickInterests は一覧の id の分野を選ぶ。
func pickInterests(t *testing.T, db *DB, ids ...string) []domain.SmartFolder {
	t.Helper()
	interests, err := domain.PickInterests(ids)
	if err != nil {
		t.Fatal(err)
	}
	shelves, err := db.Home().PickInterests(context.Background(), interests)
	if err != nil {
		t.Fatalf("PickInterests(%v) error = %v", ids, err)
	}
	return shelves
}

// tagRow はタグ id の元の名前・種類の名前（無ければ空）・仮かどうかを返す。
func tagRow(t *testing.T, db *DB, id int64) (name, kind string, tentative bool) {
	t.Helper()
	var kindName sql.NullString
	if err := db.sql.QueryRow(`select tn.name, tk.name, tg.tentative from tags tg
		join tag_names tn on tn.tag_id = tg.id and tn.canonical = 1
		left join tag_kinds tk on tk.id = tg.kind_id where tg.id = ?`, id).Scan(&name, &kindName, &tentative); err != nil {
		t.Fatal(err)
	}
	return name, kindName.String, tentative
}

// shelfTag は分野の棚の条件のタグの id を返す。
func shelfTag(t *testing.T, shelf domain.SmartFolder) int64 {
	t.Helper()
	if len(shelf.Rule.Tags) != 1 || !domain.SameSmartRule(shelf.Rule, domain.SmartRule{Tags: shelf.Rule.Tags}) {
		t.Fatalf("shelf %q rule = %+v, want one tag", shelf.Name, shelf.Rule)
	}
	return shelf.Rule.Tags[0]
}

// 分野を 2 つ選ぶと、種類 Genre のタグが 2 つと、今の棚の後ろに所有者の棚が 2 つできる（受け入れ条件 3）。
func TestPickInterestsMakesGenreTagsAndOwnerShelves(t *testing.T) {
	db, tags := homeFixture(t, "Cats")
	applyProposal(t, db, domain.HomeRebuild, tagShelf("Cats", tags["Cats"]))
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	shelves := pickInterests(t, db, "anime", "music")

	if got, want := homeRows(t, db), []string{"Cats:false", "Anime:true", "Music:true"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("home = %v, want %v", got, want)
	}
	if len(shelves) != 2 || shelves[0].Name != "Anime" || shelves[1].Name != "Music" {
		t.Fatalf("added shelves = %+v", shelves)
	}
	for i, want := range []string{"Anime", "Music"} {
		shelf := shelves[i]
		if !shelf.OwnerKept || shelf.ParentID != 0 || shelf.Sort != domain.SortAddedDesc || shelf.HomePosition != i+2 {
			t.Errorf("shelf %q = %+v, want an owner-kept root shelf at %d", shelf.Name, shelf, i+2)
		}
		name, kind, tentative := tagRow(t, db, shelfTag(t, shelf))
		if name != want || kind != domain.InterestTagKind || tentative {
			t.Errorf("tag = %q, kind %q, tentative %t; want %q, Genre, confirmed", name, kind, tentative, want)
		}
	}
	tag, err := db.Tags().AddSynonym(context.Background(), shelfTag(t, shelves[0]), "アニメ", nil)
	if err != nil || len(tag.Synonyms) != 1 || tag.Synonyms[0] != "アニメ" {
		t.Errorf("synonyms of Anime = %+v, %v; want アニメ already there", tag.Synonyms, err)
	}
	if got := homeChanges(recorder); got != 1 {
		t.Errorf("HomeChanged published %d times, want 1", got)
	}
	if !publishedSmartFoldersChanged(recorder) {
		t.Error("SmartFoldersChanged was not published for the new tags")
	}
}

func publishedSmartFoldersChanged(recorder *eventRecorder) bool {
	for _, event := range recorder.events {
		if _, ok := event.(domain.SmartFoldersChanged); ok {
			return true
		}
	}
	return false
}

// 名前か同義語がすでにタグにある分野は、そのタグを使い 2 つ目を作らない。今の種類は変えず、種類の無い
// タグには Genre を付け、仮のタグは確定する（Edge Cases、research.md R-6）。
func TestPickInterestsUsesTheTagOfTheSameNameOrSynonym(t *testing.T) {
	db, tags := homeFixture(t, "アニメ", "Music", "Cooking")
	ctx := context.Background()
	place := tagKindID(t, db, "Place")
	if _, err := db.Tags().BatchTags(ctx, domain.TagBatchRequest{Action: domain.TagBatchSetKind, KindID: place},
		[]int64{tags["Music"]}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`update tags set tentative = 1 where id = ?`, tags["Cooking"]); err != nil {
		t.Fatal(err)
	}
	before := allTagCount(t, db)

	shelves := pickInterests(t, db, "anime", "music", "cooking")

	if got := allTagCount(t, db); got != before {
		t.Fatalf("tags = %d, want %d (no second tag)", got, before)
	}
	want := []struct {
		tag        int64
		name, kind string
	}{
		{tags["アニメ"], "アニメ", "Genre"},
		{tags["Music"], "Music", "Place"},
		{tags["Cooking"], "Cooking", "Genre"},
	}
	if len(shelves) != len(want) {
		t.Fatalf("added shelves = %+v", shelves)
	}
	for i, w := range want {
		if got := shelfTag(t, shelves[i]); got != w.tag {
			t.Errorf("shelf %q tag = %d, want %d", shelves[i].Name, got, w.tag)
		}
		name, kind, tentative := tagRow(t, db, w.tag)
		if name != w.name || kind != w.kind || tentative {
			t.Errorf("tag %d = %q, kind %q, tentative %t; want %q, %q, confirmed", w.tag, name, kind, tentative, w.name, w.kind)
		}
	}
}

func tagKindID(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	var id int64
	if err := db.sql.QueryRow(`select id from tag_kinds where name = ?`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// 所有者が消した Genre は作り直される。
func TestPickInterestsRemakesADeletedGenreKind(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if err := db.Tags().DeleteTagKind(ctx, tagKindID(t, db, "Genre")); err != nil {
		t.Fatal(err)
	}

	shelves := pickInterests(t, db, "travel")

	if _, kind, _ := tagRow(t, db, shelfTag(t, shelves[0])); kind != domain.InterestTagKind {
		t.Fatalf("kind = %q, want Genre made again", kind)
	}
}

// 同じ分野を 2 度選んでも棚は 1 つで、2 度目は何も書かず、HomeChanged も出さない。
func TestPickingAnInterestTwiceKeepsOneShelf(t *testing.T) {
	db := migratedDB(t)
	pickInterests(t, db, "games")
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if again := pickInterests(t, db, "games"); len(again) != 0 {
		t.Fatalf("second pick added %+v", again)
	}
	if got, want := homeRows(t, db), []string{"Games:true"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("home = %v, want %v", got, want)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("events = %+v, want none", recorder.events)
	}
}

// 分野と同じ条件の棚が整理役の棚なら、棚は足さずにその棚を所有者の棚にし、HomeChanged を出す。
// その後の再整理の案がその条件を繰り返さなくても、棚は消えない（research.md R-6、R-8）。
func TestPickingAnInterestKeepsTheOrganizerShelfOfTheSameRule(t *testing.T) {
	db, tags := homeFixture(t, "Anime", "Cats")
	applyProposal(t, db, domain.HomeRebuild, tagShelf("Anime", tags["Anime"]))
	organizer := shelfByName(t, db, "Anime")
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if added := pickInterests(t, db, "anime"); len(added) != 0 {
		t.Fatalf("pick added %+v, want none", added)
	}
	if got, want := homeRows(t, db), []string{"Anime:true"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("home = %v, want %v", got, want)
	}
	if kept := shelfByName(t, db, "Anime"); kept.ID != organizer.ID {
		t.Fatalf("kept shelf id = %d, want %d", kept.ID, organizer.ID)
	}
	if got := homeChanges(recorder); got != 1 {
		t.Fatalf("HomeChanged published %d times, want 1", got)
	}
	applyProposal(t, db, domain.HomeRebuild, tagShelf("Cats", tags["Cats"]))
	if got, want := homeRows(t, db), []string{"Anime:true", "Cats:false"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("home after rebuild = %v, want %v", got, want)
	}
}

// 棚が 50 を超えるなら ErrHomeShelvesFull で、タグも棚も書かない。
func TestPickInterestsRefusesToPassTheShelfLimit(t *testing.T) {
	db, tags := homeFixture(t, "Cats")
	ctx := context.Background()
	for i := range domain.MaxHomeShelves - 1 {
		if _, err := db.sql.Exec(`insert into smart_folders
			(name, title_key, rule, sort, owner_kept, home_position, created_at, updated_at)
			values (?, ?, ?, 'addedDesc', 1, ?, 1, 1)`,
			fmt.Sprintf("Shelf %d", i), fmt.Sprintf("shelf %d", i), fmt.Sprintf(`{"tags":[%d]}`, tags["Cats"]), i+1); err != nil {
			t.Fatal(err)
		}
	}
	before, tagsBefore := smartFolderRows(t, db), allTagCount(t, db)
	interests, err := domain.PickInterests([]string{"news", "art"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Home().PickInterests(ctx, interests); !errors.Is(err, domain.ErrHomeShelvesFull) {
		t.Fatalf("PickInterests error = %v, want ErrHomeShelvesFull", err)
	}
	if got := smartFolderRows(t, db); !reflect.DeepEqual(got, before) || allTagCount(t, db) != tagsBefore {
		t.Fatalf("folders = %v and %d tags, want them unchanged", got, allTagCount(t, db))
	}
	pickInterests(t, db, "news")
	if got := len(homeRows(t, db)); got != domain.MaxHomeShelves {
		t.Fatalf("shelves = %d, want %d", got, domain.MaxHomeShelves)
	}
}

// 分野の一覧は一覧の順で、棚がもうある分野だけ onHome が真である。
func TestInterestsTellWhichAreOnTheHome(t *testing.T) {
	db := migratedDB(t)
	pickInterests(t, db, "cars")

	statuses, err := db.Home().Interests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != len(domain.InterestCatalog) {
		t.Fatalf("interests = %d, want %d", len(statuses), len(domain.InterestCatalog))
	}
	for i, status := range statuses {
		if status.Interest != domain.InterestCatalog[i] || status.OnHome != (status.ID == "cars") {
			t.Errorf("interest %d = %+v", i, status)
		}
	}
}

// 段階が無ければ、メディアフォルダがあれば done、無ければ folders で、書けば後の書き込みが残る。段階が
// 変わったときだけ HomeChanged を出す。
func TestOnboardingStepDefaultsAndRoundTrips(t *testing.T) {
	ctx := context.Background()
	empty, err := OpenMigratedForTest(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = empty.Close() })
	if step, err := empty.Home().OnboardingStep(ctx); err != nil || step != domain.OnboardingFolders {
		t.Fatalf("step without a media folder = %q, %v; want folders", step, err)
	}

	db := migratedDB(t)
	home := db.Home()
	if step, err := home.OnboardingStep(ctx); err != nil || step != domain.OnboardingDone {
		t.Fatalf("step with a media folder = %q, %v; want done", step, err)
	}
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	for _, step := range []domain.OnboardingStep{domain.OnboardingInterests, domain.OnboardingScan, domain.OnboardingScan} {
		if err := home.SaveOnboardingStep(ctx, step); err != nil {
			t.Fatal(err)
		}
	}
	if step, err := home.OnboardingStep(ctx); err != nil || step != domain.OnboardingScan {
		t.Fatalf("step = %q, %v; want the later write, scan", step, err)
	}
	if got := homeChanges(recorder); got != 2 {
		t.Errorf("HomeChanged published %d times, want 2 (none for the same step)", got)
	}
	if err := home.SaveOnboardingStep(ctx, "later"); err == nil {
		t.Error("SaveOnboardingStep(later) error = nil")
	}
}
