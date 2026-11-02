package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// The snapshot keeps an unchanged row's switch across reconciliations, refuses a link that is gone, and is
// deleted with its link.
func TestPeerFolderSnapshotLivesWithItsLink(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	peers := db.Peers()
	listed := []domain.PeerFolder{{ID: 4, Path: "/b", Version: 1}, {ID: 2, Path: "/a", Version: 3}}
	if _, err := peers.ReplacePeerFolders(ctx, testNodeID, listed, authNow); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("リンクの無い照合: err = %v, want ErrNotFound", err)
	}
	addTestLink(t, db)
	if changed, err := peers.ReplacePeerFolders(ctx, testNodeID, listed, authNow); err != nil || !changed {
		t.Fatalf("最初の照合: changed = %v, err = %v", changed, err)
	}
	folder, changed, err := peers.SetPeerFolderEnabled(ctx, testNodeID, 4, false)
	if err != nil || !changed || folder.Enabled || folder.Path != "/b" {
		t.Fatalf("スイッチ: %+v, changed = %v, err = %v", folder, changed, err)
	}
	if _, _, err := peers.SetPeerFolderEnabled(ctx, testNodeID, 9, false); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("写しに無いフォルダのスイッチ: err = %v, want ErrNotFound", err)
	}

	later := authNow.Add(time.Hour)
	if changed, err := peers.ReplacePeerFolders(ctx, testNodeID, listed, later); err != nil || changed {
		t.Fatalf("同じ一覧の照合: changed = %v, err = %v", changed, err)
	}
	rows, err := peers.PeerFolders(ctx, testNodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].FolderID != 2 || !rows[0].Enabled || rows[1].FolderID != 4 || rows[1].Enabled ||
		!rows[1].RefreshedAt.Equal(later) {
		t.Fatalf("写し = %+v, want id 順に 2 オン、4 オフ、照合の時刻", rows)
	}

	if _, err := peers.DeleteLink(ctx, testNodeID); err != nil {
		t.Fatal(err)
	}
	if rows, err := peers.PeerFolders(ctx, testNodeID); err != nil || len(rows) != 0 {
		t.Errorf("リンクを外した後の写し = %+v, %v, want 空", rows, err)
	}
}
