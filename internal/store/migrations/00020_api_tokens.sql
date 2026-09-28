-- +goose Up
-- 外部連携の API トークン（specs/026-external-api/data-model.md §1）。
-- 作り直せない設定のデータで、スキャンでは戻らない。失えば発行し直す（ARCHITECTURE.md
-- 「Rebuildable and user data」）。
create table api_tokens (
    -- autoincrement で、失効した行の id を新しいトークンに再利用しない。
    id              integer primary key autoincrement,
    -- 用途の名前。domain.NormalizeAPITokenName を通した値。重複してよい。
    name            text    not null,
    -- 平文の SHA-256（16進）。平文はどこにも保存しない。
    token_hash      text    not null unique,
    -- 発行したときの account.version。一致しない行は無効である（research.md R-2）。
    account_version integer not null,
    created_at      integer not null,
    -- 最後に使った時刻（Unix 秒）。未使用なら null。60 秒より細かくは書かない（R-9）。
    last_used_at    integer
);

-- +goose Down
drop table if exists api_tokens;
