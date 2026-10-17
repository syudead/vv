package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 「同じ動画かもしれない」候補の自動の判定の保存と反映。

// judgementFixture は a（720p）と b（1080p）が候補の 1 組になり、c は指紋が違って候補にならない 3 本を
// 作る。d は取り込むだけ。
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

// claimJudgement は候補を 1 組取り出し、それが (key-a, key-b) であることを確かめる。
func claimJudgement(t *testing.T, db *DB) domain.VersionJudgeJob {
	t.Helper()
	job, err := db.Versions().ClaimJudgement(context.Background())
	if err != nil {
		t.Fatalf("ClaimJudgement: %v", err)
	}
	if job.Keys != [2]string{"key-a", "key-b"} {
		t.Fatalf("取り出した組 = %v, want [key-a key-b]", job.Keys)
	}
	return job
}

// applyJudgement は結果を反映し、反映したことと VersionCandidatesChanged の発行を確かめる。
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

// hasEvent は events に型 E の変化があるかを返す。
func hasEvent[E domain.Event](events []domain.Event) bool {
	for _, event := range events {
		if _, ok := event.(E); ok {
			return true
		}
	}
	return false
}

// candidateJudgement は (key-a, key-b) の候補の行の判定・失敗の数・再試行の時刻・確率を返す。
func candidateJudgement(t *testing.T, db *DB) (judgement *string, attempts int, retryAt *int64, probability *float64) {
	t.Helper()
	if err := db.sql.QueryRow(`select judgement, attempts, retry_at, probability from video_version_candidates
		where key_a = 'key-a' and key_b = 'key-b'`).Scan(&judgement, &attempts, &retryAt, &probability); err != nil {
		t.Fatal(err)
	}
	return judgement, attempts, retryAt, probability
}

func expectNoClaim(t *testing.T, db *DB, why string) {
	t.Helper()
	if job, err := db.Versions().ClaimJudgement(context.Background()); !errors.Is(err, domain.ErrNoJob) {
		t.Errorf("%s: ClaimJudgement = %v, %v, want domain.ErrNoJob", why, job.Keys, err)
	}
}

// 「同じ」は R-7 の代表で束ね（タグの付いた 720p が新しい 1080p の複製より代表に残る）、候補が消える
// （受け入れ条件 1・8）。取り出しは両方の指紋を返す。
func TestJudgementSameBundlesWithOwnersRepresentative(t *testing.T) {
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
	if versions.RepresentativeID != a || len(versions.Items) != 2 {
		t.Errorf("代表 = %d（%d 本）, want タグの付いた %d（2 本）", versions.RepresentativeID, len(versions.Items), a)
	}
	if rows := candidateRows(t, db); rows != 0 {
		t.Errorf("束ねたあとの候補の行 = %d, want 0", rows)
	}
	// 結果は 1 度だけ反映する。
	if applied, err := db.Versions().ApplyJudgement(context.Background(), job.Keys,
		domain.VersionJudgeResult{Verdict: domain.VersionSame}); err != nil || applied {
		t.Errorf("消えた候補への結果 = %v, %v, want 捨てる", applied, err)
	}
}

// 新しい動画と集まりの 1 本が「同じ」なら、新しい動画がその集まりに加わる。その集まりを手で分けて
// 束ね直した後も同じ（受け入れ条件 2、R-6: 同じ側の中の組は妨げない）。
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

// 「違う」は「違う動画」を記録して候補を消す（受け入れ条件 3・8）。
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

// 束ねる「同じ」は一致を変えうるので、すべてのスマートフォルダを名指す知らせを 1 つ伴う。束ねない
// 「違う」は伴わない。
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

// 却下した組をまたぐ「同じ」は候補を undecided に残し、undecided の組は取り出さない（受け入れ条件 4・5）。
// 却下は判定の途中で記録されても勝つ（R-6）。
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

// 「決められない」は確率を残し、その組は取り出さない（受け入れ条件 5・8）。
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

// failed の行は retry_at の後にだけ取り出され、その attempts は回復を経ても残る（受け入れ条件 6・8、R-5）。
func TestJudgementFailedRetriesAfterRetryAt(t *testing.T) {
	db, _ := judgementFixture(t)
	ctx := context.Background()
	job := claimJudgement(t, db)
	before := time.Now()
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: "connection refused"})

	judgement, attempts, retryAt, _ := candidateJudgement(t, db)
	if judgement == nil || *judgement != "failed" || attempts != 1 || retryAt == nil {
		t.Fatalf("判定 = %v, attempts %d, retry_at %v, want failed, 1, 10 分後", judgement, attempts, retryAt)
	}
	if want := before.Add(10 * time.Minute).Unix(); *retryAt < want-1 || *retryAt > want+5 {
		t.Errorf("retry_at = %d, want 約 %d", *retryAt, want)
	}
	var message string
	if err := db.sql.QueryRow(`select judgement_error from video_version_candidates`).Scan(&message); err != nil || message != "connection refused" {
		t.Errorf("judgement_error = %q, %v", message, err)
	}
	expectNoClaim(t, db, "retry_at の前の failed の組")

	if _, err := db.sql.Exec(`update video_version_candidates set retry_at = ?`, time.Now().Add(-time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	if job := claimJudgement(t, db); job.Attempts != 1 {
		t.Errorf("取り出した attempts = %d, want 1", job.Attempts)
	}
	// 判定中のまま終わった行は、起動時に待ちへ戻り、attempts を保つ。
	recovered, err := db.Versions().RecoverRunningJudgements(ctx)
	if err != nil || recovered != 1 {
		t.Fatalf("RecoverRunningJudgements = %d, %v, want 1", recovered, err)
	}
	if judgement, attempts, _, _ := candidateJudgement(t, db); judgement != nil || attempts != 1 {
		t.Errorf("回復したあと = %v, attempts %d, want null, 1", judgement, attempts)
	}
	job = claimJudgement(t, db)
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: "again"})
	if _, attempts, _, _ := candidateJudgement(t, db); attempts != 2 {
		t.Errorf("2 度目の失敗の attempts = %d, want 2", attempts)
	}
}

// Unbundle は外した動画と残る各メンバーの組を記録し、対応する候補が一覧に戻るが、その組は取り出さない
// （受け入れ条件 5・7・8、R-8）。
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

// 同じパスの中身が変わって引き継ぐと、外した組の鍵も付け替える。
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

// 重複の自動判定の設定は、行が無ければ切で、保存した値を返す。
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

// 接続が無くてモデルに聞けない組は no_connection になって取り出されず、一覧に残る。接続の保存で
// 待ちに戻る。
func TestJudgementNoConnectionWaitsForConnection(t *testing.T) {
	db, _ := judgementFixture(t)
	ctx := context.Background()
	job := claimJudgement(t, db)
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionNoConnection})

	if judgement, attempts, retryAt, _ := candidateJudgement(t, db); judgement == nil || *judgement != "no_connection" ||
		attempts != 0 || retryAt != nil {
		t.Fatalf("判定 = %v, attempts %d, retry_at %v, want no_connection, 0, null", judgement, attempts, retryAt)
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

// 次に取り出せる時刻は failed の行の retry_at のうち最も早いもので、無ければ見つからない（R-5）。
func TestNextJudgementRetry(t *testing.T) {
	db, _ := judgementFixture(t)
	ctx := context.Background()
	if _, found, err := db.Versions().NextJudgementRetry(ctx); err != nil || found {
		t.Fatalf("失敗の無いとき = %v, %v, want 見つからない", found, err)
	}
	job := claimJudgement(t, db)
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: "down"})
	_, _, retryAt, _ := candidateJudgement(t, db)
	next, found, err := db.Versions().NextJudgementRetry(ctx)
	if err != nil || !found || next.Unix() != *retryAt {
		t.Errorf("NextJudgementRetry = %v, %v, %v, want %d", next, found, err, *retryAt)
	}
}

// 取り出せない failed の組（両側の登録の所在が無くなった組）は次の再試行の時刻に数えない。数えると
// 取り出せない組の時刻を過ぎたあと、ワーカーが毎秒問い合わせ続ける（R-5）。
func TestNextJudgementRetryIgnoresUnclaimablePairs(t *testing.T) {
	db, _ := judgementFixture(t)
	ctx := context.Background()
	job := claimJudgement(t, db)
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: "down"})
	if _, err := db.sql.Exec(`delete from media_folders`); err != nil {
		t.Fatal(err)
	}
	if next, found, err := db.Versions().NextJudgementRetry(ctx); err != nil || found {
		t.Errorf("NextJudgementRetry = %v, %v, %v, want 見つからない", next, found, err)
	}
}

// Ollama の接続先は、行を保存したときだけ保存済みである。既定の接続先に倒した値は数えない（R-13）。
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

// SavedOllamaSettings は接続先と保存したかを 1 度に返し、保存したかは OllamaConnectionSaved と
// 同じ答えになる（前の版のキー、模型だけの行を含む。docs/design-docs/design-system.md）。
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
