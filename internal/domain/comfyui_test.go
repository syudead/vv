package domain

import (
	"errors"
	"testing"
)

func TestApplyComfyUISettingsUpdate(t *testing.T) {
	key := func(s string) *string { return &s }
	saved := ComfyUISettings{Provider: ComfyUICloud, URL: "http://gpu:8188", APIKey: "k1"}
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
			ComfyUISettings{Provider: ComfyUISelfHosted, URL: "https://h"}, nil},
		{"Cloud は要求の URL を使わない", ComfyUISettings{URL: "http://old"},
			ComfyUISettingsUpdate{Provider: ComfyUICloud, URL: "ftp://x", APIKey: key(" k2 ")},
			ComfyUISettings{Provider: ComfyUICloud, URL: "http://old", APIKey: "k2"}, nil},
		{"自前の URL が無い", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUISelfHosted}, ComfyUISettings{}, ErrComfyUIURLInvalid},
		{"自前の URL が http でない", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUISelfHosted, URL: "ftp://h"}, ComfyUISettings{}, ErrComfyUIURLInvalid},
		{"自前の URL に利用者が入る", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUISelfHosted, URL: "http://u:p@h"}, ComfyUISettings{}, ErrComfyUIURLInvalid},
		{"Cloud のキーが保存も送信もされていない", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUICloud}, ComfyUISettings{}, ErrComfyUIAPIKeyMissing},
		{"Cloud でキーを消す", saved,
			ComfyUISettingsUpdate{Provider: ComfyUICloud, APIKey: key("")}, ComfyUISettings{}, ErrComfyUIAPIKeyMissing},
		{"キーに空白が入る", ComfyUISettings{},
			ComfyUISettingsUpdate{Provider: ComfyUICloud, APIKey: key("a b")}, ComfyUISettings{}, ErrInvalidComfyUISettings},
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
