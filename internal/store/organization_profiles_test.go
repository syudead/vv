package store

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestUpdateOrganizationProfileReplacesOnlyPatchedFields(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	org := createCredit(t, db, domain.CreditOrganization, domain.CreditInput{Name: "Studio",
		OrganizationProfile: domain.OrganizationProfile{
			FoundedDate: "1990-04", Location: "東京", Bio: "a", Links: []string{"https://a.example", "https://b.example"},
		}})
	updated, err := db.Credits().Update(ctx, domain.CreditOrganization, org.ID, domain.CreditPatch{
		OrganizationProfile: &domain.OrganizationProfilePatch{
			Fields: []domain.OrganizationProfileField{domain.OrganizationDissolvedDate, domain.OrganizationLocation, domain.OrganizationLinks},
			Values: domain.OrganizationProfile{DissolvedDate: "2001", Links: []string{"https://b.example"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.OrganizationProfile{FoundedDate: "1990-04", DissolvedDate: "2001", Bio: "a", Links: []string{"https://b.example"}}
	got, err := db.Credits().Get(ctx, domain.CreditOrganization, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.OrganizationProfile, want) || !reflect.DeepEqual(got.OrganizationProfile, want) {
		t.Errorf("profile = %+v, read back %+v; want %+v", updated.OrganizationProfile, got.OrganizationProfile, want)
	}
}

func TestOrganizationLogoChangesItsRevisionWithoutAFaceJob(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	org := createOrganization(t, db, "Studio", nil)
	set, err := db.Credits().SetCreditImage(ctx, domain.CreditOrganization, org.ID, face("logo"))
	if err != nil || set.ImageRevision != 1 {
		t.Fatalf("SetCreditImage = rev %d, %v; want rev 1", set.ImageRevision, err)
	}
	var jobs int
	if err := db.sql.QueryRowContext(ctx, `select count(*) from jobs`).Scan(&jobs); err != nil || jobs != 0 {
		t.Errorf("jobs after a logo = %d, %v; want none", jobs, err)
	}
	removed, err := db.Credits().DeleteCreditImage(ctx, domain.CreditOrganization, org.ID)
	if err != nil || removed.ImageRevision != 0 {
		t.Fatalf("DeleteCreditImage = rev %d, %v; want no image", removed.ImageRevision, err)
	}
	if _, err := db.Credits().CreditImage(ctx, domain.CreditOrganization, org.ID); !errors.Is(err, domain.ErrPersonImageNotFound) {
		t.Errorf("CreditImage after removal = %v, want ErrPersonImageNotFound", err)
	}
}

func TestListOrganizationsFavoriteOnly(t *testing.T) {
	db := migratedDB(t)
	liked := createOrganization(t, db, "Liked", nil)
	createOrganization(t, db, "Other", nil)
	setFavorites(t, db, domain.FavoriteChange{OrganizationIDs: []int64{liked.ID, 999_999}, Favorite: true})

	page, err := db.Credits().List(context.Background(), owner, domain.CreditOrganization, domain.CreditQuery{FavoriteOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != liked.ID || !page.Items[0].Favorite || page.Total != 1 {
		t.Errorf("お気に入りだけの団体一覧 = %+v", page)
	}
}

func TestMergeOrganizationsFillsProfileLogoAndFavoriteFromSources(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target := createCredit(t, db, domain.CreditOrganization, domain.CreditInput{Name: "T",
		OrganizationProfile: domain.OrganizationProfile{Location: "kept"}})
	source := createCredit(t, db, domain.CreditOrganization, domain.CreditInput{Name: "S",
		OrganizationProfile: domain.OrganizationProfile{Location: "lost", Bio: "filled"}})
	if _, err := db.Credits().SetCreditImage(ctx, domain.CreditOrganization, source.ID, face("logo")); err != nil {
		t.Fatal(err)
	}
	setFavorites(t, db, domain.FavoriteChange{OrganizationIDs: []int64{source.ID}, Favorite: true})

	if _, err := db.Credits().Merge(ctx, domain.CreditOrganization, target.ID, []int64{source.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := db.Credits().Get(ctx, domain.CreditOrganization, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.OrganizationProfile{Location: "kept", Bio: "filled"}
	if !got.OrganizationProfile.Equal(want) || !got.Favorite || got.ImageRevision != 1 {
		t.Errorf("merged = profile %+v favorite %v rev %d; want %+v, favorite, rev 1",
			got.OrganizationProfile, got.Favorite, got.ImageRevision, want)
	}
	if logo, err := db.Credits().CreditImage(ctx, domain.CreditOrganization, target.ID); err != nil || !bytes.Equal(logo.JPEG, []byte("logo")) {
		t.Errorf("merged logo = %q, %v", logo.JPEG, err)
	}
}
