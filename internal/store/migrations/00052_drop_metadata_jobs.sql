-- +goose Up
-- 取り込み済みの動画を解析し直す仕事（kind = 'metadata'）は作らないことにした
-- （specs/056-media-info/research.md R-5、メンテナーの判断）。追加の情報は、新しいか変わった
-- ファイルの取り込みの解析だけが書く。
--
-- この版は以前、jobs を作り直して kind に 'metadata' を足し、既存の動画に解析し直しの仕事を
-- 積んでいた。その版を適用済みのデータベースがありうるので、番号は残し、残った仕事と問題だけを
-- 消す。そのデータベースの jobs の kind の検査に残る 'metadata' は、もう積まれないので害が無い。
update scans set issues_revision = issues_revision + 1
 where id = (select max(id) from scans)
   and exists (select 1 from scan_issues where kind = 'metadata_failed');
delete from scan_issues where kind = 'metadata_failed';
delete from jobs where kind = 'metadata';

-- +goose Down
select 1;
