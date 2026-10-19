package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func personPluginUpload(name, digest string, sources ...string) domain.WorkInfoPluginUpload {
	upload := pluginUpload(name, digest)
	upload.Manifest.Kind, upload.Manifest.Sources = domain.WorkInfoPluginKindPerson, sources
	return upload
}

func TestPersonPluginUploadStoresKindAndSources(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()

	added, _ := savePlugin(t, s, personPluginUpload("people", "d1", "site", "wiki"))
	listed, err := s.ListWorkInfoPlugins(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("List = %+v, %v", listed, err)
	}
	if got := listed[0]; got.Kind() != domain.WorkInfoPluginKindPerson ||
		!slices.Equal(got.Manifest.Sources, []string{"site", "wiki"}) || !got.AutoLookup {
		t.Fatalf("listed = %+v; want a person plugin with its sources and automatic lookups on", got)
	}

	if _, err := s.SetWorkInfoPluginAutoLookup(ctx, added.ID, false); err != nil {
		t.Fatalf("SetWorkInfoPluginAutoLookup: %v", err)
	}
	work, replaced := savePlugin(t, s, pluginUpload("people", "d2"))
	if !replaced || work.Kind() != domain.WorkInfoPluginKindWork || work.Manifest.Sources != nil || work.AutoLookup {
		t.Fatalf("work replacement = %+v; want the new kind, no sources and automatic lookups still off", work)
	}
	if _, err := s.SetWorkInfoPluginAutoLookup(ctx, added.ID, true); !errors.Is(err, domain.ErrWorkInfoPluginKindMismatch) {
		t.Fatalf("SetWorkInfoPluginAutoLookup on a work-info plugin err = %v, want ErrWorkInfoPluginKindMismatch", err)
	}
	person, _ := savePlugin(t, s, personPluginUpload("people", "d3", "wiki"))
	if person.Kind() != domain.WorkInfoPluginKindPerson || !slices.Equal(person.Manifest.Sources, []string{"wiki"}) ||
		person.AutoLookup {
		t.Fatalf("person replacement = %+v; want the new sources and automatic lookups still off", person)
	}
}
