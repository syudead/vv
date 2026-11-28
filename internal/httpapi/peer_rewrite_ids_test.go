package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Each id the sample answer carries is sampleID, and each object holding an id is named sampleName, which the join
// holds under a distinct local id per vocabulary kind.
const (
	sampleID   = 7
	sampleName = "N"
)

var sampleJoin = domain.VocabularyJoin{
	Slot:          1,
	Tags:          map[string]int64{sampleName: 101},
	TagKinds:      map[string]int64{sampleName: 102},
	People:        map[string]int64{sampleName: 103},
	Organizations: map[string]int64{sampleName: 104},
}

// joinedKinds are the kinds the screens join by name; an id of any other kind takes the offset.
var joinedKinds = map[string]int64{"tag": 101, "tagKind": 102, "person": 103, "organization": 104}

// answerRewrites are the rewriters merged lists and folder reads apply to a whole answer, outside remoteRewrites.
var answerRewrites = map[string]remoteRewrite{
	"VideoPage":      rewriteAs(rewriteRemoteVideoPage),
	"FolderListing":  rewriteAs(rewriteRemoteFolderListing),
	"GroupListPage":  rewriteAs(rewriteRemoteGroupList),
	"OrganizeResult": rewriteAs(rewriteRemoteOrganizeResult),
	"RelatedVideos":  rewriteAs(rewriteRemoteRelated),
}

// droppedIDs are the id fields a rewriter leaves out of the answer, as no id of this node can stand for them.
var droppedIDs = map[string][]string{
	// The undo entry is in the peer's activity log, not this node's.
	"FaceAppearanceMove":        {"/undoActivityId"},
	"FaceCandidateConfirmation": {"/undoActivityId"},
	// The grouping is edited on the node that holds the folder.
	"FolderListing": {"/folder/grouping/groupId", "/folders/*/grouping/groupId"},
}

// Every id openapi.yaml declares in a rewritten answer comes back in this node's id space for its kind: a vocabulary
// entry joined by name, any other entity with the offset. A new id field fails here until its rewriter covers it.
func TestRemoteRewritesCoverEveryResponseID(t *testing.T) {
	rewrites := maps.Clone(remoteRewrites)
	maps.Copy(rewrites, answerRewrites)
	for _, schema := range slices.Sorted(maps.Keys(rewrites)) {
		ids := gen.ScreenResponseIDs[schema]
		if rewrites[schema] == nil {
			if len(ids) > 0 {
				t.Errorf("%s passes through as the peer sent it but carries ids %v", schema, ids)
			}
			continue
		}
		for _, id := range ids {
			if id.Kind == "" {
				t.Errorf("%s %s: no x-vv-entity names its kind", schema, id.Field)
			}
		}
		for _, problem := range rewrittenIDProblems(rewrites[schema], ids, droppedIDs[schema]) {
			t.Errorf("%s %s", schema, problem)
		}
	}
}

// The check names an id joined as the wrong kind, one the rewriter drops, and one it keeps though listed as dropped.
func TestRewrittenIDProblemsNamesEachUncoveredID(t *testing.T) {
	rewrite := rewriteAs(func(tag *gen.Tag, join domain.VocabularyJoin) {
		tag.Id = join.Person(tag.Id, tag.Name)
		tag.Kind = nil
	})
	ids := []gen.ScreenID{{Field: "/id", Kind: "tag"}, {Field: "/kind/id", Kind: "tagKind"}}
	want := []string{"/id = 103, want 101", "/kind/id is left out of the answer"}

	if got := rewrittenIDProblems(rewrite, ids, nil); !slices.Equal(got, want) {
		t.Errorf("problems = %q, want %q", got, want)
	}
	got := rewrittenIDProblems(rewrite, ids, []string{"/id", "/kind/id"})
	if want := []string{"/id is kept, though no id of this node stands for it"}; !slices.Equal(got, want) {
		t.Errorf("problems with dropped ids = %q, want %q", got, want)
	}
}

// rewrittenIDProblems rewrites an answer that holds every id of ids and names each id the result does not carry in
// this node's id space, unless dropped lists it.
func rewrittenIDProblems(rewrite remoteRewrite, ids []gen.ScreenID, dropped []string) []string {
	var sample any
	for _, id := range ids {
		sample = withSampleID(sample, strings.Split(id.Field, "/")[1:])
	}
	body, err := json.Marshal(sample)
	if err != nil {
		return []string{err.Error()}
	}
	value, err := rewrite(body, sampleJoin)
	if err != nil {
		return []string{"cannot decode the sample answer: " + err.Error()}
	}
	out, err := json.Marshal(value)
	if err != nil {
		return []string{err.Error()}
	}
	decoder := json.NewDecoder(bytes.NewReader(out))
	decoder.UseNumber()
	var answer any
	if err := decoder.Decode(&answer); err != nil {
		return []string{err.Error()}
	}
	var problems []string
	for _, id := range ids {
		segments := strings.Split(id.Field, "/")[1:]
		switch {
		case !slices.Contains(dropped, id.Field):
			problems = append(problems, idProblems(answer, id, segments)...)
		case carries(answer, segments):
			problems = append(problems, id.Field+" is kept, though no id of this node stands for it")
		}
	}
	return problems
}

func carries(node any, segments []string) bool {
	if len(segments) == 0 {
		return node != nil
	}
	if segments[0] == "*" {
		items, _ := node.([]any)
		return slices.ContainsFunc(items, func(item any) bool { return carries(item, segments[1:]) })
	}
	object, _ := node.(map[string]any)
	return carries(object[segments[0]], segments[1:])
}

// withSampleID sets sampleID at the pointer segments of node, one item for each "*", and names each object that
// holds an id.
func withSampleID(node any, segments []string) any {
	if len(segments) == 0 {
		return sampleID
	}
	if segments[0] == "*" {
		items, _ := node.([]any)
		if len(items) == 0 {
			items = []any{nil}
		}
		items[0] = withSampleID(items[0], segments[1:])
		return items
	}
	object, _ := node.(map[string]any)
	if object == nil {
		object = map[string]any{}
	}
	if len(segments) == 1 || (len(segments) == 2 && segments[1] == "*") {
		object["name"] = sampleName
	}
	object[segments[0]] = withSampleID(object[segments[0]], segments[1:])
	return object
}

// idProblems checks every value at the pointer segments of node against the id's kind.
func idProblems(node any, id gen.ScreenID, segments []string) []string {
	if segments[0] == "*" {
		items, _ := node.([]any)
		if len(segments) == 1 {
			return idValueProblems(id, items, node)
		}
		var problems []string
		for _, item := range items {
			problems = append(problems, idProblems(item, id, segments[1:])...)
		}
		return problems
	}
	object, _ := node.(map[string]any)
	value, found := object[segments[0]]
	switch {
	case !found || value == nil:
		return []string{id.Field + " is left out of the answer"}
	case len(segments) > 1:
		return idProblems(value, id, segments[1:])
	}
	return idValueProblems(id, []any{value}, object)
}

// idValueProblems compares the ids the answer carries with the one the kind gives sampleID: the local id of the
// sample name when the holding object has that name and the kind is joined, otherwise the offset.
func idValueProblems(id gen.ScreenID, values []any, holder any) []string {
	want := domain.RemoteID(sampleJoin.Slot, sampleID)
	if local, joined := joinedKinds[id.Kind]; joined {
		if object, _ := holder.(map[string]any); object["name"] == sampleName {
			want = local
		}
	}
	if len(values) == 0 {
		return []string{id.Field + " is left out of the answer"}
	}
	var problems []string
	for _, value := range values {
		if got := fmt.Sprint(value); got != fmt.Sprint(want) {
			problems = append(problems, fmt.Sprintf("%s = %s, want %d", id.Field, got, want))
		}
	}
	return problems
}
