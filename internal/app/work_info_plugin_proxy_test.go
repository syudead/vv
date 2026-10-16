package app

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// プラグインのプロキシの設定と、実行への渡し方（specs/056-plugin-tooling/research.md R-4）。

// memoryProxyStore は URL を 1 つ持つ保存先で、書き込みを数える。
type memoryProxyStore struct {
	value  string
	writes int
}

func (s *memoryProxyStore) WorkInfoPluginProxyURL(context.Context) (string, error) {
	return s.value, nil
}

func (s *memoryProxyStore) SaveWorkInfoPluginProxyURL(_ context.Context, value string) error {
	s.writes++
	s.value = value
	return nil
}

// 保存したあとの実行は URL を受け取り、消したあとの実行は何も受け取らない。
func TestWorkInfoRunsUseTheSavedProxy(t *testing.T) {
	ctx := context.Background()
	e := newFetchEnv()
	proxy := NewWorkInfoPluginProxy(&memoryProxyStore{})
	e.plugins.network = proxy

	e.fetch(t, "before")
	const saved = "http://user:pass@proxy.example:3128"
	if got, err := proxy.SetURL(ctx, saved); err != nil || got != saved {
		t.Fatalf("SetURL = %q, %v", got, err)
	}
	e.fetch(t, "with proxy")
	if _, err := proxy.SetURL(ctx, ""); err != nil {
		t.Fatal(err)
	}
	e.fetch(t, "cleared")

	networks := e.runtime.networks
	if len(networks) != 3 {
		t.Fatalf("runs = %d, want 3", len(networks))
	}
	if networks[0].Proxy != nil {
		t.Errorf("before saving: proxy = %v, want none", networks[0].Proxy)
	}
	if networks[1].Proxy == nil || networks[1].Proxy.String() != saved {
		t.Errorf("after saving: proxy = %v, want %s", networks[1].Proxy, saved)
	}
	if networks[2].Proxy != nil {
		t.Errorf("after clearing: proxy = %v, want none", networks[2].Proxy)
	}
}

func TestSetWorkInfoPluginProxyURLRefusesBrokenURLs(t *testing.T) {
	store := &memoryProxyStore{value: "http://kept.example:8080"}
	proxy := NewWorkInfoPluginProxy(store)
	for _, raw := range []string{"ftp://proxy.example", "http://proxy.example/path", "proxy.example:3128"} {
		if _, err := proxy.SetURL(context.Background(), raw); !errors.Is(err, domain.ErrWorkInfoPluginProxyURLInvalid) {
			t.Errorf("SetURL(%q) = %v, want ErrWorkInfoPluginProxyURLInvalid", raw, err)
		}
	}
	if store.writes != 0 || store.value != "http://kept.example:8080" {
		t.Fatalf("refused URLs wrote %d times; value = %q", store.writes, store.value)
	}
}

// 手で書き換えられて規則に外れた保存値では、プロキシを飛ばして直接つながず、実行しない。
func TestWorkInfoRunRefusesABrokenSavedProxy(t *testing.T) {
	e := newFetchEnv()
	e.plugins.network = NewWorkInfoPluginProxy(&memoryProxyStore{value: "http://proxy.example/path"})
	_, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "https://example.com/works/1")
	if !errors.Is(err, domain.ErrWorkInfoPluginProxyURLInvalid) {
		t.Fatalf("FetchWorkInfo = %v, want ErrWorkInfoPluginProxyURLInvalid", err)
	}
	if len(e.runtime.requests) != 0 {
		t.Fatal("the module ran with a broken proxy")
	}
}
