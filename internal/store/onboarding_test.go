package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func pickInterests(t *testing.T, db *DB, ids ...string) []domain.SmartFolder {
	t.Helper()
	interests, err := domain.PickInterests(ids)
	if err != nil {
		t.Fatal(err)
	}
	folders, err := db.Onboarding().PickInterests(context.Background(), interests)
	if err != nil {
		t.Fatalf("PickInterests(%v) error = %v", ids, err)
	}
	return folders
}

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

func folderTag(t *testing.T, folder domain.SmartFolder) int64 {
	t.Helper()
	if len(folder.Rule.Tags) != 1 || !domain.SameSmartRule(folder.Rule, domain.SmartRule{Tags: folder.Rule.Tags}) {
		t.Fatalf("folder %q rule = %+v, want one tag", folder.Name, folder.Rule)
	}
	return folder.Rule.Tags[0]
}

func TestPickInterestsMakesGenreTagsAndOwnerFolders(t *testing.T) {
	db := migratedDB(t)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	folders := pickInterests(t, db, "anime", "music")

	if len(folders) != 2 || folders[0].Name != "Anime" || folders[1].Name != "Music" {
		t.Fatalf("added folders = %+v", folders)
	}
	for i, want := range []string{"Anime", "Music"} {
		folder := folders[i]
		if folder.ParentID != 0 || folder.Sort != domain.SortAddedDesc {
			t.Errorf("folder %q = %+v, want an root folder (interest %d)", folder.Name, folder, i+2)
		}
		name, kind, tentative := tagRow(t, db, folderTag(t, folder))
		if name != want || kind != domain.InterestTagKind || tentative {
			t.Errorf("tag = %q, kind %q, tentative %t; want %q, Genre, confirmed", name, kind, tentative, want)
		}
	}
	tag, err := db.Tags().AddSynonym(context.Background(), folderTag(t, folders[0]), "アニメ", nil)
	if err != nil || len(tag.Synonyms) != 1 || tag.Synonyms[0] != "アニメ" {
		t.Errorf("synonyms of Anime = %+v, %v; want アニメ already there", tag.Synonyms, err)
	}
	if got := onboardingChanges(recorder); got != 0 {
		t.Errorf("OnboardingChanged published %d times, want 1", got)
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

func TestPickInterestsUsesTheTagOfTheSameNameOrSynonym(t *testing.T) {
	db, tags := interestFixture(t, "アニメ", "Music", "Cooking")
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

	folders := pickInterests(t, db, "anime", "music", "cooking")

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
	if len(folders) != len(want) {
		t.Fatalf("added folders = %+v", folders)
	}
	for i, w := range want {
		if got := folderTag(t, folders[i]); got != w.tag {
			t.Errorf("folder %q tag = %d, want %d", folders[i].Name, got, w.tag)
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

func TestPickInterestsRemakesADeletedGenreKind(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if err := db.Tags().DeleteTagKind(ctx, tagKindID(t, db, "Genre")); err != nil {
		t.Fatal(err)
	}

	folders := pickInterests(t, db, "travel")

	if _, kind, _ := tagRow(t, db, folderTag(t, folders[0])); kind != domain.InterestTagKind {
		t.Fatalf("kind = %q, want Genre made again", kind)
	}
}

func TestPickingAnInterestTwiceKeepsOneFolder(t *testing.T) {
	db := migratedDB(t)
	folders := pickInterests(t, db, "games")
	name := "My games"
	if _, err := db.SmartFolders().ChangeSmartFolder(context.Background(), folders[0].ID, domain.SmartFolderChange{Name: &name}); err != nil {
		t.Fatal(err)
	}
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if again := pickInterests(t, db, "games"); len(again) != 0 {
		t.Fatalf("second pick added %+v", again)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("events = %+v, want none", recorder.events)
	}
}

func TestInterestsTellWhichAreAlreadyCreated(t *testing.T) {
	db := migratedDB(t)
	pickInterests(t, db, "cars")

	statuses, err := db.Onboarding().Interests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != len(domain.InterestCatalog) {
		t.Fatalf("interests = %d, want %d", len(statuses), len(domain.InterestCatalog))
	}
	for i, status := range statuses {
		if status.Interest != domain.InterestCatalog[i] || status.HasFolder != (status.ID == "cars") {
			t.Errorf("interest %d = %+v", i, status)
		}
	}
}

func TestOnboardingStepDefaultsAndRoundTrips(t *testing.T) {
	ctx := context.Background()
	empty, err := OpenMigratedForTest(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = empty.Close() })
	if step, err := empty.Onboarding().OnboardingStep(ctx); err != nil || step != domain.OnboardingFolders {
		t.Fatalf("step without a media folder = %q, %v; want folders", step, err)
	}

	db := migratedDB(t)
	home := db.Onboarding()
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
	if got := onboardingChanges(recorder); got != 2 {
		t.Errorf("OnboardingChanged published %d times, want 2 (none for the same step)", got)
	}
	if err := home.SaveOnboardingStep(ctx, "later"); err == nil {
		t.Error("SaveOnboardingStep(later) error = nil")
	}
}

func interestFixture(t *testing.T, names ...string) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	tags := map[string]int64{}
	for _, name := range names {
		tag, err := db.Tags().CreateTag(context.Background(), name)
		if err != nil {
			t.Fatal(err)
		}
		tags[name] = tag.ID
	}
	return db, tags
}
func onboardingChanges(recorder *eventRecorder) int {
	count := 0
	for _, event := range recorder.events {
		if _, ok := event.(domain.OnboardingChanged); ok {
			count++
		}
	}
	return count
}
