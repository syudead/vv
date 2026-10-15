package domain

import (
	"testing"
	"time"
)

// 失敗した判定は 10 分、1 時間、6 時間、以後は毎日あとに取り出せる（specs/054-auto-dup/research.md R-5）。
func TestVersionJudgeRetryAt(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, c := range []struct {
		attempts int
		want     time.Duration
	}{
		{0, 10 * time.Minute},
		{1, 10 * time.Minute},
		{2, time.Hour},
		{3, 6 * time.Hour},
		{4, 24 * time.Hour},
		{40, 24 * time.Hour},
	} {
		if got := VersionJudgeRetryAt(c.attempts, now).Sub(now); got != c.want {
			t.Errorf("%d 回目の失敗の待ち = %v, want %v", c.attempts, got, c.want)
		}
	}
}

// 側をまたぐ「違う」と分かっている組だけが束ねるのを妨げる（research.md R-6）。
func TestVersionSidesConflict(t *testing.T) {
	a, b := []string{"a", "c"}, []string{"b"}
	for _, c := range []struct {
		name      string
		differing [][2]string
		want      bool
	}{
		{"組なし", nil, false},
		{"同じ側の中の組", [][2]string{{"a", "c"}}, false},
		{"側をまたぐ組", [][2]string{{"b", "c"}}, true},
		{"鍵の順に依らない", [][2]string{{"c", "b"}}, true},
		{"どちらの側でもない鍵", [][2]string{{"b", "x"}}, false},
	} {
		if got := VersionSidesConflict(a, b, c.differing); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// 代表は集まりの側、所有者のデータ、再生できる、音声、画素、ビットレート、id の順に選ぶ（research.md R-7）。
func TestChooseVersionRepresentative(t *testing.T) {
	ptr := func(v int) *int { return &v }
	ms := func(v int64) *int64 { return &v }
	base := func(id int64) Video {
		return Video{ID: id, Playable: true, AudioCodec: "aac", Width: ptr(1280), Height: ptr(720),
			SizeBytes: 1000, DurationMs: ms(100_000)}
	}
	hd := base(2)
	hd.Width, hd.Height = ptr(1920), ptr(1080)
	silent := base(2)
	silent.AudioCodec = ""
	unplayable := base(2)
	unplayable.Playable = false
	dense := base(2)
	dense.SizeBytes = 2000
	for _, c := range []struct {
		name  string
		sides [2]VersionSide
		want  int
	}{
		{"タグの付いた 720p は新しい 1080p に勝つ", [2]VersionSide{{Video: base(1), OwnerData: true}, {Video: hd}}, 0},
		{"集まりの側が勝つ", [2]VersionSide{{Video: hd, OwnerData: true}, {Video: base(1), Bundled: true}}, 1},
		{"両方が集まりなら規則で比べる", [2]VersionSide{{Video: base(1), Bundled: true}, {Video: hd, Bundled: true}}, 1},
		{"再生できる方", [2]VersionSide{{Video: unplayable}, {Video: base(3)}}, 1},
		{"音声がある方", [2]VersionSide{{Video: silent}, {Video: base(3)}}, 1},
		{"画素の多い方", [2]VersionSide{{Video: base(1)}, {Video: hd}}, 1},
		{"ビットレートの高い方", [2]VersionSide{{Video: base(1)}, {Video: dense}}, 1},
		{"id の小さい方", [2]VersionSide{{Video: base(3)}, {Video: base(2)}}, 1},
	} {
		if got := ChooseVersionRepresentative(c.sides); got != c.want {
			t.Errorf("%s: 側 %d, want %d", c.name, got, c.want)
		}
	}
}
