-- +goose Up
-- 「同じ動画かもしれない」候補の自動の判定（specs/054-auto-dup/data-model.md、research.md R-4・R-5）。
-- 候補は索引なので、作り直すと判定も null と 0 に戻り、もう一度判定する。
-- judgement は null（待ち）か running・undecided・failed・no_connection。
alter table video_version_candidates add column judgement text;
-- これまでの失敗の数。取り出しと起動時の回復を越えて残す。
alter table video_version_candidates add column attempts integer not null default 0;
-- failed の行を再び取り出せる時刻（Unix 秒）。それ以外は null。
alter table video_version_candidates add column retry_at integer;
-- failed の行の提供元のメッセージ。
alter table video_version_candidates add column judgement_error text;
-- undecided の行のモデルの答え。衝突で決められなかったときは null。
alter table video_version_candidates add column probability real;

-- 集まりから外した組（research.md R-8）。利用者データで、自動では束ねない。key_a < key_b に並べて 1 行。
create table video_version_unbundles (
    key_a      text    not null,
    key_b      text    not null,
    created_at integer not null,
    primary key (key_a, key_b),
    check (key_a < key_b)
) without rowid;

-- +goose Down
drop table if exists video_version_unbundles;
alter table video_version_candidates drop column probability;
alter table video_version_candidates drop column judgement_error;
alter table video_version_candidates drop column retry_at;
alter table video_version_candidates drop column attempts;
alter table video_version_candidates drop column judgement;
