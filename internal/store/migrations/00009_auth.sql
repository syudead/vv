-- +goose Up
-- 単一アカウントの認証（specs/016-single-account-auth/data-model.md §1）。
-- account は作り直せない利用者データ、sessions は消えても再ログインで戻る一時的な状態である
-- （ARCHITECTURE.md「Two kinds of data」）。

-- 唯一のアカウント。行が無ければ「未設定」、あれば「設定済み」である。
-- 行を作るのは初回設定だけで、作った後は消さない。
create table account (
    id            integer primary key check (id = 1),
    -- 照合は既定の BINARY。大文字と小文字を区別した完全一致になる。
    username      text    not null,
    -- Argon2id の PHC 文字列（$argon2id$v=19$m=…,t=…,p=…$<salt>$<hash>）。平文は持たない。
    password_hash text    not null,
    -- ユーザー名かパスワードを書き換えるたびに 1 増やす（data-model.md §4）。
    version       integer not null default 1 check (version >= 1),
    updated_at    integer not null
);

-- ログインと初回設定で発行したセッション。
create table sessions (
    -- セッション ID の SHA-256（16進）。ID そのものは Cookie にだけあり、DB には無い。
    token_hash      text    primary key,
    -- 発行したときの account.version。一致しない行は無効である（data-model.md §4）。
    account_version integer not null,
    created_at      integer not null,
    -- created_at + 90 日。延長しない。
    expires_at      integer not null
) without rowid;

create index sessions_expires_at on sessions (expires_at);

-- +goose Down
drop table if exists sessions;
drop table if exists account;
