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
// apply.Digest のままであることを読み直す。外れれば domain.ErrWorkInfoFetchStale で、何も書かない。
// 動画が無いか登録フォルダの下に所在が無ければ domain.ErrNotFound である。題名は
// domain.NormalizeDisplayName で整え、規則に合わなければ *domain.InvalidDisplayNameError で何も書かない。
// 確定後に domain.VideoOverrideChanged を 1 回発行する。
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

	if err := checkFetchedPlugin(ctx, tx, apply.PluginID, apply.Digest); err != nil {
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
	if len(apply.Tags) > 0 {
		rejected, err = writeVideoTagsByName(ctx, tx, userKeys[:1], domain.VideoTagsAdd, apply.Tags, true, now)
		if err != nil {
			return domain.Video{}, nil, err
		}
	}
	video, err := ownerVideoWithWorkInfo(ctx, tx, apply.VideoID)
	if err != nil {
		return domain.Video{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Video{}, nil, fmt.Errorf("cannot apply the fetched work information: %w", err)
	}
	s.db.publishEvents(domain.VideoOverrideChanged{VideoID: apply.VideoID})
	return video, rejected, nil
}

// checkFetchedPlugin は取引 tx の中でプラグイン id の行を読み直し、行が無いか、問題があるか、承認済みで
// ないか、ダイジェストが digest でなければ domain.ErrWorkInfoFetchStale を返す。
func checkFetchedPlugin(ctx context.Context, tx *sql.Tx, id int64, digest string) error {
	var (
		current           string
		problem, approved sql.NullString
	)
	err := tx.QueryRowContext(ctx,
		`select digest, problem, approved_digest from work_info_plugins where id = ?`, id,
	).Scan(&current, &problem, &approved)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrWorkInfoFetchStale
	}
	if err != nil {
		return fmt.Errorf("cannot read the work-info plugin: %w", err)
	}
	state := domain.WorkInfoPluginStateOf(domain.WorkInfoPluginProblem(problem.String), current, approved.String)
	if state != domain.WorkInfoPluginApproved || current != digest {
		return domain.ErrWorkInfoFetchStale
	}
	return nil
}
