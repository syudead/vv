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
	endpoint, ok := normalizeBaseURL(s.Endpoint)
	if !ok {
		return OllamaSettings{}, fmt.Errorf("%w: the endpoint must be an http or https URL", ErrInvalidOllamaSettings)
	}
	s.Endpoint = endpoint
	s.Model = strings.TrimSpace(s.Model)
	if s.Model == "" || len(s.Model) > ollamaMaxModelLength || strings.ContainsFunc(s.Model, isControlOrSpace) {
		return OllamaSettings{}, fmt.Errorf("%w: the model must be a model name", ErrInvalidOllamaSettings)
	}
	return s, nil
}

// normalizeBaseURL は前後の空白と末尾の / を落とし、利用者を含まない http か https の
// 基底 URL（問い合わせと断片を持たない）かを返す。提供元の接続先に共通の規則である。
// 経路はこの URL の後ろへ連ねるので、空の問い合わせや断片（末尾の ? や #）も受け付けない。
func normalizeBaseURL(raw string) (string, bool) {
	normalized := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(normalized)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || strings.ContainsAny(normalized, "?#") {
		return "", false
	}
	return normalized, true
}

func isControlOrSpace(r rune) bool {
	return r <= ' ' || r == 0x7f
}
