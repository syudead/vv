package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// checkTimeout は接続の確かめの上限時間である。模型を読み込まないので短い。
const checkTimeout = 15 * time.Second

type versionBody struct {
	Version string `json:"version"`
}

type tagsBody struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

// Check は保存した接続先に Ollama が動いていて、保存した模型があるかを確かめる。判定は
// させず、模型も読み込まないので数秒で終わる。判定に使えるかは確かめない（それは判定の口を
// 使う機能が確かめる）。使えなければ、つながらない・Ollama でない・模型が無いのどれかを
// 述べた誤りを返す。接続先は保存した設定だけから取る。
func (c *Classifier) Check(ctx context.Context) error {
	settings, err := c.settings.OllamaSettings(ctx)
	if err != nil {
		return fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	var version versionBody
	if err := c.getJSON(ctx, settings.Endpoint, "/api/version", &version); err != nil {
		return err
	}
	if version.Version == "" {
		return fmt.Errorf("the server at %s is not Ollama", settings.Endpoint)
	}
	var tags tagsBody
	if err := c.getJSON(ctx, settings.Endpoint, "/api/tags", &tags); err != nil {
		return err
	}
	for _, m := range tags.Models {
		if sameModel(m.Name, settings.Model) || sameModel(m.Model, settings.Model) {
			return nil
		}
	}
	return fmt.Errorf("ollama %s at %s has no model named %q; pull it first", version.Version, settings.Endpoint, settings.Model)
}

// sameModel は Ollama の模型名 listed が設定の名前 want と同じ模型かを返す。タグを省いた名前は
// :latest を指す。
func sameModel(listed, want string) bool {
	if listed == "" {
		return false
	}
	if listed == want {
		return true
	}
	return !strings.Contains(want, ":") && listed == want+":latest"
}

// getJSON は接続先の path を GET し、200 の JSON を out に読む。応答の本文そのものは、問い
// 合わせ先が Ollama でないときに別のサービスの中身を画面へ写すことになるので誤りに含めない。
func (c *Classifier) getJSON(ctx context.Context, endpoint, path string, out any) error {
	url := strings.TrimRight(endpoint, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("cannot build the request to Ollama: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach Ollama at %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the server at %s is not Ollama (%s answered %d)", endpoint, path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("the server at %s is not Ollama (%s is not JSON)", endpoint, path)
	}
	return nil
}
