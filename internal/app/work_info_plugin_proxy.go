package app

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// WorkInfoPluginProxyStore は作品情報プラグインの要求を送るプロキシの URL の保存先である。internal/store の
// *SettingsStore がこれを満たす。
type WorkInfoPluginProxyStore interface {
	// WorkInfoPluginProxyURL は保存した URL を返す。保存が無ければ "" である。
	WorkInfoPluginProxyURL(ctx context.Context) (string, error)
	// SaveWorkInfoPluginProxyURL は URL を保存する。"" はプロキシなしである。
	SaveWorkInfoPluginProxyURL(ctx context.Context, value string) error
}

// WorkInfoPluginProxy は所有者が設定画面で選ぶ、プラグインの要求を送るプロキシである
// （specs/056-plugin-tooling/research.md R-4）。値はサーバーに持たず、読むたびに保存先から読むので、
// 次の実行は保存した値を使い、進行中の実行は始めたときの値で終わる。
type WorkInfoPluginProxy struct {
	store WorkInfoPluginProxyStore
}

// NewWorkInfoPluginProxy は保存先 store のプロキシの設定を作る。
func NewWorkInfoPluginProxy(store WorkInfoPluginProxyStore) *WorkInfoPluginProxy {
	return &WorkInfoPluginProxy{store: store}
}

// URL は保存した URL を返す。保存が無ければ "" である。
func (p *WorkInfoPluginProxy) URL(ctx context.Context) (string, error) {
	return p.store.WorkInfoPluginProxyURL(ctx)
}

// SetURL は URL raw を domain.ParseWorkInfoPluginProxyURL で確かめて保存し、保存した値を返す。"" は
// プロキシなしである。規則に外れれば domain.ErrWorkInfoPluginProxyURLInvalid を返し、何も書かない。
func (p *WorkInfoPluginProxy) SetURL(ctx context.Context, raw string) (string, error) {
	if _, err := domain.ParseWorkInfoPluginProxyURL(raw); err != nil {
		return "", err
	}
	if err := p.store.SaveWorkInfoPluginProxyURL(ctx, raw); err != nil {
		return "", err
	}
	return raw, nil
}

// RunNetwork は次の 1 回の実行のネットワークの選択肢を、保存した URL から作る。保存が無いか "" なら
// プロキシなしである。保存した値が規則に外れれば（手で書き換えられたときだけ起こる）誤りを返し、
// プロキシを飛ばして直接つなぐことはしない。
func (p *WorkInfoPluginProxy) RunNetwork(ctx context.Context) (domain.WorkInfoRunNetwork, error) {
	raw, err := p.store.WorkInfoPluginProxyURL(ctx)
	if err != nil {
		return domain.WorkInfoRunNetwork{}, err
	}
	proxy, err := domain.ParseWorkInfoPluginProxyURL(raw)
	if err != nil {
		return domain.WorkInfoRunNetwork{}, fmt.Errorf("cannot use the saved work-info plugin proxy: %w", err)
	}
	return domain.WorkInfoRunNetwork{Proxy: proxy}, nil
}
