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
		Code: "CTNOZ-148", Description: "紹介文", Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: "2022-04-01"}},
		People:        []domain.FetchedPerson{{Role: domain.PersonPerformer, Name: "早美れむ", Profile: domain.PersonProfile{HeightCm: new(150)}}},
		Organizations: []domain.FetchedOrganization{{Role: domain.OrganizationMaker, Name: "二代目つば飲みおじさん"}},
		URLs:          []string{"https://shop.test/w/1"},
		ExternalIDs:   []domain.ExternalID{{Source: "fanza", Value: "ctnoz148"}},
	}}
	fa, fb, b, uploads := workInfoNodes(t, runtime)
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
		`{"fields":["title","tags","cover","code","description","dates.release"],"people":[0],`+
			`"organizations":[0],"urls":["https://shop.test/w/1"],"externalIds":[{"source":"fanza","value":"ctnoz148"}],"coverIndex":0}`)
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
	assertPeerWorkDetails(t, onA.WorkInfo)
	if saved, err := fa.env.db.Credits().Get(ctx, domain.CreditPerson, performer.ID); err != nil ||
		saved.Profile.HeightCm == nil || *saved.Profile.HeightCm != 150 {
		t.Errorf("a's performer profile = %+v, %v, want heightCm 150", saved.Profile, err)
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

// A fill over a selection with the peer's videos lists this node's targets, then the peer's in their remote ids.
func TestWorkInfoFillTargetsListThePeersVideos(t *testing.T) {
	fa, fb, b, _ := workInfoNodes(t, coverRuntime{})
	remote := domain.ToRemoteID(fa.ids["a"])
	rec := b.owner(http.MethodPost, "/api/work-info-fill-targets", fmt.Sprintf(`{"videoIds":[%d,%d]}`, remote, fb.ids["b"]))
	if rec.Code != http.StatusOK {
		t.Fatalf("fill targets: status = %d: %s", rec.Code, rec.Body)
	}
	want := []gen.WorkInfoFillTarget{{VideoId: fb.ids["b"], Title: "b", Query: "b"}, {VideoId: remote, Title: "a", Query: "a"}}
	if got := decode[gen.WorkInfoFillTargetList](t, rec).Targets; !slices.Equal(got, want) {
		t.Errorf("targets = %+v, want %+v", got, want)
	}
}

// workInfoNodes pairs two nodes with real work-info storage whose plugin runs runtime answers; uploads records each
// node's thumbnail uploads by its database.
func workInfoNodes(t *testing.T, runtime coverRuntime) (fa, fb *guestFixture, b *peerNode, uploads map[*store.DB][]recordedUpload) {
	t.Helper()
	uploads = map[*store.DB][]recordedUpload{}
	fa, fb, _, b, _ = mergedNodesWith(t, func(db *store.DB, opts *Options) {
		library := db.Library()
		opts.WorkInfoPlugins = app.NewWorkInfoPlugins(app.WorkInfoPluginsOptions{
			Store: db.WorkInfoPlugins(), Runtime: runtime, Videos: library, Applier: db.Overrides(), Matcher: db.Credits(),
			FillTargets: db.Overrides(),
		})
		opts.ThumbnailUploads = thumbnailUploadFunc(func(ctx context.Context, id int64, key string, _ []byte) (domain.Video, error) {
			uploads[db] = append(uploads[db], recordedUpload{videoID: id, key: key})
			return library.GetVideo(ctx, domain.AudienceOwner, id)
		})
	})
	return fa, fb, b, uploads
}

// assertPeerWorkDetails checks that every chosen detail reached the peer's video unchanged.
func assertPeerWorkDetails(t *testing.T, info *gen.WorkInfo) {
	t.Helper()
	if info == nil {
		t.Fatal("a's video has no work information")
	}
	var makers []string
	for _, o := range info.Organizations {
		makers = append(makers, string(o.Role)+":"+o.Organization.Name)
	}
	got := fmt.Sprintf("%v|%v|%v|%v|%v|%v", valueOrEmpty(info.Code), valueOrEmpty(info.Description), info.Dates, makers,
		info.Urls, info.ExternalIds)
	want := fmt.Sprintf("%v|%v|%v|%v|%v|%v", "CTNOZ-148", "紹介文", []gen.WorkDate{{Kind: gen.Release, Date: "2022-04-01"}},
		[]string{"maker:二代目つば飲みおじさん"}, []string{"https://shop.test/w/1"}, []gen.ExternalId{{Source: "fanza", Value: "ctnoz148"}})
	if got != want {
		t.Errorf("a's work information = %s, want %s", got, want)
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
