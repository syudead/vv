package store

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 一覧の項目にも集まりの要約（Video.Versions）を載せる。

// checkVersionsRefs は videos のうち representative に本数 count・代表 representative の要約があり、
// ほかの動画には要約が無いことを確かめる。
func checkVersionsRefs(t *testing.T, where string, videos []domain.Video, representative int64, count int) {
	t.Helper()
	found := false
	for _, video := range videos {
		if video.ID != representative {
			if video.Versions != nil {
				t.Errorf("%s: 束ねていない動画 %d に要約がある: %+v", where, video.ID, *video.Versions)
			}
			continue
		}
		found = true
		want := domain.VideoVersionsRef{Count: count, RepresentativeID: representative}
		if video.Versions == nil || *video.Versions != want {
			t.Errorf("%s: 代表 %d の要約 = %+v, want %+v", where, video.ID, video.Versions, want)
		}
	}
	if !found {
		t.Errorf("%s: 代表 %d が無い", where, representative)
	}
}

// A と B を A を代表に束ねると、ライブラリ・動画一覧・フォルダの一覧で A に versions.count 2 が入り、
// 束ねていない C・D には入らない。ゲストにも同じ規則で入る。
func TestListsCarryTheVersionsSummary(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	bundle(t, db, ids["a"], ids["a"], ids["b"])

	for _, audience := range []domain.Audience{domain.AudienceOwner, domain.AudienceGuest} {
		if audience == domain.AudienceGuest {
			if _, err := db.Visibility().SetVideosPublic(ctx, []int64{ids["a"], ids["c"]}, true); err != nil {
				t.Fatal(err)
			}
		}
		library, err := db.Library().ListLibrary(ctx, audience, domain.VideoQuery{Limit: domain.MaxLimit})
		if err != nil {
			t.Fatal(err)
		}
		var items []domain.Video
		for _, item := range library.Items {
			if item.Video != nil {
				items = append(items, *item.Video)
			}
		}
		checkVersionsRefs(t, "ListLibrary", items, ids["a"], 2)
		checkVersionsRefs(t, "ListVideos", listedPage(t, db, audience, domain.VideoQuery{}).Items, ids["a"], 2)
		folder, err := db.Library().ListFolderVideos(ctx, audience, domain.FolderVideoQuery{Dir: fixturePath("/media"), Limit: domain.MaxLimit})
		if err != nil {
			t.Fatal(err)
		}
		checkVersionsRefs(t, "ListFolderVideos", folder.Items, ids["a"], 2)
	}
}

// 束ねた代表が自動グループの項目なら、グループのページの項目にも、グループのメンバー（再生の
// 順番やプレイリストの再生の手順が選ぶ動画）にも要約が入る。
func TestGroupItemsCarryTheVersionsSummary(t *testing.T) {
	db, ids := folderFixture(t,
		sampleFile(fixturePath("/media/A/a.mp4"), "a", "key-a", 1, 0),
		sampleFile(fixturePath("/media/A/c.mp4"), "c", "key-c", 1, 0),
		sampleFile(fixturePath("/media/B/b.mp4"), "b", "key-b", 1, 0),
	)
	ctx := context.Background()
	a, b := ids[fixturePath("/media/A/a.mp4")], ids[fixturePath("/media/B/b.mp4")]
	rebuildIndexForTest(t, db)
	bundle(t, db, a, a, b)

	var groupID int64
	if err := db.sql.QueryRow(`select group_id from group_items where content_key = ?`, ownerVideo(t, db, a).UserKey).Scan(&groupID); err != nil {
		t.Fatalf("A の入ったグループが無い: %v", err)
	}
	page, err := db.Library().Group(ctx, domain.AudienceOwner, groupID, nil)
	if err != nil {
		t.Fatal(err)
	}
	var videos []domain.Video
	for _, item := range page.Items {
		if item.Video != nil {
			videos = append(videos, *item.Video)
		}
	}
	checkVersionsRefs(t, "Group", videos, a, 2)
	checkVersionsRefs(t, "Group.Members", page.Group.Members, a, 2)
}
