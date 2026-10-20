package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func publicKeyCount(t *testing.T, db *DB, key string) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from public_videos where content_key = ?`, key).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestApplyVideoEditWritesEveryPartToSharedKeys(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	hanako := createPerson(t, db, "山田花子")
	added := attachNamedTag(t, db, "新", ids["d"])
	old := attachNamedTag(t, db, "旧", ids["a"], ids["c"])
	// d is already in the target state, so the edit leaves it unchanged and does not count it.
	setWorkInfo(t, db, ids["d"], domain.WorkInfo{
		People: []domain.WorkPerson{performer(hanako.ID)}, Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: "2020"}},
	})
	setPublic(t, db, true, ids["d"])

	result, err := db.VideoEdits().Apply(ctx, []int64{ids["a"], ids["c"], ids["d"], 999_999}, domain.VideoEdit{
		TagsAdd:    []domain.TagInput{{Name: new(" 新 ")}, {ID: new(added)}},
		TagsRemove: []int64{old},
		Credits:    []domain.CreditEdit{{Kind: domain.CreditPerson, Role: string(domain.PersonPerformer), Add: []int64{hanako.ID}}},
		Dates:      []domain.DateEdit{{Kind: domain.DateRelease, Date: new("2020")}},
		Public:     new(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied != 2 || len(result.Tags) != 1 || result.Tags[0].ID != added || result.ContentKeys != nil {
		t.Errorf("result = %+v, want 2 applied (a and c), the added tag once and no revoked keys", result)
	}
	want := domain.WorkInfo{
		Dates:  []domain.WorkDate{{Kind: domain.DateRelease, Date: "2020"}},
		People: []domain.WorkPerson{performer(hanako.ID)},
	}
	for _, name := range []string{"b", "c"} {
		key := ownerVideo(t, db, ids[name]).UserKey
		if tags := manualTagNames(t, db, key); !reflect.DeepEqual(tags, []string{"新"}) {
			t.Errorf("%s manual tags = %v, want [新]", name, tags)
		}
		info, _ := workInfoOf(t, db, ids[name])
		if !info.Equal(want) {
			t.Errorf("%s work info = %+v, want %+v", name, info, want)
		}
		if publicKeyCount(t, db, key) != 1 {
			t.Errorf("%s is not public", name)
		}
	}
}

func TestApplyVideoEditWritesNothingWhenALaterPartFails(t *testing.T) {
	db, ids := versionFixture(t)
	before := workInfoRowCount(t, db)

	_, err := db.VideoEdits().Apply(context.Background(), []int64{ids["a"]}, domain.VideoEdit{
		TagsAdd: []domain.TagInput{{Name: new("まだない")}},
		Credits: []domain.CreditEdit{{Kind: domain.CreditPerson, Role: string(domain.PersonDirector), Remove: []int64{999_999}}},
		Public:  new(true),
	})
	var missing *domain.CreditNotFoundError
	if !errors.As(err, &missing) || !reflect.DeepEqual(*missing, domain.CreditNotFoundError{
		Kind: domain.CreditPerson, ID: 999_999, Field: "credits[0].remove[0]",
	}) {
		t.Fatalf("err = %v, want person 999999 missing at credits[0].remove[0]", err)
	}
	var tags int
	if err := db.sql.QueryRow(`select count(*) from tag_names where name = 'まだない'`).Scan(&tags); err != nil {
		t.Fatal(err)
	}
	key := ownerVideo(t, db, ids["a"]).UserKey
	if tags != 0 || publicKeyCount(t, db, key) != 0 || workInfoRowCount(t, db) != before {
		t.Errorf("tags named = %d, a public = %d, work-info rows = %d; want nothing written",
			tags, publicKeyCount(t, db, key), workInfoRowCount(t, db))
	}
}

func TestVideoEditSummaryCountsSharedKeysOnce(t *testing.T) {
	db, ids := versionFixture(t)
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	p2 := createPerson(t, db, "P2")
	p10 := createPerson(t, db, "P10")
	maker := createOrganization(t, db, "メーカー", nil)
	setWorkInfo(t, db, ids["a"], domain.WorkInfo{
		People: []domain.WorkPerson{performer(p2.ID)}, Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: "2020"}},
	})
	setWorkInfo(t, db, ids["c"], domain.WorkInfo{
		People: []domain.WorkPerson{performer(p10.ID), performer(p2.ID)},
		Organizations: []domain.WorkOrganization{
			{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: maker.ID}},
		},
		Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: "2020"}, {Kind: domain.DateRecorded, Date: "2019"}},
	})
	setWorkInfo(t, db, ids["d"], domain.WorkInfo{Dates: []domain.WorkDate{{Kind: domain.DateRecorded, Date: "2018"}}})
	setPublic(t, db, true, ids["d"])
	tag := attachNamedTag(t, db, "旅行", ids["c"])

	summary, err := db.VideoEdits().Summary(context.Background(), []int64{ids["a"], ids["b"], ids["c"], ids["d"]})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.VideoEditSummary{
		Total: 3, Public: 1,
		Tags: []domain.TagSummaryItem{{Tag: domain.TagRef{ID: tag, Name: "旅行"}, Count: 1, ManualCount: 1}},
		People: []domain.VideoEditPersonCount{
			{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: p2.ID, Name: "P2"}, Count: 2},
			{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: p10.ID, Name: "P10"}, Count: 1},
		},
		Organizations: []domain.VideoEditOrganizationCount{
			{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: maker.ID, Name: "メーカー"}, Count: 1},
		},
		Dates: []domain.VideoEditDateCount{
			{Kind: domain.DateRelease, Count: 2, Date: "2020"},
			{Kind: domain.DateRecorded, Count: 2},
		},
	}
	if !reflect.DeepEqual(summary, want) {
		t.Errorf("summary = %+v\nwant %+v", summary, want)
	}
}
