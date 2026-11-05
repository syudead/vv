package app

import (
	"context"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/syudead/vv/internal/domain"
)

// The snapshot is read when the stream opens and again on the link's mediaFolders, which reaches the screens as
// a view-only MediaFoldersChanged instead of a forwarded PeerChanged.
func TestConnectionRefreshesTheLinkFoldersOnOpenAndOnTheirEvent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		atOpen := []domain.PeerFolder{{ID: 1, Path: "/a", Version: 1}, {ID: 2, Path: "/b", Version: 1}}
		afterEvent := []domain.PeerFolder{{ID: 2, Path: "/b", Version: 1}, {ID: 3, Path: "/c", Version: 1}}
		link := newStreamLink(watchStep{open: true, changed: &domain.PeerChanged{MediaFolders: true}})
		link.folders = [][]domain.PeerFolder{atOpen, afterEvent}
		node := startConnection(t, link)

		if got := node.store.storedListings(); !reflect.DeepEqual(got, [][]domain.PeerFolder{atOpen, afterEvent}) {
			t.Fatalf("照合した一覧 = %+v, want 開いたときとイベントの 2 回", got)
		}
		folders, err := node.pairing.LinkFolders(context.Background(), connectionPeerID)
		if err != nil {
			t.Fatal(err)
		}
		if folders.State != domain.PeerConnected || len(folders.Folders) != 2 || folders.Folders[0].FolderID != 2 ||
			folders.Folders[1].FolderID != 3 {
			t.Errorf("写し = %+v, want connected で 2 と 3", folders)
		}
		node.events.mu.Lock()
		published, forwarded := node.events.folders, node.events.changed
		node.events.mu.Unlock()
		want := []domain.MediaFoldersChanged{{ViewOnly: true}, {ViewOnly: true}}
		if !reflect.DeepEqual(published, want) || len(forwarded) != 0 {
			t.Errorf("知らせ = %+v, 転送 = %+v, want ViewOnly の mediaFolders を 2 回だけ", published, forwarded)
		}
	})
}
