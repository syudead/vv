package httpapi

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// tagNamesOf lists the names of the tags of n's own video id.
func tagNamesOf(t *testing.T, n *peerNode, id int64) []string {
	t.Helper()
	rec := n.owner(http.MethodGet, "/api/videos/"+strconv.FormatInt(id, 10), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("video %d: status = %d: %s", id, rec.Code, rec.Body)
	}
	got := decode[gen.Video](t, rec)
	names := []string{}
	for _, tag := range got.Tags {
		names = append(names, tag.Name)
	}
	return names
}

func updateVideoTags(t *testing.T, n *peerNode, body string) gen.VideoTagsResponse {
	t.Helper()
	rec := n.owner(http.MethodPost, "/api/video-tags", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("video-tags %s: status = %d: %s", body, rec.Code, rec.Body)
	}
	return decode[gen.VideoTagsResponse](t, rec)
}

func TestTaggingAMixedSelectionTagsThePeersVideosByName(t *testing.T) {
	fa, fb, a, b, _ := mergedNodesWith(t, nil)
	star := tagOn(t, fb, "星")

	got := updateVideoTags(t, b, fmt.Sprintf(`{"action":"add","tag":{"id":%d},"videoIds":[%d,%d]}`,
		star.ID, fb.ids["a"], domain.ToRemoteID(fa.ids["a"])))
	if got.Applied != 2 || got.Tag.Id != star.ID || got.Remote == nil || got.Remote.Applied != 1 || len(got.Remote.FailedVideoIds) != 0 {
		t.Errorf("response = %+v, remote %+v; want 2 applied, tag %d, 1 applied on the peer", got, got.Remote, star.ID)
	}
	for node, n := range map[string]struct {
		node *peerNode
		id   int64
	}{"a": {a, fa.ids["a"]}, "b": {b, fb.ids["a"]}} {
		if names := tagNamesOf(t, n.node, n.id); !slices.Contains(names, "星") {
			t.Errorf("%s's video a has tags %v, want 星 among them", node, names)
		}
	}

	// The fixture tags every video of both nodes with 秘密の名前, which joins by name like 星 now does.
	rec := b.owner(http.MethodPost, "/api/video-tags/summary", fmt.Sprintf(`{"videoIds":[%d,%d,%d]}`,
		fb.ids["a"], fb.ids["b"], domain.ToRemoteID(fa.ids["a"])))
	if rec.Code != http.StatusOK {
		t.Fatalf("summary: status = %d: %s", rec.Code, rec.Body)
	}
	summary := decode[gen.VideoTagsSummary](t, rec)
	counts := map[string]int{}
	for _, item := range summary.Items {
		if domain.IsRemoteID(item.Tag.Id) {
			t.Errorf("tag %s keeps the peer's id %d", item.Tag.Name, item.Tag.Id)
		}
		counts[item.Tag.Name] += item.Count
	}
	if want := map[string]int{"星": 2, "秘密の名前": 3}; summary.Total != 3 || !maps.Equal(counts, want) {
		t.Errorf("summary = %+v, want total 3 and counts %v", summary, want)
	}
	if rec := a.owner(http.MethodGet, "/api/tags?q=星", ""); len(decode[gen.TagList](t, rec).Items) != 1 {
		t.Errorf("a's tags named 星: %s, want one", rec.Body)
	}
}

func TestSelectionKeepsTheLocalPartWhenThePeerStops(t *testing.T) {
	fa, fb, _, b, stopA := mergedNodesWith(t, nil)
	stopA()

	remote := domain.ToRemoteID(fa.ids["a"])
	got := updateVideoTags(t, b, fmt.Sprintf(`{"action":"add","tag":{"name":"星"},"videoIds":[%d,%d]}`, fb.ids["a"], remote))
	if got.Applied != 1 || got.Remote == nil || got.Remote.Applied != 0 || !slices.Equal(got.Remote.FailedVideoIds, []int64{remote}) ||
		got.Remote.Reason == nil || *got.Remote.Reason != gen.ErrorReasonPeerUnreachable {
		t.Errorf("response = %+v, remote %+v; want 1 applied and [%d] failed with peer_unreachable", got, got.Remote, remote)
	}
	if names := tagNamesOf(t, b, fb.ids["a"]); !slices.Contains(names, "星") {
		t.Errorf("b's video a has tags %v, want 星 among them", names)
	}
}

func TestSmartFolderTargetExpandsToBothNodesMatches(t *testing.T) {
	fa, fb, a, b, _ := smartFolderNodes(t)
	cat := tagOn(t, fb, "猫", "b")
	peerCat := tagOn(t, fa, "猫", "c")
	local := createSmartFolder(t, b, fmt.Sprintf(`{"name":"猫","rule":{"tags":[%d]}}`, cat.ID))
	remote := domain.ToRemoteID(createSmartFolder(t, a, fmt.Sprintf(`{"name":"猫","rule":{"tags":[%d]}}`, peerCat.ID)))

	for _, tc := range []struct {
		name   string
		folder int64
		tag    string
	}{{"this node's folder", local, "白"}, {"the peer's folder", remote, "黒"}} {
		got := updateVideoTags(t, b, fmt.Sprintf(`{"action":"add","tag":{"name":"%s"},"videoIds":[],"smartFolderIds":[%d]}`, tc.tag, tc.folder))
		if got.Applied != 2 || got.Remote == nil || got.Remote.Applied != 1 {
			t.Errorf("%s: response = %+v, remote %+v; want 2 applied, 1 on the peer", tc.name, got, got.Remote)
		}
		if !slices.Contains(tagNamesOf(t, b, fb.ids["b"]), tc.tag) || !slices.Contains(tagNamesOf(t, a, fa.ids["c"]), tc.tag) {
			t.Errorf("%s: b:b has %v and a:c has %v, want %s on both", tc.name, tagNamesOf(t, b, fb.ids["b"]), tagNamesOf(t, a, fa.ids["c"]), tc.tag)
		}
	}
}

func TestCreditEditRefusesAProvisionalPersonForThePeersVideos(t *testing.T) {
	fa, fb, _, b, _ := mergedNodesWith(t, nil)
	person := provisionalPersonOn(t, fb)

	rec := b.owner(http.MethodPost, "/api/video-edits", fmt.Sprintf(
		`{"videoIds":[%d,%d],"credits":[{"kind":"person","role":"performer","add":[%d]}]}`,
		fb.ids["a"], domain.ToRemoteID(fa.ids["a"]), person))
	problem := decode[gen.Error](t, rec)
	if rec.Code != http.StatusBadRequest || problem.Code != codeInvalidRequest || problem.Reason == nil ||
		*problem.Reason != gen.ErrorReasonProvisionalPersonRemote || problem.Field == nil || *problem.Field != "credits[0].add[0]" {
		t.Errorf("status = %d, body %s; want 400 provisional_person_remote at credits[0].add[0]", rec.Code, rec.Body)
	}
	video, err := fb.env.db.Library().GetVideo(context.Background(), domain.AudienceOwner, fb.ids["a"])
	if err != nil || (video.WorkInfo != nil && len(video.WorkInfo.People) != 0) {
		t.Errorf("b's video a has work info %+v (%v), want no people", video.WorkInfo, err)
	}
}
