package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// coverRuntime answers every fetch with work and one readable cover.
type coverRuntime struct {
	manifestRuntime
	work  domain.FetchedWorkInfo
	cover []byte
}

func (r coverRuntime) Run(
	context.Context, string, []byte, domain.WorkInfoRunRequest, domain.WorkInfoRunNetwork, func(domain.WorkInfoRunLog) error,
) (domain.WorkInfoRunResult, domain.WorkInfoRunLog, error) {
	return domain.WorkInfoRunResult{WorkInfo: r.work, CoverImages: [][]byte{r.cover}}, domain.WorkInfoRunLog{}, nil
}

type recordedUpload struct {
	videoID int64
	key     string
}

// A fetch for the peer's video runs this node's plugin and writes on the peer: the title, the tentative tag, the
// performer matched to the peer's person and the cover, while this node gains no person.
func TestRemoteVideoWorkInfoFetchAppliesOnThePeer(t *testing.T) {
	var cover bytes.Buffer
	if err := png.Encode(&cover, image.NewGray(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	runtime := coverRuntime{cover: cover.Bytes(), work: domain.FetchedWorkInfo{
		Title: "遠くの作品", Tags: []string{"レム"}, CoverURLs: []string{"https://shop.test/cover.png"},
		People: []domain.FetchedPerson{{Role: domain.PersonPerformer, Name: "早美れむ"}},
	}}
	uploads := map[*store.DB][]recordedUpload{}
	fa, fb, _, b, _ := mergedNodesWith(t, func(db *store.DB, opts *Options) {
		library := db.Library()
		opts.WorkInfoPlugins = app.NewWorkInfoPlugins(app.WorkInfoPluginsOptions{
			Store: db.WorkInfoPlugins(), Runtime: runtime, Videos: library, Applier: db.Overrides(), Matcher: db.Credits(),
		})
		opts.ThumbnailUploads = thumbnailUploadFunc(func(ctx context.Context, id int64, key string, _ []byte) (domain.Video, error) {
			uploads[db] = append(uploads[db], recordedUpload{videoID: id, key: key})
			return library.GetVideo(ctx, domain.AudienceOwner, id)
		})
	})
	ctx := context.Background()
	performer, err := fa.env.db.Credits().Create(ctx, domain.CreditPerson, domain.CreditInput{Name: "早美れむ"})
	if err != nil {
		t.Fatal(err)
	}
	pluginID := approvedFetchPlugin(t, fb.env.db)
	remote := domain.ToRemoteID(fa.ids["a"])
	path := fmt.Sprintf("/api/videos/%d/work-info-fetches", remote)

	rec := b.owner(http.MethodPost, path, fmt.Sprintf(`{"pluginId":%d,"id":"W-1"}`, pluginID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("fetch: status = %d: %s", rec.Code, rec.Body)
	}
	fetched := decode[gen.WorkInfoFetch](t, rec)
	if match := fetched.People[0].Match; match.State != gen.WorkInfoMatchStateExisting || match.Id == nil ||
		*match.Id != domain.ToRemoteID(performer.ID) {
		t.Errorf("performer match = %+v, want the peer's person %d", match, domain.ToRemoteID(performer.ID))
	}
	if rec := b.owner(http.MethodGet, fmt.Sprintf("%s/%s/covers/0", path, fetched.FetchId), ""); rec.Code != http.StatusOK {
		t.Errorf("cover: status = %d: %s", rec.Code, rec.Body)
	}

	rec = b.owner(http.MethodPost, fmt.Sprintf("%s/%s/apply", path, fetched.FetchId),
		`{"fields":["title","tags","cover"],"people":[0],"coverIndex":0}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: status = %d: %s", rec.Code, rec.Body)
	}
	applied := decode[gen.WorkInfoFetchApplied](t, rec)
	if applied.Video.Id != remote || applied.Video.Title != "遠くの作品" || len(applied.LinkedPeople) != 0 {
		t.Errorf("apply answer: id = %d, title = %q, linked = %v, want %d, 遠くの作品 and none",
			applied.Video.Id, applied.Video.Title, applied.LinkedPeople, remote)
	}
	onA := decode[gen.Video](t, fa.env.get(fa.videoPath("a", ""), fa.owner))
	tentative := slices.ContainsFunc(onA.Tags, func(tag gen.VideoTag) bool { return tag.Name == "レム" && tag.Tentative })
	performers := []int64{}
	if onA.WorkInfo != nil {
		for _, p := range onA.WorkInfo.People {
			performers = append(performers, p.Person.Id)
		}
	}
	if onA.Title != "遠くの作品" || !tentative || !slices.Equal(performers, []int64{performer.ID}) {
		t.Errorf("a's video: title = %q, tentative レム = %v, people = %v, want 遠くの作品, true and [%d]",
			onA.Title, tentative, performers, performer.ID)
	}
	aVideo, err := fa.env.db.Library().GetVideo(ctx, domain.AudienceOwner, fa.ids["a"])
	if err != nil {
		t.Fatal(err)
	}
	want := []recordedUpload{{videoID: fa.ids["a"], key: aVideo.ContentKey}}
	if !slices.Equal(uploads[fa.env.db], want) || len(uploads[fb.env.db]) != 0 {
		t.Errorf("cover uploads: a = %v, b = %v, want a = %v and none on b", uploads[fa.env.db], uploads[fb.env.db], want)
	}
	matches, err := fb.env.db.Credits().Match(ctx, domain.CreditPerson, []domain.CreditMatchEntry{{Name: "早美れむ"}})
	if err != nil || matches[0].State != domain.CreditMatchNew {
		t.Errorf("b's people: match = %+v, %v, want no person 早美れむ on b", matches, err)
	}
}

// approvedFetchPlugin saves and approves a fetch plugin, whose runs the node's runtime answers.
func approvedFetchPlugin(t *testing.T, db *store.DB) int64 {
	t.Helper()
	ctx := context.Background()
	plugins := db.WorkInfoPlugins()
	saved, _, err := plugins.SaveWorkInfoPlugin(ctx, domain.WorkInfoPluginUpload{
		FileName: "works.wasm", Module: []byte("module"), Digest: "digest",
		Manifest: domain.WorkInfoPluginManifest{
			Contract: 1, Name: "example.works", Version: "1.0.0",
			Operations: []domain.WorkInfoOperation{domain.WorkInfoFetch},
		},
	}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plugins.ApproveWorkInfoPlugin(ctx, saved.ID, "digest", time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	return saved.ID
}
