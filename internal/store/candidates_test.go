package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 「同じ動画かもしれない」候補と「違う動画」の記録（specs/030-video-versions/data-model.md §1・§7・§8、
// research.md R-7）。

// testFingerprint は 12 コマの指紋を返す。コマ i のハッシュは seed から決まり、flip のビットを
// 反転する（flip が小さければ同じ場面の再エンコードに当たる）。
func testFingerprint(seed, flip uint64) domain.Fingerprint {
	frames := make([]domain.FrameHash, 12)
	state := seed
	for i := range frames {
		state = state*6364136223846793005 + 1442695040888963407
		frames[i] = domain.FrameHash{Hash: state ^ flip}
	}
	return domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: 5000, Frames: frames}
}

// probeWithDuration は尺 durationMs で解析の完了を記録する。
func probeWithDuration(t *testing.T, db *DB, videoID, durationMs int64) {
	t.Helper()
	probe := domain.Probe{DurationMs: durationMs, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(context.Background(), videoID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
}

// applyFingerprint は動画の指紋の仕事を積んで専有し、指紋を書いて完了を記録する。
func applyFingerprint(t *testing.T, db *DB, videoID int64, fingerprint domain.Fingerprint) {
	t.Helper()
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, domain.JobFingerprint, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if job.VideoID != videoID {
		t.Fatalf("専有した指紋の仕事の動画 = %d, want %d", job.VideoID, videoID)
	}
	if applied, err := db.Ingest().ApplyFingerprintForJob(ctx, job, fingerprint); err != nil || !applied {
		t.Fatalf("ApplyFingerprintForJob = %v, %v", applied, err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
}

// fingerprintVideo は尺と指紋を持つ動画にする（解析・シーク用サムネイル・指紋）。
func fingerprintVideo(t *testing.T, db *DB, videoID, durationMs int64, fingerprint domain.Fingerprint) {
	t.Helper()
	probeWithDuration(t, db, videoID, durationMs)
	seekDone(t, db, videoID)
	applyFingerprint(t, db, videoID, fingerprint)
}

// candidatePairs は候補の一覧の各組の動画の id を返す。
func candidatePairs(t *testing.T, db *DB) ([][2]int64, domain.VersionCandidatePage) {
	t.Helper()
	page, err := db.Versions().Candidates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pairs := [][2]int64{}
	for _, item := range page.Items {
		pairs = append(pairs, [2]int64{item.Videos[0].ID, item.Videos[1].ID})
	}
	return pairs, page
}

// candidateRows は候補の表の行の数である。
func candidateRows(t *testing.T, db *DB) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from video_version_candidates`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// candidateFixture は a と b が尺の幅の内で指紋が一致し、c は尺だけ a と同じで指紋が違い、d は
// 指紋が a と同じで尺が幅の外である 4 本を作る。
func candidateFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db, ids := versionFixture(t)
	fingerprintVideo(t, db, ids["a"], 100_000, testFingerprint(1, 0))
	fingerprintVideo(t, db, ids["b"], 100_400, testFingerprint(1, 0b101))
	fingerprintVideo(t, db, ids["c"], 100_000, testFingerprint(2, 0))
	fingerprintVideo(t, db, ids["d"], 200_000, testFingerprint(1, 0))
	return db, ids
}

// 尺が幅の内で指紋が一致する 2 本は候補の 1 組になり（受け入れ条件 3）、尺だけ同じで指紋が違う
// 動画と、指紋が同じで尺が違う動画は候補にならない（受け入れ条件 4）。
func TestVersionCandidatesFromFingerprints(t *testing.T) {
	db, ids := candidateFixture(t)
	a, b := ids["a"], ids["b"]

	pairs, page := candidatePairs(t, db)
	if len(pairs) != 1 || pairs[0] != [2]int64{min(a, b), max(a, b)} || page.Total != 1 {
		t.Fatalf("候補 = %v (total %d), want [[%d %d]]", pairs, page.Total, a, b)
	}
	item := page.Items[0]
	if item.Distance != 2 {
		t.Errorf("距離 = %d, want 2", item.Distance)
	}
	for _, video := range item.Videos {
		if video.Path == "" || video.DurationMs == nil {
			t.Errorf("候補の動画 %d に所在か尺が無い: %+v", video.ID, video)
		}
	}
	if rows := candidateRows(t, db); rows != 1 {
		t.Errorf("候補の行 = %d, want 1", rows)
	}

	// 指紋を書き直しても組は 1 つのまま。
	applyFingerprint(t, db, a, testFingerprint(1, 0))
	if pairs, _ := candidatePairs(t, db); len(pairs) != 1 {
		t.Errorf("書き直したあとの候補 = %v", pairs)
	}
	// 指紋が変わって一致しなくなれば組は消える。
	applyFingerprint(t, db, b, testFingerprint(3, 0))
	if pairs, _ := candidatePairs(t, db); len(pairs) != 0 {
		t.Errorf("一致しなくなったあとの候補 = %v", pairs)
	}
}

// 「違う動画」と記録した組は候補から消え、指紋を書き直しても候補にならない（受け入れ条件 5）。
// 候補に無い組も記録でき、無い動画は domain.ErrNotFound。
func TestDismissVersionCandidate(t *testing.T) {
	db, ids := candidateFixture(t)
	ctx := context.Background()
	a, b := ids["a"], ids["b"]

	if err := db.Versions().Dismiss(ctx, [2]int64{b, a}); err != nil {
		t.Fatal(err)
	}
	if pairs, _ := candidatePairs(t, db); len(pairs) != 0 {
		t.Fatalf("却下したあとの候補 = %v", pairs)
	}
	var keyA, keyB string
	if err := db.sql.QueryRow(`select key_a, key_b from video_version_dismissals`).Scan(&keyA, &keyB); err != nil {
		t.Fatal(err)
	}
	if keyA != "key-a" || keyB != "key-b" {
		t.Errorf("却下の行 = (%s, %s), want (key-a, key-b)", keyA, keyB)
	}
	applyFingerprint(t, db, b, testFingerprint(1, 0b101))
	applyFingerprint(t, db, a, testFingerprint(1, 0))
	if rows := candidateRows(t, db); rows != 0 {
		t.Errorf("却下した組の指紋を書き直したら候補の行が %d できた", rows)
	}

	// 候補に無い組も記録する。2 度目も成功する。
	for range 2 {
		if err := db.Versions().Dismiss(ctx, [2]int64{ids["c"], ids["d"]}); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.sql.QueryRow(`select count(*) from video_version_dismissals`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("却下の行 = %d, want 2", count)
	}
	if err := db.Versions().Dismiss(ctx, [2]int64{a, 999_999}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い動画の却下 = %v, want domain.ErrNotFound", err)
	}
}

// 束ねると同じ集まりになった組の候補が消え、同じ集まりの 2 本は指紋を書き直しても候補にならない。
func TestBundleRemovesVersionCandidate(t *testing.T) {
	db, ids := candidateFixture(t)
	a, b := ids["a"], ids["b"]

	bundle(t, db, a, a, b)
	if rows := candidateRows(t, db); rows != 0 {
		t.Fatalf("束ねたあとの候補の行 = %d, want 0", rows)
	}
	applyFingerprint(t, db, b, testFingerprint(1, 0b101))
	if rows := candidateRows(t, db); rows != 0 {
		t.Errorf("同じ集まりの 2 本の指紋を書き直したら候補の行が %d できた", rows)
	}
}

// 片方の動画の行が消えると、その鍵の候補が消える（Edge Case）。
func TestDeletedVideoRemovesVersionCandidate(t *testing.T) {
	db, ids := candidateFixture(t)
	if err := db.ScanIndex().DeleteVideos(context.Background(), []int64{ids["b"]}); err != nil {
		t.Fatal(err)
	}
	if rows := candidateRows(t, db); rows != 0 {
		t.Errorf("片方を消したあとの候補の行 = %d, want 0", rows)
	}
}

// 片方が登録フォルダの下に所在を持たない組は一覧に出ない。
func TestVersionCandidatesNeedRegisteredLocations(t *testing.T) {
	db, _ := candidateFixture(t)
	if _, err := db.sql.Exec(`delete from media_folders`); err != nil {
		t.Fatal(err)
	}
	pairs, page := candidatePairs(t, db)
	if len(pairs) != 0 || page.Total != 0 {
		t.Errorf("登録の所在の無い候補 = %v (total %d)", pairs, page.Total)
	}
}
