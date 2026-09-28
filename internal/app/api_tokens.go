package app

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// API トークン（specs/026-external-api/research.md R-1・R-2・R-9）。平文は "vvt_" と
// 32 バイトの乱数の base64url（パディングなし）で、乱数はセッション ID と同じ作りである。
// 保存先には平文を渡し、ハッシュへの変換は保存先が行う。

// apiTokenPrefix は API トークンの平文の接頭辞である。利用者が設定ファイルやログの中で
// vv のトークンと見分けるためのものである。
const apiTokenPrefix = "vvt_"

// CreateAPIToken は、セッション sessionToken の所有者の求めで名前 name の API トークンを
// 発行し、保存した行と平文を返す。平文はこの戻り値にしか現れない。名前が規則を外れれば
// *domain.InvalidAPITokenNameError を、保存の時点でセッションが有効でなければ
// domain.ErrSessionNotValid を返す（research.md R-2）。
func (a *Auth) CreateAPIToken(ctx context.Context, sessionToken, name string) (domain.APIToken, string, error) {
	normalized, err := domain.NormalizeAPITokenName(name)
	if err != nil {
		return domain.APIToken{}, "", err
	}
	raw := make([]byte, sessionTokenBytes)
	if _, err := io.ReadFull(a.random, raw); err != nil {
		return domain.APIToken{}, "", fmt.Errorf("cannot create an API token: %w", err)
	}
	secret := apiTokenPrefix + sessionTokenEncoding.EncodeToString(raw)
	token, err := a.store.AddAPIToken(ctx, sessionToken, normalized, secret, a.now())
	if err != nil {
		return domain.APIToken{}, "", err
	}
	return token, secret, nil
}

// ListAPITokens は有効な API トークンを作成日時の降順で返す。平文とハッシュは含まない。
func (a *Auth) ListAPITokens(ctx context.Context) ([]domain.APIToken, error) {
	return a.store.ListAPITokens(ctx)
}

// RevokeAPIToken は id の API トークンを失効させる。無い id では何もしない。
func (a *Auth) RevokeAPIToken(ctx context.Context, id int64) error {
	return a.store.DeleteAPIToken(ctx, id)
}

// CheckAPIToken は Bearer で送られた平文 secret が有効な API トークンかを確かめ、有効なら
// その行を返す。形式の違う値は保存先に問い合わせずに無効とする。問い合わせが失敗したら、
// 無効とせずに誤りを返す（specs/026-external-api/data-model.md §1）。
func (a *Auth) CheckAPIToken(ctx context.Context, secret string) (domain.APIToken, bool, error) {
	if !validAPITokenFormat(secret) {
		return domain.APIToken{}, false, nil
	}
	return a.store.APIToken(ctx, secret)
}

// RecordAPITokenUse は id の API トークンを今使ったことを記録する。保存先は 60 秒より
// 細かくは書かない（research.md R-9）。
func (a *Auth) RecordAPITokenUse(ctx context.Context, id int64) error {
	return a.store.TouchAPIToken(ctx, id, a.now())
}

// validAPITokenFormat は値が API トークンの平文の形（"vvt_" と 32 バイトの base64url）かを返す。
func validAPITokenFormat(secret string) bool {
	rest, ok := strings.CutPrefix(secret, apiTokenPrefix)
	return ok && validTokenFormat(rest)
}
