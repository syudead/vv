package domain

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestValidateAutoTagSettings(t *testing.T) {
	want := AutoTagSettings{Enabled: true, AfterImport: true, Threshold: 1, TentativeThreshold: 0.6}
	if got, err := ValidateAutoTagSettings(want); err != nil || got != want {
		t.Fatalf("got %+v, %v", got, err)
	}
	if DefaultAutoTagSettings().Enabled || DefaultAutoTagSettings().AfterImport {
		t.Fatal("既定で機能か自動の判定が入っている")
	}
	valid := DefaultAutoTagSettings()
	for name, change := range map[string]func(*AutoTagSettings){
		"閾値 0":       func(s *AutoTagSettings) { s.Threshold = 0 },
		"閾値 1 超":     func(s *AutoTagSettings) { s.Threshold = 1.01 },
		"閾値 NaN":     func(s *AutoTagSettings) { s.Threshold = math.NaN() },
		"仮の閾値 0":     func(s *AutoTagSettings) { s.TentativeThreshold = 0 },
		"仮の閾値が閾値より上": func(s *AutoTagSettings) { s.TentativeThreshold = s.Threshold + 0.01 },
	} {
		s := valid
		change(&s)
		if _, err := ValidateAutoTagSettings(s); !errors.Is(err, ErrInvalidAutoTagSettings) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestNormalizeOllamaSettings(t *testing.T) {
	got, err := NormalizeOllamaSettings(OllamaSettings{Endpoint: " http://gpu.local:11434/ ", Model: " clef "})
	if err != nil {
		t.Fatal(err)
	}
	if want := (OllamaSettings{Endpoint: "http://gpu.local:11434", Model: "clef"}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	valid := DefaultOllamaSettings()
	for name, change := range map[string]func(*OllamaSettings){
		"空の URL":   func(s *OllamaSettings) { s.Endpoint = "" },
		"http 以外":  func(s *OllamaSettings) { s.Endpoint = "ftp://host" },
		"ホスト無し":    func(s *OllamaSettings) { s.Endpoint = "http://" },
		"資格情報つき":   func(s *OllamaSettings) { s.Endpoint = "http://u:p@host" },
		"空の模型名":    func(s *OllamaSettings) { s.Model = " " },
		"空白を含む模型名": func(s *OllamaSettings) { s.Model = "clef flash" },
		"長すぎる模型名":  func(s *OllamaSettings) { s.Model = strings.Repeat("m", ollamaMaxModelLength+1) },
	} {
		s := valid
		change(&s)
		if _, err := NormalizeOllamaSettings(s); !errors.Is(err, ErrInvalidOllamaSettings) {
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

func TestSelectAutoTagsSplitsByThresholds(t *testing.T) {
	candidates := []AutoTagCandidate{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}, {ID: 6}}
	probabilities := map[string]float64{"tag_1": 0.8, "tag_2": 0.79, "tag_3": 0.5, "tag_4": 0.49, "tag_5": math.NaN()}
	settings := AutoTagSettings{Threshold: 0.8, TentativeThreshold: 0.5}
	want := AutoTagSelection{Confirmed: []int64{1}, Tentative: []int64{2, 3}}
	if got := SelectAutoTags(candidates, probabilities, settings); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// 2 つの閾値が同じなら仮には付けない。
	settings.TentativeThreshold = 0.8
	if got := SelectAutoTags(candidates, probabilities, settings); len(got.Tentative) != 0 {
		t.Fatalf("tentative = %v", got.Tentative)
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

func TestSpreadFrameIndexes(t *testing.T) {
	for _, tc := range []struct {
		frames, n int
		want      []int
	}{
		{81, 8, []int{5, 15, 25, 35, 45, 55, 65, 75}},
		{150, 3, []int{25, 75, 125}},
		{4, 8, []int{0, 1, 2, 3}},
		{1, 8, []int{0}},
		{0, 8, nil},
		{81, 0, nil},
	} {
		if got := SpreadFrameIndexes(tc.frames, tc.n); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SpreadFrameIndexes(%d, %d) = %v, want %v", tc.frames, tc.n, got, tc.want)
		}
	}
}
