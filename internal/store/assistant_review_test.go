package store

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func reviewSetKeys(t *testing.T, db *DB, kind domain.AssistantReviewKind) []string {
	t.Helper()
	sets, err := db.Assistant().FindReviewSets(context.Background(), kind)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, set := range sets {
		keys = append(keys, set.SetKey())
	}
	return keys
}

func pairKey(a, b int64) string {
	ids := []string{strconv.FormatInt(a, 10), strconv.FormatInt(b, 10)}
	slices.Sort(ids)
	return ids[0] + "," + ids[1]
}

func createTags(t *testing.T, db *DB, names ...string) []int64 {
	t.Helper()
	ids := make([]int64, len(names))
	for i, name := range names {
		tag, err := db.Tags().CreateTag(context.Background(), name)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = tag.ID
	}
	return ids
}

func TestFindTagReviewSetsPairsNearNamesOnly(t *testing.T) {
	db := migratedDB(t)
	ids := createTags(t, db, "selfie", "selfy", "cat", "car")

	if got, want := reviewSetKeys(t, db, domain.AssistantReviewTags), []string{pairKey(ids[0], ids[1])}; !slices.Equal(got, want) {
		t.Errorf("tag sets = %v, want %v (selfie/selfy only)", got, want)
	}
}

func TestFindPeopleReviewSetsPairsAnEqualFoldedAliasOnly(t *testing.T) {
	db := migratedDB(t)
	hanako := createPerson(t, db, "山田花子", "Hanako Yamada")
	other := createPerson(t, db, "ＨＡＮＡＫＯ　ＹＡＭＡＤＡ")
	createPerson(t, db, "Alice")
	createPerson(t, db, "Alicia")

	if got, want := reviewSetKeys(t, db, domain.AssistantReviewPeople), []string{pairKey(hanako.ID, other.ID)}; !slices.Equal(got, want) {
		t.Errorf("people sets = %v, want %v", got, want)
	}
}

func TestFindWorkReviewSetsGroupsPartsOfUngroupedVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := map[string]int64{}
	for _, v := range []struct{ path, title, key string }{
		{"/media/Trip 1.mp4", "Trip 1", "t1"}, {"/media/Trip part 2.mp4", "Trip part 2", "t2"},
		{"/media/Walk 1.mp4", "Walk 1", "w1"}, {"/media/Walk 2.mp4", "Walk 2", "w2"},
	} {
		result, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath(v.path), v.title, v.key, 10, 0))
		if err != nil {
			t.Fatal(err)
		}
		ids[v.key] = result.ID
	}
	rebuildIndexForTest(t, db)
	if _, err := db.Groups().CreateGroup(ctx, "Walk", domain.GroupAdd{VideoIDs: []int64{ids["w1"]}}); err != nil {
		t.Fatal(err)
	}

	if got, want := reviewSetKeys(t, db, domain.AssistantReviewWorks), []string{"t1,t2"}; !slices.Equal(got, want) {
		t.Errorf("work sets = %v, want %v (a grouped part leaves its work alone)", got, want)
	}
}

// listTagPair runs one tags review that lists the pair of the two tags and returns the run and its candidate.
func listTagPair(t *testing.T, db *DB, a, b int64) (domain.AssistantReviewRun, domain.AssistantReviewCandidate) {
	t.Helper()
	ctx := context.Background()
	run, started, err := db.Assistant().StartReviewRun(ctx, domain.AssistantReviewTags, "タグを整理して", assistantNow)
	if err != nil || !started {
		t.Fatalf("start = %v, %v", started, err)
	}
	set := domain.AssistantReviewSet{Members: []domain.AssistantReviewMember{
		{Kind: domain.AssistantLabelTag, ID: a, Name: "a"}, {Kind: domain.AssistantLabelTag, ID: b, Name: "b"},
	}}
	if err := db.Assistant().RecordReviewJudgement(ctx, run.ID, set, 0.9, true); err != nil {
		t.Fatal(err)
	}
	if err := db.Assistant().FinishReviewRun(ctx, run.ID, domain.AssistantReviewDone, "", assistantNow); err != nil {
		t.Fatal(err)
	}
	items, _, err := db.Assistant().ReviewCandidates(ctx, run.ID, "", 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("candidates = %+v, %v", items, err)
	}
	return run, items[0]
}

func TestRejectedReviewSetIsSkippedByTheNextRun(t *testing.T) {
	db := migratedDB(t)
	ids := createTags(t, db, "selfie", "selfy")
	run, candidate := listTagPair(t, db, ids[0], ids[1])

	rejected, err := db.Assistant().RejectReviewCandidates(context.Background(), run.ID, []int64{candidate.ID}, assistantNow)
	if err != nil || !slices.Equal(rejected, []int64{candidate.ID}) {
		t.Fatalf("rejected = %v, %v", rejected, err)
	}
	if got := reviewSetKeys(t, db, domain.AssistantReviewTags); len(got) != 0 {
		t.Errorf("tag sets after the rejection = %v, want none", got)
	}
}

func TestReviewCandidateOfAMergedTagLeavesTheList(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := createTags(t, db, "selfie", "selfy")
	run, candidate := listTagPair(t, db, ids[0], ids[1])
	if candidate.Members[0].Name != "selfie" || candidate.Members[1].Name != "selfy" {
		t.Errorf("members = %+v, want the current names", candidate.Members)
	}

	if _, err := db.Tags().MergeTags(ctx, ids[0], []int64{ids[1]}); err != nil {
		t.Fatal(err)
	}
	items, _, err := db.Assistant().ReviewCandidates(ctx, run.ID, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("candidates after the merge = %+v, want none", items)
	}
}

func TestStartReviewRunAnswersTheRunningRunOfTheKind(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	first, started, err := db.Assistant().StartReviewRun(ctx, domain.AssistantReviewTags, "タグを整理して", assistantNow)
	if err != nil || !started {
		t.Fatalf("first start = %v, %v", started, err)
	}
	again, started, err := db.Assistant().StartReviewRun(ctx, domain.AssistantReviewTags, "もう一度", assistantNow)
	if err != nil || started || again.ID != first.ID || again.Request != "タグを整理して" {
		t.Errorf("second start = %+v, %v, %v; want the running run", again, started, err)
	}
	if _, started, err := db.Assistant().StartReviewRun(ctx, domain.AssistantReviewPeople, "人物", assistantNow); err != nil || !started {
		t.Errorf("another kind: started = %v, %v", started, err)
	}
	if recovered, err := db.Assistant().RecoverReviewRuns(ctx, assistantNow); err != nil || recovered != 2 {
		t.Fatalf("recovered = %d, %v", recovered, err)
	}
	if run, err := db.Assistant().ReviewRun(ctx, first.ID); err != nil || run.Status != domain.AssistantReviewInterrupted {
		t.Errorf("after recovery = %+v, %v; want interrupted", run, err)
	}
}

func TestAdoptReviewCandidateStoresOneApprovedProposal(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := createTags(t, db, "selfie", "selfy")
	_, candidate := listTagPair(t, db, ids[0], ids[1])
	plan, err := domain.AssistantReviewPlan(domain.AssistantReviewTags, candidate.Members)
	if err != nil {
		t.Fatal(err)
	}

	p, err := db.Assistant().AdoptReviewCandidate(ctx, candidate.ID, "タグを整理して", plan, assistantNow)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != domain.AssistantApproved || p.ActivityID == 0 || p.Plan.ActionID != "mergeTag" {
		t.Errorf("proposal = %+v, want an approved mergeTag with its entry", p)
	}
	if _, err := db.Assistant().AdoptReviewCandidate(ctx, candidate.ID, "x", plan, assistantNow); !errors.Is(err, domain.ErrAssistantCandidateNotListed) {
		t.Errorf("second adoption: err = %v, want not listed", err)
	}
	stored, err := db.Assistant().ReviewCandidatesByID(ctx, candidate.RunID, []int64{candidate.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := stored[candidate.ID]; got.State != domain.AssistantCandidateAdopted || got.ProposalID != p.ID {
		t.Errorf("candidate = %+v, want adopted by proposal %d", got, p.ID)
	}
	if active, err := db.Assistant().ActiveProposals(ctx, assistantNow); err != nil || len(active) != 1 {
		t.Errorf("active proposals = %+v, %v; want only the adopted one, the failed second adoption rolled back", active, err)
	}
}
