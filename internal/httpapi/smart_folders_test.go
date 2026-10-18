package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func smartFolderPath(id int64, suffix string) string {
	return "/api/smart-folders/" + strconv.FormatInt(id, 10) + suffix
}

func (f *libraryFixture) createSmartFolder(t *testing.T, body map[string]any) gen.SmartFolderSummary {
	t.Helper()
	rec := f.groupRequest(http.MethodPost, "/api/smart-folders", groupJSON(body), f.owner)
	if rec.Code != http.StatusCreated {
		t.Fatalf("作成 %v: status = %d: %s", body, rec.Code, rec.Body)
	}
	return decode[gen.SmartFolderSummary](t, rec)
}

func (f *libraryFixture) smartFolderTag(t *testing.T, name string, videos ...string) int64 {
	t.Helper()
	ctx := context.Background()
	tag, err := f.env.db.Tags().CreateTag(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(videos))
	for _, video := range videos {
		ids = append(ids, f.ids[video])
	}
	if _, _, err := f.env.db.Tags().AttachTagByID(ctx, ids, tag.ID); err != nil {
		t.Fatal(err)
	}
	return tag.ID
}

var (
	wantInvalidSmartRule   = wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidSmartRule}
	wantSmartFolderMissing = wantError{status: http.StatusNotFound, code: gen.ErrorCodeSmartFolderNotFound}
	wantSmartFolderCycle   = wantError{status: http.StatusConflict, code: gen.ErrorCodeSmartFolderCycle}
)

func TestCreateSmartFolderStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	parent := f.createSmartFolder(t, map[string]any{"name": "  猫  ", "rule": map[string]any{"tags": []int64{f.manual}}})
	if parent.Name != "猫" || parent.ParentId != nil || parent.Favorite != nil {
		t.Errorf("作成の応答 = %+v", parent)
	}
	child := f.createSmartFolder(t, map[string]any{
		"name": "未視聴", "rule": map[string]any{"watch": "unwatched"}, "sort": "titleAsc", "parentId": parent.Id,
	})
	if child.ParentId == nil || *child.ParentId != parent.Id {
		t.Errorf("子の応答 = %+v", child)
	}

	for _, tc := range []struct {
		label string
		body  map[string]any
		want  wantError
	}{
		{"範囲の外の addedWithin", map[string]any{"name": "a", "rule": map[string]any{"addedWithin": 0}}, wantInvalidSmartRule},
		{"知らない項目", map[string]any{"name": "a", "rule": map[string]any{"color": "red"}}, wantInvalidSmartRule},
		{"null の項目", map[string]any{"name": "a", "rule": map[string]any{"q": nil}}, wantInvalidSmartRule},
		{"条件が配列", map[string]any{"name": "a", "rule": []int{}}, wantInvalidSmartRule},
		{"random の並べ順", map[string]any{"name": "a", "rule": map[string]any{}, "sort": "random"}, wantInvalidSmartRule},
		{"知らない並べ順", map[string]any{"name": "a", "rule": map[string]any{}, "sort": "sideways"}, wantInvalidSmartRule},
		{"空の名前", map[string]any{"name": "   ", "rule": map[string]any{}}, wantGroupInvalid},
		{"101 文字", map[string]any{"name": strings.Repeat("あ", 101), "rule": map[string]any{}}, wantGroupInvalid},
		{"条件なし", map[string]any{"name": "a"}, wantGroupInvalid},
		{"名前なし", map[string]any{"rule": map[string]any{}}, wantGroupInvalid},
		{"無い親", map[string]any{"name": "a", "rule": map[string]any{}, "parentId": 999999}, wantSmartFolderMissing},
		{"null の並べ順", map[string]any{"name": "a", "rule": map[string]any{}, "sort": nil}, wantGroupInvalid},
		{"null の親", map[string]any{"name": "a", "rule": map[string]any{}, "parentId": nil}, wantGroupInvalid},
	} {
		rec := f.groupRequest(http.MethodPost, "/api/smart-folders", groupJSON(tc.body), f.owner)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), tc.want)
	}
	rec := f.groupRequest(http.MethodPost, "/api/smart-folders", `{"name": "a", "rule": {}`, f.owner)
	assertErrorBody(t, "壊れた JSON", rec.Code, rec.Body.Bytes(), wantGroupInvalid)
	list := decode[gen.SmartFolderList](t, f.env.get("/api/smart-folders", f.owner))
	if len(list.SmartFolders) != 2 {
		t.Errorf("失敗した作成のあとの一覧 = %+v", list.SmartFolders)
	}
}

func TestChangeAndDeleteSmartFolderStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	parent := f.createSmartFolder(t, map[string]any{"name": "親", "rule": map[string]any{}})
	child := f.createSmartFolder(t, map[string]any{"name": "子", "rule": map[string]any{}, "parentId": parent.Id})

	rec := f.groupRequest(http.MethodPatch, smartFolderPath(child.Id, ""), groupJSON(map[string]any{"name": " 子2 "}), f.owner)
	if got := decode[gen.SmartFolderSummary](t, rec); rec.Code != http.StatusOK || got.Name != "子2" || got.ParentId == nil {
		t.Errorf("名前の変更 = %d %+v", rec.Code, got)
	}
	for _, tc := range []struct {
		label string
		id    int64
		body  string
		want  wantError
	}{
		{"自分の子の下", parent.Id, fmt.Sprintf(`{"parentId": %d}`, child.Id), wantSmartFolderCycle},
		{"自分の下", parent.Id, fmt.Sprintf(`{"parentId": %d}`, parent.Id), wantSmartFolderCycle},
		{"無い親", child.Id, `{"parentId": 999999}`, wantSmartFolderMissing},
		{"整数でない親", child.Id, `{"parentId": "x"}`, wantGroupInvalid},
		{"空の本文", child.Id, `{}`, wantGroupInvalid},
		{"範囲の外の条件", child.Id, `{"rule": {"minDuration": 10, "maxDuration": 5}}`, wantInvalidSmartRule},
		{"random の並べ順", child.Id, `{"sort": "random"}`, wantInvalidSmartRule},
		{"空の名前", child.Id, `{"name": " "}`, wantGroupInvalid},
		{"無いフォルダ", 999999, `{"name": "x"}`, wantGroupNotFound},
		{"null の名前と親の移動", child.Id, `{"name": null, "parentId": null}`, wantGroupInvalid},
		{"null の並べ順と親の移動", child.Id, `{"sort": null, "parentId": null}`, wantGroupInvalid},
		{"壊れた JSON", child.Id, `{"name": "x"`, wantGroupInvalid},
	} {
		rec := f.groupRequest(http.MethodPatch, smartFolderPath(tc.id, ""), tc.body, f.owner)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), tc.want)
	}
	for _, folder := range decode[gen.SmartFolderList](t, f.env.get("/api/smart-folders", f.owner)).SmartFolders {
		if folder.Id == child.Id && (folder.ParentId == nil || *folder.ParentId != parent.Id) {
			t.Errorf("失敗した変更のあとの子 = %+v", folder)
		}
	}

	rec = f.groupRequest(http.MethodPatch, smartFolderPath(child.Id, ""), `{"parentId": null}`, f.owner)
	if got := decode[gen.SmartFolderSummary](t, rec); rec.Code != http.StatusOK || got.ParentId != nil {
		t.Errorf("一番上へ = %d %+v", rec.Code, got)
	}
	rec = f.groupRequest(http.MethodPatch, smartFolderPath(child.Id, ""), fmt.Sprintf(`{"parentId": %d}`, parent.Id), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("戻す: %d %s", rec.Code, rec.Body)
	}

	if rec := f.groupRequest(http.MethodDelete, smartFolderPath(parent.Id, ""), "", f.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("削除: %d %s", rec.Code, rec.Body)
	}
	for _, id := range []int64{parent.Id, child.Id} {
		rec := f.env.get(smartFolderPath(id, ""), f.owner)
		assertErrorBody(t, "消したあとの GET", rec.Code, rec.Body.Bytes(), wantGroupNotFound)
	}
	rec = f.groupRequest(http.MethodDelete, smartFolderPath(parent.Id, ""), "", f.owner)
	assertErrorBody(t, "2 回目の削除", rec.Code, rec.Body.Bytes(), wantGroupNotFound)
}

func TestSmartFolderGuestAccess(t *testing.T) {
	f := newLibraryFixture(t)
	shown := f.smartFolderTag(t, "公開", "solo", "ep10")
	hidden := f.smartFolderTag(t, "非公開", "ep10")
	open := f.createSmartFolder(t, map[string]any{"name": "a 公開", "rule": map[string]any{"tags": []int64{shown}}})
	closed := f.createSmartFolder(t, map[string]any{"name": "b 非公開", "rule": map[string]any{"tags": []int64{hidden}}})

	for _, tc := range []struct{ method, target, body string }{
		{http.MethodPost, "/api/smart-folders", `{"name": "x", "rule": {}}`},
		{http.MethodPatch, smartFolderPath(open.Id, ""), `{"name": "x"}`},
		{http.MethodDelete, smartFolderPath(open.Id, ""), ""},
	} {
		assertUnauthenticated(t, "ゲストの "+tc.method, f.groupRequest(tc.method, tc.target, tc.body))
	}

	list := decode[gen.SmartFolderList](t, f.env.get("/api/smart-folders"))
	if len(list.SmartFolders) != 1 || list.SmartFolders[0].Id != open.Id || list.SmartFolders[0].Favorite != nil {
		t.Errorf("ゲストの一覧 = %+v", list.SmartFolders)
	}
	rec := f.env.get(smartFolderPath(open.Id, ""))
	detail := decode[gen.SmartFolderDetail](t, rec)
	card := detail.SmartFolder
	if rec.Code != http.StatusOK || detail.Settings != nil || card.Favorite != nil || card.WatchState != nil ||
		card.WatchedCount != nil || len(card.Tags) != 0 || card.VideoCount != 1 || len(detail.Items) != 1 {
		t.Errorf("ゲストの詳細 = %d %+v", rec.Code, detail)
	}
	rec = f.env.get(smartFolderPath(closed.Id, ""))
	assertErrorBody(t, "ゲストに見せないフォルダ", rec.Code, rec.Body.Bytes(), wantGroupNotFound)

	owner := decode[gen.SmartFolderDetail](t, f.env.get(smartFolderPath(open.Id, ""), f.owner))
	if owner.Settings == nil || owner.SmartFolder.Favorite == nil || owner.SmartFolder.WatchState == nil ||
		owner.SmartFolder.VideoCount != 2 {
		t.Errorf("所有者の詳細 = %+v", owner)
	}
}

func TestSmartFolderSettingsNameAMissingTag(t *testing.T) {
	f := newLibraryFixture(t)
	gone := f.smartFolderTag(t, "消す", "solo")
	folder := f.createSmartFolder(t, map[string]any{
		"name": "消す", "rule": map[string]any{"tags": []int64{gone}, "watch": "unwatched"}, "sort": "titleAsc",
	})
	child := f.createSmartFolder(t, map[string]any{"name": "子", "rule": map[string]any{}, "parentId": folder.Id})
	if err := f.env.db.Tags().DeleteTag(context.Background(), gone); err != nil {
		t.Fatal(err)
	}
	detail := decode[gen.SmartFolderDetail](t, f.env.get(smartFolderPath(folder.Id, ""), f.owner))
	settings := detail.Settings
	if settings == nil || settings.Sort != gen.VideoSortTitleAsc || settings.Rule.Tags == nil ||
		!slices.Equal(*settings.Rule.Tags, []int64{gone}) || settings.Rule.Watch == nil || *settings.Rule.Watch != gen.SmartRuleWatchUnwatched {
		t.Fatalf("settings = %+v", settings)
	}
	if !slices.Equal(settings.Missing, []gen.SmartRuleRef{{Kind: gen.SmartRuleRefKindTag, Id: gone}}) {
		t.Errorf("missing = %+v", settings.Missing)
	}
	if len(detail.Items) != 1 || detail.Items[0].Kind != gen.SmartFolderItemKindSmartFolder ||
		detail.Items[0].SmartFolder.Id != child.Id || detail.SmartFolder.VideoCount != 0 ||
		detail.SmartFolder.AddedAt != nil || detail.SmartFolder.OpenVideoId != nil {
		t.Errorf("中身 = %+v, カード = %+v", detail.Items, detail.SmartFolder)
	}
	childDetail := decode[gen.SmartFolderDetail](t, f.env.get(smartFolderPath(child.Id, ""), f.owner))
	if !slices.Equal(childDetail.Settings.Missing, settings.Missing) ||
		!slices.Equal(childDetail.Trail, []gen.GroupRef{{Id: folder.Id, Name: "消す"}}) {
		t.Errorf("子の missing = %+v, trail = %+v", childDetail.Settings.Missing, childDetail.Trail)
	}
}

func TestSmartFolderItemsPage(t *testing.T) {
	f := newLibraryFixture(t)
	folder := f.createSmartFolder(t, map[string]any{"name": "全部", "rule": map[string]any{}, "sort": "titleAsc"})
	f.createSmartFolder(t, map[string]any{"name": "子", "rule": map[string]any{}, "parentId": folder.Id})

	detail := decode[gen.SmartFolderDetail](t, f.env.get(smartFolderPath(folder.Id, "?limit=1"), f.owner))
	if len(detail.Items) != 2 || detail.Items[0].Kind != gen.SmartFolderItemKindSmartFolder || detail.NextCursor == nil {
		t.Fatalf("最初のページ = %+v", detail)
	}
	kinds := []gen.SmartFolderItemKind{detail.Items[1].Kind}
	cursor := detail.NextCursor
	for cursor != nil {
		page := decode[gen.SmartFolderItemPage](t, f.env.get(smartFolderPath(folder.Id, "/items?limit=1&cursor="+*cursor), f.owner))
		for _, item := range page.Items {
			kinds = append(kinds, item.Kind)
		}
		cursor = page.NextCursor
	}
	if !slices.Equal(kinds, []gen.SmartFolderItemKind{gen.SmartFolderItemKindGroup, gen.SmartFolderItemKindGroup, gen.SmartFolderItemKindVideo}) {
		t.Errorf("項目の種類 = %v", kinds)
	}
	wantInvalidCursor := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidCursor}
	rec := f.env.get(smartFolderPath(folder.Id, "/items?cursor=x"), f.owner)
	assertErrorBody(t, "読めないカーソル", rec.Code, rec.Body.Bytes(), wantInvalidCursor)
	rec = f.env.get(smartFolderPath(folder.Id, "/items?cursor="), f.owner)
	assertErrorBody(t, "空のカーソル", rec.Code, rec.Body.Bytes(), wantInvalidCursor)
	rec = f.env.get(smartFolderPath(folder.Id, "?limit=201"), f.owner)
	assertErrorBody(t, "範囲の外の limit", rec.Code, rec.Body.Bytes(), wantGroupInvalid)
}

func TestSmartFolderCardsStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	shown := f.smartFolderTag(t, "公開", "solo", "ep10")
	hidden := f.smartFolderTag(t, "非公開", "ep10")
	open := f.createSmartFolder(t, map[string]any{"name": "公開", "rule": map[string]any{"tags": []int64{shown}}})
	closed := f.createSmartFolder(t, map[string]any{"name": "非公開", "rule": map[string]any{"tags": []int64{hidden}}})

	sixty := make([]string, 60)
	for i := range sixty {
		sixty[i] = strconv.FormatInt(open.Id, 10)
	}
	for _, target := range []string{
		"/api/smart-folders/cards",
		"/api/smart-folders/cards?ids=",
		"/api/smart-folders/cards?ids=a",
		"/api/smart-folders/cards?ids=" + strings.Join(append(slices.Clone(sixty), "1"), ","),
	} {
		rec := f.env.get(target, f.owner)
		assertErrorBody(t, target, rec.Code, rec.Body.Bytes(), wantGroupInvalid)
	}
	if rec := f.env.get("/api/smart-folders/cards?ids="+strings.Join(sixty, ","), f.owner); rec.Code != http.StatusOK ||
		len(decode[gen.SmartFolderCards](t, rec).Cards) != 1 {
		t.Errorf("60 件: %d %s", rec.Code, rec.Body)
	}

	target := fmt.Sprintf("/api/smart-folders/cards?ids=%d,999999,%d", closed.Id, open.Id)
	owner := decode[gen.SmartFolderCards](t, f.env.get(target, f.owner))
	if len(owner.Cards) != 2 || owner.Cards[0].Id != closed.Id || owner.Cards[1].Id != open.Id ||
		owner.Cards[1].VideoCount != 2 || owner.Cards[1].WatchState == nil || owner.Cards[1].Favorite == nil ||
		owner.Cards[1].OpenVideoId == nil || !slices.ContainsFunc(owner.Cards[1].Tags, func(tag gen.VideoTag) bool { return tag.Id == shown }) {
		t.Errorf("所有者のカード = %+v", owner.Cards)
	}
	guest := decode[gen.SmartFolderCards](t, f.env.get(target))
	if len(guest.Cards) != 1 || guest.Cards[0].Id != open.Id || guest.Cards[0].VideoCount != 1 ||
		guest.Cards[0].WatchState != nil || guest.Cards[0].Favorite != nil || len(guest.Cards[0].Tags) != 0 {
		t.Errorf("ゲストのカード = %+v", guest.Cards)
	}
}

func TestSmartFoldersInLibraryFavoritesAndPlaylists(t *testing.T) {
	f := newLibraryFixture(t)
	folder := f.createSmartFolder(t, map[string]any{"name": "全部", "rule": map[string]any{}})
	f.createSmartFolder(t, map[string]any{"name": "子", "rule": map[string]any{}, "parentId": folder.Id})

	var items []gen.SmartFolderSummary
	for _, item := range decode[gen.LibraryPage](t, f.env.get("/api/library", f.owner)).Items {
		if item.Kind == gen.LibraryItemKindSmartFolder {
			items = append(items, *item.SmartFolder)
		}
	}
	if len(items) != 1 || items[0].Id != folder.Id || items[0].Favorite == nil || *items[0].Favorite {
		t.Errorf("ライブラリのフォルダ = %+v", items)
	}
	ids := decode[gen.VideoIdsResponse](t, f.env.get("/api/library/ids", f.owner))
	if ids.SmartFolderIds == nil || !slices.Equal(*ids.SmartFolderIds, []int64{folder.Id}) {
		t.Errorf("smartFolderIds = %v", ids.SmartFolderIds)
	}

	rec := f.groupRequest(http.MethodPut, "/api/favorites",
		groupJSON(map[string]any{"smartFolderIds": []int64{folder.Id, 999999}, "favorite": true}), f.owner)
	if got := decode[gen.FavoritesResponse](t, rec); rec.Code != http.StatusOK || got.AppliedSmartFolders != 1 {
		t.Errorf("お気に入り = %d %+v", rec.Code, got)
	}
	for _, got := range decode[gen.SmartFolderList](t, f.env.get("/api/smart-folders", f.owner)).SmartFolders {
		if got.Favorite == nil || *got.Favorite != (got.Id == folder.Id) {
			t.Errorf("お気に入りのあとの一覧の %s = %v", got.Name, got.Favorite)
		}
	}

	rec = f.groupRequest(http.MethodPost, "/api/playlists", `{"name": "P"}`, f.owner)
	playlist := decode[gen.PlaylistSummary](t, rec)
	rec = f.groupRequest(http.MethodPost, "/api/playlists/"+strconv.FormatInt(playlist.Id, 10)+"/items",
		groupJSON(map[string]any{"smartFolderIds": []int64{folder.Id}}), f.owner)
	if got := decode[gen.PlaylistItemsAdded](t, rec); rec.Code != http.StatusOK || got.Added != 1 {
		t.Fatalf("プレイリストへ足す = %d %s", rec.Code, rec.Body)
	}
	got := decode[gen.Playlist](t, f.env.get("/api/playlists/"+strconv.FormatInt(playlist.Id, 10), f.owner))
	if len(got.Items) != 1 || got.Items[0].Kind != gen.PlaylistItemKindSmartFolder || got.Items[0].SmartFolder == nil ||
		got.Items[0].SmartFolder.Id != folder.Id || got.Items[0].SmartFolder.VideoCount != 6 {
		t.Errorf("プレイリスト = %+v", got.Items)
	}
}

func TestVideoDetailInASmartFolder(t *testing.T) {
	f := newVideoGroupFixture(t)
	tag := f.smartFolderTag(t, "両方", "solo", "p1")
	folder := f.createSmartFolder(t, map[string]any{"name": "両方", "rule": map[string]any{"tags": []int64{tag}}, "sort": "titleAsc"})
	p1 := strconv.FormatInt(f.ids["p1"], 10)

	video := decode[gen.Video](t, f.env.get(fmt.Sprintf("/api/videos/%s?smartFolder=%d", p1, folder.Id), f.owner))
	if video.Group != nil || video.SmartFolder == nil ||
		*video.SmartFolder != (gen.VideoSmartFolderRef{Id: folder.Id, Name: "両方", Position: 1, Count: 2}) {
		t.Errorf("フォルダの文脈 = group %+v smartFolder %+v", video.Group, video.SmartFolder)
	}
	pair := f.groupID(t, "pair")
	video = decode[gen.Video](t, f.env.get(fmt.Sprintf("/api/videos/%s?group=%d&smartFolder=%d", p1, pair, folder.Id), f.owner))
	if video.SmartFolder != nil || video.Group == nil || video.Group.Id != pair {
		t.Errorf("group が勝つ = group %+v smartFolder %+v", video.Group, video.SmartFolder)
	}
	related := decode[gen.RelatedVideos](t, f.env.get(fmt.Sprintf("/api/videos/%s/related?smartFolder=%d", p1, folder.Id), f.owner))
	if related.NextId == nil || *related.NextId != f.ids["solo"] {
		t.Errorf("フォルダの次 = %v, want solo", related.NextId)
	}
}

func TestChangeSmartFolderRuleReplacesTheRule(t *testing.T) {
	f := newLibraryFixture(t)
	cat := f.smartFolderTag(t, "猫", "solo")
	folder := f.createSmartFolder(t, map[string]any{"name": "全部", "rule": map[string]any{}, "sort": "titleAsc"})

	rec := f.groupRequest(http.MethodPatch, smartFolderPath(folder.Id, ""), fmt.Sprintf(`{"rule": {"tags": [%d]}}`, cat), f.owner)
	if got := decode[gen.SmartFolderSummary](t, rec); rec.Code != http.StatusOK || got.Name != "全部" {
		t.Fatalf("条件の変更 = %d %+v", rec.Code, got)
	}
	detail := decode[gen.SmartFolderDetail](t, f.env.get(smartFolderPath(folder.Id, ""), f.owner))
	settings := detail.Settings
	if settings == nil || settings.Sort != gen.VideoSortTitleAsc || settings.Rule.Tags == nil ||
		!slices.Equal(*settings.Rule.Tags, []int64{cat}) || len(settings.Missing) != 0 {
		t.Fatalf("変えたあとの settings = %+v", settings)
	}
	if len(detail.Items) != 1 || detail.Items[0].Kind != gen.SmartFolderItemKindVideo || detail.SmartFolder.VideoCount != 1 {
		t.Errorf("変えたあとの中身 = %+v, カード = %+v", detail.Items, detail.SmartFolder)
	}
}

type failingSmartFolders struct{}

var errSmartFolderStore = errors.New("database unavailable")

func (failingSmartFolders) ListSmartFolders(context.Context, domain.Audience) ([]domain.SmartFolderSummary, error) {
	return nil, errSmartFolderStore
}

func (failingSmartFolders) CreateSmartFolder(context.Context, domain.SmartFolderCreate) (domain.SmartFolder, error) {
	return domain.SmartFolder{}, errSmartFolderStore
}

func (failingSmartFolders) ChangeSmartFolder(context.Context, int64, domain.SmartFolderChange) (domain.SmartFolder, error) {
	return domain.SmartFolder{}, errSmartFolderStore
}

func (failingSmartFolders) DeleteSmartFolder(context.Context, int64) error {
	return errSmartFolderStore
}

func (failingSmartFolders) SmartFolderContents(context.Context, domain.Audience, int64, string, int) (domain.SmartFolderPage, error) {
	return domain.SmartFolderPage{}, errSmartFolderStore
}

func (failingSmartFolders) SmartFolderCards(context.Context, domain.Audience, []int64) ([]domain.SmartFolderCard, error) {
	return nil, errSmartFolderStore
}

func TestSmartFolderRoutesAnswerInternalErrors(t *testing.T) {
	wantInternal := wantError{status: http.StatusInternalServerError, code: gen.ErrorCodeInternal}
	routes := []struct{ method, target, body string }{
		{http.MethodGet, "/api/smart-folders", ""},
		{http.MethodPost, "/api/smart-folders", `{"name": "a", "rule": {}}`},
		{http.MethodPatch, "/api/smart-folders/3", `{"rule": {"watch": "watched"}}`},
		{http.MethodDelete, "/api/smart-folders/3", ""},
		{http.MethodGet, "/api/smart-folders/3", ""},
		{http.MethodGet, "/api/smart-folders/3/items?cursor=c1", ""},
		{http.MethodGet, "/api/smart-folders/cards?ids=3", ""},
	}
	for _, server := range []struct {
		label string
		opts  Options
	}{
		{"保存先なし", Options{}},
		{"保存先の誤り", Options{SmartFolders: failingSmartFolders{}}},
	} {
		handler := newTestServer(t, server.opts)
		for _, route := range routes {
			rec := jsonRequest(t, handler, route.method, route.target, route.body)
			assertErrorBody(t, server.label+" "+route.method+" "+route.target, rec.Code, rec.Body.Bytes(), wantInternal)
		}
	}
}
