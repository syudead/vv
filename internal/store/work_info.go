package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 動画の作品情報（specs/047-video-work-info/data-model.md「Writes (WorkInfoStore)」「Reads」「Rules」、
// research.md R-2・R-5・R-11・R-13・R-14）。作品情報は利用者データの鍵（userKeyExpr）に結ぶ 6 つの表の
// 行で持ち、1 つの物として書き換える。video_work_info の行は品番か説明があるときだけ置き、code_key は
// 値と同じ文で書く。

// workInfoTables は動画ごとの作品情報の 6 表である。書き換えと引き継ぎは 6 表をまとめて扱う。
var workInfoTables = []string{
	"video_work_info", "video_work_dates", "video_work_urls", "video_work_external_ids",
	"video_work_people", "video_work_organizations",
}

// Set は動画 videoID の作品情報を info でまるごと置き換え、作品情報を含む動画を返す。info は
// domain.NormalizeWorkInfo で整え、規則に合わなければ *domain.InvalidWorkInfoError で何も書かない。
//
// videoID は利用者データの鍵へ引き直し（userKeysForVideoIDs）、引けなければ domain.ErrNotFound。
// 無い人物・団体を指せば *domain.CreditNotFoundError（Field は送られた一覧の位置）で何も書かない。
// 保存済みの値と同じなら書かず、更新日時も進めず、イベントも発行しない。変わったときは同じ取引で
// その鍵の内容（集まりなら全メンバー）の更新日時を進め、確定後に domain.VideoWorkInfoChanged を
// 1 回発行する。
func (s *WorkInfoStore) Set(ctx context.Context, videoID int64, info domain.WorkInfo) (domain.Video, error) {
	normalized, _, err := domain.NormalizeWorkInfo(info)
	if err != nil {
		return domain.Video{}, err
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
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
	if _, err := checkCreditRefs(ctx, tx, []domain.WorkInfo{info}); err != nil {
		return domain.Video{}, err
	}
	now := time.Now()
	edited, err := applyWorkInfo(ctx, tx, keys[:1], map[string]domain.WorkInfo{keys[0]: normalized})
	if err != nil {
		return domain.Video{}, err
	}
	if err := touchEditedAtForUserKeys(ctx, tx, edited, now); err != nil {
		return domain.Video{}, err
	}
	video, err := ownerVideoWithWorkInfo(ctx, tx, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	events, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return domain.Video{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the work information: %w", err)
	}
	s.db.publishEvents(events...)
	return video, nil
}

// SetMany は外部連携の一括操作で、changes の各動画の作品情報をまるごと置き換え、反映後の動画を
// changes の順に返す（同じ動画を 2 度指せば 2 度返す）。同じ利用者データの鍵（同じ動画か同じ集まりの
// 動画）を 2 度指せば、後の指定がその鍵の値になる。
//
// 全体を 1 つの取引で行い、1 件でも失敗すれば何も残さない。作品情報が規則に合わないか無い人物・団体を
// 指せば *domain.WorkInfoAtError、動画を引けなければ *domain.VideoRefNotFoundError で、どちらも位置を
// 運ぶ。件数の上限は呼び出し側が確かめる。更新日時は取引の初めと最後の値を比べて変わった鍵だけ進め、
// 確定後に変わった鍵があれば domain.VideoWorkInfoChanged を 1 回発行する。
func (s *WorkInfoStore) SetMany(ctx context.Context, changes []domain.WorkInfoChange) ([]domain.Video, error) {
	raw := make([]domain.WorkInfo, 0, len(changes))
	infos := make([]domain.WorkInfo, 0, len(changes))
	refs := make([]domain.VideoRef, 0, len(changes))
	for i, change := range changes {
		normalized, _, err := domain.NormalizeWorkInfo(change.WorkInfo)
		if err != nil {
			return nil, &domain.WorkInfoAtError{Index: i, Err: err}
		}
		raw = append(raw, change.WorkInfo)
		infos = append(infos, normalized)
		refs = append(refs, change.Video)
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot update the work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := resolveTaggableVideos(ctx, tx, refs)
	if err != nil {
		return nil, err
	}
	if index, err := checkCreditRefs(ctx, tx, raw); err != nil {
		if index < 0 {
			return nil, err
		}
		return nil, &domain.WorkInfoAtError{Index: index, Err: err}
	}
	final := make(map[string]domain.WorkInfo, len(targets))
	for i, target := range targets {
		final[target.userKey] = infos[i]
	}
	now := time.Now()
	edited, err := applyWorkInfo(ctx, tx, uniqueUserKeys(targets), final)
	if err != nil {
		return nil, err
	}
	if err := touchEditedAtForUserKeys(ctx, tx, edited, now); err != nil {
		return nil, err
	}
	out, err := readTargetVideos(ctx, tx, targets)
	if err != nil {
		return nil, err
	}
	if err := attachWorkInfo(ctx, tx, out); err != nil {
		return nil, err
	}
	events, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot update the work information: %w", err)
	}
	s.db.publishEvents(events...)
	return out, nil
}

// SetDetails は全画面の編集画面の保存で、動画 videoID の表示名を displayName に、作品情報を info に
// 1 つの取引で置き換え、両方を含む動画を返す（research.md R-14）。
//
// 表示名を domain.NormalizeDisplayName、次に作品情報を domain.NormalizeWorkInfo で整え、どちらの誤りも
// 何も書く前に返す（*domain.InvalidDisplayNameError、*domain.InvalidWorkInfoError）。動画を引けなければ
// domain.ErrNotFound、無い人物・団体を指せば *domain.CreditNotFoundError で、どれも何も書かない。
//
// 表示名は内容の識別子に、作品情報は利用者データの鍵に書き、それぞれ保存済みと違うときだけ書く。更新
// 日時は 1 回の touchEditedAt で、表示名が変われば動画の内容、作品情報が変われば鍵の全メンバーの内容を
// 進める。確定後、表示名が変われば domain.VideoOverrideChanged、作品情報が変われば
// domain.VideoWorkInfoChanged を発行し、どちらも変わらなければ何も発行しない。
func (s *WorkInfoStore) SetDetails(ctx context.Context, videoID int64, displayName string, info domain.WorkInfo) (domain.Video, error) {
	name, _, err := domain.NormalizeDisplayName(displayName)
	if err != nil {
		return domain.Video{}, err
	}
	normalized, _, err := domain.NormalizeWorkInfo(info)
	if err != nil {
		return domain.Video{}, err
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the video details: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	contentKeys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	userKeys, err := userKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return domain.Video{}, err
	}
	if len(contentKeys) == 0 || len(userKeys) == 0 {
		return domain.Video{}, domain.ErrNotFound
	}
	if _, err := checkCreditRefs(ctx, tx, []domain.WorkInfo{info}); err != nil {
		return domain.Video{}, err
	}
	now := time.Now()
	touched, nameChanged, err := writeChangedDisplayName(ctx, tx, contentKeys[0], name, now)
	if err != nil {
		return domain.Video{}, err
	}
	edited, err := applyWorkInfo(ctx, tx, userKeys[:1], map[string]domain.WorkInfo{userKeys[0]: normalized})
	if err != nil {
		return domain.Video{}, err
	}
	members, err := contentKeysForUserKeys(ctx, tx, edited)
	if err != nil {
		return domain.Video{}, err
	}
	touched = append(touched, members...)
	slices.Sort(touched)
	if err := touchEditedAt(ctx, tx, slices.Compact(touched), now); err != nil {
		return domain.Video{}, err
	}
	video, err := ownerVideoWithWorkInfo(ctx, tx, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	events, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return domain.Video{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Video{}, fmt.Errorf("cannot update the video details: %w", err)
	}
	if nameChanged {
		events = append([]domain.Event{domain.VideoOverrideChanged{VideoID: videoID}}, events...)
	}
	s.db.publishEvents(events...)
	return video, nil
}

// ByUserKeys は利用者データの鍵 userKeys の作品情報を返す（data-model.md「Reads」）。どの表にも行の
// 無い鍵は含めない。人物と団体は役割の順・位置の順で今の名前を運び、URL は位置の順、日付は
// domain.DateKinds の順、外部 ID は (source, value) の順である。
//
// 6 つの表は別々の問い合わせなので、同じ読み取りスナップショット（読み取り専用の取引）で読み、
// 書き込みと重なっても古い行と新しい行が混ざらないようにする。
func (s *WorkInfoStore) ByUserKeys(ctx context.Context, userKeys []string) (map[string]domain.WorkInfo, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the work information: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return workInfoByUserKeys(ctx, tx, userKeys)
}

// writeChangedDisplayName は内容 key の表示名を整え済みの name にし、保存済みと違ったときだけ書いて
// その内容の所在の照合用の鍵を作り直す。書いたときは更新日時を進める内容の識別子と真を返す。
func writeChangedDisplayName(ctx context.Context, tx *sql.Tx, key, name string, now time.Time) ([]string, bool, error) {
	before, err := displayNamesOf(ctx, tx, []string{key})
	if err != nil {
		return nil, false, err
	}
	if before[key] == name {
		return nil, false, nil
	}
	if err := writeDisplayName(ctx, tx, key, name, now); err != nil {
		return nil, false, err
	}
	if err := refreshSearchKeysForContentKeys(ctx, tx, []string{key}); err != nil {
		return nil, false, err
	}
	return []string{key}, true, nil
}

// checkCreditRefs は infos（整える前の値）の人物と団体の id がすべてあることを確かめる。無い id が
// あれば、その作品情報の infos での位置と *domain.CreditNotFoundError を返す。Field は送られた一覧の
// 位置（people[2].person.id）で、infos の順、各作品情報の中では人物、団体の順に最初の 1 件を返す。
// 読み出しの誤りの位置は -1 である。
func checkCreditRefs(ctx context.Context, tx *sql.Tx, infos []domain.WorkInfo) (int, error) {
	var personIDs, organizationIDs []int64
	for _, info := range infos {
		for _, p := range info.People {
			personIDs = append(personIDs, p.Person.ID)
		}
		for _, o := range info.Organizations {
			organizationIDs = append(organizationIDs, o.Organization.ID)
		}
	}
	people, err := existingCreditIDs(ctx, tx, creditTablesByKind[domain.CreditPerson], personIDs)
	if err != nil {
		return -1, err
	}
	organizations, err := existingCreditIDs(ctx, tx, creditTablesByKind[domain.CreditOrganization], organizationIDs)
	if err != nil {
		return -1, err
	}
	for index, info := range infos {
		for i, p := range info.People {
			if !people[p.Person.ID] {
				return index, &domain.CreditNotFoundError{
					Kind: domain.CreditPerson, ID: p.Person.ID, Field: fmt.Sprintf("people[%d].person.id", i),
				}
			}
		}
		for i, o := range info.Organizations {
			if !organizations[o.Organization.ID] {
				return index, &domain.CreditNotFoundError{
					Kind: domain.CreditOrganization, ID: o.Organization.ID,
					Field: fmt.Sprintf("organizations[%d].organization.id", i),
				}
			}
		}
	}
	return -1, nil
}

// applyWorkInfo は鍵 keys の作品情報を final の整え済みの値にそろえ、値が変わった鍵を keys の順で
// 返す。変わった鍵だけを書く。更新日時は呼び出し側が進める。
func applyWorkInfo(ctx context.Context, tx *sql.Tx, keys []string, final map[string]domain.WorkInfo) ([]string, error) {
	before, err := workInfoByUserKeys(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	var edited []string
	for _, key := range keys {
		if before[key].Equal(final[key]) {
			continue
		}
		if err := replaceWorkInfo(ctx, tx, key, final[key]); err != nil {
			return nil, err
		}
		edited = append(edited, key)
	}
	return edited, nil
}

// replaceWorkInfo は鍵 key の 6 表の行を消し、整え済みの info の行を書き直す。品番と説明が両方空なら
// video_work_info の行は書かない。位置は一覧の添字（人物と団体は役割の中の添字）である。
func replaceWorkInfo(ctx context.Context, tx *sql.Tx, key string, info domain.WorkInfo) error {
	if err := deleteWorkInfo(ctx, tx, key); err != nil {
		return err
	}
	if info.Code != "" || info.Description != "" {
		if _, err := tx.ExecContext(ctx,
			`insert into video_work_info (content_key, code, code_key, description, search_version) values (?, ?, ?, ?, ?)`,
			key, nullableString(info.Code), domain.FoldForMatch(info.Code), nullableString(info.Description),
			domain.SearchKeyVersion,
		); err != nil {
			return fmt.Errorf("cannot save the work information: %w", err)
		}
	}
	var statements []workInfoInsert
	for _, d := range info.Dates {
		statements = append(statements, workInfoInsert{`video_work_dates (content_key, kind, date) values (?, ?, ?)`,
			[]any{key, string(d.Kind), d.Date}})
	}
	for position, u := range info.URLs {
		statements = append(statements, workInfoInsert{`video_work_urls (content_key, position, url) values (?, ?, ?)`,
			[]any{key, position, u}})
	}
	for _, id := range info.ExternalIDs {
		statements = append(statements, workInfoInsert{
			`video_work_external_ids (content_key, source, value) values (?, ?, ?)`, []any{key, id.Source, id.Value}})
	}
	people := map[domain.PersonRole]int{}
	for _, p := range info.People {
		statements = append(statements, workInfoInsert{
			`video_work_people (content_key, role, position, person_id) values (?, ?, ?, ?)`,
			[]any{key, string(p.Role), people[p.Role], p.Person.ID}})
		people[p.Role]++
	}
	organizations := map[domain.OrganizationRole]int{}
	for _, o := range info.Organizations {
		statements = append(statements, workInfoInsert{
			`video_work_organizations (content_key, role, position, organization_id) values (?, ?, ?, ?)`,
			[]any{key, string(o.Role), organizations[o.Role], o.Organization.ID}})
		organizations[o.Role]++
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, `insert into `+statement.into, statement.args...); err != nil {
			return fmt.Errorf("cannot save the work information: %w", err)
		}
	}
	return nil
}

// workInfoInsert は作品情報の 1 行を書く insert into の後ろの部分と引数である。
type workInfoInsert struct {
	into string
	args []any
}

// deleteWorkInfo は鍵 key の 6 表の行を消す。
func deleteWorkInfo(ctx context.Context, tx *sql.Tx, key string) error {
	for _, table := range workInfoTables {
		if _, err := tx.ExecContext(ctx, `delete from `+table+` where content_key = ?`, key); err != nil {
			return fmt.Errorf("cannot clear the %s rows: %w", table, err)
		}
	}
	return nil
}

// moveWorkInfo は引き継ぎで鍵 from の作品情報を鍵 to へ付け替える（data-model.md「Bundles and
// succession」）。6 表を 1 つの物として扱い、from にどれかの行があれば to の 6 表の行を消してから
// 付け替える。from に行が無ければ to の作品情報を残す。
func moveWorkInfo(ctx context.Context, tx *sql.Tx, from, to string) error {
	args := make([]any, len(workInfoTables))
	for i := range args {
		args[i] = from
	}
	var present bool
	if err := tx.QueryRowContext(ctx, workInfoExistsQuery, args...).Scan(&present); err != nil {
		return fmt.Errorf("cannot read the work information: %w", err)
	}
	if !present {
		return nil
	}
	if err := deleteWorkInfo(ctx, tx, to); err != nil {
		return err
	}
	for _, table := range workInfoTables {
		if _, err := tx.ExecContext(ctx, `update `+table+` set content_key = ? where content_key = ?`, to, from); err != nil {
			return fmt.Errorf("cannot move the %s values: %w", table, err)
		}
	}
	return nil
}

// workInfoExistsQuery は 6 表のどれかに鍵（表ごとに 1 つ、同じ値の引数）の行があるかを読む問い合わせである。
var workInfoExistsQuery = func() string {
	parts := make([]string, 0, len(workInfoTables))
	for _, table := range workInfoTables {
		parts = append(parts, `exists (select 1 from `+table+` where content_key = ?)`)
	}
	return `select ` + strings.Join(parts, ` or `)
}()

// ownerVideoWithWorkInfo は所有者から見た動画 videoID を、作品情報を埋めて返す。
func ownerVideoWithWorkInfo(ctx context.Context, tx *sql.Tx, videoID int64) (domain.Video, error) {
	video, err := getVideo(ctx, tx, domain.AudienceOwner, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	videos := []domain.Video{video}
	if err := attachWorkInfo(ctx, tx, videos); err != nil {
		return domain.Video{}, err
	}
	return videos[0], nil
}

// attachWorkInfo は videos の各動画の Video.WorkInfo を、利用者データの鍵の作品情報で埋める。作品情報の
// 無い動画は nil のままにする。
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

// workInfoChangedEvents は値が変わった鍵 edited に属するすべての動画の id を運ぶイベントを返す。集まりの
// 鍵なら全メンバーの動画になる。変わった鍵が無ければ空。
func workInfoChangedEvents(ctx context.Context, tx *sql.Tx, edited []string) ([]domain.Event, error) {
	if len(edited) == 0 {
		return nil, nil
	}
	ids, err := videoIDsForUserKeys(ctx, tx, edited)
	if err != nil {
		return nil, err
	}
	return []domain.Event{domain.VideoWorkInfoChanged{VideoIDs: ids}}, nil
}

// videoIDsForUserKeys は利用者データの鍵 userKeys に属するすべての動画の id を小さい順に返す。集まりの
// 鍵なら全メンバーの動画になる。鍵に書いた値の変化を、同じ鍵を読む各動画の知らせにするのに使う。
func videoIDsForUserKeys(ctx context.Context, tx *sql.Tx, userKeys []string) ([]int64, error) {
	contentKeys, err := contentKeysForUserKeys(ctx, tx, userKeys)
	if err != nil {
		return nil, err
	}
	return videoIDsForContentKeys(ctx, tx, contentKeys)
}

// workInfoByUserKeys は鍵 userKeys の作品情報を返す（ByUserKeys の並び）。どの表にも行の無い鍵は含めず、
// 含めた鍵の一覧は空でも nil にしない。
func workInfoByUserKeys(ctx context.Context, q queryExecer, userKeys []string) (map[string]domain.WorkInfo, error) {
	infos := map[string]*domain.WorkInfo{}
	if len(userKeys) == 0 {
		return map[string]domain.WorkInfo{}, nil
	}
	encoded, err := json.Marshal(userKeys)
	if err != nil {
		return nil, fmt.Errorf("cannot build user keys: %w", err)
	}
	entry := func(key string) *domain.WorkInfo {
		info, ok := infos[key]
		if !ok {
			info = &domain.WorkInfo{
				Dates: []domain.WorkDate{}, People: []domain.WorkPerson{}, Organizations: []domain.WorkOrganization{},
				URLs: []string{}, ExternalIDs: []domain.ExternalID{},
			}
			infos[key] = info
		}
		return info
	}
	for _, read := range workInfoReads(entry) {
		if err := eachWorkInfoRow(ctx, q, read.query, string(encoded), read.scan); err != nil {
			return nil, err
		}
	}
	out := make(map[string]domain.WorkInfo, len(infos))
	for key, info := range infos {
		slices.SortStableFunc(info.Dates, func(a, b domain.WorkDate) int {
			return slices.Index(domain.DateKinds, a.Kind) - slices.Index(domain.DateKinds, b.Kind)
		})
		slices.SortStableFunc(info.People, func(a, b domain.WorkPerson) int {
			return slices.Index(domain.PersonRoles, a.Role) - slices.Index(domain.PersonRoles, b.Role)
		})
		slices.SortStableFunc(info.Organizations, func(a, b domain.WorkOrganization) int {
			return slices.Index(domain.OrganizationRoles, a.Role) - slices.Index(domain.OrganizationRoles, b.Role)
		})
		out[key] = *info
	}
	return out, nil
}

// workInfoRead は作品情報の表 1 つを読む問い合わせ（引数は鍵の JSON 配列 1 つ）と、1 行を entry の
// 作品情報へ足す関数である。
type workInfoRead struct {
	query string
	scan  func(*sql.Rows) error
}

// workInfoReads は 6 表の読み出しである。日付と人物・団体の役割の順は workInfoByUserKeys が並べ直すので、
// ここでは鍵と位置の順に読む。
func workInfoReads(entry func(string) *domain.WorkInfo) []workInfoRead {
	const keys = `content_key in (select value from json_each(?))`
	return []workInfoRead{
		{`select content_key, coalesce(code, ''), coalesce(description, '') from video_work_info where ` + keys,
			func(rows *sql.Rows) error {
				var key, code, description string
				if err := rows.Scan(&key, &code, &description); err != nil {
					return err
				}
				info := entry(key)
				info.Code, info.Description = code, description
				return nil
			}},
		{`select content_key, kind, date from video_work_dates where ` + keys,
			func(rows *sql.Rows) error {
				var key string
				var d domain.WorkDate
				if err := rows.Scan(&key, &d.Kind, &d.Date); err != nil {
					return err
				}
				info := entry(key)
				info.Dates = append(info.Dates, d)
				return nil
			}},
		{`select content_key, url from video_work_urls where ` + keys + ` order by content_key, position`,
			func(rows *sql.Rows) error {
				var key, u string
				if err := rows.Scan(&key, &u); err != nil {
					return err
				}
				info := entry(key)
				info.URLs = append(info.URLs, u)
				return nil
			}},
		{`select content_key, source, value from video_work_external_ids where ` + keys + ` order by content_key, source, value`,
			func(rows *sql.Rows) error {
				var key string
				var id domain.ExternalID
				if err := rows.Scan(&key, &id.Source, &id.Value); err != nil {
					return err
				}
				info := entry(key)
				info.ExternalIDs = append(info.ExternalIDs, id)
				return nil
			}},
		creditLinkRead(creditTablesByKind[domain.CreditPerson], func(key, role string, ref domain.CreditRef) {
			info := entry(key)
			info.People = append(info.People, domain.WorkPerson{Role: domain.PersonRole(role), Person: ref})
		}),
		creditLinkRead(creditTablesByKind[domain.CreditOrganization], func(key, role string, ref domain.CreditRef) {
			info := entry(key)
			info.Organizations = append(info.Organizations,
				domain.WorkOrganization{Role: domain.OrganizationRole(role), Organization: ref})
		}),
	}
}

// creditLinkRead は人物か団体の結び付きの表 t.links を、鍵と位置の順に今の名前を付けて読む。add が
// 1 行を作品情報へ足す。
func creditLinkRead(t creditTables, add func(key, role string, ref domain.CreditRef)) workInfoRead {
	return workInfoRead{
		query: t.q(`select l.content_key, l.role, l.{id}, n.name from {links} l
			join {names} n on n.{id} = l.{id} and n.canonical = 1
			where l.content_key in (select value from json_each(?)) order by l.content_key, l.position`),
		scan: func(rows *sql.Rows) error {
			var key, role string
			var ref domain.CreditRef
			if err := rows.Scan(&key, &role, &ref.ID, &ref.Name); err != nil {
				return err
			}
			add(key, role, ref)
			return nil
		},
	}
}

// eachWorkInfoRow は query を arg で問い合わせ、各行を scan に渡す。
func eachWorkInfoRow(ctx context.Context, q queryExecer, query, arg string, scan func(*sql.Rows) error) error {
	rows, err := q.QueryContext(ctx, query, arg)
	if err != nil {
		return fmt.Errorf("cannot read the work information: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("cannot read the work information: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the work information: %w", err)
	}
	return nil
}

// readTargetVideos は一括操作の対象の動画を、targets の順に（同じ動画は何度でも）所有者の見え方で読む。
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
