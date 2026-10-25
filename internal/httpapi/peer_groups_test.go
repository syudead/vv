package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// groupNodes pairs b with a as mergedNodes does, with groups, drag operations and collection deletes on both.
func groupNodes(t *testing.T) (fa, fb *guestFixture, a, b *peerNode, stopA func()) {
	t.Helper()
	return mergedNodesWith(t, func(db *store.DB, opts *Options) {
		opts.Groups, opts.Collections = db.Groups(), db.Collections()
		opts.Organizer = app.NewOrganizer(app.OrganizerOptions{Store: db.Organize()})
	})
}

func createGroup(t *testing.T, n *peerNode, name string, videoIDs ...int64) int64 {
	t.Helper()
	body, _ := json.Marshal(gen.GroupCreate{Name: name, VideoIds: &videoIDs})
	rec := n.owner(http.MethodPost, "/api/groups", string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: status = %d: %s", name, rec.Code, rec.Body)
	}
	return decode[gen.LibraryGroup](t, rec).Id
}

func groupPath(id int64) string { return "/api/groups/" + strconv.FormatInt(id, 10) }

// groupMembers lists the videos of the group n reads at id, or nil when it does not exist.
func groupMembers(t *testing.T, n *peerNode, id int64) []int64 {
	t.Helper()
	rec := n.owner(http.MethodGet, groupPath(id), "")
	if rec.Code == http.StatusNotFound {
		return nil
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("group %d: status = %d: %s", id, rec.Code, rec.Body)
	}
	return decode[gen.GroupDetail](t, rec).Group.VideoIds
}

func remoteIDs(ids ...int64) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = domain.ToRemoteID(id)
	}
	return out
}

func TestPeerGroupIsListedAndOpensWithRemoteIDs(t *testing.T) {
	fa, _, a, b, _ := groupNodes(t)
	group := createGroup(t, a, "g", fa.ids["b"], fa.ids["c"])
	remote, members := domain.ToRemoteID(group), remoteIDs(fa.ids["b"], fa.ids["c"])

	library := decode[gen.LibraryPage](t, b.owner(http.MethodGet, "/api/library?limit=100", ""))
	index := slices.IndexFunc(library.Items, func(item gen.LibraryItem) bool {
		return item.Group != nil && item.Group.Id == remote
	})
	if index < 0 {
		t.Fatalf("library has no group %d: %+v", remote, library.Items)
	}
	if got := library.Items[index].Group.VideoIds; !slices.Equal(got, members) {
		t.Errorf("library group members = %v, want %v", got, members)
	}

	rec := b.owner(http.MethodGet, groupPath(remote), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remote group: status = %d: %s", rec.Code, rec.Body)
	}
	detail := decode[gen.GroupDetail](t, rec)
	var items []int64
	for _, item := range detail.Items {
		if item.Video == nil || !domain.IsRemoteID(item.Id) {
			t.Fatalf("item %+v is not a remote video item", item)
		}
		items = append(items, item.Video.Id)
	}
	if detail.Group.Id != remote || !slices.Equal(items, members) {
		t.Errorf("group %d with items %v, want %d with %v", detail.Group.Id, items, remote, members)
	}

	picker := decode[gen.GroupListPage](t, b.owner(http.MethodGet, "/api/groups?node=peer", ""))
	if len(picker.Groups) != 1 || picker.Groups[0].Id != remote {
		t.Errorf("node=peer groups = %+v, want only %d", picker.Groups, remote)
	}
}

// A remote group's page keeps the reader's favorites and play history local, as the merged lists do.
func TestPeerGroupReadUsesNoneOfThePeersFavorites(t *testing.T) {
	fa, _, a, b, _ := groupNodes(t)
	group := createGroup(t, a, "g", fa.ids["b"], fa.ids["c"])
	if rec := a.owner(http.MethodPut, "/api/favorites", favoritesBody(true, []int64{fa.ids["b"]})); rec.Code != http.StatusOK {
		t.Fatalf("favorite on the peer: status = %d: %s", rec.Code, rec.Body)
	}

	// c is newer than b, so the peer's addedDesc order lists c first and its favoritedDesc order lists b first.
	for _, tc := range []struct {
		query string
		want  []int64
	}{
		{"favorite=true", nil},
		{"sort=favoritedDesc", remoteIDs(fa.ids["c"], fa.ids["b"])},
	} {
		for _, path := range []string{"", "/items"} {
			rec := b.owner(http.MethodGet, groupPath(domain.ToRemoteID(group))+path+"?"+tc.query, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("%s%s: status = %d: %s", path, tc.query, rec.Code, rec.Body)
			}
			page := decode[gen.GroupItemPage](t, rec)
			var got []int64
			for _, item := range page.Items {
				got = append(got, item.Video.Id)
			}
			if !slices.Equal(got, tc.want) || page.NextCursor != nil {
				t.Errorf("%s?%s = %v, next %v, want %v", path, tc.query, got, page.NextCursor, tc.want)
			}
		}
	}
}

func TestPeerGroupRefusesVideosOfBothNodes(t *testing.T) {
	fa, fb, a, b, _ := groupNodes(t)
	group := createGroup(t, a, "g", fa.ids["b"])

	for _, tc := range []struct{ label, path, body string }{
		{"add a local video to the peer's group", groupPath(domain.ToRemoteID(group)) + "/items",
			fmt.Sprintf(`{"videoIds":[%d]}`, fb.ids["a"])},
		{"create a group of both nodes' videos", "/api/groups",
			fmt.Sprintf(`{"name":"x","videoIds":[%d,%d]}`, fb.ids["a"], domain.ToRemoteID(fa.ids["a"]))},
	} {
		rec := b.owner(http.MethodPost, tc.path, tc.body)
		if problem := decode[gen.Error](t, rec); rec.Code != http.StatusBadRequest ||
			problem.Reason == nil || *problem.Reason != gen.ErrorReasonMixedNodes {
			t.Errorf("%s: status = %d: %s, want 400 mixed_nodes", tc.label, rec.Code, rec.Body)
		}
	}
	if got := groupMembers(t, a, group); !slices.Equal(got, []int64{fa.ids["b"]}) {
		t.Errorf("the peer's group holds %v, want only b", got)
	}
}

func TestOrganizeMoveOfPeerVideosIsForwardedAndUndone(t *testing.T) {
	fa, _, a, b, _ := groupNodes(t)
	group := createGroup(t, a, "g", fa.ids["b"])

	body := fmt.Sprintf(`{"items":[{"kind":"video","id":%d}],"to":%d}`,
		domain.ToRemoteID(fa.ids["e"]), domain.ToRemoteID(group))
	rec := b.owner(http.MethodPost, "/api/organize/move", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("move: status = %d: %s", rec.Code, rec.Body)
	}
	if got := groupMembers(t, a, group); !slices.Equal(got, []int64{fa.ids["b"], fa.ids["e"]}) {
		t.Errorf("after the move the peer's group holds %v, want b and e", got)
	}

	token := decode[gen.OrganizeResult](t, rec).UndoToken
	if !strings.HasPrefix(token, remoteUndoPrefix) {
		t.Fatalf("undo token %q does not name the peer", token)
	}
	undo, _ := json.Marshal(gen.OrganizeUndo{Token: token})
	if rec := b.owner(http.MethodPost, "/api/organize/undo", string(undo)); rec.Code != http.StatusNoContent {
		t.Fatalf("undo: status = %d: %s", rec.Code, rec.Body)
	}
	if got := groupMembers(t, a, group); !slices.Equal(got, []int64{fa.ids["b"]}) {
		t.Errorf("after the undo the peer's group holds %v, want only b", got)
	}
}

func TestCollectionsDeleteSplitsByNodeAndReportsThePeersFailure(t *testing.T) {
	fa, fb, a, b, stopA := groupNodes(t)
	// deleteOneOfEach deletes a new group of each node from b; the local one must be gone either way.
	deleteOneOfEach := func() (result gen.CollectionsDeleteResponse, remote gen.RemoteSelectionResult, peer int64) {
		t.Helper()
		local, peer := createGroup(t, b, "local", fb.ids["a"]), createGroup(t, a, "peer", fa.ids["a"])
		body := fmt.Sprintf(`{"groupIds":[%d,%d]}`, local, domain.ToRemoteID(peer))
		rec := b.owner(http.MethodPost, "/api/collections/delete", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("delete: status = %d: %s", rec.Code, rec.Body)
		}
		if groupMembers(t, b, local) != nil {
			t.Errorf("the local group %d is still there", local)
		}
		result = decode[gen.CollectionsDeleteResponse](t, rec)
		if result.Remote == nil {
			t.Fatalf("no remote result: %s", rec.Body)
		}
		return result, *result.Remote, peer
	}

	both, remote, peer := deleteOneOfEach()
	if both.DissolvedGroups != 2 || remote.Applied != 1 || len(remote.FailedVideoIds) != 0 || remote.Reason != nil {
		t.Errorf("both nodes: %+v, remote %+v, want 2 dissolved and 1 applied on the peer", both, remote)
	}
	if groupMembers(t, a, peer) != nil {
		t.Errorf("the peer's group %d is still there", peer)
	}

	stopA()
	alone, remote, peer := deleteOneOfEach()
	if alone.DissolvedGroups != 1 || remote.Applied != 0 ||
		!slices.Equal(remote.FailedVideoIds, []int64{domain.ToRemoteID(peer)}) ||
		remote.Reason == nil || *remote.Reason != gen.ErrorReasonPeerUnreachable {
		t.Errorf("peer stopped: %+v, remote %+v, want 1 dissolved and [%d] with peer_unreachable",
			alone, remote, domain.ToRemoteID(peer))
	}
	if groupMembers(t, a, peer) == nil {
		t.Errorf("the peer's group %d was removed while the peer was stopped", peer)
	}
}
