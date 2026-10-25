package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// tagOn creates name on f's node and attaches it to f's videos.
func tagOn(t *testing.T, f *guestFixture, name string, videos ...string) domain.Tag {
	t.Helper()
	ctx := context.Background()
	tag, err := f.env.db.Tags().CreateTag(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, len(videos))
	for i, video := range videos {
		ids[i] = f.ids[video]
	}
	if len(ids) > 0 {
		if _, _, err := f.env.db.Tags().AttachTagByID(ctx, ids, tag.ID); err != nil {
			t.Fatal(err)
		}
	}
	return tag
}

func kindOn(t *testing.T, f *guestFixture, name string, tagIDs ...int64) domain.TagKind {
	t.Helper()
	ctx := context.Background()
	kind, err := f.env.db.Tags().CreateTagKind(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.db.Tags().BatchTags(ctx, domain.TagBatchRequest{Action: domain.TagBatchSetKind, KindID: kind.ID}, tagIDs); err != nil {
		t.Fatal(err)
	}
	return kind
}

func TestMergedTagListJoinsByNameAndForwardsARemoteOnlyTag(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	// Both nodes hold guestSecretTag on two videos; 猫 is b's alone on three, 犬 a's alone on one.
	cat := tagOn(t, fb, "猫", "c", "d", "e")
	dog := tagOn(t, fa, "犬", "c")
	localKind := kindOn(t, fb, "動物", cat.ID)
	kindOn(t, fa, "動物", dog.ID)
	remoteKind := kindOn(t, fa, "色")

	list := decode[gen.TagList](t, b.owner(http.MethodGet, "/api/tags?sort=countDesc", ""))
	type entry struct {
		id    int64
		name  string
		count int
	}
	var got []entry
	for _, tag := range list.Items {
		got = append(got, entry{tag.Id, tag.Name, tag.VideoCount})
	}
	remoteDog := domain.ToRemoteID(dog.ID)
	want := []entry{{fb.tagID, guestSecretTag, 4}, {cat.ID, "猫", 3}, {remoteDog, "犬", 1}}
	if !slices.Equal(got, want) || list.Total != 3 || list.TotalAll != 3 {
		t.Errorf("tags = %v, total %d/%d, want %v and 3/3", got, list.Total, list.TotalAll, want)
	}
	if kind := list.Items[2].Kind; kind == nil || kind.Id != localKind.ID {
		t.Errorf("犬's kind = %+v, want b's 動物 %d", kind, localKind.ID)
	}

	kinds := decode[gen.TagKindList](t, b.owner(http.MethodGet, "/api/tag-kinds", ""))
	// Both nodes start with the same seeded kinds, which join too; the new ones follow them in id order.
	var kindIDs []int64
	for _, kind := range kinds.Items {
		kindIDs = append(kindIDs, kind.Id)
	}
	if want := []int64{localKind.ID, domain.ToRemoteID(remoteKind.ID)}; len(kindIDs) < 2 ||
		!slices.Equal(kindIDs[len(kindIDs)-2:], want) || slices.ContainsFunc(kindIDs[:len(kindIDs)-2], domain.IsRemoteID) ||
		kinds.Items[len(kindIDs)-2].TagCount != 2 {
		t.Errorf("kinds = %+v, want the seeded kinds, then ids %v with 動物 counting both nodes' tags", kinds.Items, want)
	}

	filter := url.Values{"tag": {strconv.FormatInt(remoteDog, 10)}}
	videos, _ := pageVideos(t, fa, fb, func(target string) *httptest.ResponseRecorder {
		return b.owner(http.MethodGet, target, "")
	}, "/api/videos", filter)
	if want := []string{"a:c"}; !slices.Equal(videos, want) {
		t.Errorf("the remote tag's videos = %v, want %v", videos, want)
	}

	rec := b.owner(http.MethodPatch, "/api/tags/"+strconv.FormatInt(remoteDog, 10), `{"name":"子犬"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("forwarded edit: status = %d: %s", rec.Code, rec.Body)
	}
	if renamed := decode[gen.Tag](t, rec); renamed.Id != remoteDog || renamed.Name != "子犬" {
		t.Errorf("forwarded edit = %+v, want id %d named 子犬", renamed, remoteDog)
	}
	if tag, err := fa.env.db.Tags().ListTags(context.Background(), domain.TagListQuery{Search: "子犬"}); err != nil ||
		len(tag.Items) != 1 || tag.Items[0].ID != dog.ID {
		t.Errorf("a's tags after the edit = %+v, %v", tag.Items, err)
	}
}

func TestMergedPeopleListAndARemotePersonsPageAndImage(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	ctx := context.Background()
	person := func(f *guestFixture, kind domain.CreditKind, name string) domain.Credit {
		credit, err := f.env.db.Credits().Create(ctx, kind, domain.CreditInput{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return credit
	}
	localSato := person(fb, domain.CreditPerson, "佐藤")
	person(fa, domain.CreditPerson, "佐藤")
	yamada := person(fa, domain.CreditPerson, "山田")
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewGray(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewPersonImages(fa.env.db.Credits(), app.NewIngest(app.IngestOptions{})).
		UploadPersonImage(ctx, yamada.ID, picture.Bytes()); err != nil {
		t.Fatal(err)
	}
	studio := person(fa, domain.CreditOrganization, "スタジオ")

	people := decode[gen.PersonList](t, b.owner(http.MethodGet, "/api/people", ""))
	remoteYamada := domain.ToRemoteID(yamada.ID)
	var ids []int64
	for _, item := range people.Items {
		ids = append(ids, item.Id)
	}
	if want := []int64{localSato.ID, remoteYamada}; !slices.Equal(ids, want) || people.Total != 2 {
		t.Fatalf("people = %+v, want ids %v", people.Items, want)
	}
	if people.Items[1].ImageRevision == nil {
		t.Errorf("remote 山田 has no imageRevision")
	}

	path := "/api/people/" + strconv.FormatInt(remoteYamada, 10)
	if page := decode[gen.Person](t, b.owner(http.MethodGet, path, "")); page.Id != remoteYamada || page.Name != "山田" {
		t.Errorf("forwarded person = %+v", page)
	}
	got := b.owner(http.MethodGet, path+"/image", "")
	want := fa.env.get(creditPath("/api/people", yamada.ID, "/image"), fa.owner)
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "image/jpeg" ||
		!bytes.Equal(got.Body.Bytes(), want.Body.Bytes()) || got.Header().Get("ETag") != want.Header().Get("ETag") {
		t.Errorf("forwarded image: status = %d, headers = %v, %d bytes", got.Code, got.Header(), got.Body.Len())
	}

	remoteStudio := domain.ToRemoteID(studio.ID)
	organizations := decode[gen.OrganizationList](t, b.owner(http.MethodGet, "/api/organizations?q=すたじお", ""))
	if len(organizations.Items) != 1 || organizations.Items[0].Id != remoteStudio {
		t.Errorf("organizations = %+v, want スタジオ as %d", organizations.Items, remoteStudio)
	}
	rec := b.owner(http.MethodPatch, "/api/organizations/"+strconv.FormatInt(remoteStudio, 10),
		`{"aliases":[{"name":"別名"}]}`)
	if edited := decode[gen.Organization](t, rec); rec.Code != http.StatusOK || edited.Id != remoteStudio || len(edited.Aliases) != 1 {
		t.Errorf("forwarded organization edit: status = %d: %s", rec.Code, rec.Body)
	}
}

func TestForwardedOrganizationEditSendsTheParentInThePeersIDs(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	ctx := context.Background()
	organization := func(f *guestFixture, name string) int64 {
		credit, err := f.env.db.Credits().Create(ctx, domain.CreditOrganization, domain.CreditInput{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return credit.ID
	}
	localOnly := organization(fb, "手元だけ")
	localLabel := organization(fb, "レーベル")
	studio := organization(fa, "スタジオ")
	series := organization(fa, "系列")
	peerLabel := organization(fa, "レーベル")
	if peerLabel == localLabel {
		t.Fatalf("レーベルの ID が両方の台で %d; 書き換えを確かめられない", peerLabel)
	}
	path := "/api/organizations/" + strconv.FormatInt(domain.ToRemoteID(studio), 10)
	parentOnA := func() int64 {
		credit, err := fa.env.db.Credits().Get(ctx, domain.CreditOrganization, studio)
		if err != nil {
			t.Fatal(err)
		}
		if credit.Parent == nil {
			return 0
		}
		return credit.Parent.ID
	}

	for _, tc := range []struct {
		name     string
		parentID int64
		want     int64
	}{
		{"相手だけの親", domain.ToRemoteID(series), series},
		{"同じ名前の手元の親", localLabel, peerLabel},
	} {
		rec := b.owner(http.MethodPatch, path, fmt.Sprintf(`{"parentId":%d}`, tc.parentID))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", tc.name, rec.Code, rec.Body)
		}
		if got := parentOnA(); got != tc.want {
			t.Errorf("%s: a の親 = %d, want %d", tc.name, got, tc.want)
		}
	}

	rec := b.owner(http.MethodPatch, path, fmt.Sprintf(`{"parentId":%d}`, localOnly))
	assertErrorBody(t, "相手に無い親", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonOrganizationNotFound})
	if got := parentOnA(); got != peerLabel {
		t.Errorf("拒否の後の a の親 = %d, want %d のまま", got, peerLabel)
	}
}

func TestMergedTagListPagesEveryEntryOnce(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	// a holds t000–t149 and b t100–t249, so 100 names are on both nodes; with the fixture's tag, 251 entries.
	for i := range 150 {
		tagOn(t, fa, fmt.Sprintf("t%03d", i))
		tagOn(t, fb, fmt.Sprintf("t%03d", i+100))
	}
	seen := map[string]int{}
	query := url.Values{"limit": {"100"}}
	pages := 0
	for {
		list := decode[gen.TagList](t, b.owner(http.MethodGet, "/api/tags?"+query.Encode(), ""))
		pages++
		if list.Total != 251 || list.TotalAll != 251 {
			t.Fatalf("page %d: total = %d/%d, want 251", pages, list.Total, list.TotalAll)
		}
		for _, tag := range list.Items {
			seen[tag.Name]++
			if remote := domain.IsRemoteID(tag.Id); remote != (tag.Name < "t100") {
				t.Errorf("%s: id %d, remote = %v", tag.Name, tag.Id, remote)
			}
		}
		if list.NextCursor == nil {
			break
		}
		query.Set("cursor", *list.NextCursor)
	}
	if pages != 3 || len(seen) != 251 {
		t.Errorf("pages = %d, names = %d, want 3 and 251", pages, len(seen))
	}
	for name, count := range seen {
		if count != 1 {
			t.Errorf("%s listed %d times", name, count)
		}
	}
}
