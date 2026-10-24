package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func (s *runPluginStore) ListWorkInfoPlugins(context.Context) ([]domain.WorkInfoPlugin, error) {
	var out []domain.WorkInfoPlugin
	for _, id := range slices.Sorted(func(yield func(int64) bool) {
		for id := range s.plugins {
			if !yield(id) {
				return
			}
		}
	}) {
		out = append(out, *s.plugins[id])
	}
	return out, nil
}

type finishedFetch struct{ key, plugin, failure string }

type recordingLinkStore struct {
	finished []finishedFetch
	// recorded, when set, receives each outcome after it is recorded.
	recorded chan struct{}
}

func (s *recordingLinkStore) FinishLinkFetch(_ context.Context, key, plugin, failure string) error {
	s.finished = append(s.finished, finishedFetch{key, plugin, failure})
	if s.recorded != nil {
		s.recorded <- struct{}{}
	}
	return nil
}

// newLinkEnv makes plugin 1 handle example.com first and plugin 2 every link after it.
func newLinkEnv() (*fetchEnv, *recordingLinkStore) {
	e := newFetchEnv()
	e.store.plugins[1].Manifest.Domains, e.store.plugins[1].LinkRank = []string{"example.com"}, 1
	e.store.plugins[2].Manifest.Domains, e.store.plugins[2].LinkRank = []string{domain.AnyLinkDomain}, 2
	links := &recordingLinkStore{}
	e.plugins.links = &linkFetches{store: links, logger: discardLogger()}
	return e, links
}

var linkVideo = domain.Video{ID: fetchVideo, ContentKey: "virtual:1"}

// The first plugin in link order fills only by link, with every fetched field offered as a fill of empty fields.
func TestFillFromLinkRunsTheFirstMatchingPluginAsAFill(t *testing.T) {
	e, _ := newLinkEnv()
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: domain.FetchedWorkInfo{
		Title: "Fetched", Description: "notes", Dates: []domain.WorkDate{{Kind: domain.DatePublished, Date: "2024-01-02"}},
	}}
	plugin, failure := e.plugins.fillFromLink(context.Background(), linkVideo, "https://www.example.com/v/1")
	if plugin != "one" || failure != "" || len(e.runtime.requests) != 1 || e.runtime.requests[0].URL != "https://www.example.com/v/1" {
		t.Fatalf("fill = %q, %q, requests %+v; want plugin one by link", plugin, failure, e.runtime.requests)
	}
	apply := e.applier.calls[0]
	want := []domain.WorkInfoField{domain.WorkInfoFieldDescription, domain.DateField(domain.DatePublished)}
	if !apply.FillEmpty || apply.Title == nil || *apply.Title != "Fetched" || !slices.Equal(apply.DetailFields, want) ||
		apply.ContentKey != "virtual:1" {
		t.Fatalf("apply = %+v; want a fill of the title, description and date", apply)
	}
}

// A failing plugin hands the link to the next; when every plugin fails the first failure is reported, and a link
// no usable plugin handles runs nothing.
func TestFillFromLinkFallsBackAndReportsTheFirstFailure(t *testing.T) {
	e, _ := newLinkEnv()
	e.runtime.err = &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed, PluginMessage: "login required"}
	plugin, failure := e.plugins.fillFromLink(context.Background(), linkVideo, "https://example.com/v/1")
	if plugin != "two" || failure != "one: login required" || len(e.runtime.requests) != 2 {
		t.Fatalf("fill = %q, %q after %d runs; want both tried and the first failure", plugin, failure, len(e.runtime.requests))
	}
	e.store.plugins[2].ApprovedDigest = ""
	e.runtime.requests = nil
	if _, failure := e.plugins.fillFromLink(context.Background(), linkVideo, "https://other.test/v"); failure == "" ||
		len(e.runtime.requests) != 0 {
		t.Fatalf("unhandled link = %q after %d runs; want a failure without a run", failure, len(e.runtime.requests))
	}
}

// A video deleted or merged during the fetch ends it without a failure.
func TestFillFromLinkIgnoresAVideoGoneSinceCreation(t *testing.T) {
	e, _ := newLinkEnv()
	e.applier.err = domain.ErrNotFound
	if _, failure := e.plugins.fillFromLink(context.Background(), linkVideo, "https://example.com/v/1"); failure != "" {
		t.Fatalf("failure = %q; want none for a video gone since", failure)
	}
}

// StartLinkFetch queues the fill behind the request and records its outcome.
func TestStartLinkFetchRecordsTheOutcome(t *testing.T) {
	e, links := newLinkEnv()
	links.recorded = make(chan struct{}, 1)
	e.plugins.StartLinkFetches(context.Background(), links, discardLogger())
	defer e.plugins.Close()
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: domain.FetchedWorkInfo{Title: "Fetched"}}
	e.plugins.StartLinkFetch(linkVideo, "https://example.com/v/1")
	<-links.recorded
	if !slices.Equal(links.finished, []finishedFetch{{"virtual:1", "one", ""}}) {
		t.Fatalf("finished = %+v; want one done fetch", links.finished)
	}
}

// A link outside the plugin's domains is refused before any run.
func TestFetchWorkInfoByLinkChecksTheDomains(t *testing.T) {
	e, _ := newLinkEnv()
	if _, err := e.plugins.FetchWorkInfoByLink(context.Background(), fetchVideo, 1, "https://other.test/v"); !errors.Is(err, domain.ErrWorkInfoLinkUnsupported) {
		t.Fatalf("err = %v; want ErrWorkInfoLinkUnsupported", err)
	}
	if _, err := e.plugins.FetchWorkInfoByLink(context.Background(), fetchVideo, 1, "example.com/v"); !errors.Is(err, domain.ErrWorkInfoRunInvalid) {
		t.Fatalf("err = %v; want ErrWorkInfoRunInvalid", err)
	}
	if _, err := e.plugins.FetchWorkInfoByLink(context.Background(), fetchVideo, 1, "https://example.com/v"); err != nil ||
		e.runtime.requests[0].URL != "https://example.com/v" {
		t.Fatalf("err = %v, requests %+v; want a fetch by link", err, e.runtime.requests)
	}
}

// A fill skips a cover whose header names more than linkCoverMaxPixels and applies the next one.
func TestFillCoverSkipsCoversTooLargeToDecodeUnseen(t *testing.T) {
	e, _ := newLinkEnv()
	covers := &fakeCovers{}
	e.plugins.covers = covers
	huge := domain.WorkInfoCover{Data: []byte("huge"), Width: 12_000, Height: 10_000}
	small := domain.WorkInfoCover{Data: []byte("small"), Width: 1280, Height: 720}
	e.plugins.fillCover(context.Background(), linkVideo, []domain.WorkInfoCover{huge, small})
	if len(covers.images) != 1 || string(covers.images[0]) != "small" {
		t.Fatalf("applied %q; want only the small cover", covers.images)
	}
	covers.images = nil
	e.plugins.fillCover(context.Background(), linkVideo, []domain.WorkInfoCover{huge})
	if len(covers.images) != 0 {
		t.Fatalf("applied %q; want nothing", covers.images)
	}
}
