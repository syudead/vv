package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"time"

	"modernc.org/sqlite"

	"github.com/syudead/vv/internal/domain"
)

// 映像の指紋と「同じ動画かもしれない」候補（specs/030-video-versions/data-model.md §1・§6・§7、
// research.md R-6・R-7）。video_fingerprints と video_version_candidates は索引で、内容の参照が
// 無くなるときに releaseContentIndex が消す。

// fingerprintMissingCondition は、別名 alias の videos の行のシーク用スプライトが完成して
// いるのに、今の domain.FingerprintVersion の指紋が無い条件である。走査が指紋の仕事を
// 積み直す判断（domain.IndexedVideo.FingerprintMissing と EnsureJob）に使う。
func fingerprintMissingCondition(alias string) string {
	return alias + `.seek_thumbnail_state = '` + string(domain.SeekThumbnailDone) + `' and ` +
		alias + `.content_key <> '' and not exists (
		select 1 from video_fingerprints f where f.content_key = ` + alias + `.content_key and f.version = ` +
		strconv.Itoa(domain.FingerprintVersion) + `)`
}

// ApplyFingerprintForJob は専有した時点の内容鍵・所在・所在の世代が今も同じときだけ、
// その内容の指紋を置き換え、反映したかを返す。同じ取引でその動画の fingerprint_failed を
// 直近の取り込みの問題から消す。同じ取引でその内容の候補を作り直す（data-model.md §7）。
func (s *IngestStore) ApplyFingerprintForJob(
	ctx context.Context, job domain.Job, fingerprint domain.Fingerprint,
) (bool, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("cannot start saving the fingerprint (id=%d): %w", job.VideoID, err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := jobIdentityCurrent(ctx, tx, job)
	if err != nil {
		return false, fmt.Errorf("cannot check the fingerprint job location (id=%d): %w", job.VideoID, err)
	}
	if !current {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `insert into video_fingerprints (content_key, version, interval_ms, hashes, updated_at)
		values (?, ?, ?, ?, ?)
		on conflict (content_key) do update
		   set version = excluded.version, interval_ms = excluded.interval_ms,
		       hashes = excluded.hashes, updated_at = excluded.updated_at`,
		job.ContentKey, fingerprint.Version, fingerprint.IntervalMs, fingerprint.Encode(), time.Now().Unix(),
	); err != nil {
		return false, fmt.Errorf("cannot save the fingerprint (id=%d): %w", job.VideoID, err)
	}
	if err := rebuildVersionCandidates(ctx, tx, job.ContentKey, fingerprint); err != nil {
		return false, err
	}
	if err := clearFailedIssue(ctx, tx, domain.JobFingerprint, job.VideoID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("cannot commit the fingerprint (id=%d): %w", job.VideoID, err)
	}
	return true, nil
}

// fingerprintDistanceFunction は 2 本の指紋の距離を SQLite から呼ぶ名前である
// （data-model.md §7、research.md R-7）。引数は (hashes_a, interval_a, hashes_b, interval_b) で、
// どちらも今の domain.FingerprintVersion の指紋として読む（呼び出し側が版で絞る）。
// domain.CompareFingerprints はコマを時刻で組にするので、保存の形のハッシュに加えて各指紋の
// 間隔が要る。比べられない・読めないときは -1 を返す。
const fingerprintDistanceFunction = "vv_fingerprint_distance"

// 登録の仕方は vv_shuffle_key（listing.go）と同じで、パッケージの初期化で一度だけ行う。
func init() {
	err := sqlite.RegisterDeterministicScalarFunction(fingerprintDistanceFunction, 4,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			a, okA := decodeFingerprintArgs(args[0], args[1])
			b, okB := decodeFingerprintArgs(args[2], args[3])
			if !okA || !okB {
				return int64(-1), nil
			}
			distance, ok := domain.CompareFingerprints(a, b)
			if !ok {
				return int64(-1), nil
			}
			return int64(distance), nil
		})
	if err != nil {
		panic(fmt.Sprintf("cannot register %s: %v", fingerprintDistanceFunction, err))
	}
}

// decodeFingerprintArgs は SQL の関数の引数 1 組（保存の形のハッシュと間隔）を指紋に読む。
func decodeFingerprintArgs(hashes, interval driver.Value) (domain.Fingerprint, bool) {
	var data []byte
	switch value := hashes.(type) {
	case []byte:
		data = value
	case string:
		data = []byte(value)
	default:
		return domain.Fingerprint{}, false
	}
	intervalMs, ok := interval.(int64)
	if !ok {
		return domain.Fingerprint{}, false
	}
	fingerprint, err := domain.DecodeFingerprint(data, domain.FingerprintVersion, intervalMs)
	if err != nil {
		return domain.Fingerprint{}, false
	}
	return fingerprint, true
}

// rebuildVersionCandidates は内容 key の候補をいったん消し、fingerprint と比べて作り直す
// （data-model.md §7）。相手は同じ版の指紋を持ち、尺が domain.DurationsMatch の幅にある内容で、
// 却下された組と同じ集まりの組を除き、距離が 0 以上 domain.FingerprintMatchMaxDistance 以下の
// もの。key の動画の尺が分からなければ候補を作らない。
func rebuildVersionCandidates(ctx context.Context, tx *sql.Tx, key string, fingerprint domain.Fingerprint) error {
	if _, err := tx.ExecContext(ctx,
		`delete from video_version_candidates where key_a = ? or key_b = ?`, key, key); err != nil {
		return fmt.Errorf("cannot clear the version candidates: %w", err)
	}
	if fingerprint.Version != domain.FingerprintVersion {
		return nil
	}
	var duration sql.NullInt64
	err := tx.QueryRowContext(ctx, `select duration_ms from videos where content_key = ?`, key).Scan(&duration)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the duration for the version candidates: %w", err)
	}
	if !duration.Valid || duration.Int64 <= 0 {
		return nil
	}
	// 尺の条件は domain.DurationsMatch と同じ（差が 1 秒と長い方の 0.5% の大きい方以下）。
	if _, err := tx.ExecContext(ctx, `insert or ignore into video_version_candidates (key_a, key_b, distance, created_at)
		select min(?1, c.content_key), max(?1, c.content_key), c.distance, ?2
		  from (select f.content_key,
		               `+fingerprintDistanceFunction+`(?3, ?4, f.hashes, f.interval_ms) as distance
		          from video_fingerprints f
		          join videos v on v.content_key = f.content_key and v.content_key <> ''
		         where f.content_key <> ?1 and f.version = ?5
		           and v.duration_ms > 0
		           and abs(v.duration_ms - ?6) <= max(1000, max(v.duration_ms, ?6) * 5 / 1000)
		           and not exists (select 1 from video_version_dismissals d
		                            where d.key_a = min(?1, f.content_key) and d.key_b = max(?1, f.content_key))
		           and not exists (select 1 from video_bundle_members ma
		                             join video_bundle_members mb on mb.bundle_id = ma.bundle_id
		                            where ma.content_key = ?1 and mb.content_key = f.content_key)) c
		 where c.distance between 0 and ?7`,
		key, time.Now().Unix(), fingerprint.Encode(), fingerprint.IntervalMs,
		domain.FingerprintVersion, duration.Int64, domain.FingerprintMatchMaxDistance,
	); err != nil {
		return fmt.Errorf("cannot compute the version candidates: %w", err)
	}
	return nil
}

// pruneVersionCandidates は、却下された組と同じ集まりの 2 本の組を候補から消す
// （data-model.md §1 の不変条件）。束ねる操作と、スキャン時の引き継ぎが集まりのメンバーや
// 却下の鍵を付け替えたあとに呼ぶ。
func pruneVersionCandidates(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `delete from video_version_candidates
		where exists (select 1 from video_version_dismissals d
		               where d.key_a = video_version_candidates.key_a and d.key_b = video_version_candidates.key_b)
		   or exists (select 1 from video_bundle_members ma
		                join video_bundle_members mb on mb.bundle_id = ma.bundle_id
		               where ma.content_key = video_version_candidates.key_a
		                 and mb.content_key = video_version_candidates.key_b)`); err != nil {
		return fmt.Errorf("cannot prune the version candidates: %w", err)
	}
	return nil
}

// candidatePairsFrom は候補の組 c と、その 2 つの鍵の動画 va・vb を結ぶ。content_key が空でない
// 条件は結ぶ相手を変えない（候補の鍵は空でない）が、部分インデックス videos_content_key_idx
// （空でない content_key だけの索引）を使わせるために要る。無いと va・vb を全件走査する総当たりになる。
const candidatePairsFrom = `from video_version_candidates c
	join videos va on va.content_key = c.key_a and va.content_key <> ''
	join videos vb on vb.content_key = c.key_b and vb.content_key <> ''
	where `

// candidatePairsCondition は、候補の組 c の両方の鍵に登録の所在を持つ動画があり、却下されて
// おらず、同じ集まりでもない条件である（research.md R-7: 却下は算出と読み出しの両方で除く）。
func candidatePairsCondition() string {
	return registeredVideoCondition("va") + ` and ` + registeredVideoCondition("vb") + `
	and not exists (select 1 from video_version_dismissals d where d.key_a = c.key_a and d.key_b = c.key_b)
	and not exists (select 1 from video_bundle_members ma join video_bundle_members mb on mb.bundle_id = ma.bundle_id
	                 where ma.content_key = c.key_a and mb.content_key = c.key_b)`
}

// Candidates は「同じ動画かもしれない」候補のうち、両方の鍵に登録の所在を持つ動画がある組を
// 新しい順（created_at、同じなら鍵の順）に最大 domain.MaxVersionCandidates 組返し、Total に
// 全件の数を入れる（data-model.md §7）。各組の動画は所有者から見た詳細（GET /api/videos/{id}
// と同じ値）で、id の小さい順に並べる。所有者だけの経路が使う。
func (s *VersionStore) Candidates(ctx context.Context) (domain.VersionCandidatePage, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.VersionCandidatePage{}, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	page := domain.VersionCandidatePage{Items: []domain.VersionCandidate{}}
	condition := candidatePairsCondition()
	if err := tx.QueryRowContext(ctx, `select count(*) `+candidatePairsFrom+condition).Scan(&page.Total); err != nil {
		return domain.VersionCandidatePage{}, fmt.Errorf("cannot count the version candidates: %w", err)
	}
	pairs, err := candidatePairIDs(ctx, tx, condition)
	if err != nil {
		return domain.VersionCandidatePage{}, err
	}

	videos := map[int64]domain.Video{}
	for _, p := range pairs {
		var item domain.VersionCandidate
		for i, id := range p.ids {
			video, ok := videos[id]
			if !ok {
				video, err = getVideo(ctx, tx, domain.AudienceOwner, id)
				if err != nil {
					return domain.VersionCandidatePage{}, err
				}
				videos[id] = video
			}
			item.Videos[i] = video
		}
		item.Distance = p.distance
		page.Items = append(page.Items, item)
	}
	if err := tx.Commit(); err != nil {
		return domain.VersionCandidatePage{}, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	return page, nil
}

// candidatePair は候補の 1 組の動画の id（小さい順）と距離である。
type candidatePair struct {
	ids      [2]int64
	distance int
}

// candidatePairIDs は候補の組を新しい順に最大 domain.MaxVersionCandidates 組読む。
func candidatePairIDs(ctx context.Context, tx *sql.Tx, condition string) ([]candidatePair, error) {
	rows, err := tx.QueryContext(ctx, `select va.id, vb.id, c.distance `+candidatePairsFrom+condition+`
		order by c.created_at desc, c.key_a, c.key_b limit ?`, domain.MaxVersionCandidates)
	if err != nil {
		return nil, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var pairs []candidatePair
	for rows.Next() {
		var p candidatePair
		if err := rows.Scan(&p.ids[0], &p.ids[1], &p.distance); err != nil {
			return nil, fmt.Errorf("cannot read the version candidates: %w", err)
		}
		if p.ids[0] > p.ids[1] {
			p.ids[0], p.ids[1] = p.ids[1], p.ids[0]
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	return pairs, nil
}

// Dismiss は 2 本の動画を「違う動画」と記録し（video_version_dismissals、key_a < key_b）、その組の
// 候補を消す（data-model.md §7・§8）。候補に無い組でも記録する。どちらかがいまライブラリに
// 無ければ domain.ErrNotFound、2 本が同じ内容なら domain.ErrTooFewVersions で、何も変えない。
func (s *VersionStore) Dismiss(ctx context.Context, videoIDs [2]int64) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot dismiss the version candidate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var keys [2]string
	for i, id := range videoIDs {
		keys[i], err = bundlableContentKey(ctx, tx, id)
		if err != nil {
			return err
		}
	}
	keyA, keyB := min(keys[0], keys[1]), max(keys[0], keys[1])
	if keyA == keyB {
		return domain.ErrTooFewVersions
	}
	if _, err := tx.ExecContext(ctx, `insert or ignore into video_version_dismissals (key_a, key_b, created_at)
		values (?, ?, ?)`, keyA, keyB, time.Now().Unix()); err != nil {
		return fmt.Errorf("cannot record the dismissal: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `delete from video_version_candidates where key_a = ? and key_b = ?`,
		keyA, keyB); err != nil {
		return fmt.Errorf("cannot remove the version candidate: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot dismiss the version candidate: %w", err)
	}
	return nil
}
