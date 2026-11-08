package store

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestGroupCardsReturnReadFailuresInsteadOfPartialCards(t *testing.T) {
	db := migratedDB(t)
	video := looseVideo(t, db, "cover")
	id := createGroup(t, db, "group", domain.GroupAdd{VideoIDs: []int64{video}})
	execSQL(t, db, `update videos set thumbnail_state = 'done'`)
	refreshCollections(t, db)
	for _, tc := range []struct{ name, fault, message string }{
		{"summary", `update group_values set duration_ms = 'invalid duration'`, "invalid duration"},
		{"preview", `update videos set width = 'invalid width'`, "invalid width"},
		{"tags", `alter table tag_names rename to unavailable_tag_names`, "cannot read group card tags"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tx, err := db.sql.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback() }()
			if _, err := tx.ExecContext(ctx, tc.fault); err != nil {
				t.Fatal(err)
			}
			cards, err := loadGroupCards(ctx, tx, domain.AudienceOwner, []int64{id})
			if err == nil || !strings.Contains(err.Error(), tc.message) || cards != nil {
				t.Fatalf("cards = %+v, error = %v; want no partial cards and %q", cards, err, tc.message)
			}
		})
	}
}

// A card's tag is unconfirmed when the first member carrying it holds it tentatively, whatever later members hold.
func TestGroupCardTagFollowsItsFirstTaggedMember(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	plain, early, late := looseVideo(t, db, "plain"), looseVideo(t, db, "early"), looseVideo(t, db, "late")
	createGroup(t, db, "group", domain.GroupAdd{VideoIDs: []int64{plain, early, late}})
	attach := func(id int64, name string, tentative bool) {
		t.Helper()
		if _, err := db.Tags().ApplyVideoTags(ctx, []domain.VideoRef{{ID: id}}, domain.VideoTagsAdd, []string{name}, false); err != nil {
			t.Fatal(err)
		}
		if tentative {
			execSQL(t, db, `update video_tags set tentative = 1 where content_key = (select content_key from videos where id = `+
				strconv.FormatInt(id, 10)+`) and tag_id = (select tag_id from tag_names where name = '`+name+`')`)
		}
	}
	attach(early, "first tentative", true)
	attach(late, "first tentative", false)
	attach(early, "first confirmed", false)
	attach(late, "first confirmed", true)

	page, err := db.Library().ListLibrary(ctx, domain.AudienceOwner, domain.VideoQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Group == nil || page.Items[0].Group.Card == nil {
		t.Fatalf("items = %+v, want one group card", page.Items)
	}
	got := map[string]bool{}
	for _, tag := range page.Items[0].Group.Card.Tags {
		if !tag.Manual {
			t.Errorf("tag %q is not manual", tag.Name)
		}
		got[tag.Name] = tag.Unconfirmed
	}
	want := map[string]bool{"first tentative": true, "first confirmed": false}
	if len(got) != len(want) || got["first tentative"] != true || got["first confirmed"] != false {
		t.Errorf("card tags unconfirmed = %v, want %v", got, want)
	}
}
