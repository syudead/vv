package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 取得した作品の詳細の反映（specs/052-plugin-work-info-fields/research.md R-3・R-4、#1158）。

var performerField = domain.PersonField(domain.PersonPerformer)

// applyDetails は動画 videoID に、承認済みのプラグインの取得として fetched のうち fields を反映する。
func applyDetails(
	t *testing.T, db *DB, videoID int64, fetched domain.FetchedWorkInfo, fields ...domain.WorkInfoField,
) domain.FetchedWorkInfoApplied {
	t.Helper()
	plugin := approvedPlugin(t, db, "details", "dd")
	applied, err := db.Overrides().ApplyFetchedWorkInfo(context.Background(), domain.FetchedWorkInfoApply{
		VideoID: videoID, PluginID: plugin.ID, Digest: "dd", Generation: plugin.Generation,
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

// 受け入れ条件 2・3: 保存済みの出演者の横に 2 人目が足され、既存の人物の外部 ID を持つ件は新しい行を作らずに
// 結び付き、何にも当たらない件は取得した外部 ID を持つ人物を作る。
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

// 結果に 2 回ある人物と、その役割にすでに結び付いている人物は 1 回だけ結び付く。取得と反映の間に作られた
// 人物には結び付き、重複して作られない。
func TestApplyFetchedDetailsLinksEachPersonOnce(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	linked := createPerson(t, db, "Linked")
	setWorkInfo(t, db, alpha, domain.WorkInfo{People: []domain.WorkPerson{
		{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: linked.ID}},
	}})
	// 取得の時には無かった人物が、反映の前に作られた。
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

// Edge Cases: 曖昧な件は結び付かず作られず報告され、100 人いる動画への新しい出演者は too_many で人物の
// 行を残さない。
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

// 受け入れ条件 5: 名前が実体の上限を超える新しい出演者は invalid で報告され、選んだ品番は書かれる。
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

// 動画が無いときは、新しい人物も作品情報の行も残らない。
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

// VideoWorkInfoChanged は作品情報が変わったとき確定後に 1 回出て、題名だけが変わったときは出ない。
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
			VideoID: alpha, PluginID: plugin.ID, Digest: "dd", Generation: plugin.Generation, Title: &title,
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
