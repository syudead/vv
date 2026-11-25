package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// synonymFixture is the tag Anime with the synonym アニメ on video key-1, and a tentative tag Cartoon; a tag holding
// a synonym is always confirmed.
type synonymFixture struct {
	db             *DB
	anime, cartoon int64
}

func newSynonymFixture(t *testing.T) synonymFixture {
	t.Helper()
	db := migratedDB(t)
	ctx := context.Background()
	anime, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, anime.ID, "アニメ", nil); err != nil {
		t.Fatal(err)
	}
	cartoon, err := db.Tags().CreateTag(ctx, "Cartoon")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`update tags set tentative = 1 where id = ?`, cartoon.ID); err != nil {
		t.Fatal(err)
	}
	attachTag(t, db, "key-1", anime.ID)
	return synonymFixture{db: db, anime: anime.ID, cartoon: cartoon.ID}
}

func (f synonymFixture) tentative(t *testing.T, id int64) bool {
	t.Helper()
	var tentative bool
	if err := f.db.sql.QueryRow(`select tentative from tags where id = ?`, id).Scan(&tentative); err != nil {
		t.Fatal(err)
	}
	return tentative
}

func TestPromoteSynonymSwapsTheNamesAndConfirmsTheTag(t *testing.T) {
	f := newSynonymFixture(t)

	tag, err := f.db.Tags().PromoteSynonym(context.Background(), f.anime, "アニメ")
	if err != nil {
		t.Fatalf("PromoteSynonym() error = %v", err)
	}
	if tag.Name != "アニメ" || !slices.Equal(tag.Synonyms, []string{"Anime"}) || tag.Tentative {
		t.Errorf("tag = %+v, want confirmed アニメ with the synonym Anime", tag)
	}
	if want := map[string]bool{"アニメ": true, "Anime": false}; !reflect.DeepEqual(tagNamesOf(t, f.db, f.anime), want) {
		t.Errorf("names = %v, want %v", tagNamesOf(t, f.db, f.anime), want)
	}
	if assignmentCount(t, f.db, "key-1", f.anime) != 1 {
		t.Error("the promote changed the tag's attachment")
	}
	assertTagInvariantsNow(t, f.db)
}

func TestMoveSynonymConfirmsTheTargetAndLeavesTheSource(t *testing.T) {
	f := newSynonymFixture(t)

	source, target, err := f.db.Tags().MoveSynonym(context.Background(), f.anime, "アニメ", f.cartoon)
	if err != nil {
		t.Fatalf("MoveSynonym() error = %v", err)
	}
	if source.Name != "Anime" || len(source.Synonyms) != 0 || source.Tentative {
		t.Errorf("source = %+v, want Anime, still confirmed, without synonyms", source)
	}
	if target.Name != "Cartoon" || !slices.Equal(target.Synonyms, []string{"アニメ"}) || target.Tentative {
		t.Errorf("target = %+v, want confirmed Cartoon with the synonym アニメ", target)
	}
	if assignmentCount(t, f.db, "key-1", f.anime) != 1 || assignmentCount(t, f.db, "key-1", f.cartoon) != 0 {
		t.Error("the attachment moved with the synonym")
	}
	assertTagInvariantsNow(t, f.db)
}

func TestDetachSynonymCreatesAConfirmedTagWithoutAttachments(t *testing.T) {
	f := newSynonymFixture(t)

	source, created, err := f.db.Tags().DetachSynonym(context.Background(), f.anime, "アニメ")
	if err != nil {
		t.Fatalf("DetachSynonym() error = %v", err)
	}
	if source.Name != "Anime" || len(source.Synonyms) != 0 || source.Tentative {
		t.Errorf("source = %+v, want Anime, still confirmed, without synonyms", source)
	}
	if created.Name != "アニメ" || len(created.Synonyms) != 0 || created.Tentative || created.Kind != nil ||
		created.VideoCount != 0 {
		t.Errorf("created = %+v, want a confirmed アニメ with no kind and no videos", created)
	}
	if assignmentCount(t, f.db, "key-1", created.ID) != 0 || assignmentCount(t, f.db, "key-1", f.anime) != 1 {
		t.Error("the attachment moved to the detached tag")
	}
	assertTagInvariantsNow(t, f.db)
}

// A name or tag that is gone answers its error and leaves every name and tentative state as it was.
func TestSynonymOperationsChangeNothingWhenTheSynonymOrTagIsGone(t *testing.T) {
	const missing = int64(999_999)
	cases := []struct {
		name string
		run  func(f synonymFixture) error
		want error
	}{
		{"promote the canonical name", func(f synonymFixture) error {
			_, err := f.db.Tags().PromoteSynonym(context.Background(), f.anime, "Anime")
			return err
		}, domain.ErrTagSynonymNotFound},
		{"promote a missing tag", func(f synonymFixture) error {
			_, err := f.db.Tags().PromoteSynonym(context.Background(), missing, "アニメ")
			return err
		}, domain.ErrTagNotFound},
		{"move another tag's name", func(f synonymFixture) error {
			_, _, err := f.db.Tags().MoveSynonym(context.Background(), f.anime, "Cartoon", f.cartoon)
			return err
		}, domain.ErrTagSynonymNotFound},
		{"move to a missing tag", func(f synonymFixture) error {
			_, _, err := f.db.Tags().MoveSynonym(context.Background(), f.anime, "アニメ", missing)
			return err
		}, domain.ErrTagNotFound},
		{"detach a name no tag holds", func(f synonymFixture) error {
			_, _, err := f.db.Tags().DetachSynonym(context.Background(), f.anime, "まんが")
			return err
		}, domain.ErrTagSynonymNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSynonymFixture(t)
			if err := tc.run(f); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if want := map[string]bool{"Anime": true, "アニメ": false}; !reflect.DeepEqual(tagNamesOf(t, f.db, f.anime), want) {
				t.Errorf("Anime's names = %v, want %v", tagNamesOf(t, f.db, f.anime), want)
			}
			if want := map[string]bool{"Cartoon": true}; !reflect.DeepEqual(tagNamesOf(t, f.db, f.cartoon), want) {
				t.Errorf("Cartoon's names = %v, want %v", tagNamesOf(t, f.db, f.cartoon), want)
			}
			if !f.tentative(t, f.cartoon) {
				t.Error("a failed operation confirmed Cartoon")
			}
		})
	}
}

// Folder-derived tags follow the name, so filtering by the tag now holding the folder's name finds the video.
func TestFolderTagsFollowAPromotedMovedOrDetachedName(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	video := upsertFolderVideo(t, db, fixturePath("/media/アニメ/1.mp4"), "k1")
	rebuildIndexForTest(t, db)
	anime, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatal(err)
	}
	cartoon, err := db.Tags().CreateTag(ctx, "Cartoon")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, anime.ID, "アニメ", nil); err != nil {
		t.Fatal(err)
	}
	filtered := func(tagID int64) []int64 {
		return listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{tagID}})
	}

	promoted, err := db.Tags().PromoteSynonym(ctx, anime.ID, "アニメ")
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.VideoTag{{TagRef: domain.TagRef{ID: anime.ID, Name: "アニメ"}, FromFolder: true}}
	if got := videoTagsOf(t, db, "k1"); !reflect.DeepEqual(got, want) || !slices.Equal(filtered(anime.ID), []int64{video}) {
		t.Errorf("after promote: tags = %+v, want %+v (promoted %+v)", got, want, promoted)
	}

	// Anime's canonical name is アニメ now, so make Anime a synonym again before moving it.
	if _, err := db.Tags().PromoteSynonym(ctx, anime.ID, "Anime"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().MoveSynonym(ctx, anime.ID, "アニメ", cartoon.ID); err != nil {
		t.Fatal(err)
	}
	if got := filtered(cartoon.ID); !slices.Equal(got, []int64{video}) || len(filtered(anime.ID)) != 0 {
		t.Errorf("after move: Cartoon = %v, Anime = %v, want only Cartoon on %d", got, filtered(anime.ID), video)
	}

	_, created, err := db.Tags().DetachSynonym(ctx, cartoon.ID, "アニメ")
	if err != nil {
		t.Fatal(err)
	}
	if got := filtered(created.ID); !slices.Equal(got, []int64{video}) || len(filtered(cartoon.ID)) != 0 {
		t.Errorf("after detach: アニメ = %v, Cartoon = %v, want only アニメ on %d", got, filtered(cartoon.ID), video)
	}
}

// The peer's promote swaps a synonym it holds and otherwise adds the new name, keeping the old one as a synonym as
// the local promote does, since the nodes may hold different synonyms; a promote already applied changes nothing.
func TestApplyTagByNamePromoteKeepsTheOldName(t *testing.T) {
	ctx := context.Background()
	promote := func(name, newName string) domain.VocabularyChange {
		return domain.VocabularyChange{Kind: domain.VocabularyTag, Action: domain.VocabularyPromote, Name: name, NewName: newName}
	}

	f := newSynonymFixture(t)
	if outcome, err := f.db.Tags().ApplyTagByName(ctx, promote("Anime", "アニメ")); err != nil || !outcome.Changed {
		t.Fatalf("swap: outcome = %+v, err = %v", outcome, err)
	}
	if want := map[string]bool{"アニメ": true, "Anime": false}; !reflect.DeepEqual(tagNamesOf(t, f.db, f.anime), want) {
		t.Errorf("after the swap: names = %v, want %v", tagNamesOf(t, f.db, f.anime), want)
	}
	if outcome, err := f.db.Tags().ApplyTagByName(ctx, promote("Anime", "アニメ")); err != nil || outcome.Changed {
		t.Errorf("repeated promote: outcome = %+v, err = %v, want no change", outcome, err)
	}

	if outcome, err := f.db.Tags().ApplyTagByName(ctx, promote("Cartoon", "まんが")); err != nil || !outcome.Changed {
		t.Fatalf("new name: outcome = %+v, err = %v", outcome, err)
	}
	if want := map[string]bool{"まんが": true, "Cartoon": false}; !reflect.DeepEqual(tagNamesOf(t, f.db, f.cartoon), want) {
		t.Errorf("after the new name: names = %v, want %v", tagNamesOf(t, f.db, f.cartoon), want)
	}
	if f.tentative(t, f.cartoon) {
		t.Error("after the new name: the tag stays tentative, want confirmed")
	}

	if outcome, err := f.db.Tags().ApplyTagByName(ctx, promote("Drama", "ドラマ")); err != nil || outcome.Changed {
		t.Errorf("missing name: outcome = %+v, err = %v, want no change", outcome, err)
	}
	var conflict *domain.TagNameConflict
	if _, err := f.db.Tags().ApplyTagByName(ctx, promote("まんが", "Anime")); !errors.As(err, &conflict) {
		t.Errorf("another tag's synonym: err = %v, want a TagNameConflict", err)
	}
}
