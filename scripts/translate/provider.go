package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// translator turns one English segment into Japanese.
type translator interface {
	Translate(ctx context.Context, text string) (string, error)
}

// config selects the translation backend. Every backend speaks the OpenAI
// HTTP API, which vLLM, Ollama, llama.cpp and most hosted services serve.
type config struct {
	Provider string // "chat" or "plamo"
	BaseURL  string // e.g. https://api.example.com/v1
	Model    string
	APIKey   string
	Glossary []term
}

type term struct{ en, ja string }

func newTranslator(c config) (translator, error) {
	if c.BaseURL == "" || c.Model == "" {
		return nil, errors.New("VV_TRANSLATE_BASE_URL and VV_TRANSLATE_MODEL are required unless -offline is set")
	}
	h := &httpClient{base: strings.TrimSuffix(c.BaseURL, "/"), key: c.APIKey, client: &http.Client{Timeout: 5 * time.Minute}}
	switch c.Provider {
	case "", "chat":
		return &chatTranslator{http: h, model: c.Model, glossary: c.Glossary}, nil
	case "plamo":
		return &plamoTranslator{http: h, model: c.Model}, nil
	}
	return nil, fmt.Errorf("unknown VV_TRANSLATE_PROVIDER %q (want chat or plamo)", c.Provider)
}

// chatTranslator drives a general or translation-tuned chat model (for
// example TranslateGemma, or any OpenAI-compatible chat endpoint) through
// /chat/completions with a fixed instruction and the matching glossary terms.
type chatTranslator struct {
	http     *httpClient
	model    string
	glossary []term
}

const chatInstruction = `Translate the user's text from English to Japanese.
The text is one paragraph, list item, table cell or heading of the technical
documentation of vv, a self-hosted video library.

Rules:
- Output only the Japanese translation. No notes, no quotes around it.
- Keep every tag such as <x0/>, <a1> and </a1> exactly as written. Place each
  tag where it belongs in the Japanese sentence; keep <aN> before </aN>.
- Keep Markdown emphasis markers (**, *) around the translated words.
- Keep identifiers, file paths, commands and product names in English.
- Use a plain technical register (dearu style), short sentences, and no
  added explanation.`

func (t *chatTranslator) Translate(ctx context.Context, text string) (string, error) {
	system := chatInstruction
	if terms := matchingTerms(t.glossary, text); len(terms) > 0 {
		system += "\n\nUse these fixed translations:\n" + strings.Join(terms, "\n")
	}
	req := map[string]any{
		"model":       t.model,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": text},
		},
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := t.http.post(ctx, "/chat/completions", req, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("chat completion returned no choices")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func matchingTerms(glossary []term, text string) []string {
	lower := strings.ToLower(text)
	var out []string
	for _, g := range glossary {
		if strings.Contains(lower, strings.ToLower(g.en)) {
			out = append(out, "- "+g.en+" => "+g.ja)
		}
	}
	return out
}

// plamoTranslator drives PLaMo Translate (pfnet/plamo-2-translate), a model
// trained only for translation, through /completions with the prompt format
// from its model card. It takes no instruction, so the glossary is not sent.
type plamoTranslator struct {
	http  *httpClient
	model string
}

func (t *plamoTranslator) Translate(ctx context.Context, text string) (string, error) {
	prompt := "<|plamo:op|>dataset\ntranslation\n" +
		"<|plamo:op|>input lang=English\n" + text + "\n" +
		"<|plamo:op|>output lang=Japanese\n"
	req := map[string]any{
		"model":       t.model,
		"prompt":      prompt,
		"temperature": 0,
		"max_tokens":  4 * len(text),
		"stop":        []string{"<|plamo:op|>"},
	}
	var resp struct {
		Choices []struct {
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := t.http.post(ctx, "/completions", req, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("completion returned no choices")
	}
	return strings.TrimSpace(resp.Choices[0].Text), nil
}

type httpClient struct {
	base   string
	key    string
	client *http.Client
}

// post sends a JSON request and retries rate limits and server errors with
// exponential backoff.
func (h *httpClient) post(ctx context.Context, endpoint string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.base+endpoint, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if h.key != "" {
			req.Header.Set("Authorization", "Bearer "+h.key)
		}
		resp, err := h.client.Do(req)
		if err != nil {
			last = err
			continue
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			last = fmt.Errorf("%s: %s", resp.Status, truncate(data))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: %s", resp.Status, truncate(data))
		}
		return json.Unmarshal(data, out)
	}
	return fmt.Errorf("giving up after retries: %w", last)
}

func truncate(b []byte) string {
	if len(b) > 300 {
		b = b[:300]
	}
	return string(b)
}
