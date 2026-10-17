-- +goose Up
-- 作品情報プラグインと、所有者が入力したその設定の値。2 表とも利用者データで、動画への外部キーを持たず、再スキャンで作り直さない。

-- autoincrement で、消したプラグインの id を使い回さない。画面の API は id で指すので、消したあとに
-- 開いていた画面の承認が別のプラグインに当たらないようにする。
create table work_info_plugins (
    id              integer primary key autoincrement,
    -- マニフェストの name。読めなかったときは null で、null どうしは重ならない。
    name            text    unique,
    file_name       text    not null,
    version         text,
    description     text,
    contract        integer,
    -- マニフェストの operations を , でつないだもの（search・fetch）。
    operations      text,
    -- マニフェストの settings の宣言の JSON。
    settings        text,
    -- モジュール。problem があるときは null。
    module          blob,
    -- アップロードしたバイト列の SHA-256 の小文字の 16 進。
    digest          text    not null,
    problem         text,
    -- 所有者が承認したダイジェスト。digest と等しく problem が null のときだけ承認済み。
    approved_digest text,
    -- 最初のアップロードと、最後のアップロードか承認の変更の時刻（Unix 秒）。
    added_at        integer not null,
    updated_at      integer not null
);

create table work_info_plugin_settings (
    plugin_id  integer not null references work_info_plugins (id) on delete cascade,
    key        text    not null,
    -- 2000 文字まで。宣言されたキーにだけ書く（internal/app と internal/store が確かめる）。
    value      text    not null,
    updated_at integer not null,
    primary key (plugin_id, key)
) without rowid;

-- +goose Down
drop table if exists work_info_plugin_settings;
drop table if exists work_info_plugins;
