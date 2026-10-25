package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"slices"
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
