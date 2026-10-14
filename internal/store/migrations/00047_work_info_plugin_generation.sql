-- +goose Up
-- 作品情報プラグインの行の世代（specs/049-work-info-plugins/research.md R-8）。アップロードによる差し替えと
-- 承認の取り消しのたびに 1 増やす。承認を失った行は、どちらかを通らなければ承認済みに戻らない。取得の結果は取得した時の世代を持ち、反映は世代が変わっていれば
-- work_info_fetch_stale にする。ダイジェストと承認の今の値だけでは、取得のあとに同じバイト列を
-- 差し替えて再承認したことや、取り消して再承認したことを見分けられないため。
alter table work_info_plugins add column generation integer not null default 0;

-- +goose Down
alter table work_info_plugins drop column generation;
