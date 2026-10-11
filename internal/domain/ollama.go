package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Ollama の接続先の設定である。判定モデルの提供元 Ollama のアダプタ（internal/ollama）が
// 読み、判定モデルを使う機能（自動タグ付け）の設定とは別に持つ（docs/design-docs/ai-layer.md）。

const (
	// DefaultOllamaEndpoint は Ollama が既定で待ち受ける場所である。
	DefaultOllamaEndpoint = "http://127.0.0.1:11434"
	// DefaultOllamaModel は速さを優先した Clef の小さい版である。
	DefaultOllamaModel = "clef-flash"
)

// ollamaMaxModelLength は模型名の長さの上限である。
const ollamaMaxModelLength = 200

// ErrInvalidOllamaSettings は、Ollama の接続先の設定が規則に合わないことを表す。
var ErrInvalidOllamaSettings = errors.New("invalid Ollama settings")

// OllamaSettings は Ollama の接続先である。
type OllamaSettings struct {
	// Endpoint は Ollama の基底 URL（http か https）である。
	Endpoint string
	// Model は Ollama の模型名（clef か clef-flash）である。
	Model string
}

// DefaultOllamaSettings は保存値が無いときの接続先を返す。
func DefaultOllamaSettings() OllamaSettings {
	return OllamaSettings{Endpoint: DefaultOllamaEndpoint, Model: DefaultOllamaModel}
}

// NormalizeOllamaSettings は前後の空白と Endpoint の末尾の / を落とし、規則に合うかを確かめる。
// 合わなければ ErrInvalidOllamaSettings を包んだ誤りを返す。
func NormalizeOllamaSettings(s OllamaSettings) (OllamaSettings, error) {
	s.Endpoint = strings.TrimRight(strings.TrimSpace(s.Endpoint), "/")
	s.Model = strings.TrimSpace(s.Model)
	parsed, err := url.Parse(s.Endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return OllamaSettings{}, fmt.Errorf("%w: the endpoint must be an http or https URL", ErrInvalidOllamaSettings)
	}
	if s.Model == "" || len(s.Model) > ollamaMaxModelLength || strings.ContainsFunc(s.Model, isControlOrSpace) {
		return OllamaSettings{}, fmt.Errorf("%w: the model must be a model name", ErrInvalidOllamaSettings)
	}
	return s, nil
}

func isControlOrSpace(r rune) bool {
	return r <= ' ' || r == 0x7f
}
