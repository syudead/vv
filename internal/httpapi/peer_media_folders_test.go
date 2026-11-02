package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func linkFoldersPath(nodeID string) string { return "/api/peer/links/" + nodeID + "/media-folders" }

func decodeLinkFolders(t *testing.T, rec *httptest.ResponseRecorder) gen.PeerLinkMediaFolders {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("リンクのフォルダ一覧: status = %d: %s", rec.Code, rec.Body)
	}
	var folders gen.PeerLinkMediaFolders
	if err := json.Unmarshal(rec.Body.Bytes(), &folders); err != nil {
		t.Fatal(err)
	}
	return folders
}

// B keeps A's folders from A's own listing, switches one off on B only, and every request B sends A carries it:
// A's boundary turns it into the scope its store reads with.
func TestALinksSwitchedOffFolderReachesTheReadNodeAsItsScope(t *testing.T) {
	ctx := context.Background()
	a, b := pairTestNodes(t)
	kept, err := a.env.db.Settings().AddMediaFolder(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	off, err := a.env.db.Settings().AddMediaFolder(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if rec := a.owner(http.MethodGet, "/api/peer/media-folders", ""); rec.Code != http.StatusNotFound {
		t.Errorf("セッションクッキーでの読まれる側の一覧: status = %d, want 404", rec.Code)
	}
	stream, _ := openEvents(t, b.env.handler)
	b.runConnection(t)
	nextEventNamed(t, stream, "mediaFolders")

	nodeID := b.onlyLink(t).NodeID
	rec := b.owner(http.MethodGet, linkFoldersPath(nodeID), "")
	folders := decodeLinkFolders(t, rec)
	if folders.State != gen.PeerLinkMediaFoldersStateConnected || len(folders.Folders) != 2 ||
		folders.Folders[0].Id != kept.ID || folders.Folders[1].Id != off.ID || !folders.Folders[1].Enabled ||
		folders.Folders[1].Path != off.Path {
		t.Fatalf("リンクのフォルダ = %+v, want A の 2 つがオン", folders)
	}

	rec = b.owner(http.MethodPut, fmt.Sprintf("%s/%d/enabled", linkFoldersPath(nodeID), off.ID), `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("リンクのフォルダのスイッチ: status = %d: %s", rec.Code, rec.Body)
	}
	var switched gen.PeerMediaFolder
	if err := json.Unmarshal(rec.Body.Bytes(), &switched); err != nil || switched.Id != off.ID || switched.Enabled {
		t.Fatalf("スイッチの答え = %+v, %v, want %d がオフ", switched, err, off.ID)
	}
	nextEventNamed(t, stream, "mediaFolders")
	if stored, err := a.env.db.Settings().ListMediaFolders(ctx); err != nil || !stored[1].Enabled {
		t.Errorf("A の自分のスイッチ = %+v, %v, want オンのまま", stored, err)
	}

	if rec := b.owner(http.MethodGet, "/api/videos", ""); rec.Code != http.StatusOK {
		t.Fatalf("B の一覧: status = %d: %s", rec.Code, rec.Body)
	}
	got := a.library.audiences()
	want := domain.AudienceOwner.WithExcludedFolders([]int64{off.ID})
	if len(got) == 0 || got[len(got)-1] != want {
		t.Errorf("A のストアに渡った視聴者 = %+v, want 最後が %+v", got, want)
	}
}

// The section renders while the link is down: the snapshot answers with the link's state and its switch works.
func TestLinkFoldersAnswerTheSnapshotWhileTheLinkIsUnreachable(t *testing.T) {
	ctx := context.Background()
	b := newPeerNode(t, "node-b")
	peers := b.env.db.Peers()
	if _, err := peers.AddLink(ctx, domain.PeerLink{
		NodeID: otherPeerNodeID, Name: "node-a", Address: "http://127.0.0.1:1", Secret: "vvp_outgoing", PairedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := peers.ReplacePeerFolders(ctx, otherPeerNodeID, []domain.PeerFolder{{ID: 4, Path: "/a", Version: 1}}, time.Now()); err != nil {
		t.Fatal(err)
	}

	rec := b.owner(http.MethodGet, linkFoldersPath(otherPeerNodeID), "")
	folders := decodeLinkFolders(t, rec)
	if folders.State != gen.PeerLinkMediaFoldersStateUnreachable || folders.Reason == nil ||
		*folders.Reason != gen.PeerLinkMediaFoldersReasonNetwork || len(folders.Folders) != 1 || folders.Folders[0].Id != 4 {
		t.Fatalf("届かない間のフォルダ = %+v, want unreachable/network で 4", folders)
	}
	if rec := b.owner(http.MethodPut, linkFoldersPath(otherPeerNodeID)+"/4/enabled", `{"enabled":false}`); rec.Code != http.StatusOK {
		t.Errorf("届かない間のスイッチ: status = %d: %s", rec.Code, rec.Body)
	}
	if excluded, err := b.pairing.ExcludedFolders(ctx, 1); err != nil || !slices.Equal(excluded, []int64{4}) {
		t.Errorf("リンクへ送るオフのフォルダ = %v, %v, want [4]", excluded, err)
	}

	for _, tc := range []struct{ method, target, body string }{
		{http.MethodGet, linkFoldersPath(testUnknownNodeID), ""},
		{http.MethodPut, linkFoldersPath(otherPeerNodeID) + "/9/enabled", `{"enabled":false}`},
	} {
		rec := b.owner(tc.method, tc.target, tc.body)
		assertErrorBody(t, tc.method+" "+tc.target, rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusNotFound, code: codeNotFound})
	}
}

const testUnknownNodeID = "11111111-2222-4333-8444-555555555555"

// The reader's stream learns of A's own folder changes only: a switch, or the mediaFolders A's Pairing publishes
// for a link of A's, changes nothing A answers the reader.
func TestThePeerStreamReceivesOnlyTheReadNodesFolderChanges(t *testing.T) {
	ctx := context.Background()
	a, b := pairTestNodes(t)
	a.env.db.PublishTo(storePublisher{events: a.events})
	stream, _ := openEventsWith(t, a.env.handler, http.Header{"Authorization": {"VV-Peer " + b.linkSecret(t)}})
	nextEventNamed(t, stream, "peer")

	folder, err := a.env.db.Settings().AddMediaFolder(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	nextEventNamed(t, stream, "mediaFolders")

	if _, err := a.env.db.Settings().SetMediaFolderEnabled(ctx, folder.ID, false); err != nil {
		t.Fatal(err)
	}
	a.events.Handle(domain.MediaFoldersChanged{ViewOnly: true})
	// mediaFolders is written before peer in one delivery, so one still waiting would arrive before this marker.
	a.events.PeerStateChanged()
	for {
		event := nextEvent(t, stream)
		if event.name == "mediaFolders" {
			t.Fatal("スイッチかリンクの知らせで読む側の流れに mediaFolders が届いた")
		}
		if event.name == "peer" {
			break
		}
	}
}
