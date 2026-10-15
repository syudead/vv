package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインの取得の結果の反映（specs/049-work-info-plugins/data-model.md の Rules、
// research.md R-8・R-9）。選んだ題名を表示名に、選んだタグを仮のタグとして 1 つの取引で書く。
// SetDisplayName と ApplyVideoTags を順に呼ばず、パッケージ内の関数（writeChangedDisplayName・
// writeVideoTagsByName）を共有する。047 の作品情報は書かない。

// ApplyFetchedWorkInfo は取得の結果のうち apply が選んだ項目を動画 apply.VideoID に書き、作品情報を埋めた
// 動画と、却下した名前なので付けなかったタグの名前（apply.Tags の順、重複なし）を返す。
//
// 取引の中で、プラグイン apply.PluginID の行がまだあり、問題が無く、承認済みで、ダイジェストが
// apply.Digest、世代が apply.Generation のままであることを読み直す。外れれば domain.ErrWorkInfoFetchStale で、何も書かない。
// 動画が無いか登録フォルダの下に所在が無ければ domain.ErrNotFound である。題名は
// domain.NormalizeDisplayName で整え、規則に合わなければ *domain.InvalidDisplayNameError で何も書かない。
// 確定後に、反映先と、タグを付けたなら同じ利用者データの鍵を読む動画（集まりの他のメンバー）のそれぞれに
// domain.VideoOverrideChanged を発行する。
func (s *OverrideStore) ApplyFetchedWorkInfo(
	ctx context.Context, apply domain.FetchedWorkInfoApply,
) (domain.Video, []string, error) {
	var title string
	if apply.Title != nil {
		normalized, _, err := domain.NormalizeDisplayName(*apply.Title)
		if err != nil {
			return domain.Video{}, nil, err
		}
		title = normalized
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, nil, fmt.Errorf("cannot apply the fetched work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := checkFetchedPlugin(ctx, tx, apply); err != nil {
		return domain.Video{}, nil, err
	}
	contentKeys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{apply.VideoID})
	if err != nil {
		return domain.Video{}, nil, err
	}
	userKeys, err := userKeysForVideoIDs(ctx, tx, []int64{apply.VideoID})
	if err != nil {
		return domain.Video{}, nil, err
	}
	if len(contentKeys) == 0 || len(userKeys) == 0 {
		return domain.Video{}, nil, domain.ErrNotFound
	}

	now := time.Now()
	if apply.Title != nil {
		touched, _, err := writeChangedDisplayName(ctx, tx, contentKeys[0], title, now)
		if err != nil {
			return domain.Video{}, nil, err
		}
		if err := touchEditedAt(ctx, tx, touched, now); err != nil {
			return domain.Video{}, nil, err
		}
	}
	rejected := []string{}
	changed := []int64{apply.VideoID}
	if len(apply.Tags) > 0 {
		// 取り込みは仮の付け方なので、自動タグ付けの提案を確定しない（Issue #968 の要件 6）。
		rejected, err = writeVideoTagsByName(ctx, tx, userKeys[:1], domain.VideoTagsAdd, apply.Tags,
			tagWrite{tentative: true, keepSuggested: true}, now)
		if err != nil {
			return domain.Video{}, nil, err
		}
		// タグは利用者データの鍵に付くので、集まりなら他のメンバーのタグも変わる。
		if changed, err = videoIDsForUserKeys(ctx, tx, userKeys[:1]); err != nil {
			return domain.Video{}, nil, err
		}
	}
	video, err := ownerVideoWithWorkInfo(ctx, tx, apply.VideoID)
	if err != nil {
		return domain.Video{}, nil, err
	}
	// 表示名と仮のタグを書くので、スマートフォルダの一致も変わりうる。
	if err := s.db.commitMatchChange(ctx, tx, appliedWorkInfoEvents(apply.VideoID, changed)...); err != nil {
		return domain.Video{}, nil, fmt.Errorf("cannot apply the fetched work information: %w", err)
	}
	return video, rejected, nil
}

// appliedWorkInfoEvents は反映先 videoID と、タグを書いた鍵を読む動画 changed（集まりの全メンバー）の
// それぞれに domain.VideoOverrideChanged を 1 つずつ、反映先を先に返す。画面への知らせで各動画を読み直させる。
func appliedWorkInfoEvents(videoID int64, changed []int64) []domain.Event {
	events := []domain.Event{domain.VideoOverrideChanged{VideoID: videoID}}
	for _, id := range changed {
		if id != videoID {
			events = append(events, domain.VideoOverrideChanged{VideoID: id})
		}
	}
	return events
}

// checkFetchedPlugin は取引 tx の中でプラグイン apply.PluginID の行を読み直し、行が無いか、問題があるか、
// 承認済みでないか、ダイジェストが apply.Digest でないか、世代が apply.Generation でなければ
// domain.ErrWorkInfoFetchStale を返す。世代は、取得のあとに同じバイト列で差し替えて再承認したことや、
// 取り消して再承認したことを見分ける（research.md R-8）。
func checkFetchedPlugin(ctx context.Context, tx *sql.Tx, apply domain.FetchedWorkInfoApply) error {
	var (
		current           string
		problem, approved sql.NullString
		generation        int64
	)
	err := tx.QueryRowContext(ctx,
		`select digest, problem, approved_digest, generation from work_info_plugins where id = ?`, apply.PluginID,
	).Scan(&current, &problem, &approved, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrWorkInfoFetchStale
	}
	if err != nil {
		return fmt.Errorf("cannot read the work-info plugin: %w", err)
	}
	state := domain.WorkInfoPluginStateOf(domain.WorkInfoPluginProblem(problem.String), current, approved.String)
	if state != domain.WorkInfoPluginApproved || current != apply.Digest || generation != apply.Generation {
		return domain.ErrWorkInfoFetchStale
	}
	return nil
}
