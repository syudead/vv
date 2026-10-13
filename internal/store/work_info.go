package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 動画の作品情報（specs/047-video-work-info/data-model.md「Writes」「Rules」、research.md R-2・R-5・R-8）。
// 作品情報は利用者データの鍵（userKeyExpr）に結ぶ video_work_info の 1 行と、その出演者の
// video_work_performers の行で持つ。行は少なくとも 1 つの項目か出演者があるときだけ置き、
// maker_key・name_key・search_key は値と同じ文で書く。

// Set は動画 videoID の作品情報を info でまるごと置き換え、作品情報を含む動画を返す。info は
// domain.NormalizeWorkInfo で整え、規則に合わなければ *domain.InvalidWorkInfoError で何も書かない。
//
// videoID は利用者データの鍵へ引き直し（userKeysForVideoIDs）、引けなければ domain.ErrNotFound。
// 保存済みの値と同じなら書かず、更新日時も進めず、イベントも発行しない。変わったときは同じ取引で
// その鍵の内容の更新日時を進め、確定後に domain.VideoWorkInfoChanged を 1 回発行する。
func (s *WorkInfoStore) Set(ctx context.Context, videoID int64, info domain.WorkInfo) (domain.Video, error) {
	normalized, _, err := domain.NormalizeWorkInfo(info)
	if err != nil {
		return domain.Video{}, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := userKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	if len(keys) == 0 {
		return domain.Video{}, domain.ErrNotFound
	}
	edited, err := applyWorkInfo(ctx, tx, keys, map[string]domain.WorkInfo{keys[0]: normalized})
	if err != nil {
		return domain.Video{}, err
	}
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	videos := []domain.Video{video}
	if err := attachWorkInfo(ctx, tx, videos); err != nil {
		return domain.Video{}, err
	}
	event, err := workInfoChangedEvent(ctx, tx, edited)
	if err != nil {
		return domain.Video{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the work information: %w", err)
	}
	s.publishWorkInfoChanged(event)
	return videos[0], nil
}

// SetMany は外部連携の一括操作で、changes の各動画の作品情報をまるごと置き換え、反映後の動画を
// changes の順に返す（同じ動画を 2 度指せば 2 度返し、値は後の指定が勝つ）。束ねた動画どうしは
// 同じ鍵なので、後の指定がその鍵の値になる。
//
// 全体を 1 つの取引で行い、1 件でも失敗すれば何も残さない。作品情報が規則に合わなければ
// *domain.WorkInfoAtError、動画を引けなければ *domain.VideoRefNotFoundError で、どちらも位置を
// 運ぶ。件数の上限は呼び出し側が確かめる。更新日時は取引の初めと最後の値を比べて進め、確定後に
// 変わった鍵があれば domain.VideoWorkInfoChanged を 1 回発行する。
func (s *WorkInfoStore) SetMany(ctx context.Context, changes []domain.WorkInfoChange) ([]domain.Video, error) {
	infos := make([]domain.WorkInfo, 0, len(changes))
	refs := make([]domain.VideoRef, 0, len(changes))
	for i, change := range changes {
		normalized, _, err := domain.NormalizeWorkInfo(change.WorkInfo)
		if err != nil {
			return nil, &domain.WorkInfoAtError{Index: i, Err: err}
		}
		infos = append(infos, normalized)
		refs = append(refs, change.Video)
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot update the work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := resolveTaggableVideos(ctx, tx, refs)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(targets))
	final := make(map[string]domain.WorkInfo, len(targets))
	for i, target := range targets {
		if _, ok := final[target.userKey]; !ok {
			keys = append(keys, target.userKey)
		}
		final[target.userKey] = infos[i]
	}
	edited, err := applyWorkInfo(ctx, tx, keys, final)
	if err != nil {
		return nil, err
	}
	out, err := readTargetVideos(ctx, tx, targets)
	if err != nil {
		return nil, err
	}
	if err := attachWorkInfo(ctx, tx, out); err != nil {
		return nil, err
	}
	event, err := workInfoChangedEvent(ctx, tx, edited)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot update the work information: %w", err)
	}
	s.publishWorkInfoChanged(event)
	return out, nil
}

// ByUserKeys は利用者データの鍵 userKeys の作品情報を返す。作品情報の無い鍵は含めない。
// 出演者は position の順である。
//
// 項目と出演者は別々の問い合わせなので、同じ読み取りスナップショット（読み取り専用の取引）で読み、
// 書き込みと重なっても古い項目と新しい出演者が混ざらないようにする（TagStore.TagImpact と同じ）。
func (s *WorkInfoStore) ByUserKeys(ctx context.Context, userKeys []string) (map[string]domain.WorkInfo, error) {
	tx, err := s.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return workInfoByUserKeys(ctx, tx, userKeys)
}

// RefreshSearchKeys は search_version が domain.SearchKeyVersion より小さい行の maker_key・
// search_key と、その出演者の name_key を今の規則で作り直し、作り直した行の数を返す
// （data-model.md「Writes」）。起動時、TagStore.RefreshSearchKeys の隣で呼ぶ。
func (s *WorkInfoStore) RefreshSearchKeys(ctx context.Context) (int, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	keys, err := staleNames(ctx, tx, `select content_key from video_work_info where search_version < ?`)
	if err != nil {
		return 0, fmt.Errorf("cannot read the work information: %w", err)
	}
	infos, err := workInfoByUserKeys(ctx, tx, keys)
	if err != nil {
		return 0, err
	}
	for _, key := range keys {
		info := infos[key]
		if _, err := tx.ExecContext(ctx,
			`update video_work_info set maker_key = ?, search_key = ?, search_version = ? where content_key = ?`,
			makerKey(info.Maker), workInfoSearchKey(info), domain.SearchKeyVersion, key,
		); err != nil {
			return 0, fmt.Errorf("cannot save the work information search key: %w", err)
		}
		for position, name := range info.Performers {
			if _, err := tx.ExecContext(ctx,
				`update video_work_performers set name_key = ? where content_key = ? and position = ?`,
				domain.FoldForMatch(name), key, position,
			); err != nil {
				return 0, fmt.Errorf("cannot save the performer search key: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cannot save the work information search keys: %w", err)
	}
	return len(keys), nil
}

// publishWorkInfoChanged は event に動画があれば発行する。
func (s *WorkInfoStore) publishWorkInfoChanged(event domain.VideoWorkInfoChanged) {
	if len(event.VideoIDs) > 0 {
		s.publish(event)
	}
}

// applyWorkInfo は鍵 keys の作品情報を final の整え済みの値にそろえ、値が変わった鍵を keys の順で
// 返す。変わった鍵だけを書き、その内容の更新日時を now に進める（touchEditedAtForUserKeys）。
func applyWorkInfo(ctx context.Context, tx *sql.Tx, keys []string, final map[string]domain.WorkInfo) ([]string, error) {
	before, err := workInfoByUserKeys(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var edited []string
	for _, key := range keys {
		if before[key].Equal(final[key]) {
			continue
		}
		if err := replaceWorkInfo(ctx, tx, key, final[key], now); err != nil {
			return nil, err
		}
		edited = append(edited, key)
	}
	if err := touchEditedAtForUserKeys(ctx, tx, edited, now); err != nil {
		return nil, err
	}
	return edited, nil
}

// replaceWorkInfo は鍵 key の作品情報の行と出演者を消し、info が空でなければ書き直す。
// maker_key・name_key・search_key は同じ文で今の規則の版で書く。
func replaceWorkInfo(ctx context.Context, tx *sql.Tx, key string, info domain.WorkInfo, now time.Time) error {
	// 出演者は外部キーの on delete cascade で一緒に消える。
	if _, err := tx.ExecContext(ctx, `delete from video_work_info where content_key = ?`, key); err != nil {
		return fmt.Errorf("cannot save the work information: %w", err)
	}
	if info.IsEmpty() {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`insert into video_work_info (content_key, code, release_date, maker, maker_key, description, source_url,
		     search_key, search_version, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key, nullableString(info.Code), nullableString(info.ReleaseDate), nullableString(info.Maker),
		makerKey(info.Maker), nullableString(info.Description), nullableString(info.SourceURL),
		workInfoSearchKey(info), domain.SearchKeyVersion, now.Unix(),
	); err != nil {
		return fmt.Errorf("cannot save the work information: %w", err)
	}
	for position, name := range info.Performers {
		if _, err := tx.ExecContext(ctx,
			`insert into video_work_performers (content_key, position, name, name_key) values (?, ?, ?, ?)`,
			key, position, name, domain.FoldForMatch(name),
		); err != nil {
			return fmt.Errorf("cannot save the performers: %w", err)
		}
	}
	return nil
}

// makerKey はメーカーの照合形（maker_key）を返す。メーカーが無ければ null である。
func makerKey(maker string) any {
	if maker == "" {
		return nil
	}
	return domain.FoldForMatch(maker)
}

// workInfoSearchKey は作品情報の search_key を返す。品番・メーカー・出演者名のそれぞれの
// 照合形の、値の中の改行を空白にしたものを改行でつなぐ。空の値は入れないので、1 つの語が
// 2 つの値にまたがって当たることはない（data-model.md「Writes」）。
func workInfoSearchKey(info domain.WorkInfo) string {
	values := make([]string, 0, 2+len(info.Performers))
	values = append(values, info.Code, info.Maker)
	values = append(values, info.Performers...)
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		parts = append(parts, strings.ReplaceAll(domain.FoldForMatch(value), "\n", " "))
	}
	return strings.Join(parts, "\n")
}

// workInfoByUserKeys は鍵 userKeys の作品情報を返す。作品情報の無い鍵は含めず、出演者は
// position の順である。
func workInfoByUserKeys(ctx context.Context, q queryExecer, userKeys []string) (map[string]domain.WorkInfo, error) {
	out := make(map[string]domain.WorkInfo, len(userKeys))
	if len(userKeys) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(userKeys)
	if err != nil {
		return nil, fmt.Errorf("cannot build user keys: %w", err)
	}
	rows, err := q.QueryContext(ctx, `select content_key, coalesce(code, ''), coalesce(release_date, ''),
		coalesce(maker, ''), coalesce(description, ''), coalesce(source_url, '')
		from video_work_info where content_key in (select value from json_each(?))`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the work information: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key string
		var info domain.WorkInfo
		if err := rows.Scan(&key, &info.Code, &info.ReleaseDate, &info.Maker, &info.Description, &info.SourceURL); err != nil {
			return nil, fmt.Errorf("cannot read the work information: %w", err)
		}
		out[key] = info
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the work information: %w", err)
	}
	_ = rows.Close()
	return out, readPerformers(ctx, q, string(encoded), out)
}

// readPerformers は鍵（JSON の配列 encodedKeys）の出演者を position の順に infos へ足す。
func readPerformers(ctx context.Context, q queryExecer, encodedKeys string, infos map[string]domain.WorkInfo) error {
	rows, err := q.QueryContext(ctx, `select content_key, name from video_work_performers
		where content_key in (select value from json_each(?)) order by content_key, position`, encodedKeys)
	if err != nil {
		return fmt.Errorf("cannot read the performers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key, name string
		if err := rows.Scan(&key, &name); err != nil {
			return fmt.Errorf("cannot read the performers: %w", err)
		}
		info := infos[key]
		info.Performers = append(info.Performers, name)
		infos[key] = info
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the performers: %w", err)
	}
	return nil
}

// attachWorkInfo は videos の各動画の Video.WorkInfo を、利用者データの鍵の作品情報で埋める。
// 作品情報の無い動画は nil のままにする。
func attachWorkInfo(ctx context.Context, q queryExecer, videos []domain.Video) error {
	keys := make([]string, 0, len(videos))
	for _, video := range videos {
		if video.UserKey != "" {
			keys = append(keys, video.UserKey)
		}
	}
	infos, err := workInfoByUserKeys(ctx, q, keys)
	if err != nil {
		return err
	}
	for i := range videos {
		if info, ok := infos[videos[i].UserKey]; ok {
			videos[i].WorkInfo = &info
		}
	}
	return nil
}

// readTargetVideos は一括操作の対象の動画を、targets の順に（同じ動画は何度でも）読む。
func readTargetVideos(ctx context.Context, tx *sql.Tx, targets []taggableVideo) ([]domain.Video, error) {
	ids := make([]int64, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.id)
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	byID, err := readVideosByIDs(ctx, tx, string(encoded))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Video, 0, len(targets))
	for _, target := range targets {
		video, ok := byID[target.id]
		if !ok {
			return nil, fmt.Errorf("cannot read the video (id=%d): %w", target.id, domain.ErrNotFound)
		}
		out = append(out, video)
	}
	return out, nil
}

// workInfoChangedEvent は値が変わった鍵 edited に属するすべての動画の id を運ぶイベントを作る。
// 集まりの鍵なら全メンバーの動画になる。
func workInfoChangedEvent(ctx context.Context, tx *sql.Tx, edited []string) (domain.VideoWorkInfoChanged, error) {
	if len(edited) == 0 {
		return domain.VideoWorkInfoChanged{}, nil
	}
	contentKeys, err := contentKeysForUserKeys(ctx, tx, edited)
	if err != nil {
		return domain.VideoWorkInfoChanged{}, err
	}
	ids, err := videoIDsForContentKeys(ctx, tx, contentKeys)
	if err != nil {
		return domain.VideoWorkInfoChanged{}, err
	}
	return domain.VideoWorkInfoChanged{VideoIDs: ids}, nil
}
