package domain

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeAutoTagSettings(t *testing.T) {
	got, err := NormalizeAutoTagSettings(AutoTagSettings{
		Enabled: true, Endpoint: " http://gpu.local:11434/ ", Model: " clef ", Threshold: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := AutoTagSettings{Enabled: true, Endpoint: "http://gpu.local:11434", Model: "clef", Threshold: 1}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	valid := DefaultAutoTagSettings()
	for name, change := range map[string]func(*AutoTagSettings){
		"空の URL":   func(s *AutoTagSettings) { s.Endpoint = "" },
		"http 以外":  func(s *AutoTagSettings) { s.Endpoint = "ftp://host" },
		"ホスト無し":    func(s *AutoTagSettings) { s.Endpoint = "http://" },
		"資格情報つき":   func(s *AutoTagSettings) { s.Endpoint = "http://u:p@host" },
		"空の模型名":    func(s *AutoTagSettings) { s.Model = " " },
		"空白を含む模型名": func(s *AutoTagSettings) { s.Model = "clef flash" },
		"閾値 0":     func(s *AutoTagSettings) { s.Threshold = 0 },
		"閾値 1 超":   func(s *AutoTagSettings) { s.Threshold = 1.01 },
		"閾値 NaN":   func(s *AutoTagSettings) { s.Threshold = math.NaN() },
		"長すぎる模型名":  func(s *AutoTagSettings) { s.Model = strings.Repeat("m", autoTagMaxModelLength+1) },
	} {
		s := valid
		change(&s)
		if _, err := NormalizeAutoTagSettings(s); !errors.Is(err, ErrInvalidAutoTagSettings) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestNewAutoTagSubjectSplitsPaths(t *testing.T) {
	for path, want := range map[string]AutoTagSubject{
		"/media/旅行/京都/竹林.mp4":      {Title: "t", FileName: "竹林.mp4", Folder: "/media/旅行/京都"},
		`D:\videos\kids\relay.mov`: {Title: "t", FileName: "relay.mov", Folder: `D:\videos\kids`},
		"bare.mp4":                 {Title: "t", FileName: "bare.mp4"},
	} {
		if got := NewAutoTagSubject("t", path); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, want %+v", path, got, want)
		}
	}
}

func TestAutoTagQuestionForMentionsSynonyms(t *testing.T) {
	q := AutoTagQuestionFor(AutoTagCandidate{ID: 7, Name: "cat", Synonyms: []string{"kitty", "猫"}})
	if q.Key != "tag_7" {
		t.Errorf("key = %q", q.Key)
	}
	for _, part := range []string{`"cat"`, `"kitty", "猫"`} {
		if !strings.Contains(q.Instructions, part) {
			t.Errorf("instructions %q lacks %s", q.Instructions, part)
		}
	}
}

func TestChunkAutoTagCandidates(t *testing.T) {
	candidates := make([]AutoTagCandidate, AutoTagMaxQuestions*2+1)
	chunks := ChunkAutoTagCandidates(candidates)
	if len(chunks) != 3 || len(chunks[0]) != AutoTagMaxQuestions || len(chunks[2]) != 1 {
		t.Fatalf("chunk sizes = %d", len(chunks))
	}
	if ChunkAutoTagCandidates(nil) != nil {
		t.Fatal("空の候補に塊ができた")
	}
}

func TestSelectAutoTagsUsesThreshold(t *testing.T) {
	candidates := []AutoTagCandidate{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}
	probabilities := map[string]float64{"tag_1": 0.8, "tag_2": 0.79, "tag_3": math.NaN()}
	if got := SelectAutoTags(candidates, probabilities, 0.8); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("got %v", got)
	}
}

func TestAutoTagScope(t *testing.T) {
	if !AutoTagScopeMissing.Valid() || !AutoTagScopeAll.Valid() || AutoTagScope("x").Valid() {
		t.Fatal("Valid が既知の値を見分けない")
	}
	if AutoTagScopeMissing.QueueMode() != AutoTagQueueUnlessDone || AutoTagScopeAll.QueueMode() != AutoTagQueueAgain {
		t.Fatal("QueueMode の写しが違う")
	}
}
