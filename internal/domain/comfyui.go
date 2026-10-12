package domain

import (
	"errors"
	"fmt"
	"strings"
)

// 画像編集の提供元 ComfyUI の接続先の設定である。提供元のアダプタ（internal/comfyui）が
// 読み、サムネイルの編集の機能とは別に持つ（docs/design-docs/ai-layer.md、
// specs/045-comfyui-thumbnails/research.md R-1・R-9）。

// ComfyUIProvider は ComfyUI の提供元の種類である。
type ComfyUIProvider string

const (
	// ComfyUISelfHosted は所有者が自分で動かす ComfyUI で、保存した URL へ接続する。
	ComfyUISelfHosted ComfyUIProvider = "self_hosted"
	// ComfyUICloud は Comfy Cloud で、保存した API キーで接続する。
	ComfyUICloud ComfyUIProvider = "cloud"
)

// comfyUIMaxAPIKeyLength は API キーの長さの上限である。
const comfyUIMaxAPIKeyLength = 1000

var (
	// ErrInvalidComfyUISettings は、ComfyUI の接続先の設定が規則に合わないことを表す。
	// 下の 2 つの誤りもこれを包む。
	ErrInvalidComfyUISettings = errors.New("invalid ComfyUI settings")
	// ErrComfyUIURLInvalid は、自前の ComfyUI に正しい http か https の URL が無いことを表す。
	ErrComfyUIURLInvalid = fmt.Errorf("%w: a self-hosted ComfyUI needs an http or https URL", ErrInvalidComfyUISettings)
	// ErrComfyUIAPIKeyMissing は、Comfy Cloud の API キーが保存も送信もされていないことを表す。
	ErrComfyUIAPIKeyMissing = fmt.Errorf("%w: Comfy Cloud needs an API key", ErrInvalidComfyUISettings)
)

// ComfyUISettings は ComfyUI の接続先である。Provider が空なら何も保存されていない。
// URL は自前の ComfyUI だけが、APIKey は Comfy Cloud だけが使う。もう一方の値は
// 提供元を切り替えて戻したときのために保存したままにする。APIKey は API から返さない。
type ComfyUISettings struct {
	Provider ComfyUIProvider
	URL      string
	APIKey   string
}

// Saved は接続先が保存されているかを返す。
func (s ComfyUISettings) Saved() bool {
	return s.Provider != ""
}

// ComfyUISettingsUpdate は接続先の保存の要求である。APIKey が nil なら保存済みのキーを
// 残し、空文字列ならキーを消す（R-9）。
type ComfyUISettingsUpdate struct {
	Provider ComfyUIProvider
	URL      string
	APIKey   *string
}

// ApplyComfyUISettingsUpdate は保存済みの接続先 current に update を当てて整え、保存する値を
// 返す。規則に合わなければ ErrInvalidComfyUISettings を包んだ誤り（自前の URL が正しく
// なければ ErrComfyUIURLInvalid、Cloud のキーが無ければ ErrComfyUIAPIKeyMissing）を返す。
// Comfy Cloud を選んだ要求の URL は使わず、保存済みの URL を残す。
func ApplyComfyUISettingsUpdate(current ComfyUISettings, update ComfyUISettingsUpdate) (ComfyUISettings, error) {
	next := current
	next.Provider = update.Provider
	if update.APIKey != nil {
		key := strings.TrimSpace(*update.APIKey)
		if len(key) > comfyUIMaxAPIKeyLength || strings.ContainsFunc(key, isControlOrSpace) {
			return ComfyUISettings{}, fmt.Errorf("%w: the API key must be a single token", ErrInvalidComfyUISettings)
		}
		next.APIKey = key
	}
	switch update.Provider {
	case ComfyUISelfHosted:
		normalized, ok := normalizeBaseURL(update.URL)
		if !ok {
			return ComfyUISettings{}, ErrComfyUIURLInvalid
		}
		next.URL = normalized
	case ComfyUICloud:
		if next.APIKey == "" {
			return ComfyUISettings{}, ErrComfyUIAPIKeyMissing
		}
	default:
		return ComfyUISettings{}, fmt.Errorf("%w: the provider must be self_hosted or cloud", ErrInvalidComfyUISettings)
	}
	return next, nil
}

// ComfyUIMissingKind は接続先に足りないものの種類である。
type ComfyUIMissingKind string

const (
	// ComfyUIMissingNode は vv のワークフローが使うノードの型（class_type）が無いことを表す。
	ComfyUIMissingNode ComfyUIMissingKind = "node"
	// ComfyUIMissingModel はローダーノードが読むモデルファイルが無いことを表す。
	ComfyUIMissingModel ComfyUIMissingKind = "model"
)

// ComfyUIMissingItem は接続先に足りないノードかモデルファイルの 1 つである。Folder と Source は
// モデルファイルだけが持ち、ComfyUI の models の下の置き場所と入手先のページである
// （specs/045-comfyui-thumbnails/research.md R-13）。
type ComfyUIMissingItem struct {
	Kind   ComfyUIMissingKind
	Name   string
	Folder string
	Source string
}

// ComfyUIMissingError は、接続先は応えたが vv のワークフローに要るノードかモデルファイルが
// 足りないことを表す。Items は足りないものの一覧で、空ではない。
type ComfyUIMissingError struct {
	Items []ComfyUIMissingItem
}

// Error は足りないものの数と直し方を述べる。名前は Items で返すので文には入れない。
func (e *ComfyUIMissingError) Error() string {
	nodes, models := 0, 0
	for _, item := range e.Items {
		if item.Kind == ComfyUIMissingNode {
			nodes++
		} else {
			models++
		}
	}
	parts := make([]string, 0, 2)
	if models > 0 {
		parts = append(parts, fmt.Sprintf("%d model file(s) are missing; download each into its folder", models))
	}
	if nodes > 0 {
		parts = append(parts, fmt.Sprintf("%d node type(s) are missing; update ComfyUI", nodes))
	}
	return "ComfyUI is missing files the thumbnail workflows need: " + strings.Join(parts, ", and ")
}
