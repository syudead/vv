package store

// 直近の取り込みの問題（scan_issues）を記録・消去・読み出す操作
// （specs/024-import-progress/data-model.md §3）。

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// bumpIssuesRevision は直近の走査の issues_revision を1増やす。問題の行を入れる・
// 消す取引の中で呼ぶ。件数が変わらない変化でも、画面がこの番号で一覧を読み直せる。
func bumpIssuesRevision(ctx context.Context, q queryExecer) error {
	if _, err := q.ExecContext(ctx, `update scans set issues_revision = issues_revision + 1
		where id = (select max(id) from scans)`); err != nil {
		return fmt.Errorf("cannot advance the import issue revision: %w", err)
	}
	return nil
}

// videosHaveIssues は、直近の走査に videoIDs のどれかの問題の行があるかを返す。
// 所在を足す・消す・付け替える取引は、問題のある動画の所在が変わると一覧に出る
// ファイル名とフォルダが変わるので、これが真なら issuesChanged を記録する。
func videosHaveIssues(ctx context.Context, q queryExecer, videoIDs ...int64) (bool, error) {
	if len(videoIDs) == 0 {
		return false, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(videoIDs)), ",")
	args := make([]any, 0, len(videoIDs))
	for _, id := range videoIDs {
		args = append(args, id)
	}
	//nolint:gosec // 組み立てるのはプレースホルダの数だけで、値は引数で渡す。
	rows, err := q.QueryContext(ctx, `select 1 from scan_issues
		where scan_id = (select max(id) from scans) and video_id in (`+placeholders+`) limit 1`, args...)
	if err != nil {
		return false, fmt.Errorf("cannot read the import issues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("cannot read the import issues: %w", err)
	}
	return found, nil
}

// recordScanIssue は直近の走査に問題を1行入れる。同じ動画（未登録ならパス）と種類の
// 行があれば入れない。走査が1つも無ければ何もしない。入れたときだけ番号を増やす。
// videoID が 0 なら未登録のファイルとして記録する。
func recordScanIssue(
	ctx context.Context, q queryExecer, videoID int64, path string, kind domain.ScanIssueKind, now int64,
) error {
	var video any
	if videoID != 0 {
		video = videoID
	}
	// 動画の行がもう無ければ、未登録のファイルとして記録する。
	res, err := q.ExecContext(ctx, `insert into scan_issues (scan_id, video_id, path, kind, created_at)
		select s.id, (select id from videos where id = ?), ?, ?, ? from (select max(id) as id from scans) s
		where s.id is not null
		on conflict do nothing`, video, path, string(kind), now)
	if err != nil {
		return fmt.Errorf("cannot record the import issue (%s, %s): %w", kind, path, err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot record the import issue (%s, %s): %w", kind, path, err)
	}
	if inserted == 0 {
		return nil
	}
	return bumpIssuesRevision(ctx, q)
}

// clearScanIssues は直近の走査から、その動画の kinds の問題を消す。同じ段階が後で
// 成功したとき、結果を書く取引で呼ぶ。消したときだけ番号を増やす。
func clearScanIssues(ctx context.Context, q queryExecer, videoID int64, kinds ...domain.ScanIssueKind) error {
	if len(kinds) == 0 {
		return nil
	}
	args := []any{videoID}
	for _, kind := range kinds {
		args = append(args, string(kind))
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(kinds)), ",")
	//nolint:gosec // 組み立てるのはプレースホルダの数だけで、値は引数で渡す。
	res, err := q.ExecContext(ctx, `delete from scan_issues
		where scan_id = (select max(id) from scans) and video_id = ? and kind in (`+placeholders+`)`, args...)
	if err != nil {
		return fmt.Errorf("cannot clear the import issues (video=%d): %w", videoID, err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot clear the import issues (video=%d): %w", videoID, err)
	}
	if deleted == 0 {
		return nil
	}
	return bumpIssuesRevision(ctx, q)
}

// clearFailedIssue は job の段階の *_failed を消す。その段階の成功を書いた取引で呼ぶ。
func clearFailedIssue(ctx context.Context, q queryExecer, kind domain.JobKind, videoID int64) error {
	issue, ok := domain.FailedIssueKind(kind)
	if !ok {
		return nil
	}
	return clearScanIssues(ctx, q, videoID, issue)
}

// applySubstitution は段階 kind の成功を書いた取引で、代用の行を入れる・消す。
// 代用したなら入れ、代用せずに作り直したなら消す。分からない（既存の生成物を採用した）
// ときは変えない（specs/024-import-progress/data-model.md §3）。
func applySubstitution(
	ctx context.Context, q queryExecer, kind domain.JobKind, job domain.Job, substitution domain.Substitution,
) error {
	issue, ok := domain.SubstitutedIssueKind(kind)
	if !ok {
		return nil
	}
	switch substitution {
	case domain.SubstitutionUsed:
		return recordScanIssue(ctx, q, job.VideoID, job.LocationPath, issue, time.Now().Unix())
	case domain.SubstitutionNone:
		return clearScanIssues(ctx, q, job.VideoID, issue)
	case domain.SubstitutionUnknown:
	}
	return nil
}

// RecordScanIssue は走査が1つのファイルで出会った失敗を、直近の取り込みの問題として
// 記録する。走査が知っている既存の動画があれば、その動画の問題になる。
func (s *ScanStore) RecordScanIssue(ctx context.Context, issue domain.ScanFileIssue) error {
	if !issue.Kind.Valid() || !issue.Kind.FromScan() {
		return fmt.Errorf("not a scan issue kind: %q", issue.Kind)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot record the import issue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := recordScanIssue(ctx, tx, issue.VideoID, issue.Path, issue.Kind, time.Now().Unix()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot record the import issue: %w", err)
	}
	return nil
}

// ScanIssueRecords は走査 scanID の問題を、動画（未登録ならパス）ごとにまとめて返す。
// 動画の表示の所在は、登録フォルダの中の所在のうちパスの最小のもの（一覧の代表と
// 同じ）である。そうした所在が無い動画は Path を空で返す。
func (s *ScanStore) ScanIssueRecords(ctx context.Context, scanID int64) ([]domain.ScanIssueRecord, error) {
	//nolint:gosec // registeredLocationCondition は定型SQLだけを返す。
	rows, err := s.db.sql.QueryContext(ctx, `
		select i.video_id,
		       case when i.video_id is null then i.path else (
		           select l.path from video_locations l
		            where l.video_id = i.video_id and `+registeredLocationCondition("l")+`
		            order by l.path limit 1) end,
		       group_concat(i.kind, ','),
		       exists (select 1 from scan_videos sv where sv.scan_id = i.scan_id and sv.video_id = i.video_id)
		  from scan_issues i
		 where i.scan_id = ?
		 group by i.scan_id, coalesce(i.video_id, i.path)`, scanID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the import issues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []domain.ScanIssueRecord{}
	for rows.Next() {
		var (
			videoID sql.NullInt64
			path    sql.NullString
			kinds   string
			inSet   bool
		)
		if err := rows.Scan(&videoID, &path, &kinds, &inSet); err != nil {
			return nil, fmt.Errorf("cannot read the import issues: %w", err)
		}
		record := domain.ScanIssueRecord{VideoID: videoID.Int64, Path: path.String, InImport: inSet}
		for kind := range strings.SplitSeq(kinds, ",") {
			record.Kinds = append(record.Kinds, domain.ScanIssueKind(kind))
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the import issues: %w", err)
	}
	return out, nil
}

// ScanIssues は走査 scanID の問題を、表示の形にまとめて並べて返す
// （specs/024-import-progress/contracts/scan-api.md §3）。所在がどの登録フォルダにも
// 含まれない件は除く。
func (s *ScanStore) ScanIssues(ctx context.Context, scanID int64) ([]domain.ScanIssue, error) {
	records, err := s.ScanIssueRecords(ctx, scanID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []domain.ScanIssue{}, nil
	}
	roots, err := listMediaFolders(ctx, s.db.sql)
	if err != nil {
		return nil, err
	}
	return domain.ScanIssues(roots, records), nil
}
