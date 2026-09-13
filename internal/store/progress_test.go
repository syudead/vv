package store

import (
	"context"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 鍵は content_key（videos.id ではない）。ファイルを移動・改名・置き直しても
// 再生位置が引き継がれる（FR-025／FR-026 / R-111）。
func TestSaveAndLoadProgressByContentKey(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	saved, err := db.SaveProgress(ctx, "key-a", domain.EvaluateProgress(4000, 600_000))
	if err != nil {
		t.Fatal(err)
	}
	if saved.PositionMs != 4000 {
		t.Errorf("PositionMs = %d, want 4000", saved.PositionMs)
	}
	if saved.UpdatedAt.IsZero() {
		t.Error("更新時刻が入っていない")
	}

	got, err := db.Progress(ctx, "key-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.PositionMs != 4000 || got.Completed {
		t.Errorf("読み直した値 = %+v", got)
	}
}

// 記録が無ければ「無い」と分かる誤りを返す。一覧では省略される。
func TestProgressWhenNeverPlayed(t *testing.T) {
	db := migratedDB(t)

	if _, err := db.Progress(context.Background(), "key-未再生"); err == nil {
		t.Error("記録が無いのに値が返った")
	}
}

// upsert で最後の書き込みが残る。複数のタブ・端末の競合はこれで割り切る
// （spec のエッジケース）。
func TestSaveProgressKeepsLastWrite(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	for _, position := range []int64{1000, 5000, 3000} {
		if _, err := db.SaveProgress(ctx, "key-a", domain.EvaluateProgress(position, 600_000)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := db.Progress(ctx, "key-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.PositionMs != 3000 {
		t.Errorf("PositionMs = %d, want 3000（最後の書き込みが残る）", got.PositionMs)
	}

	var rows int
	if err := db.SQL().QueryRow(`select count(*) from playback_progress`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("行 = %d, want 1（上書きであって追記ではない）", rows)
	}
}

// 動画の行を削除しても再生位置は残る（FR-025）。
// 「再構築できる索引」と「再構築できない利用者データ」の分離そのものである。
func TestProgressSurvivesVideoDeletion(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	added, err := db.UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveProgress(ctx, "key-a", domain.EvaluateProgress(4000, 600_000)); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteVideos(ctx, []int64{added.ID}); err != nil {
		t.Fatal(err)
	}

	got, err := db.Progress(ctx, "key-a")
	if err != nil {
		t.Fatalf("動画を消したら再生位置も消えた: %v", err)
	}
	if got.PositionMs != 4000 {
		t.Errorf("PositionMs = %d, want 4000", got.PositionMs)
	}
}

// 同じ内容のファイルを別のパスに置き直しても、同じ再生位置が引き継がれる
// （SC-008 / S7）。content_key を鍵にした判断が実際に効いていることの確認。
func TestProgressFollowsContentAcrossPaths(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	if _, err := db.UpsertVideo(ctx, sampleFile("/media/元.mp4", "元", "key-a", 1024, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveProgress(ctx, "key-a", domain.EvaluateProgress(4000, 600_000)); err != nil {
		t.Fatal(err)
	}

	// 消して、同じ内容を別の場所へ置き直す。
	if err := db.DeleteVideos(ctx, []int64{1}); err != nil {
		t.Fatal(err)
	}
	moved, err := db.UpsertVideo(ctx, sampleFile("/media/2026/別名.mp4", "別名", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}

	byKey, err := db.ProgressByContentKeys(ctx, []string{"key-a"})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := byKey["key-a"]
	if !ok {
		t.Fatalf("置き直したあとに再生位置が引けない (video=%d)", moved.ID)
	}
	if got.PositionMs != 4000 {
		t.Errorf("PositionMs = %d, want 4000", got.PositionMs)
	}
}

// 一覧に載せるため、複数の content_key をまとめて引ける。1件ずつ引くと
// 60 件の一覧で 60 回の問い合わせになる。
func TestProgressByContentKeys(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	for _, key := range []string{"key-a", "key-b", "key-c"} {
		if _, err := db.SaveProgress(ctx, key, domain.EvaluateProgress(1000, 600_000)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := db.ProgressByContentKeys(ctx, []string{"key-a", "key-c", "key-未再生"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("引けた数 = %d, want 2", len(got))
	}
	if _, ok := got["key-b"]; ok {
		t.Error("求めていない鍵が返った")
	}

	// 空の指定で全件を引かない。
	empty, err := db.ProgressByContentKeys(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("空の指定で %d 件返った", len(empty))
	}
}

// 視聴済みの判定結果が保存され、読み直しても保たれること。
func TestSaveProgressStoresCompletion(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	if _, err := db.SaveProgress(ctx, "key-a", domain.EvaluateProgress(7_990, 8_000)); err != nil {
		t.Fatal(err)
	}

	got, err := db.Progress(ctx, "key-a")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Completed {
		t.Error("視聴済みが保存されていない")
	}
	if got.ResumePosition() != 0 {
		t.Errorf("見終わった動画の再開位置 = %d, want 0", got.ResumePosition())
	}
	if got.UpdatedAt.Before(time.Now().Add(-time.Minute)) {
		t.Errorf("更新時刻が古い: %v", got.UpdatedAt)
	}
}
