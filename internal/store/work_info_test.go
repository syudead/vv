package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func setWorkInfo(t *testing.T, db *DB, id int64, info domain.WorkInfo) domain.Video {
	t.Helper()
	video, err := db.WorkInfo().Set(context.Background(), id, info)
	if err != nil {
		t.Fatalf("作品情報を書けない: %v", err)
	}
	return video
}

func workInfoOf(t *testing.T, db *DB, id int64) (domain.WorkInfo, bool) {
	t.Helper()
	key := ownerVideo(t, db, id).UserKey
	infos, err := db.WorkInfo().ByUserKeys(context.Background(), []string{key})
	if err != nil {
		t.Fatal(err)
	}
	info, ok := infos[key]
	return info, ok
}

func workInfoRowCount(t *testing.T, db *DB) int {
	t.Helper()
	total := 0
	for _, table := range workInfoTables {
		var count int
		if err := db.sql.QueryRow(`select count(*) from ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		total += count
	}
	return total
}

func performer(id int64) domain.WorkPerson {
	return domain.WorkPerson{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: id}}
}

func onlyDate(date string) domain.WorkInfo {
	return domain.WorkInfo{Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: date}}}
}

func TestSetWorkInfoReadsBackInOrderWithCurrentNames(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	hanako := createPerson(t, db, "山田花子")
	taro := createPerson(t, db, "佐藤太郎")
	maker := createOrganization(t, db, "メーカーA", nil)

	returned := setWorkInfo(t, db, ids["a"], domain.WorkInfo{
		Code: " ABC-123 ", Description: "一行目\n二行目",
		Dates: []domain.WorkDate{{Kind: domain.DatePublished, Date: "2024-03"}, {Kind: domain.DateRelease, Date: "2024-02-29"}},
		People: []domain.WorkPerson{
			{Role: domain.PersonDirector, Person: domain.CreditRef{ID: hanako.ID}},
			performer(taro.ID), performer(hanako.ID),
		},
		Organizations: []domain.WorkOrganization{{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: maker.ID}}},
		URLs:          []string{"https://example.com/b", "https://example.com/a"},
		ExternalIDs:   []domain.ExternalID{{Source: "z", Value: "1"}, {Source: "a", Value: "9"}},
	})
	newName := "山田はなこ"
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, hanako.ID, domain.CreditPatch{Name: &newName}); err != nil {
		t.Fatal(err)
	}

	got, ok := workInfoOf(t, db, ids["a"])
	if !ok {
		t.Fatal("作品情報が読めない")
	}
	want := domain.WorkInfo{
		Code: "ABC-123", Description: "一行目\n二行目",
		Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: "2024-02-29"}, {Kind: domain.DatePublished, Date: "2024-03"}},
		People: []domain.WorkPerson{
			{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: taro.ID, Name: "佐藤太郎"}},
			{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: hanako.ID, Name: newName}},
			{Role: domain.PersonDirector, Person: domain.CreditRef{ID: hanako.ID, Name: newName}},
		},
		Organizations: []domain.WorkOrganization{
			{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: maker.ID, Name: "メーカーA"}},
		},
		URLs:        []string{"https://example.com/b", "https://example.com/a"},
		ExternalIDs: []domain.ExternalID{{Source: "a", Value: "9"}, {Source: "z", Value: "1"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("読み返した作品情報 =\n%+v\nwant\n%+v", got, want)
	}
	if returned.WorkInfo == nil || returned.WorkInfo.Code != "ABC-123" || len(returned.WorkInfo.People) != 3 ||
		returned.WorkInfo.People[1].Person.Name != "山田花子" {
		t.Errorf("戻り値の作品情報 = %+v", returned.WorkInfo)
	}
	if _, ok := workInfoOf(t, db, ids["b"]); ok {
		t.Error("書いていない動画に作品情報がある")
	}
}

func TestSetWorkInfoSkipsUnchangedValues(t *testing.T) {
	db, ids := versionFixture(t)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	person := createPerson(t, db, "山田花子")
	info := domain.WorkInfo{Code: "ABC-1", People: []domain.WorkPerson{performer(person.ID)}}

	start := time.Now()
	setWorkInfo(t, db, ids["a"], info)
	assertEditedSince(t, db, ids["a"], start)
	want := []domain.Event{domain.VideoWorkInfoChanged{VideoIDs: []int64{ids["a"]}}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Errorf("events = %v, want %v", recorder.events, want)
	}

	resetEditedAt(t, db)
	recorder.events = nil
	info.Code = " ABC-1 "
	setWorkInfo(t, db, ids["a"], info)
	assertEditedAt(t, db, ids["a"], staleEditedAt)
	if len(recorder.events) != 0 {
		t.Errorf("同じ値で events = %v", recorder.events)
	}
}

func TestSetWorkInfoRefusesMissingReferences(t *testing.T) {
	db, ids := versionFixture(t)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	person := createPerson(t, db, "山田花子")
	maker := createOrganization(t, db, "メーカーA", nil)

	_, err := db.WorkInfo().Set(context.Background(), ids["a"], domain.WorkInfo{
		Code:   "ABC-1",
		People: []domain.WorkPerson{performer(person.ID), performer(person.ID + 100)},
	})
	var missing *domain.CreditNotFoundError
	if !errors.As(err, &missing) || *missing != (domain.CreditNotFoundError{Kind: domain.CreditPerson, ID: person.ID + 100, Field: "people[1].person.id"}) {
		t.Fatalf("err = %v, want CreditNotFoundError people[1].person.id", err)
	}
	_, err = db.WorkInfo().Set(context.Background(), ids["a"], domain.WorkInfo{
		Organizations: []domain.WorkOrganization{
			{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: maker.ID}},
			{Role: domain.OrganizationLabel, Organization: domain.CreditRef{ID: maker.ID + 100}},
		},
	})
	if !errors.As(err, &missing) || missing.Field != "organizations[1].organization.id" || missing.Kind != domain.CreditOrganization {
		t.Fatalf("err = %v, want CreditNotFoundError organizations[1].organization.id", err)
	}
	if _, err := db.WorkInfo().Set(context.Background(), 9999, domain.WorkInfo{Code: "A"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い動画 = %v, want ErrNotFound", err)
	}
	if workInfoRowCount(t, db) != 0 || len(recorder.events) != 0 || editRowCount(t, db) != 0 {
		t.Errorf("誤りで残した: 行 %d, events %v, 更新日時 %d", workInfoRowCount(t, db), recorder.events, editRowCount(t, db))
	}
}

func TestSetWorkInfoClearsEveryTable(t *testing.T) {
	db, ids := versionFixture(t)
	person := createPerson(t, db, "山田花子")
	maker := createOrganization(t, db, "メーカーA", nil)
	setWorkInfo(t, db, ids["a"], domain.WorkInfo{
		Code: "A", Description: "B", Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: "2024"}},
		People:        []domain.WorkPerson{performer(person.ID)},
		Organizations: []domain.WorkOrganization{{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: maker.ID}}},
		URLs:          []string{"https://example.com"}, ExternalIDs: []domain.ExternalID{{Source: "s", Value: "v"}},
	})
	if workInfoRowCount(t, db) != 6 {
		t.Fatalf("行 = %d, want 6", workInfoRowCount(t, db))
	}
	cleared := setWorkInfo(t, db, ids["a"], domain.WorkInfo{Code: " ", URLs: []string{""}})
	if cleared.WorkInfo != nil || workInfoRowCount(t, db) != 0 {
		t.Errorf("消した後: WorkInfo %+v, 行 %d", cleared.WorkInfo, workInfoRowCount(t, db))
	}

	setWorkInfo(t, db, ids["a"], onlyDate("2024"))
	var infoRows int
	if err := db.sql.QueryRow(`select count(*) from video_work_info`).Scan(&infoRows); err != nil {
		t.Fatal(err)
	}
	if got, ok := workInfoOf(t, db, ids["a"]); !ok || infoRows != 0 || !got.Equal(onlyDate("2024")) {
		t.Errorf("日付だけ = %+v (ok %v), video_work_info の行 %d", got, ok, infoRows)
	}
}

func TestWorkInfoFollowsTheBundle(t *testing.T) {
	db, ids := versionFixture(t)
	setWorkInfo(t, db, ids["a"], onlyDate("2020"))
	setWorkInfo(t, db, ids["b"], domain.WorkInfo{Code: "B-1", URLs: []string{"https://example.com/b"}})

	bundle(t, db, ids["a"], ids["a"], ids["b"])
	for _, id := range []int64{ids["a"], ids["b"]} {
		if got, ok := workInfoOf(t, db, id); !ok || !got.Equal(onlyDate("2020")) {
			t.Errorf("束の動画 %d の作品情報 = %+v (ok %v), want 代表の日付だけ", id, got, ok)
		}
	}

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	setWorkInfo(t, db, ids["b"], onlyDate("2021"))
	want := []domain.Event{domain.VideoWorkInfoChanged{VideoIDs: []int64{ids["a"], ids["b"]}}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Errorf("束への書き込みの events = %v, want %v", recorder.events, want)
	}

	if _, err := db.Versions().Unbundle(context.Background(), ids["b"]); err != nil {
		t.Fatal(err)
	}
	if got, _ := workInfoOf(t, db, ids["b"]); !got.Equal(domain.WorkInfo{Code: "B-1", URLs: []string{"https://example.com/b"}}) {
		t.Errorf("外した B の作品情報 = %+v, want B の値", got)
	}
	if got, _ := workInfoOf(t, db, ids["a"]); !got.Equal(onlyDate("2021")) {
		t.Errorf("残った A の作品情報 = %+v, want 束の値", got)
	}
}

func TestWorkInfoSurvivesAMove(t *testing.T) {
	db, a := taggedVideoFixture(t)
	person := createPerson(t, db, "山田花子")
	info := domain.WorkInfo{Code: "ABC-1", People: []domain.WorkPerson{performer(person.ID)}}
	setWorkInfo(t, db, a, info)

	scan := startTestScan(t, db)
	moved := upsertOne(t, db, listingFile(fixturePath("/media/moved.mp4"), "moved", "key-a", 1))
	finishTestScan(t, db, scan, domain.ScanDone)

	if got, ok := workInfoOf(t, db, moved); !ok || !got.Equal(info) {
		t.Errorf("移った動画の作品情報 = %+v (ok %v)", got, ok)
	}
}

// Replace all six work-info tables together, removing destination fields absent from the source.
func TestSuccessionMovesWorkInfoAsOneObject(t *testing.T) {
	for name, existing := range map[string]*domain.WorkInfo{
		"新しい鍵に作品情報が無い":     nil,
		"新しい鍵に日付と URL がある": {Dates: []domain.WorkDate{{Kind: domain.DateRecorded, Date: "2019"}}, URLs: []string{"https://example.com/new"}},
	} {
		t.Run(name, func(t *testing.T) {
			db, a := taggedVideoFixture(t)
			person := createPerson(t, db, "山田花子")
			info := domain.WorkInfo{Code: "ABC-1", People: []domain.WorkPerson{performer(person.ID)}}
			setWorkInfo(t, db, a, info)

			scan := startTestScan(t, db)
			b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
			finishTestScan(t, db, scan, domain.ScanDone)
			if existing != nil {
				setWorkInfo(t, db, b, *existing)
			}
			probeDuration(t, db, b, 100_000)

			got, ok := workInfoOf(t, db, b)
			if !ok || !got.Equal(info) || len(got.People) != 1 || got.People[0].Person.Name != "山田花子" {
				t.Errorf("引き継いだ作品情報 = %+v (ok %v), want %+v", got, ok, info)
			}
			var oldRows int
			for _, table := range workInfoTables {
				var count int
				if err := db.sql.QueryRow(`select count(*) from ` + table + ` where content_key = 'key-a'`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				oldRows += count
			}
			if oldRows != 0 {
				t.Errorf("前の鍵に %d 行残った", oldRows)
			}
		})
	}
}

func TestSetManyWorkInfo(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	person := createPerson(t, db, "山田花子")

	out, err := db.WorkInfo().SetMany(ctx, []domain.WorkInfoChange{
		{Video: domain.VideoRef{ID: ids["a"]}, WorkInfo: domain.WorkInfo{Code: "first"}},
		{Video: domain.VideoRef{ID: ids["b"]}, WorkInfo: domain.WorkInfo{People: []domain.WorkPerson{performer(person.ID)}}},
		{Video: domain.VideoRef{ID: ids["a"]}, WorkInfo: domain.WorkInfo{Code: "second"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[0].WorkInfo == nil || out[0].WorkInfo.Code != "second" || out[1].WorkInfo == nil ||
		out[1].WorkInfo.People[0].Person.Name != "山田花子" {
		t.Errorf("SetMany = %+v", out)
	}
	want := []domain.Event{domain.VideoWorkInfoChanged{VideoIDs: []int64{ids["a"], ids["b"]}}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Errorf("events = %v, want %v", recorder.events, want)
	}

	recorder.events = nil
	before := workInfoRowCount(t, db)
	for name, c := range map[string]struct {
		changes []domain.WorkInfoChange
		index   int
		check   func(error) bool
	}{
		"規則に合わない": {[]domain.WorkInfoChange{
			{Video: domain.VideoRef{ID: ids["c"]}, WorkInfo: domain.WorkInfo{Code: "c"}},
			{Video: domain.VideoRef{ID: ids["d"]}, WorkInfo: onlyDate("2024-02-30")},
		}, 1, func(err error) bool { return errors.Is(err, domain.ErrInvalidWorkInfo) }},
		"無い人物": {[]domain.WorkInfoChange{
			{Video: domain.VideoRef{ID: ids["c"]}, WorkInfo: domain.WorkInfo{Code: "c"}},
			{Video: domain.VideoRef{ID: ids["d"]}, WorkInfo: domain.WorkInfo{People: []domain.WorkPerson{performer(person.ID + 100)}}},
		}, 1, func(err error) bool { return errors.Is(err, domain.ErrNotFound) }},
	} {
		_, err := db.WorkInfo().SetMany(ctx, c.changes)
		var at *domain.WorkInfoAtError
		if !errors.As(err, &at) || at.Index != c.index || !c.check(err) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	_, err = db.WorkInfo().SetMany(ctx, []domain.WorkInfoChange{
		{Video: domain.VideoRef{ID: ids["c"]}, WorkInfo: domain.WorkInfo{Code: "c"}},
		{Video: domain.VideoRef{ID: 9999}, WorkInfo: domain.WorkInfo{Code: "x"}},
	})
	var notFound *domain.VideoRefNotFoundError
	if !errors.As(err, &notFound) || notFound.Index != 1 {
		t.Errorf("無い動画: err = %v", err)
	}
	if workInfoRowCount(t, db) != before || len(recorder.events) != 0 {
		t.Errorf("誤りで書いた: 行 %d → %d, events %v", before, workInfoRowCount(t, db), recorder.events)
	}
}

func TestSetDetailsWritesBoth(t *testing.T) {
	db, ids := versionFixture(t)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	person := createPerson(t, db, "山田花子")

	start := time.Now()
	video, err := db.WorkInfo().SetDetails(context.Background(), ids["a"], " 新しい題名 ",
		domain.WorkInfo{Code: "ABC-1", People: []domain.WorkPerson{performer(person.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	if video.Title != "新しい題名" || video.DisplayName != "新しい題名" || video.WorkInfo == nil || video.WorkInfo.Code != "ABC-1" {
		t.Errorf("戻り値 = Title %q, DisplayName %q, WorkInfo %+v", video.Title, video.DisplayName, video.WorkInfo)
	}
	read := ownerVideo(t, db, ids["a"])
	if got, _ := workInfoOf(t, db, ids["a"]); read.DisplayName != "新しい題名" || got.Code != "ABC-1" {
		t.Errorf("読み返し = DisplayName %q, WorkInfo %+v", read.DisplayName, got)
	}
	if got := searchTitles(t, db, "新しい"); !slices.Equal(got, []string{"新しい題名"}) {
		t.Errorf("新しい表示名で検索 = %v", got)
	}
	assertEditedSince(t, db, ids["a"], start)
	if editRowCount(t, db) != 1 {
		t.Errorf("更新日時の行 = %d, want 1", editRowCount(t, db))
	}
	want := []domain.Event{
		domain.VideoOverrideChanged{VideoID: ids["a"]},
		domain.VideoWorkInfoChanged{VideoIDs: []int64{ids["a"]}},
	}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Errorf("events = %v, want %v", recorder.events, want)
	}
}

// Work info is shared by all bundle members; the name changes only on the target video.
func TestSetDetailsOnABundleMember(t *testing.T) {
	db, ids := versionFixture(t)
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	ctx := context.Background()

	start := time.Now()
	if _, err := db.WorkInfo().SetDetails(ctx, ids["b"], "", domain.WorkInfo{Code: "ABC-1"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{ids["a"], ids["b"]} {
		assertEditedSince(t, db, id, start)
	}

	resetEditedAt(t, db)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	if _, err := db.WorkInfo().SetDetails(ctx, ids["b"], "B の題名", domain.WorkInfo{Code: "ABC-1"}); err != nil {
		t.Fatal(err)
	}
	assertEditedAt(t, db, ids["a"], staleEditedAt)
	if editedAt(t, db, ids["b"]).Equal(staleEditedAt) {
		t.Error("表示名を変えたメンバーの更新日時が進まない")
	}
	want := []domain.Event{domain.VideoOverrideChanged{VideoID: ids["b"]}}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Errorf("events = %v, want %v", recorder.events, want)
	}
}

func TestSetDetailsWritesNothingOnErrorOrNoChange(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	person := createPerson(t, db, "山田花子")
	stored := domain.WorkInfo{Code: "ABC-1", People: []domain.WorkPerson{performer(person.ID)}}
	if _, err := db.WorkInfo().SetDetails(ctx, ids["a"], "題名", stored); err != nil {
		t.Fatal(err)
	}
	resetEditedAt(t, db)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	for name, c := range map[string]struct {
		name string
		info domain.WorkInfo
		want error
	}{
		"正しい表示名と不正な日付":   {"別の題名", onlyDate("2024-13"), domain.ErrInvalidWorkInfo},
		"上限を超える表示名と正しい値": {strings.Repeat("あ", domain.DisplayNameMaxLength+1), domain.WorkInfo{Code: "XYZ"}, domain.ErrInvalidDisplayName},
		"無い人物": {"別の題名", domain.WorkInfo{People: []domain.WorkPerson{performer(person.ID + 100)}}, domain.ErrNotFound},
	} {
		if _, err := db.WorkInfo().SetDetails(ctx, ids["a"], c.name, c.info); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	if _, err := db.WorkInfo().SetDetails(ctx, ids["a"], " 題名 ", domain.WorkInfo{Code: "ABC-1 ", People: stored.People}); err != nil {
		t.Fatal(err)
	}
	if got, _ := workInfoOf(t, db, ids["a"]); ownerVideo(t, db, ids["a"]).DisplayName != "題名" || !got.Equal(stored) {
		t.Errorf("保存済みの値が変わった: DisplayName %q, WorkInfo %+v", ownerVideo(t, db, ids["a"]).DisplayName, got)
	}
	assertEditedAt(t, db, ids["a"], staleEditedAt)
	if len(recorder.events) != 0 {
		t.Errorf("events = %v", recorder.events)
	}
}
