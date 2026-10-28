package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// peerPeople is the paired node: it holds fakePluginPeople's people without the plugin, which it never rechecks.
type peerPeople struct {
	*fakePluginPeople
	faces recordingFaces
}

func (p *peerPeople) ApplyPersonPluginFetch(
	_ context.Context, apply domain.PersonPluginApply,
) (domain.PersonPluginApplied, error) {
	p.applies = append(p.applies, apply)
	return domain.PersonPluginApplied{Person: domain.Credit{ID: apply.PersonID}}, nil
}

func (p *peerPeople) ReplacePersonImage(ctx context.Context, id int64, data []byte) (int64, error) {
	return p.faces.ReplacePersonImage(ctx, id, data)
}

// A fetch for the peer's person runs this node's plugin with the peer's stored ID; the apply rechecks the plugin here,
// then the peer writes the values without the image and takes the image as the face.
func TestPeerPersonPluginFetchRunsHereAndWritesOnThePeer(t *testing.T) {
	e := newPersonFetchEnv()
	remote := domain.ToRemoteID(fetchPerson)
	peer := &peerPeople{fakePluginPeople: &fakePluginPeople{people: map[int64]domain.Credit{remote: e.people.people[fetchPerson]}}}
	e.runtime.result = domain.WorkInfoRunResult{
		Person:             domain.FetchedPluginPerson{Name: "Hanako", Profile: domain.PersonProfile{Birthplace: "Tokyo"}},
		PersonPluginImages: [][]byte{testPNG(t, 2, 2)},
	}
	ctx := context.Background()
	result, err := e.plugins.FetchPeerPersonWithPlugin(ctx, peer, remote, 1, domain.PersonFetchKey{})
	if err != nil {
		t.Fatalf("FetchPeerPersonWithPlugin: %v", err)
	}
	if req := e.runtime.requests[0]; req.Source != "wiki" || req.ID != "w-1" {
		t.Fatalf("request = %+v; want the peer person's first accepted ID", req)
	}
	selection := domain.PersonPluginSelection{
		Items: []domain.PersonPluginItem{domain.PersonItemImage, domain.PersonProfileItem(domain.ProfileBirthplace)},
	}

	stale := *e.store.plugins[1]
	e.store.plugins[1].Digest, e.store.plugins[1].ApprovedDigest = "d1b", "d1b"
	if _, err := e.plugins.ApplyPeerPersonPluginFetch(ctx, peer, remote, result.ID, selection, 0); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("apply after a replacement err = %v; want ErrWorkInfoFetchStale", err)
	}
	if len(peer.applies) != 0 {
		t.Fatalf("peer applies = %+v; want none after the plugin changed", peer.applies)
	}

	*e.store.plugins[1] = stale
	result, err = e.plugins.FetchPeerPersonWithPlugin(ctx, peer, remote, 1, domain.PersonFetchKey{})
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if _, err := e.plugins.ApplyPeerPersonPluginFetch(ctx, peer, remote, result.ID, selection, 0); err != nil {
		t.Fatalf("ApplyPeerPersonPluginFetch: %v", err)
	}
	want := []domain.PersonPluginItem{domain.PersonProfileItem(domain.ProfileBirthplace)}
	if len(peer.applies) != 1 || peer.applies[0].PersonID != remote || !reflect.DeepEqual(peer.applies[0].Items, want) {
		t.Fatalf("peer applies = %+v; want one for %d with items %v", peer.applies, remote, want)
	}
	if len(peer.faces.replaced) != 1 || peer.faces.replaced[0].id != remote || len(e.faces.replaced) != 0 {
		t.Fatalf("faces: peer %+v, here %+v; want the image as the peer person's face only", peer.faces, e.faces)
	}
}

// What the peer fetched writes without a plugin recheck, and only with items that carry a value and are not the image.
func TestApplyPeerFetchedPersonWritesOnlyItemsWithValues(t *testing.T) {
	fetched := domain.FetchedPluginPerson{Name: "Hanako", Profile: domain.PersonProfile{Birthplace: "Tokyo"}}
	birthplace := domain.PersonProfileItem(domain.ProfileBirthplace)
	cases := []struct {
		name    string
		items   []domain.PersonPluginItem
		wantErr error
	}{
		{name: "profile item", items: []domain.PersonPluginItem{birthplace}},
		{name: "image", items: []domain.PersonPluginItem{birthplace, domain.PersonItemImage}, wantErr: domain.ErrWorkInfoApplyInvalid},
		{name: "item without a value", items: []domain.PersonPluginItem{domain.PersonItemAliases}, wantErr: domain.ErrWorkInfoApplyInvalid},
		{name: "unknown item", items: []domain.PersonPluginItem{"profile.weight"}, wantErr: domain.ErrWorkInfoApplyInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newPersonFetchEnv()
			_, err := e.plugins.ApplyPeerFetchedPerson(context.Background(), fetchPerson, fetched, tc.items)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if len(e.people.applies) != 0 {
					t.Errorf("applied %+v, want nothing written", e.people.applies)
				}
				return
			}
			want := domain.PersonPluginApply{PersonID: fetchPerson, FetchedByPeer: true, Fetched: fetched, Items: tc.items}
			if len(e.people.applies) != 1 || !reflect.DeepEqual(e.people.applies[0], want) {
				t.Errorf("applied %+v, want %+v", e.people.applies, want)
			}
		})
	}
}
