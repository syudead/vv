package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func linkFetchOf(t *testing.T, db *DB, key string) domain.LinkFetch {
	t.Helper()
	fetch, _, err := db.VirtualVideos().LinkFetch(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return fetch
}

// A video created from a link without a title is titled by the link, holds the link and starts its fetch; a fill
// then replaces that title and empty fields, and leaves the owner's description and the stored link alone.
func TestCreateFromLinkThenFillOnlyEmptyFields(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	link := "https://example.com/watch?v=1&utm_source=x"
	video, err := db.VirtualVideos().CreateFromLink(ctx, "", link)
	if err != nil {
		t.Fatalf("CreateFromLink: %v", err)
	}
	if video.Title != link || linkFetchOf(t, db, video.UserKey).State != domain.LinkFetchRunning {
		t.Fatalf("created = %q, %+v; want the link as title and a running fetch", video.Title, linkFetchOf(t, db, video.UserKey))
	}
	if _, err := db.VirtualVideos().SaveDetails(ctx, video.ID, video.UserKey, link,
		domain.WorkInfo{Description: "owner's notes", URLs: []string{link}}); err != nil {
		t.Fatal(err)
	}
	plugin := approvedPlugin(t, db, "example", "d1")
	title := "Fetched title"
	if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
		VideoID: video.ID, ContentKey: video.ContentKey, PluginID: plugin.ID, Digest: "d1",
		Generation: plugin.Generation, Title: &title, FillEmpty: true,
		Details: domain.FetchedWorkInfo{
			Description: "site text", URLs: []string{"https://example.com/other"},
			Dates: []domain.WorkDate{{Kind: domain.DatePublished, Date: "2024-03-01"}},
		},
		DetailFields: []domain.WorkInfoField{
			domain.WorkInfoFieldDescription, domain.DateField(domain.DatePublished), domain.WorkInfoFieldURLs,
		},
	}); err != nil {
		t.Fatalf("fill: %v", err)
	}
	got, err := db.Library().GetVideo(ctx, domain.AudienceOwner, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := db.WorkInfo().ByUserKeys(ctx, []string{video.UserKey})
	if err != nil {
		t.Fatal(err)
	}
	info := infos[video.UserKey]
	if got.Title != title || info.Description != "owner's notes" || !slices.Equal(info.URLs, []string{link}) ||
		!slices.Equal(info.Dates, []domain.WorkDate{{Kind: domain.DatePublished, Date: "2024-03-01"}}) {
		t.Fatalf("after the fill: title %q, info %+v; want the fetched title and date only", got.Title, info)
	}
}

// A title the owner typed, at creation or later, is never replaced by a fill.
func TestFillKeepsTheOwnersTitle(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	typed, err := db.VirtualVideos().CreateFromLink(ctx, "Typed", "https://example.com/1")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := db.VirtualVideos().CreateFromLink(ctx, "", "https://example.com/2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.VirtualVideos().SaveTitle(ctx, renamed.ID, renamed.UserKey, "Renamed"); err != nil {
		t.Fatal(err)
	}
	plugin := approvedPlugin(t, db, "example", "d1")
	for _, video := range []domain.Video{typed, renamed} {
		title := "Fetched"
		if _, err := db.Overrides().ApplyFetchedWorkInfo(ctx, domain.FetchedWorkInfoApply{
			VideoID: video.ID, ContentKey: video.ContentKey, PluginID: plugin.ID, Digest: "d1",
			Generation: plugin.Generation, Title: &title, FillEmpty: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for want, video := range map[string]domain.Video{"Typed": typed, "Renamed": renamed} {
		got, err := db.Library().GetVideo(ctx, domain.AudienceOwner, video.ID)
		if err != nil || got.Title != want {
			t.Errorf("title = %q, %v; want %q", got.Title, err, want)
		}
	}
}

// Finishing records the outcome only while the fetch runs, writes nothing for a deleted video, and a restart turns a
// running fetch into an interrupted failure.
func TestFinishLinkFetchAndInterruption(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	done, err := db.VirtualVideos().CreateFromLink(ctx, "", "https://example.com/1")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.VirtualVideos().FinishLinkFetch(ctx, done.UserKey, "vv.link", "Nothing found."); err != nil {
		t.Fatal(err)
	}
	if err := db.VirtualVideos().FinishLinkFetch(ctx, done.UserKey, "vv.link", ""); err != nil {
		t.Fatal(err)
	}
	if got := linkFetchOf(t, db, done.UserKey); got != (domain.LinkFetch{State: domain.LinkFetchFailed, Failure: "Nothing found.", Plugin: "vv.link"}) {
		t.Fatalf("link fetch = %+v; want the first outcome kept", got)
	}
	deleted, err := db.VirtualVideos().CreateFromLink(ctx, "", "https://example.com/2")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.VirtualVideos().Delete(ctx, deleted.ID, deleted.UserKey); err != nil {
		t.Fatal(err)
	}
	if err := db.VirtualVideos().FinishLinkFetch(ctx, deleted.UserKey, "vv.link", ""); err != nil {
		t.Fatalf("finish for a deleted video = %v; want nothing written", err)
	}
	running, err := db.VirtualVideos().CreateFromLink(ctx, "", "https://example.com/3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Library().RefreshSearchKeys(ctx); err != nil {
		t.Fatal(err)
	}
	if got := linkFetchOf(t, db, running.UserKey); got.State != domain.LinkFetchFailed || got.Failure != linkFetchInterrupted {
		t.Fatalf("after a restart = %+v; want an interrupted failure", got)
	}
}
