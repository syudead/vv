package domain

import (
	"errors"
	"testing"
)

func TestApplyComfyUISettingsUpdate(t *testing.T) {
	key := func(s string) *string { return &s }
	model := func(m ComfyUIModel) *ComfyUIModel { return &m }
	saved := ComfyUISettings{Provider: ComfyUICloud, URL: "http://gpu:8188", APIKey: "k1", Model: ComfyUIModelQwenImageEdit}
	cases := []struct {
		name    string
		current ComfyUISettings
		update  ComfyUISettingsUpdate
		want    ComfyUISettings
		wantErr error
	}{
		{"自前の URL を ollama.endpoint と同じに整える", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUISelfHosted, URL: " http://gpu:8188/ "},
			ComfyUISettings{Provider: ComfyUISelfHosted, URL: "http://gpu:8188"}, nil},
		{"キーが無ければ保存済みのキーを残す", saved,
			ComfyUISettingsUpdate{Provider: ComfyUICloud},
			saved, nil},
		{"空文字列のキーは消す", saved,
			ComfyUISettingsUpdate{Provider: ComfyUISelfHosted, URL: "https://h", APIKey: key("")},
			ComfyUISettings{Provider: ComfyUISelfHosted, URL: "https://h", Model: ComfyUIModelQwenImageEdit}, nil},
		{"Cloud は要求の URL を使わない", ComfyUISettings{URL: "http://old"},
			ComfyUISettingsUpdate{Provider: ComfyUICloud, URL: "ftp://x", APIKey: key(" k2 ")},
			ComfyUISettings{Provider: ComfyUICloud, URL: "http://old", APIKey: "k2"}, nil},
		{"自前の URL が無い", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUISelfHosted}, ComfyUISettings{}, ErrComfyUIURLInvalid},
		{"自前の URL が http でない", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUISelfHosted, URL: "ftp://h"}, ComfyUISettings{}, ErrComfyUIURLInvalid},
		{"自前の URL に利用者が入る", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUISelfHosted, URL: "http://u:p@h"}, ComfyUISettings{}, ErrComfyUIURLInvalid},
		{"自前の URL が空の問い合わせで終わる", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUISelfHosted, URL: "http://h?"}, ComfyUISettings{}, ErrComfyUIURLInvalid},
		{"自前の URL が空の断片で終わる", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUISelfHosted, URL: "http://h/#"}, ComfyUISettings{}, ErrComfyUIURLInvalid},
		{"Cloud のキーが保存も送信もされていない", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUICloud}, ComfyUISettings{}, ErrComfyUIAPIKeyMissing},
		{"Cloud でキーを消す", saved,
			ComfyUISettingsUpdate{Provider: ComfyUICloud, APIKey: key("")}, ComfyUISettings{}, ErrComfyUIAPIKeyMissing},
		{"キーに空白が入る", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUICloud, APIKey: key("a b")}, ComfyUISettings{}, ErrInvalidComfyUISettings},
		{"モデルが無ければ保存済みのモデルを残す", saved,
			ComfyUISettingsUpdate{Provider: ComfyUICloud},
			saved, nil},
		{"モデルを選ぶ", saved,
			ComfyUISettingsUpdate{Provider: ComfyUICloud, Model: model(ComfyUIModelFlux2Klein)},
			ComfyUISettings{Provider: ComfyUICloud, URL: "http://gpu:8188", APIKey: "k1", Model: ComfyUIModelFlux2Klein}, nil},
		{"知らないモデル", saved,
			ComfyUISettingsUpdate{Provider: ComfyUICloud, Model: model("sdxl")}, ComfyUISettings{}, ErrInvalidComfyUISettings},
		{"知らない提供元", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: "other", URL: "http://h"}, ComfyUISettings{}, ErrInvalidComfyUISettings},
	}
	for _, c := range cases {
		got, err := ApplyComfyUISettingsUpdate(c.current, c.update)
		if c.wantErr != nil {
			if !errors.Is(err, c.wantErr) || !errors.Is(err, ErrInvalidComfyUISettings) {
				t.Errorf("%s: err = %v, want %v", c.name, err, c.wantErr)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
}

// モデルが保存されていなければ既定のモデルを使う。
func TestComfyUISettingsEffectiveModel(t *testing.T) {
	if got := (ComfyUISettings{}).EffectiveModel(); got != ComfyUIModelFlux2Klein {
		t.Errorf("empty = %s", got)
	}
	if got := (ComfyUISettings{Model: ComfyUIModelQwenImageEdit}).EffectiveModel(); got != ComfyUIModelQwenImageEdit {
		t.Errorf("qwen = %s", got)
	}
}
