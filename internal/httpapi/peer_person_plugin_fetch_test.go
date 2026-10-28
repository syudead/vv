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

// remotePersonFetch makes 早美れむ with a wiki ID on a, approves a person plugin answering runtime on b, and fetches
// for a's person from b; it returns a's person, b's fetch path for it and the fetch.
func remotePersonFetch(
	t *testing.T, runtime personRuntime,
) (fa, fb *guestFixture, b *peerNode, person domain.Credit, path string, fetched gen.PersonPluginFetch) {
	t.Helper()
	fa, fb, b, _ = workInfoNodes(t, runtime)
	person, err := fa.env.db.Credits().Create(context.Background(), domain.CreditPerson, domain.CreditInput{
		Name: "早美れむ", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "w-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pluginID := approvedPlugin(t, fb.env.db, domain.WorkInfoPluginManifest{
		Contract: 1, Kind: domain.WorkInfoPluginKindPerson, Name: "example.people", Version: "1.0.0",
		Operations: []domain.WorkInfoOperation{domain.WorkInfoSearch, domain.WorkInfoFetch}, Sources: []string{"wiki"},
	})
	path = fmt.Sprintf("/api/people/%d/plugin-fetches", domain.ToRemoteID(person.ID))
	rec := b.owner(http.MethodPost, path, fmt.Sprintf(`{"pluginId":%d}`, pluginID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("fetch: status = %d: %s", rec.Code, rec.Body)
	}
	return fa, fb, b, person, path, decode[gen.PersonPluginFetch](t, rec)
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
	fa, fb, b, person, path, fetched := remotePersonFetch(t, runtime)
	ctx := context.Background()
	remote := domain.ToRemoteID(person.ID)
	if rec := b.owner(http.MethodGet, fmt.Sprintf("%s/%s/images/0", path, fetched.FetchId), ""); rec.Code != http.StatusOK {
		t.Errorf("image: status = %d: %s", rec.Code, rec.Body)
	}

	rec := b.owner(http.MethodPost, fmt.Sprintf("%s/%s/apply", path, fetched.FetchId),
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

// An apply for a person the peer has since deleted answers person_not_found, as a local apply does.
func TestRemotePersonPluginApplyOfAPersonThePeerDeleted(t *testing.T) {
	runtime := personRuntime{person: domain.FetchedPluginPerson{Name: "早美れむ", Profile: domain.PersonProfile{Birthplace: "東京都"}}}
	fa, _, b, person, path, fetched := remotePersonFetch(t, runtime)
	if err := fa.env.db.Credits().Delete(context.Background(), domain.CreditPerson, person.ID); err != nil {
		t.Fatal(err)
	}
	rec := b.owner(http.MethodPost, fmt.Sprintf("%s/%s/apply", path, fetched.FetchId), `{"items":["profile.birthplace"]}`)
	assertErrorBody(t, "apply", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound})
}
