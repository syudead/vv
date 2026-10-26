package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// A remote video's work information is saved on the peer, with this node's people and organizations sent as
// the peer's of the same name.
func TestRemoteVideoWorkInfoIsSavedOnThePeerInItsIDs(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	ctx := context.Background()
	credit := func(f *guestFixture, kind domain.CreditKind, name string) int64 {
		created, err := f.env.db.Credits().Create(ctx, kind, domain.CreditInput{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return created.ID
	}
	// 山田 comes first so that 佐藤 has different ids on the two nodes.
	onlyOnA := credit(fa, domain.CreditPerson, "山田")
	sharedOnA, sharedOnB := credit(fa, domain.CreditPerson, "佐藤"), credit(fb, domain.CreditPerson, "佐藤")
	studioOnA, studioOnB := credit(fa, domain.CreditOrganization, "スタジオ"), credit(fb, domain.CreditOrganization, "スタジオ")
	remote := domain.ToRemoteID(fa.ids["a"])
	path := fmt.Sprintf("/api/videos/%d", remote)

	rec := b.owner(http.MethodPut, path+"/details", fmt.Sprintf(
		`{"displayName":"遠くの名前","workInfo":{"people":[{"role":"performer","person":{"id":%d}},{"role":"director","person":{"id":%d}}]}}`,
		sharedOnB, domain.ToRemoteID(onlyOnA)))
	if rec.Code != http.StatusOK {
		t.Fatalf("details: status = %d: %s", rec.Code, rec.Body)
	}
	if saved := decode[gen.Video](t, rec); saved.Id != remote || saved.Title != "遠くの名前" {
		t.Errorf("details answer: id = %d, title = %q, want %d and 遠くの名前", saved.Id, saved.Title, remote)
	}
	onA := decode[gen.Video](t, fa.env.get(fa.videoPath("a", ""), fa.owner))
	var people []int64
	if onA.WorkInfo != nil {
		for _, p := range onA.WorkInfo.People {
			people = append(people, p.Person.Id)
		}
	}
	if want := []int64{sharedOnA, onlyOnA}; onA.Title != "遠くの名前" || !slices.Equal(people, want) {
		t.Errorf("a's video: title = %q, people = %v, want 遠くの名前 and %v", onA.Title, people, want)
	}

	rec = b.owner(http.MethodPut, path+"/work-info",
		fmt.Sprintf(`{"organizations":[{"role":"maker","organization":{"id":%d}}]}`, studioOnB))
	if rec.Code != http.StatusOK {
		t.Fatalf("work-info: status = %d: %s", rec.Code, rec.Body)
	}
	onA = decode[gen.Video](t, fa.env.get(fa.videoPath("a", ""), fa.owner))
	if onA.WorkInfo == nil || len(onA.WorkInfo.Organizations) != 1 || onA.WorkInfo.Organizations[0].Organization.Id != studioOnA {
		t.Errorf("a's organizations = %+v, want スタジオ %d", onA.WorkInfo, studioOnA)
	}
}

// A person or organization new to the screen goes to the peer by name: the peer creates it once, and this node
// creates nothing. A local video refuses a credit by name, since its screen creates credits first.
func TestRemoteVideoCreditByNameIsCreatedOnlyOnThePeer(t *testing.T) {
	fa, fb, b, _ := mergedNodes(t)
	ctx := context.Background()
	remote := domain.ToRemoteID(fa.ids["a"])
	rec := b.owner(http.MethodPut, fmt.Sprintf("/api/videos/%d/details", remote),
		`{"displayName":"x","workInfo":{"people":[{"role":"performer","person":{"name":"新人"}},`+
			`{"role":"director","person":{"name":"新人"}}],"organizations":[{"role":"maker","organization":{"name":"新スタジオ"}}]}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("details: status = %d: %s", rec.Code, rec.Body)
	}
	names := func(f *guestFixture, kind domain.CreditKind) map[int64]string {
		page, err := f.env.db.Credits().List(ctx, kind, domain.CreditQuery{})
		if err != nil {
			t.Fatal(err)
		}
		out := map[int64]string{}
		for _, credit := range page.Items {
			out[credit.ID] = credit.Name
		}
		return out
	}
	peopleOnA := names(fa, domain.CreditPerson)
	onA := decode[gen.Video](t, fa.env.get(fa.videoPath("a", ""), fa.owner))
	if onA.WorkInfo == nil || len(onA.WorkInfo.People) != 2 || len(onA.WorkInfo.Organizations) != 1 ||
		peopleOnA[onA.WorkInfo.People[0].Person.Id] != "新人" || onA.WorkInfo.People[1].Person.Id != onA.WorkInfo.People[0].Person.Id ||
		len(peopleOnA) != 1 || onA.WorkInfo.Organizations[0].Organization.Name != "新スタジオ" {
		t.Errorf("a の作品情報 = %+v, a の人物 = %v, want 1 人の 新人 と 新スタジオ", onA.WorkInfo, peopleOnA)
	}
	if people, organizations := names(fb, domain.CreditPerson), names(fb, domain.CreditOrganization); len(people)+len(organizations) != 0 {
		t.Errorf("b に作られた: 人物 %v, 組織 %v", people, organizations)
	}

	rec = b.owner(http.MethodPut, fb.videoPath("a", "/work-info"), `{"people":[{"role":"performer","person":{"name":"新人"}}]}`)
	if rec.Code != http.StatusBadRequest || decode[gen.Error](t, rec).Message != "people[0].person.name is only for a video of the paired VVMDM." {
		t.Errorf("ローカルの名前指定: status = %d: %s, want 400", rec.Code, rec.Body)
	}
}

// A forwarded save missing a required nested field is refused here: re-encoding would fill it with a zero value
// that the peer then accepts, dropping the date and clearing the dates it already has.
func TestRemoteWorkInfoMissingARequiredFieldIsNotForwarded(t *testing.T) {
	for _, tc := range []struct{ operation, body, want string }{
		{"PUT /api/videos/{id}/details", `{"displayName":"x","workInfo":{"dates":[{"kind":"release"}]}}`,
			"dates[0].date is required."},
		{"PUT /api/videos/{id}/work-info", `{"externalIds":[{"source":"fanza","value":null}]}`,
			"externalIds[0].value is required."},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/videos/1", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		forward := remoteVideoOperations[tc.operation].request(&server{logger: discardLogger()}, rec, req)
		if forward || rec.Code != http.StatusBadRequest || decode[gen.Error](t, rec).Message != tc.want {
			t.Errorf("%s: forward = %v, status = %d, body = %s, want 400 and %q",
				tc.operation, forward, rec.Code, rec.Body, tc.want)
		}
	}
}
