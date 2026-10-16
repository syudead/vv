-- +goose Up
-- 取り込み済みの動画を解析し直す仕事（kind = 'metadata'）は作らないことにした
-- （specs/056-media-info/research.md R-5、メンテナーの判断）。追加の情報は、新しいか変わった
-- ファイルの取り込みの解析だけが書く。
--
-- 00052 は jobs の kind の検査に 'metadata' を足し、既存の動画に解析し直しの仕事を積む。
-- 00052 を適用済みのデータベースがあるので 00052 は変えず、ここで残った仕事と問題を消す。
-- 対応するワーカーはもう無いので、残すと直近の走査の残りの仕事に数えられ続ける。
-- jobs の kind の検査に残る 'metadata' は、もう積まれないので害が無い。
-- 直近の走査の完了の時刻は、起動時の積み直し（RequeueRunningJobs）が決め直す。
update scans set issues_revision = issues_revision + 1
 where id = (select max(id) from scans)
   and exists (select 1 from scan_issues where kind = 'metadata_failed');
delete from scan_issues where kind = 'metadata_failed';
delete from jobs where kind = 'metadata';

-- +goose Down
select 1;
