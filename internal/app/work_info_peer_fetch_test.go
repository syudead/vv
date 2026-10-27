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
