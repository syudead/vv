package store

import (
	"bytes"
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func face(label string) domain.PersonImage {
	return domain.PersonImage{JPEG: []byte(label), Width: 3, Height: 4}
}

func TestDeletingAPersonRemovesItsFace(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "山田"})
	if _, err := db.Credits().SetPersonImage(ctx, person.ID, face("a")); err != nil {
		t.Fatal(err)
	}
	if err := db.Credits().Delete(ctx, domain.CreditPerson, person.ID); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := db.sql.QueryRowContext(ctx, `select count(*) from person_images`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("人物の削除後の顔 = %d 行, %v", rows, err)
	}
}

func TestAddPersonImageKeepsAFaceAlreadySet(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "山田"})
	if added, err := db.Credits().AddPersonImage(ctx, person.ID, face("first")); err != nil || !added {
		t.Fatalf("AddPersonImage(faceless) = %v, %v; want stored", added, err)
	}
	if added, err := db.Credits().AddPersonImage(ctx, person.ID, face("second")); err != nil || added {
		t.Fatalf("AddPersonImage(with a face) = %v, %v; want kept", added, err)
	}
	got, err := db.Credits().PersonImage(ctx, person.ID)
	if err != nil || !bytes.Equal(got.JPEG, []byte("first")) || got.Revision != 1 {
		t.Errorf("face = %q rev %d, %v; want first rev 1", got.JPEG, got.Revision, err)
	}
}

func TestMergeGivesAFacelessTargetTheFirstSourceFace(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "T"})
	faceless := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "S0"})
	second := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "S2"})
	first := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "S1"})
	for _, source := range []struct {
		id    int64
		label string
	}{{first.ID, "first"}, {second.ID, "second"}} {
		if _, err := db.Credits().SetPersonImage(ctx, source.id, face(source.label)); err != nil {
			t.Fatal(err)
		}
	}

	// second comes before first in sourceIds, so its face wins.
	outcome, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{faceless.ID, second.ID, first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Credit.ImageRevision == 0 {
		t.Error("マージ後の人物に imageRevision が無い")
	}
	got, err := db.Credits().PersonImage(ctx, target.ID)
	if err != nil || !bytes.Equal(got.JPEG, []byte("second")) || got.Revision != outcome.Credit.ImageRevision {
		t.Errorf("マージ後の顔 = %q rev %d, %v; want second rev %d", got.JPEG, got.Revision, err, outcome.Credit.ImageRevision)
	}
}

func TestMergeKeepsTheTargetFace(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "T"})
	source := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "S"})
	before, err := db.Credits().SetPersonImage(ctx, target.ID, face("target"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Credits().SetPersonImage(ctx, source.ID, face("source")); err != nil {
		t.Fatal(err)
	}
	outcome, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{source.ID})
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.Credits().PersonImage(ctx, target.ID)
	if err != nil || !bytes.Equal(got.JPEG, []byte("target")) || outcome.Credit.ImageRevision != before.ImageRevision {
		t.Errorf("マージ後の顔 = %q, rev %d → %d, %v", got.JPEG, before.ImageRevision, outcome.Credit.ImageRevision, err)
	}
}

func TestWorkInfoCarriesTheImageRevisionOfPeopleWithAFace(t *testing.T) {
	db, ids := versionFixture(t)
	withFace := createPerson(t, db, "顔あり")
	faceless := createPerson(t, db, "顔なし")
	setWorkInfo(t, db, ids["a"], domain.WorkInfo{People: []domain.WorkPerson{performer(withFace.ID), performer(faceless.ID)}})
	updated, err := db.Credits().SetPersonImage(context.Background(), withFace.ID, face("a"))
	if err != nil {
		t.Fatal(err)
	}
	info, _ := workInfoOf(t, db, ids["a"])
	if len(info.People) != 2 || info.People[0].ImageRevision != updated.ImageRevision || updated.ImageRevision == 0 ||
		info.People[1].ImageRevision != 0 {
		t.Errorf("作品情報の人物 = %+v, want revision %d と 0", info.People, updated.ImageRevision)
	}
}
