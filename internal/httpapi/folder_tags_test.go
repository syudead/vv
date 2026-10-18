package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestFolderTagsForOwnerAndGuest(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	ctx := context.Background()
	db := env.db

	const folderTag = "フォルダの秘密"
	tag, err := db.Tags().CreateTag(ctx, folderTag)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "pub", nil); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}

	owner := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner))
	var found *gen.VideoTag
	for i := range owner.Tags {
		if owner.Tags[i].Id == tag.ID {
			found = &owner.Tags[i]
		}
	}
	if found == nil || found.Name != folderTag || found.Manual || !found.FromFolder {
		t.Errorf("所有者の a のタグ = %+v, want %s (fromFolder のみ)", owner.Tags, folderTag)
	}

	rec := env.get(f.videoPath("a", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストの詳細: status = %d: %s", rec.Code, rec.Body)
	}
	if guest := decode[gen.Video](t, rec); len(guest.Tags) != 0 {
		t.Errorf("ゲストの a のタグ = %+v, want 空", guest.Tags)
	}

	if page := decode[gen.VideoPage](t, env.get("/api/videos?query="+folderTag, f.owner)); page.Total != 2 {
		t.Errorf("所有者のフォルダ由来のタグ名での検索 = %d 件, want 2", page.Total)
	}
	if page := decode[gen.VideoPage](t, env.get("/api/videos?query="+folderTag)); page.Total != 0 {
		t.Errorf("ゲストのフォルダ由来のタグ名での検索が %d 件に当たった", page.Total)
	}
}
