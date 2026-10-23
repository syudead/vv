package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

var performerField = domain.PersonField(domain.PersonPerformer)

func applyDetails(
	t *testing.T, db *DB, videoID int64, fetched domain.FetchedWorkInfo, fields ...domain.WorkInfoField,
) domain.FetchedWorkInfoApplied {
	t.Helper()
	plugin := approvedPlugin(t, db, "details", "dd")
	applied, err := db.Overrides().ApplyFetchedWorkInfo(context.Background(), domain.FetchedWorkInfoApply{
		VideoID: videoID, ContentKey: videoContentKey(t, db, videoID),
		PluginID: plugin.ID, Digest: "dd", Generation: plugin.Generation,
		Details: fetched, DetailFields: fields,
	})
	if err != nil {
		t.Fatalf("ApplyFetchedWorkInfo: %v", err)
	}
	return applied
}

func personCount(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	if err := db.sql.QueryRow(`select count(*) from people`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func performerIDs(info *domain.WorkInfo) []int64 {
	var ids []int64
	if info == nil {
		return nil
	}
	for _, p := range info.People {
		if p.Role == domain.PersonPerformer {
			ids = append(ids, p.Person.ID)
		}
	}
	return ids
}

func fetchedPerformer(name string, ids ...domain.ExternalID) domain.FetchedPerson {
	return domain.FetchedPerson{Role: domain.PersonPerformer, Name: name, ExternalIDs: ids}
}

func TestApplyFetchedDetailsLinksAndCreatesPeople(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	stored := createPerson(t, db, "Stored")
	holder := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Aoi", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "a-1"}},
	})
	setWorkInfo(t, db, alpha, domain.WorkInfo{People: []domain.WorkPerson{
		{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: stored.ID}},
	}})
	before := personCount(t, db)

	applied := applyDetails(t, db, alpha, domain.FetchedWorkInfo{People: []domain.FetchedPerson{
		fetchedPerformer("Another spelling", domain.ExternalID{Source: "fanza", Value: "a-1"}),
		fetchedPerformer("Mika", domain.ExternalID{Source: "fanza", Value: "m-1"}),
	}}, performerField)

	got := performerIDs(applied.Video.WorkInfo)
	if len(got) != 3 || got[0] != stored.ID || got[1] != holder.ID {
		t.Fatalf("performers = %v; want stored, the holder, then a new person", got)
	}
	if personCount(t, db) != before+1 {
		t.Fatalf("people = %d; want one new row", personCount(t, db))
	}
	created, err := db.Credits().Get(context.Background(), domain.CreditPerson, got[2])
	if err != nil {
		t.Fatal(err)
	}
	if want := []domain.ExternalID{{Source: "fanza", Value: "m-1"}}; created.Name != "Mika" || !reflect.DeepEqual(created.ExternalIDs, want) {
		t.Fatalf("created = %+v; want Mika with %v", created, want)
	}
	if len(applied.Skipped) != 0 {
		t.Fatalf("skipped = %+v; want none", applied.Skipped)
	}
}

// Reuse people created between fetch and apply to avoid duplicates.
func TestApplyFetchedDetailsLinksEachPersonOnce(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	linked := createPerson(t, db, "Linked")
	setWorkInfo(t, db, alpha, domain.WorkInfo{People: []domain.WorkPerson{
		{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: linked.ID}},
	}})
	// Simulate a person created after fetch but before apply.
	late := createPerson(t, db, "Late")
	before := personCount(t, db)

	applied := applyDetails(t, db, alpha, domain.FetchedWorkInfo{People: []domain.FetchedPerson{
		fetchedPerformer("Linked"), fetchedPerformer("Late"), fetchedPerformer("ＬＡＴＥ"), fetchedPerformer("New one"), fetchedPerformer("new one"),
	}}, performerField)

	got := performerIDs(applied.Video.WorkInfo)
	if len(got) != 3 || got[0] != linked.ID || got[1] != late.ID {
		t.Fatalf("performers = %v; want linked, late, then one new person", got)
	}
	if personCount(t, db) != before+1 {
		t.Fatalf("people = %d; want exactly one new row", personCount(t, db))
	}
}

func TestApplyFetchedDetailsSkipsAmbiguousAndOverTheLimit(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	createPerson(t, db, "Rin")
	createPerson(t, db, "Rin Kato", "rin")
	limit := domain.WorkInfoLimits.PeoplePerRoleMaxCount
	people := make([]domain.WorkPerson, limit)
	for i := range people {
		people[i] = domain.WorkPerson{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: createPerson(t, db, "P"+strings.Repeat("x", i+1)).ID}}
	}
	setWorkInfo(t, db, alpha, domain.WorkInfo{People: people})
	before := personCount(t, db)

	applied := applyDetails(t, db, alpha, domain.FetchedWorkInfo{People: []domain.FetchedPerson{
		fetchedPerformer("Rin"), fetchedPerformer("Brand new"),
	}}, performerField)

	want := []domain.SkippedWorkInfoValue{
		{Field: performerField, Value: "Rin", Reason: domain.SkippedWorkInfoAmbiguous},
		{Field: performerField, Value: "Brand new", Reason: domain.SkippedWorkInfoTooMany},
	}
	if !reflect.DeepEqual(applied.Skipped, want) {
		t.Fatalf("skipped = %+v, want %+v", applied.Skipped, want)
	}
	if personCount(t, db) != before || len(performerIDs(applied.Video.WorkInfo)) != limit {
		t.Fatal("a skipped entry created a person or changed the performers")
	}
}

func TestApplyFetchedDetailsWritesTheCodeBesideAnInvalidPerformer(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	before := personCount(t, db)
	long := strings.Repeat("名", domain.CreditNameMaxLength+1)

	applied := applyDetails(t, db, alpha, domain.FetchedWorkInfo{
		Code: "ABC-123", People: []domain.FetchedPerson{fetchedPerformer(long)},
	}, domain.WorkInfoFieldCode, performerField)

	want := []domain.SkippedWorkInfoValue{{Field: performerField, Value: long, Reason: domain.SkippedWorkInfoInvalid}}
	if !reflect.DeepEqual(applied.Skipped, want) {
		t.Fatalf("skipped = %+v, want %+v", applied.Skipped, want)
	}
	if info, ok := workInfoOf(t, db, alpha); !ok || info.Code != "ABC-123" || len(info.People) != 0 {
		t.Fatalf("work info = %+v; want the code and no people", info)
	}
	if personCount(t, db) != before {
		t.Fatal("an invalid entry created a person")
	}
}

func TestApplyFetchedDetailsOnAMissingVideoLeavesNothing(t *testing.T) {
	db, _ := overrideFixture(t)
	plugin := approvedPlugin(t, db, "details", "dd")
	before := personCount(t, db)

	_, err := db.Overrides().ApplyFetchedWorkInfo(context.Background(), domain.FetchedWorkInfoApply{
		VideoID: 9999, PluginID: plugin.ID, Digest: "dd", Generation: plugin.Generation,
		Details:      domain.FetchedWorkInfo{Code: "ABC-1", People: []domain.FetchedPerson{fetchedPerformer("New")}},
		DetailFields: []domain.WorkInfoField{domain.WorkInfoFieldCode, performerField},
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
	var rows int
	if err := db.sql.QueryRow(`select count(*) from video_work_info`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if personCount(t, db) != before || rows != 0 {
		t.Fatalf("people = %d (was %d), work info rows = %d; want nothing written", personCount(t, db), before, rows)
	}
}

func TestApplyFetchedDetailsPublishesWorkInfoChangedOnlyForDetails(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	plugin := approvedPlugin(t, db, "details", "dd")
	count := func(events []domain.Event) int {
		n := 0
		for _, event := range events {
			if _, ok := event.(domain.VideoWorkInfoChanged); ok {
				n++
			}
		}
		return n
	}
	apply := func(title string, fields ...domain.WorkInfoField) []domain.Event {
		t.Helper()
		recorder := &eventRecorder{}
		db.PublishTo(recorder)
		if _, err := db.Overrides().ApplyFetchedWorkInfo(context.Background(), domain.FetchedWorkInfoApply{
			VideoID: alpha, ContentKey: videoContentKey(t, db, alpha),
			PluginID: plugin.ID, Digest: "dd", Generation: plugin.Generation, Title: &title,
			Details: domain.FetchedWorkInfo{Code: "ABC-1"}, DetailFields: fields,
		}); err != nil {
			t.Fatalf("ApplyFetchedWorkInfo: %v", err)
		}
		return recorder.events
	}

	if events := apply("Only the title"); count(events) != 0 {
		t.Fatalf("title only: events = %v; want no VideoWorkInfoChanged", events)
	}
	events := apply("Title and code", domain.WorkInfoFieldCode)
	if count(events) != 1 || !reflect.DeepEqual(events[len(events)-1], domain.Event(domain.VideoWorkInfoChanged{VideoIDs: []int64{alpha}})) {
		t.Fatalf("with the code: events = %v; want one VideoWorkInfoChanged for the video", events)
	}
	if events := apply("Title and code", domain.WorkInfoFieldCode); count(events) != 0 {
		t.Fatalf("same code again: events = %v; want no VideoWorkInfoChanged", events)
	}
}

func TestApplyFetchedDetailsFillsOnlyTheEmptyProfileFieldsOfLinkedPeople(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	existing := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Aoi", Profile: domain.PersonProfile{BirthDate: "1990-05-17"},
	})
	height, tooTall := 160, 1000
	aoi := fetchedPerformer("Aoi")
	aoi.Profile = domain.PersonProfile{BirthDate: "1985", HeightCm: &height}
	mika := fetchedPerformer("Mika")
	mika.Profile = domain.PersonProfile{Cup: "e", HeightCm: &tooTall}
	director := domain.FetchedPerson{Role: domain.PersonDirector, Name: "Aoi", Profile: domain.PersonProfile{Cup: "C"}}

	applied := applyDetails(t, db, alpha, domain.FetchedWorkInfo{People: []domain.FetchedPerson{aoi, mika, director}},
		performerField)

	got := performerIDs(applied.Video.WorkInfo)
	if len(got) != 2 || got[0] != existing.ID {
		t.Fatalf("performers = %v; want Aoi, then a new person", got)
	}
	if want := []int64{existing.ID, got[1], 0}; !reflect.DeepEqual(applied.PersonIDs, want) {
		t.Errorf("person IDs = %v, want %v (the director's role was not selected)", applied.PersonIDs, want)
	}
	stored, err := db.Credits().Get(context.Background(), domain.CreditPerson, existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := (domain.PersonProfile{BirthDate: "1990-05-17", HeightCm: &height}); !stored.Profile.Equal(want) {
		t.Errorf("Aoi's profile = %+v, want the height filled and the birth date kept", stored.Profile)
	}
	created, err := db.Credits().Get(context.Background(), domain.CreditPerson, got[1])
	if err != nil {
		t.Fatal(err)
	}
	if want := (domain.PersonProfile{Cup: "E"}); !created.Profile.Equal(want) {
		t.Errorf("Mika's profile = %+v, want the cup alone", created.Profile)
	}
	wantSkipped := []domain.SkippedWorkInfoValue{
		{Field: performerField, Value: "Mika: heightCm 1000", Reason: domain.SkippedWorkInfoInvalid},
	}
	if !reflect.DeepEqual(applied.Skipped, wantSkipped) {
		t.Errorf("skipped = %+v, want %+v", applied.Skipped, wantSkipped)
	}
}
