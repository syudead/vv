-- +goose Up
-- 解析と取り込みの失敗理由のコード（specs/023-english-i18n/data-model.md）。画面は自由文の
-- probe_error・scans.error を出さず、このコードから英語の説明を作る。既存の行は null
-- （コードの無いアップグレード前の失敗）のまま残し、過去の自由文を解析して埋めない。
-- 自由文の列は形も値も変えない。
alter table videos add column probe_error_code text;
alter table scans add column error_code text;
-- 理由が特定の場所（メディアフォルダ、または読めなかった場所）に結び付くときだけ入る。
alter table scans add column error_path text;

-- +goose Down
alter table scans drop column error_path;
alter table scans drop column error_code;
alter table videos drop column probe_error_code;
