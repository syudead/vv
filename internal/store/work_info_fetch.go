package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインの取得の結果の反映（specs/049-work-info-plugins/data-model.md の Rules、
// research.md R-8・R-9、specs/052-plugin-work-info-fields/research.md R-3・R-4）。選んだ題名を表示名に、
// 選んだタグを仮のタグとして、選んだ作品の詳細を 047 の作品情報として 1 つの取引で書く。
// SetDisplayName・ApplyVideoTags・WorkInfoStore.Set を順に呼ばず、パッケージ内の関数
// （writeChangedDisplayName・writeVideoTagsByName・matchCredits・insertCredit・applyWorkInfo）を共有する。

// ApplyFetchedWorkInfo は取得の結果のうち apply が選んだ項目を動画 apply.VideoID に書き、作品情報を埋めた
// 動画と、却下した名前なので付けなかったタグの名前（apply.Tags の順、重複なし）と、作品の詳細のうち
// 飛ばした値を返す。
//
// 取引の中で、プラグイン apply.PluginID の行がまだあり、問題が無く、承認済みで、ダイジェストが
// apply.Digest、世代が apply.Generation のままであることを読み直す。外れれば domain.ErrWorkInfoFetchStale で、何も書かない。
// 動画が無いか登録フォルダの下に所在が無ければ domain.ErrNotFound である。題名は
// domain.NormalizeDisplayName で整え、規則に合わなければ *domain.InvalidDisplayNameError で何も書かない。
//
// apply.DetailFields があれば、取引の中で取得した人物・団体を照合し直し、保存済みの作品情報に
// domain.MergeFetchedWorkInfo で重ね、重ねた結果が残した作る人物・団体だけを作ってから作品情報を書く。
// 作品情報が変わったときは鍵の全メンバーの更新日時を進める。
//
// 確定後に、反映先と、タグを付けたなら同じ利用者データの鍵を読む動画（集まりの他のメンバー）のそれぞれに
// domain.VideoOverrideChanged を、作品情報が変わったなら domain.VideoWorkInfoChanged を 1 回発行する。
func (s *OverrideStore) ApplyFetchedWorkInfo(
	ctx context.Context, apply domain.FetchedWorkInfoApply,
) (domain.FetchedWorkInfoApplied, error) {
	var title string
	if apply.Title != nil {
		normalized, _, err := domain.NormalizeDisplayName(*apply.Title)
		if err != nil {
			return domain.FetchedWorkInfoApplied{}, err
		}
		title = normalized
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, fmt.Errorf("cannot apply the fetched work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := checkFetchedPlugin(ctx, tx, apply); err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	contentKeys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{apply.VideoID})
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	userKeys, err := userKeysForVideoIDs(ctx, tx, []int64{apply.VideoID})
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	if len(contentKeys) == 0 || len(userKeys) == 0 {
		return domain.FetchedWorkInfoApplied{}, domain.ErrNotFound
	}

	now := time.Now()
	var touched []string
	if apply.Title != nil {
		if touched, _, err = writeChangedDisplayName(ctx, tx, contentKeys[0], title, now); err != nil {
			return domain.FetchedWorkInfoApplied{}, err
		}
	}
	out := domain.FetchedWorkInfoApplied{RejectedTags: []string{}, Skipped: []domain.SkippedWorkInfoValue{}}
	changed := []int64{apply.VideoID}
	if len(apply.Tags) > 0 {
		// 取り込みは仮の付け方なので、自動タグ付けの提案を確定しない（Issue #968 の要件 6）。
		out.RejectedTags, err = writeVideoTagsByName(ctx, tx, userKeys[:1], domain.VideoTagsAdd, apply.Tags,
			tagWrite{tentative: true, keepSuggested: true}, now)
		if err != nil {
			return domain.FetchedWorkInfoApplied{}, err
		}
		// タグは利用者データの鍵に付くので、集まりなら他のメンバーのタグも変わる。
		if changed, err = videoIDsForUserKeys(ctx, tx, userKeys[:1]); err != nil {
			return domain.FetchedWorkInfoApplied{}, err
		}
	}
	edited, skipped, err := applyFetchedDetails(ctx, tx, userKeys[0], apply)
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	out.Skipped = append(out.Skipped, skipped...)
	members, err := contentKeysForUserKeys(ctx, tx, edited)
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	touched = append(touched, members...)
	slices.Sort(touched)
	if err := touchEditedAt(ctx, tx, slices.Compact(touched), now); err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	if out.Video, err = ownerVideoWithWorkInfo(ctx, tx, apply.VideoID); err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	workInfoEvents, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return domain.FetchedWorkInfoApplied{}, err
	}
	// 表示名と仮のタグと作品情報を書くので、スマートフォルダの一致も変わりうる。
	events := append(appliedWorkInfoEvents(apply.VideoID, changed), workInfoEvents...)
	if err := s.db.commitMatchChange(ctx, tx, events...); err != nil {
		return domain.FetchedWorkInfoApplied{}, fmt.Errorf("cannot apply the fetched work information: %w", err)
	}
	return out, nil
}

// applyFetchedDetails は apply.DetailFields が選んだ作品の詳細を、利用者データの鍵 userKey の作品情報に
// 取引 tx の中で書き（R-4）、作品情報が変わったなら userKey を、飛ばした値と一緒に返す。人物・団体は
// tx の中で照合し直すので、取得から反映の間に作られた実体にも結び付く（R-3）。
func applyFetchedDetails(
	ctx context.Context, tx *sql.Tx, userKey string, apply domain.FetchedWorkInfoApply,
) ([]string, []domain.SkippedWorkInfoValue, error) {
	if len(apply.DetailFields) == 0 {
		return nil, nil, nil
	}
	stored, err := workInfoByUserKeys(ctx, tx, []string{userKey})
	if err != nil {
		return nil, nil, err
	}
	people := creditTablesByKind[domain.CreditPerson]
	organizations := creditTablesByKind[domain.CreditOrganization]
	personMatches, err := matchCredits(ctx, tx, people, domain.PersonMatchEntries(apply.Details.People))
	if err != nil {
		return nil, nil, err
	}
	organizationMatches, err := matchCredits(ctx, tx, organizations,
		domain.OrganizationMatchEntries(apply.Details.Organizations))
	if err != nil {
		return nil, nil, err
	}
	merge := domain.MergeFetchedWorkInfo(domain.WorkInfoMergeInput{
		Stored: stored[userKey], Fetched: apply.Details, Fields: apply.DetailFields,
		PersonMatches: personMatches, OrganizationMatches: organizationMatches,
	})
	err = createMergedCredits(ctx, tx, people, merge.NewPeople, func(at int, id int64) {
		merge.Info.People[at].Person.ID = id
	})
	if err != nil {
		return nil, nil, err
	}
	err = createMergedCredits(ctx, tx, organizations, merge.NewOrganizations, func(at int, id int64) {
		merge.Info.Organizations[at].Organization.ID = id
	})
	if err != nil {
		return nil, nil, err
	}
	final, _, err := domain.NormalizeWorkInfo(merge.Info)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot merge the fetched work information: %w", err)
	}
	edited, err := applyWorkInfo(ctx, tx, []string{userKey}, map[string]domain.WorkInfo{userKey: final})
	if err != nil {
		return nil, nil, err
	}
	return edited, merge.Skipped, nil
}

// createMergedCredits は t の実体 created をそれぞれ作り、その Links の位置ごとに link で新しい id を渡す。
// 外部 ID は照合で誰も持っていないことを確かめ済みで、名前と外部 ID は domain.NormalizeCredit を通っている。
func createMergedCredits(
	ctx context.Context, tx *sql.Tx, t creditTables, created []domain.NewCredit, link func(at int, id int64),
) error {
	for _, credit := range created {
		id, err := insertCredit(ctx, tx, t, credit.Input, nil)
		if err != nil {
			return err
		}
		for _, at := range credit.Links {
			link(at, id)
		}
	}
	return nil
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
