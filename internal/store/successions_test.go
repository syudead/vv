package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 同じパスの中身の引き継ぎ（specs/030-video-versions/data-model.md §5、research.md R-5）。

func probeDuration(t *testing.T, db *DB, videoID, durationMs int64) {
	t.Helper()
	probe := domain.Probe{DurationMs: durationMs, VideoCodec: "h264", AudioCodec: "aac"}
	if err := db.Ingest().ApplyProbe(context.Background(), videoID, probe, domain.EvaluatePlayability("mp4", probe)); err != nil {
		t.Fatal(err)
	}
}

func startTestScan(t *testing.T, db *DB) int64 {
	t.Helper()
	scan, _, err := db.Scans().StartScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return scan.ID
}

func finishTestScan(t *testing.T, db *DB, id int64, state domain.ScanState) {
	t.Helper()
	if err := db.Scans().FinishScan(context.Background(), id, state, nil); err != nil {
		t.Fatal(err)
	}
}

func upsertOne(t *testing.T, db *DB, file domain.VideoFile) int64 {
	t.Helper()
	return upsertAll(t, db, file)[file.Path]
}

func successionCount(t *testing.T, db *DB) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from video_successions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func saveTestProgress(t *testing.T, db *DB, key string, positionMs int64) {
	t.Helper()
	if _, err := db.Playback().SaveProgress(context.Background(), key,
		domain.Progress{PositionMs: positionMs, DurationMs: 100_000}, nil); err != nil {
		t.Fatal(err)
	}
}

// progressAt は動画の利用者データの鍵の再生位置を返す。記録が無ければ -1。
func progressAt(t *testing.T, db *DB, videoID int64) int64 {
	t.Helper()
	progress, err := db.Playback().Progress(context.Background(), ownerVideo(t, db, videoID).UserKey)
	if errors.Is(err, domain.ErrNotFound) {
		return -1
	}
	if err != nil {
		t.Fatal(err)
	}
	return progress.PositionMs
}

// taggedVideoFixture は /media/a.mp4 に尺 100 秒の動画 A を取り込み、タグと再生位置を付ける。
func taggedVideoFixture(t *testing.T) (*DB, int64) {
	t.Helper()
	db := migratedDB(t)
	a := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1))
	probeDuration(t, db, a, 100_000)
	attachNamedTag(t, db, "好き", a)
	saveTestProgress(t, db, "key-a", 40_000)
	return db, a
}

// 尺がほぼ同じ新しい中身は、走査が done で閉じて解析が終わると前の動画のタグと再生位置を
// 引き継ぐ（受け入れ条件 1）。尺が幅の外なら何も付かない（受け入れ条件 2）。
func TestSuccessionCarriesUserDataWhenDurationsMatch(t *testing.T) {
	for name, c := range map[string]struct {
		durationMs int64
		carried    bool
	}{
		"尺が同じ":  {100_500, true},
		"尺が幅の外": {120_000, false},
	} {
		t.Run(name, func(t *testing.T) {
			db, _ := taggedVideoFixture(t)
			scan := startTestScan(t, db)
			b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
			finishTestScan(t, db, scan, domain.ScanDone)
			probeDuration(t, db, b, c.durationMs)

			tags := manualTagNames(t, db, ownerVideo(t, db, b).UserKey)
			progress := progressAt(t, db, b)
			if c.carried {
				if !slices.Equal(tags, []string{"好き"}) || progress != 40_000 {
					t.Errorf("引き継がない: tags = %v, progress = %d", tags, progress)
				}
			} else if len(tags) != 0 || progress != -1 {
				t.Errorf("尺が違うのに引き継いだ: tags = %v, progress = %d", tags, progress)
			}
			if successionCount(t, db) != 0 {
				t.Errorf("判定した記録が残った")
			}
		})
	}
}

// 前の尺が分からない（解析前・失敗）動画は記録しない。
func TestSuccessionNotRecordedWithoutPreviousDuration(t *testing.T) {
	db := migratedDB(t)
	upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1))
	upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
	if got := successionCount(t, db); got != 0 {
		t.Fatalf("記録 = %d, want 0", got)
	}
}

// 同じ走査で前の中身が別のパスに現れると記録は消え、引き継がない（Edge Case「ファイルの入れ替え」）。
func TestSuccessionCancelledWhenPreviousContentMoves(t *testing.T) {
	db, _ := taggedVideoFixture(t)
	scan := startTestScan(t, db)
	b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
	if successionCount(t, db) != 1 {
		t.Fatal("記録しない")
	}
	moved := upsertOne(t, db, listingFile(fixturePath("/media/moved.mp4"), "moved", "key-a", 1))
	if successionCount(t, db) != 0 {
		t.Fatal("前の中身が現れても記録が残った")
	}
	finishTestScan(t, db, scan, domain.ScanDone)
	probeDuration(t, db, b, 100_000)
	if tags := manualTagNames(t, db, ownerVideo(t, db, b).UserKey); len(tags) != 0 {
		t.Errorf("入れ替えで引き継いだ: %v", tags)
	}
	if ownerVideo(t, db, moved).ContentKey != "key-a" {
		t.Error("移った前の中身が key-a でない")
	}
}

// 解析が走査の途中で終わっても、走査が done で閉じるまで引き継がない。その後に前の中身が
// 別のパスで見つかる順序でも引き継がない。failed で閉じた走査は判定しない。
func TestSuccessionWaitsForTheScanToFinish(t *testing.T) {
	t.Run("途中の解析のあと前の中身が現れる", func(t *testing.T) {
		db, _ := taggedVideoFixture(t)
		scan := startTestScan(t, db)
		b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
		probeDuration(t, db, b, 100_000)
		if tags := manualTagNames(t, db, ownerVideo(t, db, b).UserKey); len(tags) != 0 {
			t.Fatalf("走査の途中で引き継いだ: %v", tags)
		}
		upsertOne(t, db, listingFile(fixturePath("/media/moved.mp4"), "moved", "key-a", 1))
		finishTestScan(t, db, scan, domain.ScanDone)
		if tags := manualTagNames(t, db, ownerVideo(t, db, b).UserKey); len(tags) != 0 {
			t.Errorf("前の中身が残るのに引き継いだ: %v", tags)
		}
	})
	t.Run("途中の解析は走査が done で閉じると判定する", func(t *testing.T) {
		db, _ := taggedVideoFixture(t)
		scan := startTestScan(t, db)
		b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
		probeDuration(t, db, b, 100_000)
		finishTestScan(t, db, scan, domain.ScanFailed)
		if tags := manualTagNames(t, db, ownerVideo(t, db, b).UserKey); len(tags) != 0 || successionCount(t, db) != 1 {
			t.Fatalf("failed で閉じた走査が判定した: tags = %v", tags)
		}
		next := startTestScan(t, db)
		finishTestScan(t, db, next, domain.ScanDone)
		if tags := manualTagNames(t, db, ownerVideo(t, db, b).UserKey); !slices.Equal(tags, []string{"好き"}) {
			t.Errorf("done で閉じても引き継がない: %v", tags)
		}
	})
}

// 解析が失敗しても記録は残り、やり直しで成功したときに判定する。
func TestSuccessionSurvivesProbeFailureUntilRetrySucceeds(t *testing.T) {
	db, _ := taggedVideoFixture(t)
	ctx := context.Background()
	scan := startTestScan(t, db)
	b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
	finishTestScan(t, db, scan, domain.ScanDone)
	if err := db.Ingest().MarkProbeFailed(ctx, b, errors.New("broken")); err != nil {
		t.Fatal(err)
	}
	if successionCount(t, db) != 1 {
		t.Fatal("解析の失敗で記録が消えた")
	}
	if err := db.Ingest().RetryProbe(ctx, b); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{DurationMs: 99_500, VideoCodec: "h264", AudioCodec: "aac"}
	if written, err := db.Ingest().ApplyProbeForJob(ctx, job, probe, domain.EvaluatePlayability("mp4", probe)); err != nil || !written {
		t.Fatalf("解析の結果を書けない: %v, %v", written, err)
	}
	if got := progressAt(t, db, b); got != 40_000 {
		t.Errorf("やり直しの成功で引き継がない: progress = %d", got)
	}
}

// 前の中身が集まりの代表なら新しい中身が代表になり、代表以外なら同じ集まりのメンバーになる。
// 引き継ぐと、その集まりの全メンバーの VideoBundleChanged が発行される。
func TestSuccessionKeepsTheBundlePosition(t *testing.T) {
	for name, representative := range map[string]bool{"代表": true, "代表以外": false} {
		t.Run(name, func(t *testing.T) {
			db, ids := versionFixture(t)
			ctx := context.Background()
			for _, id := range []int64{ids["a"], ids["b"], ids["c"]} {
				probeDuration(t, db, id, 100_000)
			}
			rep := ids["b"]
			if representative {
				rep = ids["a"]
			}
			bundle(t, db, rep, ids["a"], ids["b"], ids["c"])
			bundleKey := ownerVideo(t, db, ids["a"]).UserKey

			scan := startTestScan(t, db)
			x := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-x", 9))
			finishTestScan(t, db, scan, domain.ScanDone)
			recorder := &eventRecorder{}
			db.PublishTo(recorder)
			probeDuration(t, db, x, 100_000)

			if got := ownerVideo(t, db, x).UserKey; got != bundleKey {
				t.Fatalf("新しい中身の鍵 = %q, want %q", got, bundleKey)
			}
			versions, err := db.Versions().Versions(ctx, domain.AudienceOwner, x)
			if err != nil {
				t.Fatal(err)
			}
			wantRep := ids["b"]
			if representative {
				wantRep = x
			}
			if versions.RepresentativeID != wantRep || len(versions.Items) != 3 {
				t.Errorf("集まり = 代表 %d, %d 本, want 代表 %d, 3 本", versions.RepresentativeID, len(versions.Items), wantRep)
			}
			want := []domain.Event{domain.VideoBundleChanged{VideoIDs: normalizeIDs([]int64{ids["b"], ids["c"], x})}}
			if !slices.EqualFunc(recorder.events, want, eventsEqual) {
				t.Errorf("events = %v, want %v", recorder.events, want)
			}
		})
	}
}

// 新しい中身の参照が無くなると、判定前の記録も消える（data-model.md §1）。
func TestSuccessionReleasedWithTheNewContent(t *testing.T) {
	db, _ := taggedVideoFixture(t)
	b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
	if err := db.ScanIndex().DeleteVideos(context.Background(), []int64{b}); err != nil {
		t.Fatal(err)
	}
	if got := successionCount(t, db); got != 0 {
		t.Errorf("記録 = %d, want 0", got)
	}
}
