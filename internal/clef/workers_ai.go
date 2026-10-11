package clef

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// WorkersAIBaseURL は Cloudflare の API の基底 URL である。
const WorkersAIBaseURL = "https://api.cloudflare.com/client/v4"

// workersAITimeout は Workers AI への 1 回の判定の上限時間である。GPU で動くので、
// Ollama（requestTimeout）ほど待たない。
const workersAITimeout = 2 * time.Minute

// workersAITarget は Workers AI の問い合わせ先と認証である。
type workersAITarget struct {
	baseURL   string
	accountID string
	apiToken  string
}

// workersAIResponse は Cloudflare の API の封筒である。判定の答えは result にある。
type workersAIResponse struct {
	Success bool `json:"success"`
	Result  *struct {
		Answers map[string]answer `json:"answers"`
	} `json:"result"`
	Errors []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// NewWorkersAI は Cloudflare の Workers AI（@cf/cloudflare/<模型>）を呼ぶ Client を作る。
// 問い合わせ先は baseURL のアカウントに固定し、設定の Ollama の URL は使わない。
// baseURL は通常 WorkersAIBaseURL で、テストは偽のサーバーを渡す。
func NewWorkersAI(baseURL, accountID, apiToken string) *Client {
	return &Client{
		http: &http.Client{Timeout: workersAITimeout},
		workersAI: &workersAITarget{
			baseURL:   strings.TrimRight(baseURL, "/"),
			accountID: accountID,
			apiToken:  apiToken,
		},
	}
}

func (c *Client) classifyWithWorkersAI(ctx context.Context, req domain.AutoTagRequest) (map[string]float64, error) {
	target := c.workersAI
	body := newRequestBody(req)
	if len(req.Subject.Thumbnail) > 0 {
		// Workers AI は画像を data URL で受け取る。
		body.Images = []string{"data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(req.Subject.Thumbnail)}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("cannot build the classifier request: %w", err)
	}

	endpoint := target.baseURL + "/accounts/" + url.PathEscape(target.accountID) +
		"/ai/run/@cf/cloudflare/" + url.PathEscape(req.Model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot build the classifier request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+target.apiToken)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		// 誤りに URL を含めると利用者のアカウント ID が画面に出るので、宛先は名前だけにする。
		return nil, fmt.Errorf("cannot reach Workers AI: %w", unwrapURLError(err))
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("cannot read the classifier response: %w", err)
	}
	var decoded workersAIResponse
	decodeErr := json.Unmarshal(raw, &decoded)
	if resp.StatusCode != http.StatusOK || (decodeErr == nil && !decoded.Success) {
		messages := make([]string, 0, len(decoded.Errors))
		for _, e := range decoded.Errors {
			if m := strings.TrimSpace(e.Message); m != "" {
				messages = append(messages, fmt.Sprintf("%s (%d)", m, e.Code))
			}
		}
		if decodeErr != nil || len(messages) == 0 {
			return nil, fmt.Errorf("workers AI answered %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("workers AI answered %d: %s", resp.StatusCode, truncate(strings.Join(messages, "; ")))
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("cannot read the classifier response: %w", decodeErr)
	}
	if decoded.Result == nil {
		return probabilitiesOf(nil)
	}
	return probabilitiesOf(decoded.Result.Answers)
}

// unwrapURLError は *url.Error から URL を外し、下の誤りだけを返す。
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}
