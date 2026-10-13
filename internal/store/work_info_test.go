package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 動画の作品情報（specs/047-video-work-info/data-model.md「Writes」「Bundles and succession」）。

// sampleWorkInfo は出演者を 3 人持つ作品情報である。
func sampleWorkInfo(code string, performers ...string) domain.WorkInfo {
	return domain.WorkInfo{
		Code:        code,
		ReleaseDate: "2024-05-01",
		Maker:       "メーカー",
		Performers:  performers,
		Description: "1 行目\n2 行目",
		SourceURL:   "https://example.com/works",
	}
}

func setWorkInfo(t *testing.T, db *DB, id int64, info domain.WorkInfo) domain.Video {
	t.Helper()
	video, err := db.WorkInfo().Set(context.Background(), id, info)
	if err != nil {
		t.Fatalf("動画 %d の作品情報を設定できない: %v", id, err)
	}
	return video
}

// workInfoOf は動画の利用者データの鍵の作品情報を読む。無ければ nil。
func workInfoOf(t *testing.T, db *DB, id int64) *domain.WorkInfo {
	t.Helper()
	key := ownerVideo(t, db, id).UserKey
	infos, err := db.WorkInfo().ByUserKeys(context.Background(), []string{key})
	if err != nil {
		t.Fatal(err)
	}
	info, ok := infos[key]
	if !ok {
		return nil
	}
	return &info
}

func assertWorkInfo(t *testing.T, db *DB, id int64, want domain.WorkInfo) {
	t.Helper()
	got := workInfoOf(t, db, id)
	if got == nil || !got.Equal(want) {
		t.Errorf("動画 %d の作品情報 = %+v, want %+v", id, got, want)
	}
}

func workInfoRowCounts(t *testing.T, db *DB) (rows, performers int) {
	t.Helper()
	if err := db.sql.QueryRow(`select (select count(*) from video_work_info), (select count(*) from video_work_performers)`).
		Scan(&rows, &performers); err != nil {
		t.Fatal(err)
	}
	return rows, performers
}

// 設定した値は整えられ、出演者の順番どおりに読み戻せる。返す動画にも作品情報が載る。
func TestSetWorkInfoReadsBackInOrder(t *testing.T) {
	db, ids := versionFixture(t)
	input := sampleWorkInfo(" ABC-1 ", "丙", " 甲 ", "乙", "甲")
	video := setWorkInfo(t, db, ids["a"], input)

	want := sampleWorkInfo("ABC-1", "丙", "甲", "乙")
	if video.WorkInfo == nil || !video.WorkInfo.Equal(want) {
		t.Errorf("返した動画の作品情報 = %+v, want %+v", video.WorkInfo, want)
	}
	assertWorkInfo(t, db, ids["a"], want)
	if got := workInfoOf(t, db, ids["b"]); got != nil {
		t.Errorf("設定していない動画の作品情報 = %+v", got)
	}
	if _, err := db.WorkInfo().Set(context.Background(), 9999, want); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い動画 = %v, want ErrNotFound", err)
	}
	if _, err := db.WorkInfo().Set(context.Background(), ids["a"], domain.WorkInfo{Code: "a\tb"}); !errors.Is(err, domain.ErrInvalidWorkInfo) {
		t.Errorf("制御文字 = %v, want ErrInvalidWorkInfo", err)
	}
	assertWorkInfo(t, db, ids["a"], want)
}

// 同じ値でもう一度設定しても updatedAt と更新日時は変わらず、イベントも出ない。値が変われば
// どちらも進み、確定後にイベントが 1 回出る。
func TestSetWorkInfoSameValueChangesNothing(t *testing.T) {
	db, ids := versionFixture(t)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	info := sampleWorkInfo("ABC-1", "甲", "乙")
	setWorkInfo(t, db, ids["a"], info)
	if want := []domain.Event{domain.VideoWorkInfoChanged{VideoIDs: []int64{ids["a"]}}}; !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("events = %v, want %v", recorder.events, want)
	}

	recorder.events = nil
	resetEditedAt(t, db)
	if _, err := db.sql.Exec(`update video_work_info set updated_at = 1`); err != nil {
		t.Fatal(err)
	}
	setWorkInfo(t, db, ids["a"], sampleWorkInfo(" ABC-1 ", "甲", "乙", " 乙 "))
	var updatedAt int64
	if err := db.sql.QueryRow(`select updated_at from video_work_info`).Scan(&updatedAt); err != nil {
		t.Fatal(err)
	}
	if updatedAt != 1 {
		t.Errorf("同じ値で updated_at = %d, want 1", updatedAt)
	}
	assertEditedAt(t, db, ids["a"], staleEditedAt)
	if len(recorder.events) != 0 {
		t.Errorf("同じ値でイベントを出した: %v", recorder.events)
	}

	setWorkInfo(t, db, ids["a"], sampleWorkInfo("ABC-1", "乙", "甲"))
	if err := db.sql.QueryRow(`select updated_at from video_work_info`).Scan(&updatedAt); err != nil {
		t.Fatal(err)
	}
	if updatedAt == 1 {
		t.Error("出演者の順番を変えても updated_at が進まない")
	}
	if editedAt(t, db, ids["a"]).Equal(staleEditedAt) {
		t.Error("出演者の順番を変えても更新日時が進まない")
	}
	if len(recorder.events) != 1 {
		t.Errorf("値を変えたイベント = %v, want 1 件", recorder.events)
	}
}

// すべての項目を消すと、行と出演者が消え、動画の作品情報は nil になる。
func TestSetWorkInfoClearingRemovesRows(t *testing.T) {
	db, ids := versionFixture(t)
	setWorkInfo(t, db, ids["a"], sampleWorkInfo("ABC-1", "甲", "乙"))
	video := setWorkInfo(t, db, ids["a"], domain.WorkInfo{Code: " ", Performers: []string{"", " "}})
	if video.WorkInfo != nil {
		t.Errorf("消した後の作品情報 = %+v, want nil", video.WorkInfo)
	}
	if rows, performers := workInfoRowCounts(t, db); rows != 0 || performers != 0 {
		t.Errorf("消した後の行 = %d、出演者 = %d, want 0", rows, performers)
	}
}

// 作品情報は利用者データなので、内容が別のパスで索引し直されても読める。
func TestWorkInfoSurvivesMove(t *testing.T) {
	db, ids := versionFixture(t)
	info := sampleWorkInfo("ABC-1", "甲", "乙")
	setWorkInfo(t, db, ids["a"], info)

	moved := fixturePath("/media/moved/renamed.mp4")
	upsertAll(t, db, listingFile(moved, "renamed", "key-a", 9))
	var locationID int64
	if err := db.sql.QueryRow(`select id from video_locations where path = ?`, fixturePath("/media/a.mp4")).Scan(&locationID); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().DeleteVideoLocations(context.Background(), []int64{locationID}); err != nil {
		t.Fatal(err)
	}
	if got := ownerVideo(t, db, ids["a"]).Path; got != moved {
		t.Fatalf("移動後の所在 = %q, want %q", got, moved)
	}
	assertWorkInfo(t, db, ids["a"], info)
}

// 同じパスでの中身の引き継ぎで、作品情報は出演者ごと新しい鍵へ移る。新しい鍵に作品情報が
// 既にあれば、前の鍵の値で置き換わる。
func TestWorkInfoCarriedBySuccession(t *testing.T) {
	for name, existing := range map[string]*domain.WorkInfo{
		"新しい鍵に作品情報が無い":   nil,
		"新しい鍵に作品情報が既にある": {Code: "NEW-1", Performers: []string{"丁", "戊", "己", "庚"}},
	} {
		t.Run(name, func(t *testing.T) {
			db, a := taggedVideoFixture(t)
			info := sampleWorkInfo("ABC-1", "甲", "乙")
			setWorkInfo(t, db, a, info)

			scan := startTestScan(t, db)
			b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
			if existing != nil {
				setWorkInfo(t, db, b, *existing)
			}
			finishTestScan(t, db, scan, domain.ScanDone)
			probeDuration(t, db, b, 100_000)

			if key := ownerVideo(t, db, b).UserKey; key != "key-b" {
				t.Fatalf("新しい動画の鍵 = %q", key)
			}
			assertWorkInfo(t, db, b, info)
			if rows, performers := workInfoRowCounts(t, db); rows != 1 || performers != 2 {
				t.Errorf("引き継ぎ後の行 = %d、出演者 = %d, want 1 と 2", rows, performers)
			}
		})
	}
}

// 2 本を束ねると集まりには代表の値が出て、束から外すとメンバー自身の値が戻る。
func TestWorkInfoFollowsBundle(t *testing.T) {
	db, ids := versionFixture(t)
	infoA := sampleWorkInfo("A-1", "甲", "乙")
	infoB := sampleWorkInfo("B-1", "丙")
	setWorkInfo(t, db, ids["a"], infoA)
	setWorkInfo(t, db, ids["b"], infoB)

	bundle(t, db, ids["a"], ids["a"], ids["b"])
	assertWorkInfo(t, db, ids["a"], infoA)
	assertWorkInfo(t, db, ids["b"], infoA)

	// 集まりに書いた値は集まりの鍵に入り、メンバー自身の値は変えない。
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	bundled := sampleWorkInfo("BUNDLE-1", "丁")
	setWorkInfo(t, db, ids["b"], bundled)
	assertWorkInfo(t, db, ids["a"], bundled)
	if want := []domain.Event{domain.VideoWorkInfoChanged{VideoIDs: []int64{ids["a"], ids["b"]}}}; !reflect.DeepEqual(recorder.events, want) {
		t.Errorf("集まりへの書き込みのイベント = %v, want %v", recorder.events, want)
	}

	b, err := db.Versions().Unbundle(context.Background(), ids["b"])
	if err != nil {
		t.Fatal(err)
	}
	if b.UserKey != "key-b" {
		t.Fatalf("外した B の鍵 = %q", b.UserKey)
	}
	assertWorkInfo(t, db, ids["b"], infoB)
	// 残りが 1 本になって集まりが解けたので、A には集まりの値が写る。
	assertWorkInfo(t, db, ids["a"], bundled)
}

// 一括の置き換えは 1 つの取引で、規則に合わない値か引けない動画が 1 件でもあれば位置を返して
// 何も残さない。成功すれば指定の順に動画を返し、同じ動画は後の指定が勝つ。
func TestSetManyWorkInfoIsAtomic(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	refA := domain.VideoRef{ID: ids["a"]}
	refB := domain.VideoRef{ID: ids["b"]}

	_, err := db.WorkInfo().SetMany(ctx, []domain.WorkInfoChange{
		{Video: refA, WorkInfo: sampleWorkInfo("A-1")},
		{Video: refB, WorkInfo: domain.WorkInfo{SourceURL: "nope"}},
	})
	var at *domain.WorkInfoAtError
	if !errors.As(err, &at) || at.Index != 1 || !errors.Is(err, domain.ErrInvalidWorkInfo) {
		t.Fatalf("規則に合わない値 = %v, want 位置 1 の WorkInfoAtError", err)
	}
	_, err = db.WorkInfo().SetMany(ctx, []domain.WorkInfoChange{
		{Video: refA, WorkInfo: sampleWorkInfo("A-1")},
		{Video: domain.VideoRef{ID: 9999}, WorkInfo: sampleWorkInfo("X")},
	})
	var notFound *domain.VideoRefNotFoundError
	if !errors.As(err, &notFound) || notFound.Index != 1 {
		t.Fatalf("引けない動画 = %v, want 位置 1 の VideoRefNotFoundError", err)
	}
	if rows, _ := workInfoRowCounts(t, db); rows != 0 {
		t.Fatalf("失敗した一括が %d 行を残した", rows)
	}

	videos, err := db.WorkInfo().SetMany(ctx, []domain.WorkInfoChange{
		{Video: refA, WorkInfo: sampleWorkInfo("A-1", "甲")},
		{Video: refB, WorkInfo: sampleWorkInfo("B-1")},
		{Video: refA, WorkInfo: sampleWorkInfo("A-2", "乙")},
	})
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := []int64{}
	for _, video := range videos {
		gotIDs = append(gotIDs, video.ID)
		if video.WorkInfo == nil {
			t.Errorf("動画 %d の作品情報が載っていない", video.ID)
		}
	}
	if want := []int64{ids["a"], ids["b"], ids["a"]}; !slices.Equal(gotIDs, want) {
		t.Errorf("返した動画 = %v, want %v", gotIDs, want)
	}
	assertWorkInfo(t, db, ids["a"], sampleWorkInfo("A-2", "乙"))
	assertWorkInfo(t, db, ids["b"], sampleWorkInfo("B-1"))
}

// 古い版の行の照合用の鍵は、起動時の作り直しで今の規則の値になる。
func TestRefreshWorkInfoSearchKeys(t *testing.T) {
	db, ids := versionFixture(t)
	setWorkInfo(t, db, ids["a"], domain.WorkInfo{Code: "ＡＢＣ-1", Maker: "メーカー", Performers: []string{"あい", "Ｂ"}})
	if _, err := db.sql.Exec(`update video_work_info set maker_key = 'x', search_key = 'x', search_version = 0`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`update video_work_performers set name_key = 'x'`); err != nil {
		t.Fatal(err)
	}
	refreshed, err := db.WorkInfo().RefreshSearchKeys(context.Background())
	if err != nil || refreshed != 1 {
		t.Fatalf("RefreshSearchKeys = %d, %v; want 1", refreshed, err)
	}
	var makerKey, searchKey string
	if err := db.sql.QueryRow(`select maker_key, search_key from video_work_info`).Scan(&makerKey, &searchKey); err != nil {
		t.Fatal(err)
	}
	if makerKey != "メーカー" || searchKey != "abc-1\nメーカー\nアイ\nb" {
		t.Errorf("maker_key = %q, search_key = %q", makerKey, searchKey)
	}
	var names string
	if err := db.sql.QueryRow(`select group_concat(name_key, ',' order by position) from video_work_performers`).
		Scan(&names); err != nil {
		t.Fatal(err)
	}
	if names != "アイ,b" {
		t.Errorf("name_key = %q", names)
	}
	if again, err := db.WorkInfo().RefreshSearchKeys(context.Background()); err != nil || again != 0 {
		t.Errorf("2 度目の RefreshSearchKeys = %d, %v; want 0", again, err)
	}
}
