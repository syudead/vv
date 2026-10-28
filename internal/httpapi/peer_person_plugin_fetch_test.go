package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// personRuntime answers every fetch with person and one readable image.
type personRuntime struct {
	manifestRuntime
	person domain.FetchedPluginPerson
	image  []byte
}

func (r personRuntime) Run(
	context.Context, string, []byte, domain.WorkInfoRunRequest, domain.WorkInfoRunNetwork, func(domain.WorkInfoRunLog) error,
) (domain.WorkInfoRunResult, domain.WorkInfoRunLog, error) {
	return domain.WorkInfoRunResult{Person: r.person, PersonPluginImages: [][]byte{r.image}}, domain.WorkInfoRunLog{}, nil
}

// A person page fetch for the peer's person runs this node's plugin and writes on the peer: the profile field, the
// alias and the face, while this node gains no person.
func TestRemotePersonPluginFetchAppliesOnThePeer(t *testing.T) {
	var face bytes.Buffer
	if err := png.Encode(&face, image.NewGray(image.Rect(0, 0, 64, 64))); err != nil {
		t.Fatal(err)
	}
	runtime := personRuntime{image: face.Bytes(), person: domain.FetchedPluginPerson{
		Name: "早美れむ", Aliases: []domain.Alias{{Name: "はやみれむ"}}, Profile: domain.PersonProfile{Birthplace: "東京都"},
	}}
	fa, fb, b, _ := workInfoNodes(t, runtime)
	ctx := context.Background()
	person, err := fa.env.db.Credits().Create(ctx, domain.CreditPerson, domain.CreditInput{
		Name: "早美れむ", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "w-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pluginID := approvedPlugin(t, fb.env.db, domain.WorkInfoPluginManifest{
		Contract: 1, Kind: domain.WorkInfoPluginKindPerson, Name: "example.people", Version: "1.0.0",
		Operations: []domain.WorkInfoOperation{domain.WorkInfoSearch, domain.WorkInfoFetch}, Sources: []string{"wiki"},
	})
	remote := domain.ToRemoteID(person.ID)
	path := fmt.Sprintf("/api/people/%d/plugin-fetches", remote)

	rec := b.owner(http.MethodPost, path, fmt.Sprintf(`{"pluginId":%d}`, pluginID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("fetch: status = %d: %s", rec.Code, rec.Body)
	}
	fetched := decode[gen.PersonPluginFetch](t, rec)
	if rec := b.owner(http.MethodGet, fmt.Sprintf("%s/%s/images/0", path, fetched.FetchId), ""); rec.Code != http.StatusOK {
		t.Errorf("image: status = %d: %s", rec.Code, rec.Body)
	}

	rec = b.owner(http.MethodPost, fmt.Sprintf("%s/%s/apply", path, fetched.FetchId),
		`{"items":["image","profile.birthplace"],"imageIndex":0,"aliases":["はやみれむ"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: status = %d: %s", rec.Code, rec.Body)
	}
	applied := decode[gen.PersonPluginFetchApplied](t, rec).Person
	if applied.Id != remote || valueOrEmpty(applied.Profile.Birthplace) != "東京都" || applied.ImageRevision == nil {
		t.Errorf("apply answer = %+v, want person %d born in 東京都 with a face", applied, remote)
	}
	saved, err := fa.env.db.Credits().Get(ctx, domain.CreditPerson, person.ID)
	if err != nil || saved.Profile.Birthplace != "東京都" || len(saved.Aliases) != 1 || saved.ImageRevision == 0 {
		t.Errorf("a's person = %+v, %v, want born in 東京都, alias はやみれむ and a face", saved, err)
	}
	matches, err := fb.env.db.Credits().Match(ctx, domain.CreditPerson, []domain.CreditMatchEntry{{Name: "早美れむ"}})
	if err != nil || matches[0].State != domain.CreditMatchNew {
		t.Errorf("b's people: match = %+v, %v, want no person 早美れむ on b", matches, err)
	}
}
