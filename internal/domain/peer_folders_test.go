package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestReconcilePeerFolders(t *testing.T) {
	before, now := time.Unix(100, 0), time.Unix(200, 0)
	current := []PeerMediaFolder{
		{FolderID: 1, Path: "/gone", Version: 1, Enabled: false, RefreshedAt: before},
		{FolderID: 2, Path: "/moved", Version: 1, Enabled: false, RefreshedAt: before},
		{FolderID: 3, Path: "/kept", Version: 4, Enabled: false, RefreshedAt: before},
		{FolderID: 4, Path: "/rescanned", Version: 1, Enabled: false, RefreshedAt: before},
	}
	listed := []PeerFolder{
		{ID: 5, Path: "/new", Version: 1},
		{ID: 3, Path: "/kept", Version: 4},
		{ID: 2, Path: "/moved-here", Version: 1},
		{ID: 4, Path: "/rescanned", Version: 2},
	}

	next, changed := ReconcilePeerFolders(current, listed, now)
	want := []PeerMediaFolder{
		{FolderID: 2, Path: "/moved-here", Version: 1, Enabled: true, RefreshedAt: now},
		{FolderID: 3, Path: "/kept", Version: 4, Enabled: false, RefreshedAt: now},
		{FolderID: 4, Path: "/rescanned", Version: 2, Enabled: true, RefreshedAt: now},
		{FolderID: 5, Path: "/new", Version: 1, Enabled: true, RefreshedAt: now},
	}
	if !changed || !reflect.DeepEqual(next, want) {
		t.Fatalf("照合 = %+v, changed = %v, want %+v と changed", next, changed, want)
	}

	// The same listing again changes nothing but the stamps.
	if again, changed := ReconcilePeerFolders(next, listed, now.Add(time.Minute)); changed || again[1].Enabled {
		t.Errorf("同じ一覧での照合: changed = %v, 3 = %+v, want 変化なしでオフのまま", changed, again[1])
	}
	// A folder that only left the listing is a change too.
	if _, changed := ReconcilePeerFolders(next, listed[:3], now); !changed {
		t.Error("フォルダが減っただけの照合: changed = false")
	}
}
