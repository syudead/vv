package domain

import (
	"errors"
	"math"
	"path/filepath"
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
	got, err := NormalizeOllamaSettings(OllamaSettings{Endpoint: " http://gpu.local:11434/ ", Model: " clef ", GenerationModel: " qwen3:8b "})
	if err != nil {
		t.Fatal(err)
	}
	if want := (OllamaSettings{Endpoint: "http://gpu.local:11434", Model: "clef", GenerationModel: "qwen3:8b"}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// An empty generation model turns the assistant off and is valid.
	if got, err := NormalizeOllamaSettings(OllamaSettings{Endpoint: "http://h", Model: "clef", GenerationModel: " "}); err != nil || got.GenerationModel != "" {
		t.Fatalf("空の生成用のモデル = %+v, %v", got, err)
	}
	valid := DefaultOllamaSettings()
	for name, change := range map[string]func(*OllamaSettings){
		"空の URL":        func(s *OllamaSettings) { s.Endpoint = "" },
		"http 以外":       func(s *OllamaSettings) { s.Endpoint = "ftp://host" },
		"ホスト無し":         func(s *OllamaSettings) { s.Endpoint = "http://" },
		"資格情報つき":        func(s *OllamaSettings) { s.Endpoint = "http://u:p@host" },
		"空の問い合わせ":       func(s *OllamaSettings) { s.Endpoint = "http://host?" },
		"空の断片":          func(s *OllamaSettings) { s.Endpoint = "http://host#" },
		"空の模型名":         func(s *OllamaSettings) { s.Model = " " },
		"空白を含む模型名":      func(s *OllamaSettings) { s.Model = "clef flash" },
		"長すぎる模型名":       func(s *OllamaSettings) { s.Model = strings.Repeat("m", ollamaMaxModelLength+1) },
		"空白を含む生成用のモデル名": func(s *OllamaSettings) { s.GenerationModel = "qwen 3" },
	} {
		s := valid
		change(&s)
		if _, err := NormalizeOllamaSettings(s); !errors.Is(err, ErrInvalidOllamaSettings) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestNewAutoTagSubjectUsesPathRelativeToMediaFolder(t *testing.T) {
	root := filepath.FromSlash("/mnt/nas/videos")
	for path, want := range map[string]AutoTagSubject{
		"/mnt/nas/videos/旅行/京都/竹林.mp4": {Title: "t", FileName: "竹林.mp4", Folder: "旅行/京都"},
		"/mnt/nas/videos/bare.mp4":     {Title: "t", FileName: "bare.mp4"},
		// Even invalid out-of-folder input must not expose an absolute path.
		"/elsewhere/secret/x.mp4": {Title: "t", FileName: "x.mp4"},
	} {
		if got := NewAutoTagSubject("t", root, filepath.FromSlash(path)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, want %+v", path, got, want)
		}
	}
}

func TestAutoTagQuestionForMentionsSynonyms(t *testing.T) {
	q := AutoTagQuestionFor(AutoTagCandidate{ID: 7, Name: "cat", Synonyms: []string{"kitty", "猫"}})
	if q.Key != "tag_7" || !strings.Contains(q.Text, "any part of this video") {
		t.Errorf("key = %q", q.Key)
	}
	for _, part := range []string{`"cat"`, `"kitty", "猫"`} {
		if !strings.Contains(q.Text, part) {
			t.Errorf("text %q lacks %s", q.Text, part)
		}
	}
}

func TestAutoTagSubjectClassifierInput(t *testing.T) {
	subject := NewAutoTagSubject("Kyoto", filepath.FromSlash("/media"), filepath.FromSlash("/media/travel/bamboo.mp4"))
	subject.Images = [][]byte{[]byte("thumb")}
	want := ClassifierInput{
		Facts:  map[string]string{"title": "Kyoto", "file_name": "bamboo.mp4", "folder": "travel"},
		Images: [][]byte{[]byte("thumb")},
	}
	if got := subject.ClassifierInput(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestSelectAutoTagsSplitsByThresholds(t *testing.T) {
	candidates := []AutoTagCandidate{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}, {ID: 6}}
	probabilities := map[string]float64{"tag_1": 0.8, "tag_2": 0.79, "tag_3": 0.5, "tag_4": 0.49, "tag_5": math.NaN()}
	settings := AutoTagSettings{Threshold: 0.8, TentativeThreshold: 0.5}
	want := AutoTagSelection{
		Confirmed: []int64{1}, Tentative: []int64{2, 3}, Probabilities: map[int64]float64{1: 0.8, 2: 0.79, 3: 0.5},
	}
	if got := SelectAutoTags(candidates, probabilities, settings); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
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

func TestAutoTagChoiceForNamesSynonyms(t *testing.T) {
	req := AutoTagChoiceFor(ClassifierInput{}, []AutoTagCandidate{{ID: 7, Name: "cat", Synonyms: []string{"kitten", "neko"}}})
	if len(req.Options) != 1 || req.Options[0].Key != "tag_7" || req.Options[0].Text != "cat (also known as kitten, neko)" {
		t.Fatalf("options = %+v", req.Options)
	}
	if req.None == "" {
		t.Fatal("no none option")
	}
}

// The shortlist keeps candidates at or above the floor, the most probable AutoTagShortlistSize of them, in
// candidate order.
func TestShortlistAutoTags(t *testing.T) {
	var candidates []AutoTagCandidate
	probabilities := map[string]float64{}
	for id := int64(1); id <= AutoTagShortlistSize+2; id++ {
		candidates = append(candidates, AutoTagCandidate{ID: id})
		probabilities[AutoTagQuestionKey(id)] = 0.03 + float64(id)/1000
	}
	probabilities[AutoTagQuestionKey(1)] = 0.5
	probabilities[AutoTagQuestionKey(3)] = AutoTagShortlistFloor - 0.001
	candidates = append(candidates, AutoTagCandidate{ID: 99})

	few := ShortlistAutoTags(candidates[:4], probabilities)
	if len(few) != 3 || few[0].ID != 1 || few[1].ID != 2 || few[2].ID != 4 {
		t.Fatalf("shortlist of few = %v", few)
	}
	got := ShortlistAutoTags(candidates, probabilities)
	if len(got) != AutoTagShortlistSize {
		t.Fatalf("shortlist = %d candidates", len(got))
	}
	// 3 is below the floor, 99 has no answer, and 2 is the least probable of the rest.
	if got[0].ID != 1 || got[1].ID != 4 || got[len(got)-1].ID != AutoTagShortlistSize+2 {
		t.Fatalf("shortlist = %v", got)
	}
}
