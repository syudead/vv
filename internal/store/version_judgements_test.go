package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Only the 720p/1080p pair shares a fingerprint; the third video differs and the fourth is merely indexed.
func judgementFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db, ids := versionFixture(t)
	for _, v := range []struct {
		name          string
		durationMs    int64
		width, height int
		fingerprint   domain.Fingerprint
	}{
		{"a", 100_000, 1280, 720, testFingerprint(1, 0)},
		{"b", 100_400, 1920, 1080, testFingerprint(1, 0b101)},
		{"c", 100_000, 1920, 1080, testFingerprint(2, 0)},
	} {
		probe := domain.Probe{DurationMs: v.durationMs, Width: v.width, Height: v.height, VideoCodec: "h264", AudioCodec: "aac"}
		if err := db.Ingest().ApplyProbe(context.Background(), ids[v.name], probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
			t.Fatal(err)
		}
		seekDone(t, db, ids[v.name])
		applyFingerprint(t, db, ids[v.name], v.fingerprint)
	}
	return db, ids
}

// claimVersionJudgement claims the next version_judge job as the scheduler does and reads its input.
func claimVersionJudgement(ctx context.Context, db *DB) (domain.VersionJudgeJob, error) {
	job, err := db.Ingest().ClaimJob(ctx, domain.JobVersionJudge)
	if err != nil {
		return domain.VersionJudgeJob{}, err
	}
	return db.Versions().JudgementInput(ctx, [2]string{job.ContentKey, job.ContentKeyB})
}

func claimJudgement(t *testing.T, db *DB) domain.VersionJudgeJob {
	t.Helper()
	job, err := claimVersionJudgement(context.Background(), db)
	if err != nil {
		t.Fatalf("ClaimJob(version_judge): %v", err)
	}
	if job.Keys != [2]string{"key-a", "key-b"} {
		t.Fatalf("取り出した組 = %v, want [key-a key-b]", job.Keys)
	}
	return job
}

func applyJudgement(t *testing.T, db *DB, keys [2]string, result domain.VersionJudgeResult) []domain.Event {
	t.Helper()
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	applied, err := db.Versions().ApplyJudgement(context.Background(), keys, result)
	if err != nil || !applied {
		t.Fatalf("ApplyJudgement = %v, %v", applied, err)
	}
	if !hasEvent[domain.VersionCandidatesChanged](recorder.events) {
		t.Errorf("VersionCandidatesChanged を発行しない: %v", recorder.events)
	}
	return recorder.events
}

func hasEvent[E domain.Event](events []domain.Event) bool {
	for _, event := range events {
		if _, ok := event.(E); ok {
			return true
		}
	}
	return false
}

// candidateJudgement reads the a/b pair's judgement and probability, and the attempts and retry time of its unfinished
// job.
func candidateJudgement(t *testing.T, db *DB) (judgement *string, attempts int, retryAt *int64, probability *float64) {
	t.Helper()
	if err := db.sql.QueryRow(`select c.judgement, coalesce((select j.attempts from jobs j
		where j.kind = 'version_judge' and j.subject_key = c.key_a and j.subject_key_b = c.key_b
		and j.state in ('queued', 'running')), 0), `+judgeRetryAtColumn+`, c.probability
		from video_version_candidates c where c.key_a = 'key-a' and c.key_b = 'key-b'`).
		Scan(&judgement, &attempts, &retryAt, &probability); err != nil {
		t.Fatal(err)
	}
	return judgement, attempts, retryAt, probability
}

func expectNoClaim(t *testing.T, db *DB, why string) {
	t.Helper()
	if job, err := db.Ingest().ClaimJob(context.Background(), domain.JobVersionJudge); !errors.Is(err, domain.ErrNoJob) {
		t.Errorf("%s: ClaimJob(version_judge) = %+v, %v, want domain.ErrNoJob", why, job, err)
	}
}

// The files choose the representative; the tagged side loses nothing, since the bundle merges both sides' data.
func TestJudgementSameBundlesByTheFilesKeepingBothSidesData(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b := ids["a"], ids["b"]
	attachNamedTag(t, db, "cat", a)

	job := claimJudgement(t, db)
	if len(job.Fingerprints[0].Frames) == 0 || len(job.Fingerprints[1].Frames) == 0 {
		t.Fatalf("取り出しが指紋を返さない: %+v", job.Fingerprints)
	}
	events := applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionSame})
	if !hasEvent[domain.VideoBundleChanged](events) {
		t.Errorf("VideoBundleChanged を発行しない: %v", events)
	}

	versions, err := db.Versions().Versions(context.Background(), domain.AudienceOwner, b)
	if err != nil {
		t.Fatal(err)
	}
	// b's file has the higher bit rate.
	if versions.RepresentativeID != b || len(versions.Items) != 2 {
		t.Errorf("代表 = %d（%d 本）, want ファイルで選んだ %d（2 本）", versions.RepresentativeID, len(versions.Items), b)
	}
	if got := manualTagNames(t, db, ownerVideo(t, db, b).UserKey); !slices.Equal(got, []string{"cat"}) {
		t.Errorf("集まりのタグ = %v, want a の cat", got)
	}
	if rows := candidateRows(t, db); rows != 0 {
		t.Errorf("束ねたあとの候補の行 = %d, want 0", rows)
	}
	if applied, err := db.Versions().ApplyJudgement(context.Background(), job.Keys,
		domain.VersionJudgeResult{Verdict: domain.VersionSame}); err != nil || applied {
		t.Errorf("消えた候補への結果 = %v, %v, want 捨てる", applied, err)
	}
}

// Differences within one existing side must not prevent adding a matching new video.
func TestJudgementSameJoinsExistingBundle(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(map[bool]string{false: "bundled", true: "split and bundled again"}[split], func(t *testing.T) {
			db, ids := judgementFixture(t)
			a, b, c := ids["a"], ids["b"], ids["c"]
			bundle(t, db, c, a, c)
			if split {
				if _, err := db.Versions().Unbundle(context.Background(), a); err != nil {
					t.Fatal(err)
				}
				bundle(t, db, c, a, c)
			}

			job := claimJudgement(t, db)
			applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionSame})

			versions, err := db.Versions().Versions(context.Background(), domain.AudienceOwner, b)
			if err != nil {
				t.Fatal(err)
			}
			if versions.RepresentativeID != c || len(versions.Items) != 3 {
				t.Errorf("代表 = %d（%d 本）, want 集まりの代表 %d（3 本）", versions.RepresentativeID, len(versions.Items), c)
			}
		})
	}
}

func TestJudgementDifferentDismisses(t *testing.T) {
	db, _ := judgementFixture(t)
	job := claimJudgement(t, db)
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionDifferent})

	var count int
	if err := db.sql.QueryRow(`select count(*) from video_version_dismissals where key_a = 'key-a' and key_b = 'key-b'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("却下の行 = %d, want 1", count)
	}
	if rows := candidateRows(t, db); rows != 0 {
		t.Errorf("却下したあとの候補の行 = %d, want 0", rows)
	}
}

func TestJudgementNamesSmartFoldersOnlyWhenItBundles(t *testing.T) {
	for _, tc := range []struct {
		name    string
		verdict domain.VersionVerdict
		bundles bool
	}{
		{"same", domain.VersionSame, true},
		{"different", domain.VersionDifferent, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := judgementFixture(t)
			first := createSmartFolder(t, db, "一", 0)
			second := createSmartFolder(t, db, "二", 0)
			job := claimJudgement(t, db)
			recorder := &eventRecorder{}
			recorder.events = applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: tc.verdict})

			if got := hasEvent[domain.VideoBundleChanged](recorder.events); got != tc.bundles {
				t.Fatalf("VideoBundleChanged = %v, want %v: %v", got, tc.bundles, recorder.events)
			}
			var want []domain.SmartFoldersChanged
			if tc.bundles {
				want = []domain.SmartFoldersChanged{{IDs: []int64{first.ID, second.ID}}}
			}
			if got := smartFolderNotices(recorder); !reflect.DeepEqual(got, want) {
				t.Errorf("notices = %+v, want %+v", got, want)
			}
		})
	}
}

// A cross-side dismissal recorded during judging must override the automatic same verdict.
func TestJudgementSameAcrossDismissedPairStaysUndecided(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b, c := ids["a"], ids["b"], ids["c"]
	bundle(t, db, a, a, c)

	job := claimJudgement(t, db)
	if err := db.Versions().Dismiss(context.Background(), [2]int64{b, c}); err != nil {
		t.Fatal(err)
	}
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionSame})

	if judgement, _, _, probability := candidateJudgement(t, db); judgement == nil || *judgement != "undecided" || probability != nil {
		t.Errorf("判定 = %v（確率 %v）, want undecided（確率なし）", judgement, probability)
	}
	if pairs, _ := candidatePairs(t, db); len(pairs) != 1 {
		t.Errorf("undecided の組が一覧から消えた: %v", pairs)
	}
	if versions, err := db.Versions().Versions(context.Background(), domain.AudienceOwner, b); err != nil || len(versions.Items) != 1 {
		t.Errorf("b が束ねられた: %+v, %v", versions, err)
	}
	expectNoClaim(t, db, "undecided の組")
}

func TestJudgementUndecidedKeepsProbability(t *testing.T) {
	db, _ := judgementFixture(t)
	job := claimJudgement(t, db)
	probability := 0.5
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionUndecided, Probability: &probability})

	if judgement, _, _, got := candidateJudgement(t, db); judgement == nil || *judgement != "undecided" || got == nil || *got != 0.5 {
		t.Errorf("判定 = %v（確率 %v）, want undecided（0.5）", judgement, got)
	}
	expectNoClaim(t, db, "undecided の組")
}

// A failed judgement keeps the candidate failed while its job waits 10 min, 1 h, 6 h, then daily without a limit;
// the delays are today's, now applied through the kind's retry policy.
func TestJudgementFailedRetriesAfterTheKindsDelays(t *testing.T) {
	db, _ := judgementFixture(t)
	now := fixJobClock(t)
	for i, want := range []time.Duration{10 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour, 24 * time.Hour} {
		job := claimJudgement(t, db)
		applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: "no answer"})

		judgement, attempts, retryAt, _ := candidateJudgement(t, db)
		if judgement == nil || *judgement != "failed" || attempts != i+1 || retryAt == nil {
			t.Fatalf("%d 回目の失敗のあと = %v, attempts %d, retry %v, want failed, %d, 再試行の時刻", i+1, judgement,
				attempts, retryAt, i+1)
		}
		if got := time.Duration(*retryAt-now.Unix()) * time.Second; got != want {
			t.Fatalf("%d 回目の失敗の待ち = %v, want %v", i+1, got, want)
		}
		*now = now.Add(want - time.Second)
		expectNoClaim(t, db, "待ちの終わる前の failed の組")
		*now = now.Add(time.Second)
	}
	var message string
	if err := db.sql.QueryRow(`select judgement_error from video_version_candidates`).Scan(&message); err != nil || message != "no answer" {
		t.Errorf("judgement_error = %q, %v", message, err)
	}
}

// A pair interrupted by a restart returns to waiting with its attempt counted, and its next failure waits longer.
func TestJudgementRecoveryKeepsTheAttempts(t *testing.T) {
	db, _ := judgementFixture(t)
	ctx := context.Background()
	fixJobClock(t)
	claimJudgement(t, db)
	if _, err := db.Ingest().RequeueRunningJobs(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := db.Versions().RecoverRunningJudgements(ctx)
	if err != nil || recovered != 1 {
		t.Fatalf("RecoverRunningJudgements = %d, %v, want 1", recovered, err)
	}
	if judgement, attempts, _, _ := candidateJudgement(t, db); judgement != nil || attempts != 1 {
		t.Fatalf("回復したあと = %v, attempts %d, want null, 1", judgement, attempts)
	}
	job := claimJudgement(t, db)
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: "again"})
	if _, attempts, _, _ := candidateJudgement(t, db); attempts != 2 {
		t.Errorf("回復のあとの失敗の attempts = %d, want 2", attempts)
	}
}

// A held pair's job stays queued without spending attempts, and is claimed once the hold is lifted.
func TestJudgementJobOfAHeldPairWaitsForTheHoldToLift(t *testing.T) {
	db, _ := judgementFixture(t)
	execAll(t, db, `insert into video_version_judging_holds (key_a, key_b, created_at) values ('key-a', 'key-b', 1)`)
	expectNoClaim(t, db, "保留された組")
	if _, attempts, _, _ := candidateJudgement(t, db); attempts != 0 {
		t.Fatalf("保留の間の attempts = %d, want 0", attempts)
	}
	if n := countRows(t, db, `select count(*) from jobs where kind = 'version_judge' and state = 'queued'`); n != 1 {
		t.Fatalf("保留の間の積まれた仕事 = %d, want 1", n)
	}
	execAll(t, db, `delete from video_version_judging_holds`)
	claimJudgement(t, db)
}

// The job goes with its candidate row, whichever statement removes it.
func TestJudgementJobGoesWithItsCandidate(t *testing.T) {
	db, ids := judgementFixture(t)
	if err := db.Versions().Dismiss(context.Background(), [2]int64{ids["a"], ids["b"]}); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `select count(*) from jobs where kind = 'version_judge'`); n != 0 {
		t.Fatalf("候補を消したあとの仕事 = %d, want 0", n)
	}
}

func TestUnbundleRecordsPairsAndRebuildsCandidates(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b, c := ids["a"], ids["b"], ids["c"]
	bundle(t, db, a, a, b, c)
	if rows := candidateRows(t, db); rows != 0 {
		t.Fatalf("束ねたあとの候補の行 = %d, want 0", rows)
	}

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	if _, err := db.Versions().Unbundle(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if !hasEvent[domain.VersionCandidatesChanged](recorder.events) {
		t.Errorf("Unbundle が VersionCandidatesChanged を発行しない: %v", recorder.events)
	}
	var pairs []string
	rows, err := db.sql.Query(`select key_a || '|' || key_b from video_version_unbundles order by 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var pair string
		if err := rows.Scan(&pair); err != nil {
			t.Fatal(err)
		}
		pairs = append(pairs, pair)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 2 || pairs[0] != "key-a|key-b" || pairs[1] != "key-b|key-c" {
		t.Errorf("外した組 = %v, want [key-a|key-b key-b|key-c]", pairs)
	}
	if listed, _ := candidatePairs(t, db); len(listed) != 1 || listed[0] != [2]int64{min(a, b), max(a, b)} {
		t.Errorf("外したあとの候補 = %v, want [[%d %d]]", listed, min(a, b), max(a, b))
	}
	expectNoClaim(t, db, "集まりから外した組")
}

func TestMoveKeyPairsRekeysUnbundledPairs(t *testing.T) {
	db, _ := versionFixture(t)
	ctx := context.Background()
	if _, err := db.sql.Exec(`insert into video_version_unbundles (key_a, key_b, created_at) values ('key-a', 'key-c', 1)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := moveKeyPairs(ctx, tx, "video_version_unbundles", "key-c", "key-0"); err != nil {
		t.Fatal(err)
	}
	var keyA, keyB string
	if err := tx.QueryRow(`select key_a, key_b from video_version_unbundles`).Scan(&keyA, &keyB); err != nil {
		t.Fatal(err)
	}
	if keyA != "key-0" || keyB != "key-a" {
		t.Errorf("付け替えた組 = (%s, %s), want (key-0, key-a)", keyA, keyB)
	}
}

func TestVersionJudgingSetting(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if enabled, err := db.Settings().VersionJudgingEnabled(ctx); err != nil || enabled {
		t.Fatalf("初期値 = %v, %v, want 切", enabled, err)
	}
	for _, want := range []bool{true, false} {
		if err := db.Settings().SaveVersionJudgingEnabled(ctx, want); err != nil {
			t.Fatal(err)
		}
		if got, err := db.Settings().VersionJudgingEnabled(ctx); err != nil || got != want {
			t.Errorf("保存した %v を読むと %v, %v", want, got, err)
		}
	}
}

func TestJudgementNoConnectionWaitsForConnection(t *testing.T) {
	db, _ := judgementFixture(t)
	ctx := context.Background()
	job := claimJudgement(t, db)
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionNoConnection})

	if judgement, attempts, retryAt, _ := candidateJudgement(t, db); judgement == nil || *judgement != "no_connection" ||
		attempts != 0 || retryAt != nil {
		t.Fatalf("判定 = %v, attempts %d, retry_at %v, want no_connection, 仕事なし", judgement, attempts, retryAt)
	}
	if pairs, _ := candidatePairs(t, db); len(pairs) != 1 {
		t.Errorf("no_connection の組が一覧から消えた: %v", pairs)
	}
	expectNoClaim(t, db, "no_connection の組")

	reset, err := db.Versions().ResetNoConnectionJudgements(ctx)
	if err != nil || reset != 1 {
		t.Fatalf("ResetNoConnectionJudgements = %d, %v, want 1", reset, err)
	}
	claimJudgement(t, db)
}

func TestOllamaConnectionSaved(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if saved, err := db.Settings().OllamaConnectionSaved(ctx); err != nil || saved {
		t.Fatalf("保存の前 = %v, %v, want 未保存", saved, err)
	}
	if err := db.Settings().SaveOllamaSettings(ctx, domain.DefaultOllamaSettings()); err != nil {
		t.Fatal(err)
	}
	if saved, err := db.Settings().OllamaConnectionSaved(ctx); err != nil || !saved {
		t.Errorf("保存のあと = %v, %v, want 保存済み", saved, err)
	}
}

func TestSavedOllamaSettings(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	check := func(step string, want domain.OllamaSettings, wantSaved bool) {
		t.Helper()
		got, saved, err := db.Settings().SavedOllamaSettings(ctx)
		if err != nil || got != want || saved != wantSaved {
			t.Fatalf("%s = %+v, %v, %v, want %+v, %v", step, got, saved, err, want, wantSaved)
		}
		if alone, err := db.Settings().OllamaConnectionSaved(ctx); err != nil || alone != saved {
			t.Fatalf("%s: OllamaConnectionSaved = %v, %v, want %v", step, alone, err, saved)
		}
	}
	check("保存の前", domain.DefaultOllamaSettings(), false)
	if err := writeSetting(ctx, db.sql, legacyOllamaModelKey, "old-model"); err != nil {
		t.Fatal(err)
	}
	check("模型の行だけ", domain.OllamaSettings{Endpoint: domain.DefaultOllamaEndpoint, Model: "old-model"}, false)
	if err := writeSetting(ctx, db.sql, legacyOllamaEndpointKey, "http://old:1"); err != nil {
		t.Fatal(err)
	}
	check("前の版のキー", domain.OllamaSettings{Endpoint: "http://old:1", Model: "old-model"}, true)
	want := domain.OllamaSettings{Endpoint: "http://gpu:11434", Model: "clef"}
	if err := db.Settings().SaveOllamaSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	check("保存のあと", want, true)
}

func TestJudgementSameKeepsTheRepresentativeTheOwnerChose(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b, c := ids["a"], ids["b"], ids["c"]
	bundle(t, db, a, a, c)
	job := claimJudgement(t, db)
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionSame})

	versions, err := db.Versions().Versions(context.Background(), domain.AudienceOwner, b)
	if err != nil {
		t.Fatal(err)
	}
	if versions.RepresentativeID != a || len(versions.Items) != 3 {
		t.Errorf("代表 = %d（%d 本）, want 持ち主が選んだ %d（3 本）", versions.RepresentativeID, len(versions.Items), a)
	}
}
