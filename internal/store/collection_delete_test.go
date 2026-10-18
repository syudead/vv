package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestDeleteCollectionsRemovesExistingTargetsAndKeepsVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, b := looseVideo(t, db, "ka"), looseVideo(t, db, "kb")
	child := createGroup(t, db, "Child", domain.GroupAdd{VideoIDs: []int64{a}})
	// Dissolving Child empties Parent first, so Parent must still count as a dissolved target.
	parent := createGroup(t, db, "Parent", domain.GroupAdd{GroupIDs: []int64{child}})
	kept := createGroup(t, db, "Kept", domain.GroupAdd{VideoIDs: []int64{b}})
	top := createSmartFolder(t, db, "Top", 0)
	nested := createSmartFolder(t, db, "Nested", top.ID)
	other := createSmartFolder(t, db, "Other", 0)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	got, err := db.Collections().DeleteCollections(ctx, domain.CollectionDelete{
		GroupIDs: []int64{child, 999, parent}, SmartFolderIDs: []int64{top.ID, 999, nested.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := (domain.CollectionsDeleted{DissolvedGroups: 2, DeletedSmartFolders: 2}); got != want {
		t.Fatalf("result = %+v, want %+v", got, want)
	}
	for id, wantExists := range map[int64]bool{child: false, parent: false, kept: true} {
		if groupExists(t, db, id) != wantExists {
			t.Errorf("group %d exists = %v, want %v", id, !wantExists, wantExists)
		}
	}
	if rows := smartFolderRows(t, db); !reflect.DeepEqual(rows, []string{smartFolderRow(other)}) {
		t.Errorf("smart folders = %v", rows)
	}
	for _, id := range []int64{a, b} {
		if _, err := db.Library().GetVideo(ctx, domain.AudienceOwner, id); err != nil {
			t.Errorf("video %d: %v", id, err)
		}
	}
	want := []domain.Event{
		domain.GroupsChanged{GroupIDs: []int64{child, parent}},
		domain.SmartFoldersChanged{IDs: []int64{top.ID, nested.ID, other.ID}},
	}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Errorf("events = %+v, want %+v", recorder.events, want)
	}
}
