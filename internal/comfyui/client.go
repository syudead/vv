package comfyui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// CloudBaseURL は Comfy Cloud の API の基底 URL である（research.md R-5）。
const CloudBaseURL = "https://cloud.comfy.org/api"

// checkTimeout は接続の確かめの上限時間である。画像は作らないので短い。
const checkTimeout = 15 * time.Second

// maxResponseBytes は確かめで読む応答の上限である。
const maxResponseBytes = 1 << 20

// maxObjectInfoBytes は GET /object_info の応答の上限である。すべてのノードの定義を返すので、
// カスタムノードの多い ComfyUI では数 MB になる。
const maxObjectInfoBytes = 64 << 20

// SettingsSource は保存された接続先を読む。internal/store の *SettingsStore が満たす。
type SettingsSource interface {
	ComfyUISettings(ctx context.Context) (domain.ComfyUISettings, error)
}

// Client は ComfyUI と Comfy Cloud を呼ぶ。ゼロ値は使えず、New で作る。
type Client struct {
	settings SettingsSource
	http     *http.Client
	cloud    string
}

// Options は Client の差し替えられる部分である。テストが使う。
type Options struct {
	// HTTPClient は要求を送る HTTP クライアントである。nil なら既定のものを使う。
	HTTPClient *http.Client
	// CloudBaseURL は Comfy Cloud の基底 URL である。空なら CloudBaseURL を使う。
	CloudBaseURL string
}

// New は既定の HTTP クライアント（環境のプロキシ設定に従う）で Client を作る。
func New(settings SettingsSource) *Client {
	return NewWithOptions(settings, Options{})
}

// NewWithOptions は opts で差し替えた Client を作る。
func NewWithOptions(settings SettingsSource, opts Options) *Client {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{
			// 転送先へ API キーを持ち出さないよう、確かめでは転送に従わない。転送の応答は
			// ComfyUI でない応答として扱う。
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	cloud := opts.CloudBaseURL
	if cloud == "" {
		cloud = CloudBaseURL
	}
	return &Client{settings: settings, http: client, cloud: strings.TrimRight(cloud, "/")}
}

// errAPIKeyRejected は Comfy Cloud が API キーを受け付けなかったことを表す。
var errAPIKeyRejected = errors.New("the API key was rejected by Comfy Cloud; check the key under Settings")

// Check は保存した接続先に ComfyUI が応え、vv のワークフローに要るものが揃っているかを
// 確かめる。自前の ComfyUI は保存した URL の GET /system_stats、Comfy Cloud は
// GET /jobs?limit=1 に X-API-Key を付けて問い合わせる（research.md R-5）。応えたら
// GET /object_info を読み、ワークフローのノードの型とローダーが名指すモデルファイルが
// あるかを確かめる（R-13）。使えなければ、何も保存されていない・つながらない・ComfyUI でない・
// キーが拒まれたのどれかを述べた誤りか、足りないものを挙げた *domain.ComfyUIMissingError を
// 返す。接続先は保存した設定だけから取る。
func (c *Client) Check(ctx context.Context) error {
	settings, err := c.settings.ComfyUISettings(ctx)
	if err != nil {
		return fmt.Errorf("cannot read the ComfyUI settings: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	switch settings.Provider {
	case domain.ComfyUISelfHosted:
		var stats systemStats
		name := "ComfyUI at " + settings.URL
		if err := c.getJSON(ctx, settings.URL, "/system_stats", "", name, maxResponseBytes, &stats); err != nil {
			return err
		}
		if !stats.valid() {
			return fmt.Errorf("the server at %s is not ComfyUI (/system_stats has no system information)", settings.URL)
		}
		return c.checkRequired(ctx, settings.URL, "", name)
	case domain.ComfyUICloud:
		if settings.APIKey == "" {
			return domain.ErrComfyUIAPIKeyMissing
		}
		var jobs jobList
		if err := c.getJSON(ctx, c.cloud, "/jobs?limit=1", settings.APIKey, "Comfy Cloud", maxResponseBytes, &jobs); err != nil {
			return err
		}
		if jobs.Jobs == nil {
			return fmt.Errorf("the server at %s is not Comfy Cloud (/jobs has no job list)", c.cloud)
		}
		return c.checkRequired(ctx, c.cloud, settings.APIKey, "Comfy Cloud")
	default:
		return errors.New("no ComfyUI connection is saved; choose a provider and save it first")
	}
}

// checkRequired は基底 URL base の GET /object_info を読み、埋め込んだワークフローの
// ノードの型とモデルファイルがあるかを確かめる。足りなければ *domain.ComfyUIMissingError を返す。
func (c *Client) checkRequired(ctx context.Context, base, apiKey, name string) error {
	required, err := workflowRequirement()
	if err != nil {
		return err
	}
	var info objectInfo
	if err := c.getJSON(ctx, base, "/object_info", apiKey, name, maxObjectInfoBytes, &info); err != nil {
		return err
	}
	if items := required.missing(info); len(items) > 0 {
		return &domain.ComfyUIMissingError{Items: items}
	}
	return nil
}

// systemStats は ComfyUI の GET /system_stats の応答のうち、ComfyUI であることを見分ける部分である。
// どの版の ComfyUI も system に os と python_version を、devices に装置の配列を返す。
type systemStats struct {
	System *struct {
		OS            string `json:"os"`
		PythonVersion string `json:"python_version"`
	} `json:"system"`
	Devices *[]json.RawMessage `json:"devices"`
}

func (s systemStats) valid() bool {
	return s.System != nil && s.System.OS != "" && s.System.PythonVersion != "" && s.Devices != nil
}

// jobList は Comfy Cloud の GET /jobs の応答のうち、仕事の一覧である。空の一覧も一覧である。
type jobList struct {
	Jobs *[]json.RawMessage `json:"jobs"`
}

// getJSON は基底 URL base の path を GET し、200 の JSON を最大 limit バイトまで out に読む。
// apiKey が空でなければ X-API-Key に付ける。name はつながらないときの誤りに入れる相手の名前である。応答の本文そのものは、問い合わせ先が ComfyUI でないときに別の
// サービスの中身を画面へ写すことになるので誤りに含めない。
func (c *Client) getJSON(ctx context.Context, base, path, apiKey, name string, limit int64, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return fmt.Errorf("cannot build the request to %s: %w", name, err)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	route := strings.SplitN(path, "?", 2)[0]
	if apiKey != "" && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
		return errAPIKeyRejected
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the server at %s is not ComfyUI (%s answered %d)", base, route, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, limit)).Decode(out); err != nil {
		return fmt.Errorf("the server at %s is not ComfyUI (%s is not JSON)", base, route)
	}
	return nil
}
