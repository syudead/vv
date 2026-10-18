package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func autoTagEntries(t *testing.T, db *DB) []domain.ActivityEntry {
	t.Helper()
	page, err := db.Activity().ListActivity(context.Background(), domain.AudienceOwner,
		domain.ActivityQuery{Features: []domain.ActivityFeature{domain.ActivityFeatureAutoTagging}}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	return page.Items
}

func latestAutoTagEntry(t *testing.T, db *DB) domain.ActivityEntry {
	t.Helper()
	entries := autoTagEntries(t, db)
	if len(entries) == 0 {
		t.Fatal("auto_tag の記録が無い")
	}
	return entries[0]
}

func runAutoTag(
	t *testing.T, db *DB, videoID int64, decider domain.ActivityDecider, selection domain.AutoTagSelection,
) domain.ActivityEntry {
	t.Helper()
	ctx := context.Background()
	store := db.AutoTags()
	if n, err := store.QueueVideos(ctx, []int64{videoID}, domain.AutoTagQueueAgain, decider); err != nil || n != 1 {
		t.Fatalf("積めない: %d, %v", n, err)
	}
	job := claimAutoTag(t, store)
	if err := store.Finish(ctx, job.ContentKey, selection, domain.ActivityByAI); err != nil {
		t.Fatal(err)
	}
	return latestAutoTagEntry(t, db)
}

func rejectedFor(t *testing.T, db *DB, key string, tagID int64) bool {
	t.Helper()
	var n int
	if err := db.sql.QueryRow(`select count(*) from auto_tag_rejections where content_key = ? and tag_id = ?`,
		key, tagID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestAutoTagActivityRecordsTheRun(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	store := db.AutoTags()
	solo := ids[fixturePath("/media/solo.mp4")]
	cat, dog := createTestTag(t, db, "cat"), createTestTag(t, db, "dog")

	if _, err := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueIfNew, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	waiting := latestAutoTagEntry(t, db)
	if waiting.Status != domain.ActivityRunning || waiting.Reason != domain.ActivityWaiting ||
		waiting.Decider != domain.ActivityByRule || waiting.Subject.Label != "solo" || waiting.VideoIDs[0] != solo {
		t.Fatalf("積んだ記録 = %+v", waiting)
	}
	job := claimAutoTag(t, store)
	if got := latestAutoTagEntry(t, db); got.ID != waiting.ID || got.Reason != domain.ActivityProcessing {
		t.Fatalf("取り出した記録 = %+v", got)
	}
	selection := domain.AutoTagSelection{
		Confirmed: []int64{cat.ID}, Tentative: []int64{dog.ID},
		Probabilities: map[int64]float64{cat.ID: 0.91, dog.ID: 0.6},
	}
	if err := store.Finish(ctx, job.ContentKey, selection, domain.ActivityByAI); err != nil {
		t.Fatal(err)
	}
	done := latestAutoTagEntry(t, db)
	if done.ID != waiting.ID || done.Status != domain.ActivityDone || done.Outcome != domain.ActivityCompleted ||
		done.Decider != domain.ActivityByAI {
		t.Fatalf("終えた記録 = %+v", done)
	}
	want := []domain.ActivityEntryTag{
		{TagID: cat.ID, Name: "cat", Probability: 0.91},
		{TagID: dog.ID, Name: "dog", Suggested: true, Probability: 0.6},
	}
	if !reflect.DeepEqual(done.Tags, want) {
		t.Fatalf("記録のタグ = %+v", done.Tags)
	}
	if !done.UndoOffered() || done.RetryOffered() {
		t.Fatalf("取り消し = %v, やり直し = %v", done.UndoOffered(), done.RetryOffered())
	}

	if _, err := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueAgain, domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	if got := latestAutoTagEntry(t, db); got.ID == done.ID || got.Decider != domain.ActivityByOwner {
		t.Fatalf("所有者が始めた記録 = %+v", got)
	}
}

func TestAutoTagActivityRecordsFailuresAndInterruptions(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	store := db.AutoTags()
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")

	if _, err := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueIfNew, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	claimAutoTag(t, store)
	if err := store.Fail(ctx, soloKey, "cannot reach Ollama"); err != nil {
		t.Fatal(err)
	}
	failed := latestAutoTagEntry(t, db)
	if failed.Status != domain.ActivityAttention || failed.Reason != domain.ActivityFailed ||
		failed.Detail != "cannot reach Ollama" || !failed.RetryOffered() {
		t.Fatalf("失敗した記録 = %+v", failed)
	}
	if _, err := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueAgain, domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	entries := autoTagEntries(t, db)
	if len(entries) != 2 || entries[0].Status != domain.ActivityRunning || entries[1].ID != failed.ID ||
		entries[1].Outcome != domain.ActivitySuperseded {
		t.Fatalf("やり直した記録 = %+v", entries)
	}

	claimAutoTag(t, store)
	if _, err := db.Activity().RecoverInterruptedActivity(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RequeueRunning(ctx); err != nil {
		t.Fatal(err)
	}
	if got := latestAutoTagEntry(t, db); got.Reason != domain.ActivityInterrupted {
		t.Fatalf("途切れた記録 = %+v", got)
	}
	claimAutoTag(t, store)
	entries = autoTagEntries(t, db)
	if len(entries) != 3 || entries[0].Reason != domain.ActivityProcessing || entries[0].Decider != domain.ActivityByOwner ||
		entries[1].Outcome != domain.ActivitySuperseded {
		t.Fatalf("取り出し直した記録 = %+v", entries)
	}
}

func TestUndoAutoTagRemovesTheTagFromTheBundle(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	p1, p2 := ids[fixturePath("/media/pair/p1.mp4")], ids[fixturePath("/media/pair/p2.mp4")]
	if _, err := db.Versions().Bundle(ctx, []int64{p1, p2}, p1); err != nil {
		t.Fatal(err)
	}
	bundleKey := ownerVideo(t, db, p1).UserKey
	cat, dog := createTestTag(t, db, "cat"), createTestTag(t, db, "dog")
	entry := runAutoTag(t, db, p2, domain.ActivityByRule, domain.AutoTagSelection{
		Confirmed: []int64{cat.ID, dog.ID}, Probabilities: map[int64]float64{cat.ID: 0.91, dog.ID: 0.85},
	})
	folder := createSmartFolder(t, db, "猫", 0)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	undo, err := db.Activity().UndoAutoTag(ctx, entry.ID, cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Kind != domain.ActivityUndo || undo.Outcome != domain.ActivityCompleted || undo.Decider != domain.ActivityByOwner ||
		undo.UndoesID != entry.ID || undo.Feature != domain.ActivityFeatureAutoTagging ||
		undo.Undoes == nil || undo.Undoes.TagName != "cat" {
		t.Fatalf("undo の記録 = %+v", undo)
	}
	if got := manualTagIDs(t, db, bundleKey); !slices.Equal(got, []int64{dog.ID}) {
		t.Fatalf("集まりのタグ = %v", got)
	}
	for _, id := range []int64{p1, p2} {
		key := ownerVideo(t, db, id).UserKey
		tags, err := db.Tags().TagsByContentKeys(ctx, []string{key})
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(tags[key], func(tag domain.VideoTag) bool { return tag.ID == cat.ID }) {
			t.Fatalf("動画 %d に取り消したタグが残った: %+v", id, tags[key])
		}
	}
	if !rejectedFor(t, db, bundleKey, cat.ID) {
		t.Fatal("却下が残っていない")
	}
	want := domain.AutoTagApplied{VideoIDs: []int64{min(p1, p2), max(p1, p2)}}
	if !slices.ContainsFunc(recorder.events, func(e domain.Event) bool { return reflect.DeepEqual(e, want) }) ||
		!slices.Contains(recorder.events, domain.Event(domain.ActivityChanged{})) {
		t.Fatalf("発行した変化 = %#v", recorder.events)
	}
	if got, want := smartFolderNotices(recorder), []domain.SmartFoldersChanged{{IDs: []int64{folder.ID}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("スマートフォルダの知らせ = %+v, want %+v", got, want)
	}
	undone, err := db.Activity().ActivityEntry(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undone.Tags[0].UndoneBy != undo.ID || undone.Tags[1].UndoneBy != 0 || !undone.UndoOffered() {
		t.Fatalf("取り消した記録 = %+v", undone)
	}

	if _, err := db.Activity().UndoAutoTag(ctx, entry.ID, cat.ID); !errors.Is(err, domain.ErrActivityNotUndoable) {
		t.Fatalf("取り消し済みの取り消し = %v", err)
	}
	if _, err := db.Activity().UndoAutoTag(ctx, entry.ID, cat.ID+100); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("付けていないタグの取り消し = %v", err)
	}

	store := db.AutoTags()
	if _, err := store.QueueLibrary(ctx, domain.AutoTagQueueAgain, domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	p2Key := "key" + fixturePath("/media/pair/p2.mp4")
	candidates, err := store.Candidates(ctx, p2Key)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(candidates, func(c domain.AutoTagCandidate) bool { return c.ID == cat.ID }) {
		t.Fatalf("取り消したタグを候補にした: %+v", candidates)
	}
	for {
		job, err := store.Claim(ctx)
		if errors.Is(err, domain.ErrNoJob) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Finish(ctx, job.ContentKey, domain.AutoTagSelection{Confirmed: []int64{cat.ID}}, domain.ActivityByAI); err != nil {
			t.Fatal(err)
		}
	}
	if got := manualTagIDs(t, db, bundleKey); !slices.Equal(got, []int64{dog.ID}) {
		t.Fatalf("Tag all again の後の集まりのタグ = %v", got)
	}
}

// Same-second owner edits must be protected by activity IDs, not timestamps.
func TestUndoAutoTagAfterTheOwnerChangedIt(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")
	removed, reattached, kept := createTestTag(t, db, "removed"), createTestTag(t, db, "reattached"), createTestTag(t, db, "kept")
	entry := runAutoTag(t, db, solo, domain.ActivityByRule, domain.AutoTagSelection{
		Confirmed: []int64{removed.ID, reattached.ID}, Tentative: []int64{kept.ID},
		Probabilities: map[int64]float64{removed.ID: 0.91, reattached.ID: 0.9, kept.ID: 0.6},
	})
	if _, _, err := db.Tags().DetachTag(ctx, []int64{solo}, removed.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().DetachTag(ctx, []int64{solo}, reattached.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{solo}, reattached.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoTags().ConfirmTentative(ctx, solo, kept.ID); err != nil {
		t.Fatal(err)
	}
	before := manualTagIDs(t, db, soloKey)

	for _, tag := range []domain.Tag{removed, reattached, kept} {
		undo, err := db.Activity().UndoAutoTag(ctx, entry.ID, tag.ID)
		if err != nil {
			t.Fatal(err)
		}
		if undo.Outcome != domain.ActivityAlreadyReverted || undo.Undoes == nil || undo.Undoes.TagName != tag.Name {
			t.Fatalf("%s の取り消し = %+v", tag.Name, undo)
		}
		if !rejectedFor(t, db, soloKey, tag.ID) {
			t.Fatalf("%s の却下が残っていない", tag.Name)
		}
	}
	if got := manualTagIDs(t, db, soloKey); !slices.Equal(got, before) || !slices.Contains(got, reattached.ID) ||
		!slices.Contains(got, kept.ID) {
		t.Fatalf("取り消しがタグを変えた: %v, want %v", got, before)
	}
}

// Find later owner tag changes by affected member content keys even after the bundle representative changes.
func TestUndoAutoTagKeepsTheOwnersReattachOnTheBundle(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	p1, p2, solo := ids[fixturePath("/media/pair/p1.mp4")], ids[fixturePath("/media/pair/p2.mp4")], ids[fixturePath("/media/solo.mp4")]
	if _, err := db.Versions().Bundle(ctx, []int64{p1, p2, solo}, p1); err != nil {
		t.Fatal(err)
	}
	bundleKey := ownerVideo(t, db, p1).UserKey
	cat := createTestTag(t, db, "cat")
	entry := runAutoTag(t, db, p2, domain.ActivityByRule, domain.AutoTagSelection{
		Confirmed: []int64{cat.ID}, Probabilities: map[int64]float64{cat.ID: 0.91},
	})
	if _, _, err := db.Tags().DetachTag(ctx, []int64{p2}, cat.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{p2}, cat.ID); err != nil {
		t.Fatal(err)
	}
	// The activity subject was p1, but p2's affected key still identifies the edit after solo becomes representative.
	if _, err := db.Versions().MakeRepresentative(ctx, solo); err != nil {
		t.Fatal(err)
	}

	undo, err := db.Activity().UndoAutoTag(ctx, entry.ID, cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Outcome != domain.ActivityAlreadyReverted {
		t.Fatalf("undo の記録 = %+v, want already_reverted", undo)
	}
	if got := manualTagIDs(t, db, bundleKey); !slices.Equal(got, []int64{cat.ID}) {
		t.Fatalf("取り消しが所有者のタグを変えた: %v", got)
	}
	if !rejectedFor(t, db, bundleKey, cat.ID) {
		t.Fatal("却下が残っていない")
	}
}

// Owner edits before result persistence must not block undo of that result.
func TestUndoAutoTagRemovesTheTagTheOwnerChangedWhileItWaited(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")
	cat := createTestTag(t, db, "cat")
	store := db.AutoTags()
	if n, err := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueAgain, domain.ActivityByRule); err != nil || n != 1 {
		t.Fatalf("積めない: %d, %v", n, err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{solo}, cat.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().DetachTag(ctx, []int64{solo}, cat.ID); err != nil {
		t.Fatal(err)
	}
	job := claimAutoTag(t, store)
	if err := store.Finish(ctx, job.ContentKey, domain.AutoTagSelection{
		Confirmed: []int64{cat.ID}, Probabilities: map[int64]float64{cat.ID: 0.91},
	}, domain.ActivityByAI); err != nil {
		t.Fatal(err)
	}
	entry := latestAutoTagEntry(t, db)

	undo, err := db.Activity().UndoAutoTag(ctx, entry.ID, cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Outcome != domain.ActivityCompleted {
		t.Fatalf("undo の記録 = %+v, want completed", undo)
	}
	if got := manualTagIDs(t, db, soloKey); len(got) != 0 {
		t.Fatalf("判定が付けたタグが残った: %v", got)
	}
}

// Stable tag IDs preserve owner edits across renames.
func TestUndoAutoTagKeepsTheOwnersReattachAcrossRenames(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")
	tag := createTestTag(t, db, "A")
	entry := runAutoTag(t, db, solo, domain.ActivityByRule, domain.AutoTagSelection{
		Confirmed: []int64{tag.ID}, Probabilities: map[int64]float64{tag.ID: 0.91},
	})
	if _, err := db.Tags().RenameTag(ctx, tag.ID, "B"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().DetachTag(ctx, []int64{solo}, tag.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{solo}, tag.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().RenameTag(ctx, tag.ID, "C"); err != nil {
		t.Fatal(err)
	}

	undo, err := db.Activity().UndoAutoTag(ctx, entry.ID, tag.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Outcome != domain.ActivityAlreadyReverted {
		t.Fatalf("undo の記録 = %+v, want already_reverted", undo)
	}
	if got := manualTagIDs(t, db, soloKey); !slices.Equal(got, []int64{tag.ID}) {
		t.Fatalf("取り消しが所有者のタグを変えた: %v", got)
	}
}

// Undo removes both the current bundle copy and the hidden original attachment.
func TestUndoAutoTagAfterTheSubjectJoinedABundle(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	p1, p2 := ids[fixturePath("/media/pair/p1.mp4")], ids[fixturePath("/media/pair/p2.mp4")]
	p1Key := "key" + fixturePath("/media/pair/p1.mp4")
	cat := createTestTag(t, db, "cat")
	entry := runAutoTag(t, db, p1, domain.ActivityByRule, domain.AutoTagSelection{
		Confirmed: []int64{cat.ID}, Probabilities: map[int64]float64{cat.ID: 0.91},
	})
	if _, err := db.Versions().Bundle(ctx, []int64{p1, p2}, p1); err != nil {
		t.Fatal(err)
	}
	bundleKey := ownerVideo(t, db, p1).UserKey
	if got := manualTagIDs(t, db, bundleKey); !slices.Equal(got, []int64{cat.ID}) {
		t.Fatalf("束ねた後の集まりのタグ = %v", got)
	}

	undo, err := db.Activity().UndoAutoTag(ctx, entry.ID, cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Outcome != domain.ActivityCompleted {
		t.Fatalf("undo の記録 = %+v", undo)
	}
	for _, key := range []string{bundleKey, p1Key} {
		if got := manualTagIDs(t, db, key); len(got) != 0 {
			t.Fatalf("%s に取り消したタグが残った: %v", key, got)
		}
		if !rejectedFor(t, db, key, cat.ID) {
			t.Fatalf("%s に却下が残っていない", key)
		}
	}
	candidates, err := db.AutoTags().Candidates(ctx, p1Key)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(candidates, func(c domain.AutoTagCandidate) bool { return c.ID == cat.ID }) {
		t.Fatalf("取り消したタグを候補にした: %+v", candidates)
	}
}

// The chosen representative determines whether the bundle copies the automatic or owner attachment.
func ownerTagContest(t *testing.T, representative string) (*DB, domain.ActivityEntry, domain.Tag, map[string]int64, map[string]string) {
	t.Helper()
	db, ids := itemsFixture(t)
	ctx := context.Background()
	videos := map[string]int64{"p1": ids[fixturePath("/media/pair/p1.mp4")], "p2": ids[fixturePath("/media/pair/p2.mp4")]}
	keys := map[string]string{"p1": "key" + fixturePath("/media/pair/p1.mp4"), "p2": "key" + fixturePath("/media/pair/p2.mp4")}
	cat := createTestTag(t, db, "cat")
	entry := runAutoTag(t, db, videos["p1"], domain.ActivityByRule, domain.AutoTagSelection{
		Confirmed: []int64{cat.ID}, Probabilities: map[int64]float64{cat.ID: 0.91},
	})
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{videos["p2"]}, cat.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Versions().Bundle(ctx, []int64{videos["p1"], videos["p2"]}, videos[representative]); err != nil {
		t.Fatal(err)
	}
	keys["bundle"] = ownerVideo(t, db, videos["p1"]).UserKey
	return db, entry, cat, videos, keys
}

// Preserve the owner's winning bundle attachment while removing the hidden automatic row.
func TestUndoAutoTagKeepsTheOwnersTagThatWonTheBundle(t *testing.T) {
	db, entry, cat, _, keys := ownerTagContest(t, "p2")

	undo, err := db.Activity().UndoAutoTag(context.Background(), entry.ID, cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Outcome != domain.ActivityAlreadyReverted {
		t.Fatalf("undo の記録 = %+v, want already_reverted", undo)
	}
	for _, name := range []string{"bundle", "p2"} {
		if got := manualTagIDs(t, db, keys[name]); !slices.Equal(got, []int64{cat.ID}) {
			t.Errorf("取り消しが所有者のタグを消した（%s）: %v", name, got)
		}
	}
	if got := manualTagIDs(t, db, keys["p1"]); len(got) != 0 {
		t.Errorf("判定が書いた p1 の行が残った: %v", got)
	}
	for _, name := range []string{"bundle", "p1"} {
		if !rejectedFor(t, db, keys[name], cat.ID) {
			t.Errorf("%s に却下が残っていない", name)
		}
	}
}

// When the automatic row wins, undo removes its copies but preserves the owner's hidden member row.
func TestUndoAutoTagRemovesTheRunsRowThatWonTheBundle(t *testing.T) {
	db, entry, cat, _, keys := ownerTagContest(t, "p1")

	undo, err := db.Activity().UndoAutoTag(context.Background(), entry.ID, cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Outcome != domain.ActivityCompleted {
		t.Fatalf("undo の記録 = %+v, want completed", undo)
	}
	for _, name := range []string{"bundle", "p1"} {
		if got := manualTagIDs(t, db, keys[name]); len(got) != 0 {
			t.Errorf("%s に取り消したタグが残った: %v", name, got)
		}
	}
	if got := manualTagIDs(t, db, keys["p2"]); !slices.Equal(got, []int64{cat.ID}) {
		t.Errorf("p2 の所有者のタグ = %v, want cat のまま", got)
	}
}

// A singleton inheriting the owner's bundle row no longer holds the automatic attachment.
func TestUndoAutoTagAfterTheBundleRowReplacedTheSubjects(t *testing.T) {
	db, entry, cat, videos, keys := ownerTagContest(t, "p2")
	ctx := context.Background()
	if _, err := db.Versions().Unbundle(ctx, videos["p2"]); err != nil {
		t.Fatal(err)
	}
	if got := manualTagIDs(t, db, keys["p1"]); !slices.Equal(got, []int64{cat.ID}) {
		t.Fatalf("解いた後の p1 のタグ = %v, want 集まりの cat", got)
	}

	undo, err := db.Activity().UndoAutoTag(ctx, entry.ID, cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Outcome != domain.ActivityAlreadyReverted {
		t.Fatalf("undo の記録 = %+v, want already_reverted", undo)
	}
	if got := manualTagIDs(t, db, keys["p1"]); !slices.Equal(got, []int64{cat.ID}) {
		t.Errorf("取り消しが集まりから受け継いだタグを消した: %v", got)
	}
	if !rejectedFor(t, db, keys["p1"], cat.ID) {
		t.Error("却下が残っていない")
	}
}
