package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// keyedVideos holds each video's content key.
type keyedVideos map[int64]string

func (v keyedVideos) GetVideo(_ context.Context, _ domain.Audience, id int64) (domain.Video, error) {
	key, ok := v[id]
	if !ok {
		return domain.Video{}, domain.ErrNotFound
	}
	return domain.Video{ID: id, ContentKey: key}, nil
}

type peerApplyRecorder struct{ calls []domain.FetchedWorkInfoApply }

func (r *peerApplyRecorder) ApplyFetchedWorkInfo(_ context.Context, apply domain.FetchedWorkInfoApply) (domain.FetchedWorkInfoApplied, error) {
	r.calls = append(r.calls, apply)
	return domain.FetchedWorkInfoApplied{}, nil
}

func (r *peerApplyRecorder) RecordWorkInfoFetch(context.Context, int64, string) error { return nil }

// What the peer fetched is written only to the content it was fetched for, with this node's key and no plugin
// recheck, and only with same-person pairs that name distinct fetched performers.
func TestApplyPeerFetchedWorkInfoWritesOnlyTheFetchedContent(t *testing.T) {
	people := []domain.FetchedPerson{
		{Role: domain.PersonPerformer, Name: "早美れむ"}, {Role: domain.PersonDirector, Name: "監督"},
	}
	key := "content-key-a"
	cases := []struct {
		name    string
		version string
		same    []domain.SamePerson
		wantErr error
	}{
		{name: "fetched content", version: domain.ContentVersion(key), same: []domain.SamePerson{{At: 0, PersonID: 9}}},
		{name: "changed content", version: "other", wantErr: domain.ErrNotFound},
		{name: "pair outside people", version: domain.ContentVersion(key), same: []domain.SamePerson{{At: -1, PersonID: 9}},
			wantErr: domain.ErrWorkInfoApplyInvalid},
		{name: "pair with a director", version: domain.ContentVersion(key), same: []domain.SamePerson{{At: 1, PersonID: 9}},
			wantErr: domain.ErrWorkInfoApplyInvalid},
		{name: "one performer twice", version: domain.ContentVersion(key),
			same: []domain.SamePerson{{At: 0, PersonID: 9}, {At: 0, PersonID: 8}}, wantErr: domain.ErrWorkInfoApplyInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &peerApplyRecorder{}
			plugins := NewWorkInfoPlugins(WorkInfoPluginsOptions{Videos: keyedVideos{7: key}, Applier: recorder})
			apply := domain.FetchedWorkInfoApply{
				Tags: []string{" レム "}, Details: domain.FetchedWorkInfo{People: people}, SamePeople: tc.same,
			}
			_, err := plugins.ApplyPeerFetchedWorkInfo(context.Background(), 7, tc.version, apply)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if len(recorder.calls) != 0 {
					t.Errorf("applied %+v, want nothing written", recorder.calls)
				}
				return
			}
			want := apply
			want.VideoID, want.ContentKey, want.FetchedByPeer, want.Tags = 7, key, true, []string{"レム"}
			if len(recorder.calls) != 1 || !reflect.DeepEqual(recorder.calls[0], want) {
				t.Errorf("applied %+v, want %+v", recorder.calls, want)
			}
		})
	}
}

// fakeWorkInfoPeer is a paired node holding one video, matching credits with its own answers.
type fakeWorkInfoPeer struct {
	keyedVideos
	*fakeCreditMatcher
	*peerApplyRecorder
}

func (fakeWorkInfoPeer) UploadThumbnailImage(context.Context, int64, string, []byte) (domain.Video, error) {
	return domain.Video{}, nil
}

// Nodes join credits by display name only, so an entry the peer lacks but this node knows by another name or an
// external ID links this node's entity and reaches the peer's apply under its display name, unless the peer matches
// that display name to another entity.
func TestFetchPeerWorkInfoLinksTheEntityThisNodeKnowsByAnotherName(t *testing.T) {
	e := newFetchEnv()
	sato := domain.CreditRef{ID: 11, Name: "佐藤みか"}
	peerAoi := domain.CreditRef{ID: 1<<40 + 3, Name: "Aoi"}
	maker := domain.CreditRef{ID: 21, Name: "Maker Inc."}
	e.matcher.answers = map[string]domain.CreditMatch{
		"Mika Sato": {State: domain.CreditMatchExisting, Credit: &sato},
		"Aoi":       {State: domain.CreditMatchExisting, Credit: &domain.CreditRef{ID: 12, Name: "Aoi Local"}},
		"Unnamed":   {State: domain.CreditMatchExisting, Credit: &domain.CreditRef{ID: 13, Provisional: true}},
		"Maker":     {State: domain.CreditMatchExisting, Credit: &maker},
		"Taro":      {State: domain.CreditMatchExisting, Credit: &domain.CreditRef{ID: 14, Name: "太郎"}},
		// Two people here are named 花子, so the name alone would join the other one.
		"Hana":       {State: domain.CreditMatchExisting, Credit: &domain.CreditRef{ID: 15, Name: "花子"}},
		"花子":         {State: domain.CreditMatchAmbiguous},
		"佐藤みか":       {State: domain.CreditMatchExisting, Credit: &sato},
		"太郎":         {State: domain.CreditMatchExisting, Credit: &domain.CreditRef{ID: 14, Name: "太郎"}},
		"Maker Inc.": {State: domain.CreditMatchExisting, Credit: &maker},
	}
	peer := fakeWorkInfoPeer{
		keyedVideos: keyedVideos{fetchVideo: "peer-key"},
		fakeCreditMatcher: &fakeCreditMatcher{answers: map[string]domain.CreditMatch{
			"Aoi": {State: domain.CreditMatchExisting, Credit: &peerAoi},
			// The peer's 佐藤みか joins this node's; its 次郎 holds 太郎 as another name.
			"佐藤みか": {State: domain.CreditMatchExisting, Credit: &sato},
			"太郎":   {State: domain.CreditMatchExisting, Credit: &domain.CreditRef{ID: 1<<40 + 5, Name: "次郎"}},
		}},
		peerApplyRecorder: &peerApplyRecorder{},
	}
	performers := []domain.FetchedPerson{
		{Role: domain.PersonPerformer, Name: "Mika Sato"}, {Role: domain.PersonPerformer, Name: "Aoi"},
		{Role: domain.PersonPerformer, Name: "Unnamed"}, {Role: domain.PersonPerformer, Name: "Newcomer"},
		{Role: domain.PersonPerformer, Name: "Taro"}, {Role: domain.PersonPerformer, Name: "Hana"},
	}
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: domain.FetchedWorkInfo{
		People: performers, Organizations: []domain.FetchedOrganization{{Role: domain.OrganizationMaker, Name: "Maker"}},
	}}
	ctx := context.Background()
	result, err := e.plugins.FetchPeerWorkInfo(ctx, peer, fetchVideo, 1, "W-1")
	if err != nil {
		t.Fatal(err)
	}
	newMatch := domain.CreditMatch{State: domain.CreditMatchNew}
	wantPeople := []domain.CreditMatch{
		{State: domain.CreditMatchExisting, Credit: &sato}, {State: domain.CreditMatchExisting, Credit: &peerAoi},
		newMatch, newMatch, newMatch, newMatch,
	}
	if !reflect.DeepEqual(result.PersonMatches, wantPeople) {
		t.Fatalf("person matches = %+v; want %+v", result.PersonMatches, wantPeople)
	}
	wantLocal := [][]string{
		{"Mika Sato", "Unnamed", "Newcomer", "Taro", "Hana"}, {"佐藤みか", "太郎", "花子"}, {"Maker"}, {"Maker Inc."},
	}
	if !reflect.DeepEqual(e.matcher.names, wantLocal) {
		t.Fatalf("this node matched %v; want the entries the peer lacks, then their display names, %v",
			e.matcher.names, wantLocal)
	}
	wantPeer := [][]string{
		{"Mika Sato", "Aoi", "Unnamed", "Newcomer", "Taro", "Hana"}, {"佐藤みか", "太郎"}, {"Maker"}, {"Maker Inc."},
	}
	if !reflect.DeepEqual(peer.names, wantPeer) {
		t.Fatalf("the peer matched %v; want %v", peer.names, wantPeer)
	}

	_, err = e.plugins.ApplyPeerWorkInfoFetch(ctx, peer, fetchVideo, result.ID, domain.WorkInfoSelection{
		People: []int{0, 1, 2, 3, 4, 5}, Organizations: []int{0},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, person := range peer.peerApplyRecorder.calls[0].Details.People {
		names = append(names, person.Name)
	}
	if want := []string{"佐藤みか", "Aoi", "Unnamed", "Newcomer", "Taro", "Hana"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("the peer's apply names %v; want %v", names, want)
	}
	if got := peer.peerApplyRecorder.calls[0].Details.Organizations; len(got) != 1 || got[0].Name != "Maker Inc." {
		t.Fatalf("the peer's apply organizations = %+v; want Maker Inc.", got)
	}
}
