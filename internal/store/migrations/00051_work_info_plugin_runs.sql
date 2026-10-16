-- +goose Up
-- 作品情報プラグインごとの直近の実行の標準エラー（specs/056-plugin-tooling/data-model.md、research.md R-6）。
-- 利用者データで、プラグインごとに 1 行を実行のたびに置き換える。番号は data-model.md の Migration のとおり、
-- 別の feature branch が使う 00050 の次にした。
create table work_info_plugin_runs (
    plugin_id  integer primary key references work_info_plugins (id) on delete cascade,
    -- 実行を始めたときの work_info_plugins.generation。行はこの世代がまだあるときだけ書く。
    generation integer not null,
    -- search か fetch。
    operation  text    not null,
    -- 実行の失敗の種類。成功した実行では null。
    failure    text,
    -- 実行が終わった時刻（Unix 秒）。
    ran_at     integer not null,
    -- 標準エラーの先頭 64 KiB。秘密の設定の値は [secret] に置き換えてある。
    stderr     text    not null,
    -- 64 KiB より後ろを捨てたら 1。
    truncated  integer not null
);

-- +goose Down
drop table if exists work_info_plugin_runs;
