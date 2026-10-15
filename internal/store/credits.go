package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 人物と団体の保存（specs/047-video-work-info/data-model.md「Entities」、research.md R-3・R-8）。
// 2 つの種類は表の名前だけが違うので、SQL は 1 回だけ書き、creditTables で名前を埋める。

// creditTables は種類ごとの表と列の名前である。
type creditTables struct {
	kind domain.CreditKind
	// entity は実体の表、names は名前と別名、externalIDs は外部 ID、links は動画との結び付きの表。
	entity, names, externalIDs, links string
	// idColumn は names・externalIDs・links が実体を指す列。
	idColumn string
	// cursorKind は一覧のカーソルの先頭に置く一覧の種類。
	cursorKind string
	// hasParent は実体の表が parent_id を持つ（団体）こと。
	hasParent bool
	// smartRef はスマートフォルダの条件でこの実体を名指す種類（specs/052-smart-folders）。
	smartRef domain.SmartRuleRefKind
}

var creditTablesByKind = map[domain.CreditKind]creditTables{
	domain.CreditPerson: {
		kind: domain.CreditPerson, entity: "people", names: "people_names", externalIDs: "people_external_ids",
		links: "video_work_people", idColumn: "person_id", cursorKind: "people", smartRef: domain.SmartRefPerson,
	},
	domain.CreditOrganization: {
		kind: domain.CreditOrganization, entity: "organizations", names: "organization_names",
		externalIDs: "organization_external_ids", links: "video_work_organizations",
		idColumn: "organization_id", cursorKind: "organizations", hasParent: true,
		smartRef: domain.SmartRefOrganization,
	},
}

// creditTablesFor は kind の表の名前を返す。知らない種類は誤り。
func creditTablesFor(kind domain.CreditKind) (creditTables, error) {
	t, ok := creditTablesByKind[kind]
	if !ok {
		return creditTables{}, fmt.Errorf("unknown credit kind: %q", kind)
	}
	return t, nil
}

// q は statement の {entity}・{names}・{ext}・{links}・{id} を表と列の名前に置き換える。
func (t creditTables) q(statement string) string {
	return strings.NewReplacer(
		"{entity}", t.entity, "{names}", t.names, "{ext}", t.externalIDs, "{links}", t.links, "{id}", t.idColumn,
	).Replace(statement)
}

// parentColumn は listed の parent_id に置く式である。人物では null。
func (t creditTables) parentColumn() string {
	if t.hasParent {
		return `e.parent_id`
	}
	return `null`
}

// Get は kind の実体 id を返す。無ければ domain.ErrNotFound。
func (s *CreditStore) Get(ctx context.Context, kind domain.CreditKind, id int64) (domain.Credit, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.Credit{}, err
	}
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Credit{}, fmt.Errorf("cannot start reading the %s: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	credit, err := creditByID(ctx, tx, t, id)
	if err != nil {
		return domain.Credit{}, err
	}
	return credit, tx.Commit()
}

// Lookup は外部 ID (source, value) を持つ kind の実体を返す。無ければ domain.ErrNotFound。
// 外部 ID は種類ごとに 1 つの実体だけが持つので、答えは高々 1 つである。
func (s *CreditStore) Lookup(ctx context.Context, kind domain.CreditKind, source, value string) (domain.Credit, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.Credit{}, err
	}
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Credit{}, fmt.Errorf("cannot start reading the %s: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	holder, found, err := externalIDHolder(ctx, tx, t, domain.ExternalID{Source: source, Value: value}, 0)
	if err != nil {
		return domain.Credit{}, err
	}
	if !found {
		return domain.Credit{}, domain.ErrNotFound
	}
	credit, err := creditByID(ctx, tx, t, holder)
	if err != nil {
		return domain.Credit{}, err
	}
	return credit, tx.Commit()
}

// Create は kind の実体を作って返す。入力は domain.NormalizeCredit で整え、他の実体が持つ外部 ID は
// *domain.CreditExternalIDTakenError、無い親の団体は *domain.CreditNotFoundError（Field は parentId）。
// 人物では ParentID を読まない。
func (s *CreditStore) Create(ctx context.Context, kind domain.CreditKind, input domain.CreditInput) (domain.Credit, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.Credit{}, err
	}
	normalized, err := domain.NormalizeCredit(input)
	if err != nil {
		return domain.Credit{}, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Credit{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var parentID any
	if t.hasParent && normalized.ParentID != nil {
		if err := checkCreditParent(ctx, tx, t, 0, *normalized.ParentID); err != nil {
			return domain.Credit{}, err
		}
		parentID = *normalized.ParentID
	}
	if err := checkExternalIDsFree(ctx, tx, t, 0, normalized.ExternalIDs, input.ExternalIDs); err != nil {
		return domain.Credit{}, err
	}
	statement := `insert into {entity} (created_at) values (?)`
	args := []any{time.Now().Unix()}
	if t.hasParent {
		statement = `insert into {entity} (created_at, parent_id) values (?, ?)`
		args = append(args, parentID)
	}
	res, err := tx.ExecContext(ctx, t.q(statement), args...)
	if err != nil {
		return domain.Credit{}, fmt.Errorf("cannot create the %s: %w", kind, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Credit{}, fmt.Errorf("cannot create the %s: %w", kind, err)
	}
	if err := writeCreditNames(ctx, tx, t, id, normalized.Name, normalized.Aliases); err != nil {
		return domain.Credit{}, err
	}
	if err := writeCreditExternalIDs(ctx, tx, t, id, normalized.ExternalIDs); err != nil {
		return domain.Credit{}, err
	}
	credit, err := creditByID(ctx, tx, t, id)
	if err != nil {
		return domain.Credit{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot create the %s: %w", kind, err)
	}
	return credit, nil
}

// Update は kind の実体 id のうち patch にあるフィールドを置き換えて返す。一覧は丸ごと置き換える。
// 入力は今の値に patch を重ねてから domain.NormalizeCredit で整えるので、新しい名前が残す別名と
// 同じ綴りなら name の WorkInfoDuplicate になる。何も変わらなければ書かずに今の実体を返す。無ければ
// domain.ErrNotFound、他の実体が持つ外部 ID は *domain.CreditExternalIDTakenError、親の団体が
// 自分か子孫なら domain.ErrCreditParentCycle（新しい親から根まで parent_id をたどる）。
func (s *CreditStore) Update(ctx context.Context, kind domain.CreditKind, id int64, patch domain.CreditPatch) (domain.Credit, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.Credit{}, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.Credit{}, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := creditByID(ctx, tx, t, id)
	if err != nil {
		return domain.Credit{}, err
	}
	next, err := patchedCredit(current, patch)
	if err != nil {
		return domain.Credit{}, err
	}
	changes := creditChanges(t, current, next)
	if !changes.any() {
		return current, tx.Commit()
	}
	if err := applyCreditChanges(ctx, tx, t, id, next, patch, changes); err != nil {
		return domain.Credit{}, err
	}
	updated, err := creditByID(ctx, tx, t, id)
	if err != nil {
		return domain.Credit{}, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.Credit{}, fmt.Errorf("cannot update the %s: %w", kind, err)
	}
	return updated, nil
}

// patchedCredit は今の実体 current に patch を重ね、domain.NormalizeCredit で整えた入力を返す。
// 別名が patch に無いとき、保存してある別名どうしは重ならないので、別名の Duplicate は新しい名前と
// 残す別名の重なりである。その誤りは呼び手が送った name に付け替える。
func patchedCredit(current domain.Credit, patch domain.CreditPatch) (domain.CreditInput, error) {
	merged := domain.CreditInput{Name: current.Name, Aliases: current.Aliases, ExternalIDs: current.ExternalIDs}
	if current.Parent != nil {
		parent := current.Parent.ID
		merged.ParentID = &parent
	}
	if patch.Name != nil {
		merged.Name = *patch.Name
	}
	if patch.Aliases != nil {
		merged.Aliases = *patch.Aliases
	}
	if patch.ExternalIDs != nil {
		merged.ExternalIDs = *patch.ExternalIDs
	}
	if patch.ParentSet {
		merged.ParentID = patch.ParentID
	}
	normalized, err := domain.NormalizeCredit(merged)
	var invalid *domain.InvalidWorkInfoError
	if patch.Aliases == nil && errors.As(err, &invalid) && invalid.Problem == domain.WorkInfoDuplicate &&
		strings.HasPrefix(invalid.Field, "aliases[") {
		return domain.CreditInput{}, &domain.InvalidWorkInfoError{Field: "name", Problem: domain.WorkInfoDuplicate}
	}
	return normalized, err
}

// creditChangeSet は更新で変わる部分である。
type creditChangeSet struct {
	names, externalIDs, parent bool
}

func (c creditChangeSet) any() bool { return c.names || c.externalIDs || c.parent }

// creditChanges は今の実体 current と整えた入力 next を比べる。別名と外部 ID は読み出しと同じ順に
// 並べてから比べる。人物では親を比べない。
func creditChanges(t creditTables, current domain.Credit, next domain.CreditInput) creditChangeSet {
	aliases := slices.Clone(next.Aliases)
	domain.SortAliases(aliases)
	externalIDs := slices.Clone(next.ExternalIDs)
	domain.SortExternalIDs(externalIDs)
	var changes creditChangeSet
	changes.names = current.Name != next.Name || !slices.Equal(current.Aliases, aliases)
	changes.externalIDs = !slices.Equal(current.ExternalIDs, externalIDs)
	if t.hasParent {
		var currentParent int64
		if current.Parent != nil {
			currentParent = current.Parent.ID
		}
		var nextParent int64
		if next.ParentID != nil {
			nextParent = *next.ParentID
		}
		changes.parent = currentParent != nextParent
	}
	return changes
}

// applyCreditChanges は Update の書き込みである。外部 ID と親を確かめてから、変わった部分の行を
// 書き直す。
func applyCreditChanges(ctx context.Context, tx *sql.Tx, t creditTables, id int64, next domain.CreditInput,
	patch domain.CreditPatch, changes creditChangeSet,
) error {
	if changes.externalIDs {
		if err := checkExternalIDsFree(ctx, tx, t, id, next.ExternalIDs, *patch.ExternalIDs); err != nil {
			return err
		}
	}
	if changes.parent {
		var parentID any
		if next.ParentID != nil {
			if err := checkCreditParent(ctx, tx, t, id, *next.ParentID); err != nil {
				return err
			}
			parentID = *next.ParentID
		}
		if _, err := tx.ExecContext(ctx, t.q(`update {entity} set parent_id = ? where id = ?`), parentID, id); err != nil {
			return fmt.Errorf("cannot change the parent of %s %d: %w", t.kind, id, err)
		}
	}
	if changes.names {
		if _, err := tx.ExecContext(ctx, t.q(`delete from {names} where {id} = ?`), id); err != nil {
			return fmt.Errorf("cannot replace the names of %s %d: %w", t.kind, id, err)
		}
		if err := writeCreditNames(ctx, tx, t, id, next.Name, next.Aliases); err != nil {
			return err
		}
	}
	if changes.externalIDs {
		if _, err := tx.ExecContext(ctx, t.q(`delete from {ext} where {id} = ?`), id); err != nil {
			return fmt.Errorf("cannot replace the external IDs of %s %d: %w", t.kind, id, err)
		}
		if err := writeCreditExternalIDs(ctx, tx, t, id, next.ExternalIDs); err != nil {
			return err
		}
	}
	return nil
}

// Delete は kind の実体 id を消す。名前・外部 ID・結び付きの行は外部キーで連鎖して消え、団体の子は
// parent_id が null になる。結び付きが外れた (content_key, role) の位置は詰め直す。無ければ
// domain.ErrNotFound。
func (s *CreditStore) Delete(ctx context.Context, kind domain.CreditKind, id int64) error {
	t, err := creditTablesFor(kind)
	if err != nil {
		return err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := existingCreditIDs(ctx, tx, t, []int64{id})
	if err != nil {
		return err
	}
	if !existing[id] {
		return domain.ErrNotFound
	}
	affected, err := creditLinkLists(ctx, tx, t, []int64{id})
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, t.q(`delete from {entity} where id = ?`), id); err != nil {
		return fmt.Errorf("cannot delete %s %d: %w", kind, id, err)
	}
	if err := renumberCreditLinks(ctx, tx, t, affected); err != nil {
		return err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return fmt.Errorf("cannot delete %s %d: %w", kind, id, err)
	}
	return nil
}

// Merge は sourceIDs の実体を同じ種類の targetID へ 1 つの取引で統合する（data-model.md「Entities」）。
// 統合元の結び付きは統合先がその場所を引き継ぎ（統合先が既に同じ動画の同じ役割にあれば統合元の行を
// 捨てて詰め直す）、名前と別名は統合先の別名になり（統合先が既に持つ綴りは捨てる）、外部 ID は統合先へ
// 移り、統合元の団体の子は統合先の下へ付け替わり、統合元は消える。
//
// 統合先が無ければ domain.ErrNotFound、統合元に統合先があれば domain.ErrMergeSameCredit、統合元の
// 団体が統合先の親の連鎖にあれば domain.ErrCreditParentCycle。統合後の別名が 50 件か外部 ID が 20 件を
// 超えれば aliases か externalIds の WorkInfoTooMany で、どれも何も変えない。無い統合元は NotFoundIDs。
func (s *CreditStore) Merge(ctx context.Context, kind domain.CreditKind, targetID int64, sourceIDs []int64) (domain.CreditMergeOutcome, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.CreditMergeOutcome{}, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.CreditMergeOutcome{}, err
	}
	defer func() { _ = tx.Rollback() }()

	merged, notFound, err := mergeSources(ctx, tx, t, targetID, sourceIDs)
	if err != nil {
		return domain.CreditMergeOutcome{}, err
	}
	if len(merged) > 0 {
		if err := mergeCreditsInto(ctx, tx, t, targetID, merged); err != nil {
			return domain.CreditMergeOutcome{}, err
		}
		if err := checkMergedCreditLimits(ctx, tx, t, targetID); err != nil {
			return domain.CreditMergeOutcome{}, err
		}
	}
	credit, err := creditByID(ctx, tx, t, targetID)
	if err != nil {
		return domain.CreditMergeOutcome{}, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.CreditMergeOutcome{}, fmt.Errorf("cannot merge into %s %d: %w", kind, targetID, err)
	}
	return domain.CreditMergeOutcome{Credit: credit, NotFoundIDs: notFound}, nil
}

// mergeSources は Merge の前の確かめである。統合先の有無、統合先そのものを含む統合元、親の輪を
// 確かめ、今ある統合元と無い統合元を（重複を除いて現れた順に）返す。
func mergeSources(ctx context.Context, tx *sql.Tx, t creditTables, targetID int64, sourceIDs []int64) ([]int64, []int64, error) {
	unique := uniqueTagIDs(sourceIDs)
	existing, err := existingCreditIDs(ctx, tx, t, append([]int64{targetID}, unique...))
	if err != nil {
		return nil, nil, err
	}
	if !existing[targetID] {
		return nil, nil, domain.ErrNotFound
	}
	if slices.Contains(unique, targetID) {
		return nil, nil, domain.ErrMergeSameCredit
	}
	notFound := []int64{}
	merged := make([]int64, 0, len(unique))
	for _, id := range unique {
		if existing[id] {
			merged = append(merged, id)
		} else {
			notFound = append(notFound, id)
		}
	}
	if t.hasParent && len(merged) > 0 {
		ancestors, err := organizationAncestors(ctx, tx, targetID)
		if err != nil {
			return nil, nil, err
		}
		for _, id := range merged {
			if ancestors[id] {
				return nil, nil, domain.ErrCreditParentCycle
			}
		}
	}
	return merged, notFound, nil
}

// mergeCreditsInto は同じ取引の中で sourceIDs の実体を targetID へ移して消す。統合元の数によらず
// 決まった数の文で行う（詰め直しを除く）。
func mergeCreditsInto(ctx context.Context, tx *sql.Tx, t creditTables, targetID int64, sourceIDs []int64) error {
	encoded, err := json.Marshal(sourceIDs)
	if err != nil {
		return fmt.Errorf("cannot build %s ids: %w", t.kind, err)
	}
	affected, err := creditLinkLists(ctx, tx, t, sourceIDs)
	if err != nil {
		return err
	}
	statements := []struct{ what, statement string }{
		// 統合元の行は位置を保ったまま統合先を指す。統合先が既に同じ (content_key, role) にあれば
		// 一意の制約で書き換えずに残し、次の文で消す。
		{"move the links", `update or ignore {links} set {id} = ?1 where {id} in (select value from json_each(?2))`},
		{"drop the links", `delete from {links} where {id} in (select value from json_each(?2))`},
		// 名前は別名として足す。統合先が既に持つ綴りは主キーで捨てる。
		{"move the names", `insert or ignore into {names} ({id}, name, canonical, language, search_key, sort_key, search_version)
			select ?1, name, 0, language, search_key, sort_key, search_version from {names}
			 where {id} in (select value from json_each(?2))`},
		// (source, value) は種類ごとに一意なので、統合先が同じものを持つことは無い。
		{"move the external IDs", `update {ext} set {id} = ?1 where {id} in (select value from json_each(?2))`},
	}
	if t.hasParent {
		statements = append(statements, struct{ what, statement string }{
			"move the children", `update organizations set parent_id = ?1 where parent_id in (select value from json_each(?2))`,
		})
	}
	statements = append(statements, struct{ what, statement string }{
		"delete the sources", `delete from {entity} where id in (select value from json_each(?2))`,
	})
	for _, s := range statements {
		if _, err := tx.ExecContext(ctx, t.q(s.statement), targetID, string(encoded)); err != nil {
			return fmt.Errorf("cannot %s to %s %d: %w", s.what, t.kind, targetID, err)
		}
	}
	if err := renumberCreditLinks(ctx, tx, t, affected); err != nil {
		return err
	}
	// スマートフォルダの条件が名指す統合元を統合先にする（specs/052-smart-folders/research.md R-9）。
	return rewriteSmartRuleRefs(ctx, tx, t.smartRef, targetID, sourceIDs)
}

// checkMergedCreditLimits は統合後の統合先の別名と外部 ID の件数を数え、上限を超えれば
// *domain.InvalidWorkInfoError を返す（呼び手が取引を巻き戻す）。
func checkMergedCreditLimits(ctx context.Context, tx *sql.Tx, t creditTables, targetID int64) error {
	var aliases, externalIDs int
	if err := tx.QueryRowContext(ctx, t.q(`select
		(select count(*) from {names} where {id} = ?1 and canonical = 0),
		(select count(*) from {ext} where {id} = ?1)`), targetID).Scan(&aliases, &externalIDs); err != nil {
		return fmt.Errorf("cannot count the names of %s %d: %w", t.kind, targetID, err)
	}
	if aliases > domain.CreditAliasesMaxCount {
		return &domain.InvalidWorkInfoError{Field: "aliases", Problem: domain.WorkInfoTooMany, Limit: domain.CreditAliasesMaxCount}
	}
	if externalIDs > domain.ExternalIDsMaxCount {
		return &domain.InvalidWorkInfoError{Field: "externalIds", Problem: domain.WorkInfoTooMany, Limit: domain.ExternalIDsMaxCount}
	}
	return nil
}

// existingCreditIDs は ids のうち今ある実体の id の集合を返す。
func existingCreditIDs(ctx context.Context, tx *sql.Tx, t creditTables, ids []int64) (map[int64]bool, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build %s ids: %w", t.kind, err)
	}
	rows, err := tx.QueryContext(ctx, t.q(`select id from {entity} where id in (select value from json_each(?))`), string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read %s ids: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()
	existing := make(map[int64]bool, len(ids))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read %s ids: %w", t.kind, err)
		}
		existing[id] = true
	}
	return existing, rows.Err()
}

// organizationAncestors は団体 id から根まで parent_id をたどった祖先の集合を返す（id は含まない）。
// 保存してある連鎖は輪にならない（invariants_test.go）が、念のため一度見た団体で止まる。
func organizationAncestors(ctx context.Context, tx *sql.Tx, id int64) (map[int64]bool, error) {
	ancestors := map[int64]bool{}
	current := id
	for {
		var parent sql.NullInt64
		err := tx.QueryRowContext(ctx, `select parent_id from organizations where id = ?`, current).Scan(&parent)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !parent.Valid) {
			return ancestors, nil
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read the parent of organization %d: %w", current, err)
		}
		if parent.Int64 == id || ancestors[parent.Int64] {
			return ancestors, nil
		}
		ancestors[parent.Int64] = true
		current = parent.Int64
	}
}

// checkCreditParent は parentID が団体 id（作るときは 0）の親になれるかを確かめる。無い団体は
// *domain.CreditNotFoundError（Field は parentId）、id そのものか、parentID から根までの連鎖に id が
// あれば（parentID が id の子孫なら）domain.ErrCreditParentCycle。
func checkCreditParent(ctx context.Context, tx *sql.Tx, t creditTables, id, parentID int64) error {
	existing, err := existingCreditIDs(ctx, tx, t, []int64{parentID})
	if err != nil {
		return err
	}
	if !existing[parentID] {
		return &domain.CreditNotFoundError{Kind: t.kind, ID: parentID, Field: "parentId"}
	}
	if id == 0 {
		return nil
	}
	if parentID == id {
		return domain.ErrCreditParentCycle
	}
	ancestors, err := organizationAncestors(ctx, tx, parentID)
	if err != nil {
		return err
	}
	if ancestors[id] {
		return domain.ErrCreditParentCycle
	}
	return nil
}

// externalIDHolder は外部 ID を持つ kind の実体の id を返す。exceptID（0 なら無し）の実体は数えない。
func externalIDHolder(ctx context.Context, tx *sql.Tx, t creditTables, id domain.ExternalID, exceptID int64) (int64, bool, error) {
	var holder int64
	err := tx.QueryRowContext(ctx, t.q(`select {id} from {ext} where source = ? and value = ? and {id} <> ?`),
		id.Source, id.Value, exceptID).Scan(&holder)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("cannot look up the external ID of a %s: %w", t.kind, err)
	}
	return holder, true, nil
}

// checkExternalIDsFree は整えた外部 ID のどれかを selfID 以外の実体が持てば
// *domain.CreditExternalIDTakenError を返す。Field の位置は呼び手が送った一覧 raw の位置である。
func checkExternalIDsFree(ctx context.Context, tx *sql.Tx, t creditTables, selfID int64, normalized, raw []domain.ExternalID) error {
	for _, id := range normalized {
		holder, found, err := externalIDHolder(ctx, tx, t, id, selfID)
		if err != nil {
			return err
		}
		if found {
			return &domain.CreditExternalIDTakenError{
				Kind: t.kind, Field: fmt.Sprintf("externalIds[%d]", domain.ExternalIDIndex(raw, id)), HolderID: holder,
			}
		}
	}
	return nil
}

// writeCreditNames は実体 id の正規の名前と別名の行を書く。照合用の鍵と並びの鍵も同じ文で書く。
func writeCreditNames(ctx context.Context, tx *sql.Tx, t creditTables, id int64, name string, aliases []domain.Alias) error {
	statement := t.q(`insert into {names} ({id}, name, canonical, language, search_key, sort_key, search_version)
		values (?, ?, ?, ?, ?, ?, ?)`)
	write := func(name string, canonical bool, language string) error {
		var lang any
		if language != "" {
			lang = language
		}
		if _, err := tx.ExecContext(ctx, statement, id, name, boolToInt(canonical), lang,
			domain.FoldForMatch(name), domain.NaturalSortKey(name), domain.SearchKeyVersion); err != nil {
			return fmt.Errorf("cannot save the names of %s %d: %w", t.kind, id, err)
		}
		return nil
	}
	if err := write(name, true, ""); err != nil {
		return err
	}
	for _, alias := range aliases {
		if err := write(alias.Name, false, alias.Language); err != nil {
			return err
		}
	}
	return nil
}

// writeCreditExternalIDs は実体 id の外部 ID の行を書く。
func writeCreditExternalIDs(ctx context.Context, tx *sql.Tx, t creditTables, id int64, ids []domain.ExternalID) error {
	statement := t.q(`insert into {ext} ({id}, source, value) values (?, ?, ?)`)
	for _, external := range ids {
		if _, err := tx.ExecContext(ctx, statement, id, external.Source, external.Value); err != nil {
			return fmt.Errorf("cannot save the external IDs of %s %d: %w", t.kind, id, err)
		}
	}
	return nil
}

// creditLinkList は結び付きの一覧 1 つ（動画の利用者データの鍵と役割）である。
type creditLinkList struct {
	contentKey, role string
}

// creditLinkLists は ids の実体が結び付いている一覧を返す。消すか統合した後に詰め直す対象である。
func creditLinkLists(ctx context.Context, tx *sql.Tx, t creditTables, ids []int64) ([]creditLinkList, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build %s ids: %w", t.kind, err)
	}
	rows, err := tx.QueryContext(ctx, t.q(`select distinct content_key, role from {links}
		where {id} in (select value from json_each(?))`), string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the links of a %s: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()
	var lists []creditLinkList
	for rows.Next() {
		var list creditLinkList
		if err := rows.Scan(&list.contentKey, &list.role); err != nil {
			return nil, fmt.Errorf("cannot read the links of a %s: %w", t.kind, err)
		}
		lists = append(lists, list)
	}
	return lists, rows.Err()
}

// renumberCreditLinks は一覧ごとに位置を今の並びのまま 0 から詰め直す。並びを先に読み切り、位置の
// 小さい行から書く。詰めた位置はいつも今の位置以下で、それより小さい位置は書き終えているので、
// (content_key, role, position) の主キーが途中で重ならない（renumberPlaylists と同じく、相関
// 副問い合わせで数える 1 文の update は使わない）。
func renumberCreditLinks(ctx context.Context, tx *sql.Tx, t creditTables, lists []creditLinkList) error {
	update := t.q(`update {links} set position = ? where content_key = ? and role = ? and position = ?`)
	for _, list := range lists {
		positions, err := creditLinkPositions(ctx, tx, t, list)
		if err != nil {
			return err
		}
		for i, position := range positions {
			if position == i {
				continue
			}
			if _, err := tx.ExecContext(ctx, update, i, list.contentKey, list.role, position); err != nil {
				return fmt.Errorf("cannot renumber the links of a %s: %w", t.kind, err)
			}
		}
	}
	return nil
}

func creditLinkPositions(ctx context.Context, tx *sql.Tx, t creditTables, list creditLinkList) ([]int, error) {
	rows, err := tx.QueryContext(ctx, t.q(`select position from {links} where content_key = ? and role = ? order by position`),
		list.contentKey, list.role)
	if err != nil {
		return nil, fmt.Errorf("cannot read the links of a %s: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()
	var positions []int
	for rows.Next() {
		var position int
		if err := rows.Scan(&position); err != nil {
			return nil, fmt.Errorf("cannot read the links of a %s: %w", t.kind, err)
		}
		positions = append(positions, position)
	}
	return positions, rows.Err()
}

// RefreshSearchKeys は search_version が現在の版より小さい人物・団体の名前の行の search_key と
// sort_key、作品情報の行の code_key を作り直し、作り直した行の数を返す（data-model.md「Entities」）。
// 起動時、TagStore.RefreshSearchKeys の後で呼ぶ。
func (s *CreditStore) RefreshSearchKeys(ctx context.Context) (int, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	refreshed := 0
	for _, kind := range []domain.CreditKind{domain.CreditPerson, domain.CreditOrganization} {
		n, err := refreshCreditNameKeys(ctx, tx, creditTablesByKind[kind])
		if err != nil {
			return 0, err
		}
		refreshed += n
	}
	n, err := refreshWorkInfoCodeKeys(ctx, tx)
	if err != nil {
		return 0, err
	}
	refreshed += n
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cannot save the credit search keys: %w", err)
	}
	return refreshed, nil
}

// staleKeyRow は鍵を作り直す行 1 件（実体か作品情報の鍵と、畳み込む値）である。
type staleKeyRow struct {
	id    any
	value string
}

// staleKeyRows は query（版を 1 つの引数で受け、鍵と値の 2 列を返す）の行を読み切る。
func staleKeyRows(ctx context.Context, tx *sql.Tx, query string) ([]staleKeyRow, error) {
	rows, err := tx.QueryContext(ctx, query, domain.SearchKeyVersion)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var stale []staleKeyRow
	for rows.Next() {
		var row staleKeyRow
		if err := rows.Scan(&row.id, &row.value); err != nil {
			return nil, err
		}
		stale = append(stale, row)
	}
	return stale, rows.Err()
}

func refreshCreditNameKeys(ctx context.Context, tx *sql.Tx, t creditTables) (int, error) {
	stale, err := staleKeyRows(ctx, tx, t.q(`select {id}, name from {names} where search_version < ?`))
	if err != nil {
		return 0, fmt.Errorf("cannot read the names of %s entities: %w", t.kind, err)
	}
	update := t.q(`update {names} set search_key = ?, sort_key = ?, search_version = ? where {id} = ? and name = ?`)
	for _, row := range stale {
		if _, err := tx.ExecContext(ctx, update, domain.FoldForMatch(row.value), domain.NaturalSortKey(row.value),
			domain.SearchKeyVersion, row.id, row.value); err != nil {
			return 0, fmt.Errorf("cannot save the search key of a %s name: %w", t.kind, err)
		}
	}
	return len(stale), nil
}

func refreshWorkInfoCodeKeys(ctx context.Context, tx *sql.Tx) (int, error) {
	stale, err := staleKeyRows(ctx, tx,
		`select content_key, coalesce(code, '') from video_work_info where search_version < ?`)
	if err != nil {
		return 0, fmt.Errorf("cannot read the work information codes: %w", err)
	}
	for _, row := range stale {
		if _, err := tx.ExecContext(ctx,
			`update video_work_info set code_key = ?, search_version = ? where content_key = ?`,
			domain.FoldForMatch(row.value), domain.SearchKeyVersion, row.id); err != nil {
			return 0, fmt.Errorf("cannot save the code key of the work information: %w", err)
		}
	}
	return len(stale), nil
}
