package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
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

// answerRewrites are the rewriters that merged lists, folder reads, selection operations and forwarded writes apply
// to a whole peer answer outside remoteRewrites, by response schema. Code that decodes a peer answer rewrites it with
// one of these, so that this test reaches every id it rewrites.
var answerRewrites = map[string][]remoteRewrite{
	"VideoPage":                 {rewriteAs(rewriteRemoteVideoPage), rewriteAs(rewriteRemoteVideoListing)},
	"LibraryPage":               {rewriteAs(rewriteRemoteLibraryPage)},
	"FolderListing":             {rewriteAs(rewriteRemoteFolderListing)},
	"RootFolderListing":         {rewriteAs(rewriteRemoteRootFolders)},
	"GroupListPage":             {rewriteAs(rewriteRemoteGroupList)},
	"OrganizeResult":            {rewriteAs(rewriteRemoteOrganizeResult)},
	"RelatedVideos":             {rewriteAs(rewriteRemoteRelated)},
	"WatchHistoryPage":          {rewriteAs(rewriteRemoteWatchHistory)},
	"SmartFolderList":           {rewriteAs(rewriteRemoteSmartFolderList)},
	"SmartFolderSummary":        {rewriteAs(rewriteRemoteSmartFolderSummary)},
	"SmartFolderCards":          {rewriteAs(rewriteRemoteSmartFolderCards)},
	"SmartFolderDetail":         {rewriteAs(rewriteRemoteSmartFolderDetail)},
	"SmartFolderItemPage":       {rewriteAs(rewriteRemoteSmartFolderItemPage)},
	"PersonMergeResponse":       {rewriteAs(rewriteRemotePersonMerge)},
	"OrganizationMergeResponse": {rewriteAs(rewriteRemoteOrganizationMerge)},
	"VideoEditResponse":         {rewriteAs(rewriteRemoteVideoEditResponse)},
	"VideoTagsResponse":         {rewriteAs(rewriteRemoteVideoTagsResponse)},
	"VideoEditSummary":          {rewriteAs(rewriteRemoteEditSummary)},
	"VideoTagsSummary":          {rewriteAs(rewriteRemoteTagsSummary)},
	"WorkInfoFillTargetList":    {rewriteAs(rewriteRemoteFillTargets)},
}

// droppedIDs are the id fields a rewriter leaves out of the answer, as no id of this node can stand for them.
var droppedIDs = map[string][]string{
	// The undo entry is in the peer's activity log, not this node's.
	"FaceAppearanceMove":        {"/undoActivityId"},
	"FaceCandidateConfirmation": {"/undoActivityId"},
	// The grouping is edited on the node that holds the folder.
	"FolderListing": {"/folder/grouping/groupId", "/folders/*/grouping/groupId"},
	// A folder's rule by name is for the nodes that read it, which this node has already used.
	"SmartFolderCards":  {"/cards/*/peerRule/names/*/id"},
	"SmartFolderDetail": {"/smartFolder/peerRule/names/*/id"},
	// A request of this node never gives a link a remote part, and editOnPeer reports only the link's tags.
	"VideoEditResponse": {"/merged/*/personId", "/merged/*/undoActivityId", "/remote/failedVideoIds/*", "/skippedPersonIds/*"},
	"VideoTagsResponse": {"/remote/failedVideoIds/*"},
}

// Every id openapi.yaml declares in a rewritten answer comes back in this node's id space for its kind: a vocabulary
// entry joined by name, any other entity with the offset. A new id field fails here until its rewriter covers it;
// task generate refuses an id without a kind.
func TestRemoteRewritesCoverEveryResponseID(t *testing.T) {
	rewrites := map[string][]remoteRewrite{}
	for schema, rewrite := range remoteRewrites {
		rewrites[schema] = []remoteRewrite{rewrite}
	}
	for schema, list := range answerRewrites {
		rewrites[schema] = append(rewrites[schema], list...)
	}
	for _, schema := range slices.Sorted(maps.Keys(rewrites)) {
		ids := gen.ScreenResponseIDs[schema]
		for _, rewrite := range rewrites[schema] {
			if rewrite == nil {
				if len(ids) > 0 {
					t.Errorf("%s passes through as the peer sent it but carries ids %v", schema, ids)
				}
				continue
			}
			for _, problem := range rewrittenIDProblems(rewrite, schema, ids, droppedIDs[schema]) {
				t.Errorf("%s %s", schema, problem)
			}
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

	if got := rewrittenIDProblems(rewrite, "Tag", ids, nil); !slices.Equal(got, want) {
		t.Errorf("problems = %q, want %q", got, want)
	}
	got := rewrittenIDProblems(rewrite, "Tag", ids, []string{"/id", "/kind/id"})
	if want := []string{"/id is kept, though no id of this node stands for it"}; !slices.Equal(got, want) {
		t.Errorf("problems with dropped ids = %q, want %q", got, want)
	}
	if got, want := rewrittenIDProblems(rewrite, "Person", nil, nil), []string{"rewrites gen.Tag, not gen.Person"}; !slices.Equal(got, want) {
		t.Errorf("problems for another schema = %q, want %q", got, want)
	}
}

// rewrittenIDProblems rewrites an answer of schema that holds every id of ids and names each id the result does not
// carry in this node's id space, unless dropped lists it, and a rewriter registered for another schema.
func rewrittenIDProblems(rewrite remoteRewrite, schema string, ids []gen.ScreenID, dropped []string) []string {
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
	if name := reflect.TypeOf(value).Name(); name != schema {
		return []string{fmt.Sprintf("rewrites gen.%s, not gen.%s", name, schema)}
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
