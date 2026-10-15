package store

import (
	"context"
	"reflect"
	"runtime"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func candidateIDs(candidates []domain.RelatedCandidate) []int64 {
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.VideoID)
	}
	slices.Sort(ids)
	return ids
}

func candidateOf(t *testing.T, candidates []domain.RelatedCandidate, id int64) domain.RelatedCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.VideoID == id {
			return candidate
		}
	}
	t.Fatalf("候補に %d が無い: %+v", id, candidates)
	return domain.RelatedCandidate{}
}

// 見せる動画すべての手がかりを返す。代表の所在のパス、表示名を優先した題名、追加日時、
// 手で付けた（確定・仮）タグとフォルダ名から付いたタグ、指紋を持つ。登録フォルダの下に
// 所在の無い動画は含めない。
func TestRelatedCandidates(t *testing.T) {
	db, ids := folderFixture(t,
		sampleFile(fixturePath("/media/cats/a.mp4"), "a", "key-a", 1, 0),
		sampleFile(fixturePath("/media/cats/b.mp4"), "b", "key-b", 1, 0),
		sampleFile(fixturePath("/media/other/c.mp4"), "c", "key-c", 1, 0),
		sampleFile(fixturePath("/outside/d.mp4"), "d", "key-d", 1, 0),
	)
	ctx := context.Background()
	a, b, c := ids[fixturePath("/media/cats/a.mp4")], ids[fixturePath("/media/cats/b.mp4")], ids[fixturePath("/media/other/c.mp4")]

	if _, err := db.Tags().ApplyVideoTags(ctx, []domain.VideoRef{{ID: a}}, domain.VideoTagsAdd, []string{"夜景"}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().ApplyVideoTags(ctx, []domain.VideoRef{{ID: a}, {ID: c}}, domain.VideoTagsAdd, []string{"候補"}, true); err != nil {
		t.Fatal(err)
	}
	// フォルダ名 cats と同じ名前のタグは、cats の下の動画に付いているものとして数える。
	cats, err := db.Tags().CreateTag(ctx, "cats")
	if err != nil {
		t.Fatal(err)
	}
	rebuildIndexForTest(t, db)
	if _, err := db.Overrides().SetDisplayName(ctx, b, "表示名"); err != nil {
		t.Fatal(err)
	}
	fingerprint := domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: 1000,
		Frames: []domain.FrameHash{{Hash: 1}, {Hash: 2}, {Hash: 3, Flat: true}}}
	if _, err := db.sql.Exec(`insert into video_fingerprints (content_key, version, interval_ms, hashes, updated_at)
		values ('key-c', ?, ?, ?, 1)`, fingerprint.Version, fingerprint.IntervalMs, fingerprint.Encode()); err != nil {
		t.Fatal(err)
	}

	candidates, err := db.Library().RelatedCandidates(ctx, domain.AudienceOwner, a)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := candidateIDs(candidates), []int64{a, b, c}; !slices.Equal(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	gotA := candidateOf(t, candidates, a)
	if gotA.Path != fixturePath("/media/cats/a.mp4") || gotA.Title != "a" || gotA.AddedAt.IsZero() {
		t.Errorf("a = %+v", gotA)
	}
	if len(gotA.TagIDs) != 3 || !slices.Contains(gotA.TagIDs, cats.ID) {
		t.Errorf("a のタグ = %v, want 確定・仮・フォルダ名の 3 つ（cats=%d）", gotA.TagIDs, cats.ID)
	}
	gotB := candidateOf(t, candidates, b)
	if gotB.Title != "表示名" || !slices.Equal(gotB.TagIDs, []int64{cats.ID}) || gotB.Fingerprint != nil {
		t.Errorf("b = %+v", gotB)
	}
	gotC := candidateOf(t, candidates, c)
	if len(gotC.TagIDs) != 1 || gotC.Fingerprint == nil || !reflect.DeepEqual(*gotC.Fingerprint, fingerprint) {
		t.Errorf("c = %+v", gotC)
	}
}

// 本体は指定の順で返し、登録フォルダの下に所在が無い動画は抜ける。
func TestVideosByIDsKeepsOrder(t *testing.T) {
	db, ids := folderFixture(t,
		sampleFile(fixturePath("/media/a.mp4"), "a", "key-a", 1, 0),
		sampleFile(fixturePath("/media/b.mp4"), "b", "key-b", 1, 0),
		sampleFile(fixturePath("/outside/c.mp4"), "c", "key-c", 1, 0),
	)
	videos, err := db.Library().VideosByIDs(context.Background(), domain.AudienceOwner, []int64{ids[fixturePath("/media/b.mp4")], ids[fixturePath("/outside/c.mp4")], ids[fixturePath("/media/a.mp4")]})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, video := range videos {
		titles = append(titles, video.Title)
	}
	if want := []string{"b", "a"}; !slices.Equal(titles, want) {
		t.Fatalf("titles = %v, want %v", titles, want)
	}
	empty, err := db.Library().VideosByIDs(context.Background(), domain.AudienceOwner, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty = %v, %v", empty, err)
	}
}

// 登録フォルダ `/` の直下の自動グループは origin_folder が空文字になる。それを手で作ったグループ
// （origin_folder が NULL）と取り違えず、動画のグループにフォルダ（登録フォルダそのもの）を載せる。
func TestVideoGroupLocatesRootFolderGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("登録フォルダ `/` は Unix だけのもの")
	}
	db, ids := folderFixture(t,
		sampleFile("/a.mp4", "a", "key-a", 1, 0),
		sampleFile("/b.mp4", "b", "key-b", 1, 0),
	)
	ctx := context.Background()
	result, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values ('/', 1, 0, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	auto := insertGroup(t, db, "root", "key-a", "key-b")
	if _, err := db.sql.Exec(`update groups set origin_folder = ? where id = ?`, domain.FolderKey("/"), auto); err != nil {
		t.Fatal(err)
	}
	group, found, err := db.Library().VideoGroup(ctx, domain.AudienceOwner, ids["/a.mp4"], domain.VideoContext{}, domain.GroupWindow{})
	if err != nil || !found {
		t.Fatalf("VideoGroup = %v, %v", found, err)
	}
	if group.ID != auto || group.Folder == nil || *group.Folder != (domain.VideoFolder{RootID: rootID}) {
		t.Errorf("group = id %d, folder %+v; want id %d, folder {RootID:%d Path:}", group.ID, group.Folder, auto, rootID)
	}

	// 手で作ったグループにはフォルダを載せない。
	if _, err := db.sql.Exec(`update groups set origin_folder = null where id = ?`, auto); err != nil {
		t.Fatal(err)
	}
	group, found, err = db.Library().VideoGroup(ctx, domain.AudienceOwner, ids["/a.mp4"], domain.VideoContext{}, domain.GroupWindow{})
	if err != nil || !found || group.Folder != nil {
		t.Errorf("手で作ったグループ = found %v, folder %+v, err %v; want found, no folder", found, group.Folder, err)
	}
}
