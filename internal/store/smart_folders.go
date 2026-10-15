package store

// スマートフォルダの保存（specs/052-smart-folders/data-model.md「Migration」「Edits」）。
// SmartFolderStore は書き込みの取引の中で親と入れ子を読み、domain の規則（ValidateGroupName・
// SmartRule.Encode・ValidateSmartSort・ValidateSmartFolderParent）で決める。どの書き込みも
// owner_kept を 1 にする（research.md R-10）。条件が名指す実体の有無は書き込みでは確かめず、
// 読み出し（MissingRefs）が欠けた参照として返す（R-9）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// smartFolderNow は編集の時刻の元である。
var smartFolderNow = groupNow

// smartRuleRefTables は欠けた参照を確かめる表である。タグの種類（SmartRefTagKind）は、その表が
// main に入ってから確かめる（plan.md「Match smart folders by tag kind」）。
var smartRuleRefTables = map[domain.SmartRuleRefKind]string{
	domain.SmartRefTag:          "tags",
	domain.SmartRefPerson:       "people",
	domain.SmartRefOrganization: "organizations",
}

// CreateSmartFolder はスマートフォルダを作り、その行を返す。名前は domain.ValidateGroupName で整え、
// 並べ順が空なら domain.SortAddedDesc にする。親が無ければ domain.ErrSmartFolderNotFound で、何も
// 書かない。
func (s *SmartFolderStore) CreateSmartFolder(ctx context.Context, create domain.SmartFolderCreate) (domain.SmartFolder, error) {
	if create.Sort == "" {
		create.Sort = domain.SortAddedDesc
	}
	folder := domain.SmartFolder{ParentID: create.ParentID, Name: create.Name, Rule: create.Rule, Sort: create.Sort}
	rule, err := checkSmartFolder(&folder)
	if err != nil {
		return domain.SmartFolder{}, err
	}
	var id int64
	err = writeWithEvents(ctx, s.db, "create the smart folder", func(tx *sql.Tx) ([]domain.Event, error) {
		if err := ensureSmartFolderParent(ctx, tx, 0, folder.ParentID); err != nil {
			return nil, err
		}
		now := smartFolderNow().UnixMilli()
		result, err := tx.ExecContext(ctx, `insert into smart_folders
			(parent_id, name, title_key, rule, sort, owner_kept, created_at, updated_at) values (?, ?, ?, ?, ?, 1, ?, ?)`,
			nullableID(folder.ParentID), folder.Name, domain.NaturalSortKey(folder.Name), rule, string(folder.Sort), now, now)
		if err != nil {
			return nil, fmt.Errorf("cannot create the smart folder: %w", err)
		}
		id, err = result.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("cannot create the smart folder: %w", err)
		}
		return nil, nil
	})
	if err != nil {
		return domain.SmartFolder{}, err
	}
	return s.SmartFolder(ctx, id)
}

// ChangeSmartFolder はスマートフォルダ id の名前・条件・並べ順・親のうち change が持つものを変え、
// 更新日時を進め、owner_kept を 1 にする。親を変えるのが移動で、ParentID が 0 なら根にする。
// フォルダが無ければ domain.ErrNotFound、親が無ければ domain.ErrSmartFolderNotFound、親が自分か
// 自分の子孫なら domain.ErrSmartFolderCycle で、どれも何も書かない。
func (s *SmartFolderStore) ChangeSmartFolder(ctx context.Context, id int64, change domain.SmartFolderChange) (domain.SmartFolder, error) {
	err := writeWithEvents(ctx, s.db, "change the smart folder", func(tx *sql.Tx) ([]domain.Event, error) {
		folder, err := readSmartFolder(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		applySmartFolderChange(&folder, change)
		rule, err := checkSmartFolder(&folder)
		if err != nil {
			return nil, err
		}
		if change.ParentSet {
			if err := ensureSmartFolderParent(ctx, tx, id, folder.ParentID); err != nil {
				return nil, err
			}
		}
		if _, err := tx.ExecContext(ctx, `update smart_folders
			set parent_id = ?, name = ?, title_key = ?, rule = ?, sort = ?, owner_kept = 1, updated_at = ?
			where id = ?`,
			nullableID(folder.ParentID), folder.Name, domain.NaturalSortKey(folder.Name), rule, string(folder.Sort),
			smartFolderNow().UnixMilli(), id); err != nil {
			return nil, fmt.Errorf("cannot change the smart folder: %w", err)
		}
		return nil, nil
	})
	if err != nil {
		return domain.SmartFolder{}, err
	}
	return s.SmartFolder(ctx, id)
}

// DeleteSmartFolder はスマートフォルダ id を消す。子孫とそれらのお気に入り・プレイリストの項目は
// 外部キーで消える。無ければ domain.ErrNotFound。
func (s *SmartFolderStore) DeleteSmartFolder(ctx context.Context, id int64) error {
	return writeWithEvents(ctx, s.db, "delete the smart folder", func(tx *sql.Tx) ([]domain.Event, error) {
		result, err := tx.ExecContext(ctx, `delete from smart_folders where id = ?`, id)
		if err != nil {
			return nil, fmt.Errorf("cannot delete the smart folder: %w", err)
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot delete the smart folder: %w", err)
		}
		if deleted == 0 {
			return nil, smartFolderMissing()
		}
		return nil, nil
	})
}

// SmartFolder はスマートフォルダ id の行を返す。無ければ domain.ErrNotFound。
func (s *SmartFolderStore) SmartFolder(ctx context.Context, id int64) (domain.SmartFolder, error) {
	return readSmartFolder(ctx, s.db.read, id)
}

// MissingRefs は、スマートフォルダ id とその祖先の条件が名指す実体のうち今無いものを、外側の祖先から
// 順に、同じものは 1 度だけ返す（data-model.md「Contents」の Missing reference）。フォルダが無ければ
// domain.ErrNotFound。
func (s *SmartFolderStore) MissingRefs(ctx context.Context, id int64) ([]domain.SmartRuleRef, error) {
	tx, err := s.db.read.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folder: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rules, err := smartFolderPathRules(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	missing := []domain.SmartRuleRef{}
	seen := map[domain.SmartRuleRef]bool{}
	for _, rule := range rules {
		for _, ref := range rule.Refs() {
			table, ok := smartRuleRefTables[ref.Kind]
			if !ok || seen[ref] {
				continue
			}
			seen[ref] = true
			var exists bool
			if err := tx.QueryRowContext(ctx, `select exists (select 1 from `+table+` where id = ?)`, ref.ID).
				Scan(&exists); err != nil {
				return nil, fmt.Errorf("cannot read the smart folder references: %w", err)
			}
			if !exists {
				missing = append(missing, ref)
			}
		}
	}
	return missing, nil
}

// smartFolderPathRules はスマートフォルダ id とその祖先の条件を、外側の祖先から順に返す。
func smartFolderPathRules(ctx context.Context, tx *sql.Tx, id int64) ([]domain.SmartRule, error) {
	rows, err := tx.QueryContext(ctx, `with recursive up (id, depth) as (
			select id, 0 from smart_folders where id = ?
			union
			select f.parent_id, up.depth + 1 from up join smart_folders f on f.id = up.id where f.parent_id is not null)
		select f.rule from up join smart_folders f on f.id = up.id order by up.depth desc`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folder path: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var rules []domain.SmartRule
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("cannot read the smart folder path: %w", err)
		}
		rule, err := domain.ParseSmartRule([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("cannot read the smart folder rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the smart folder path: %w", err)
	}
	if len(rules) == 0 {
		return nil, smartFolderMissing()
	}
	return rules, nil
}

// applySmartFolderChange は change が持つ項目を folder に写す。
func applySmartFolderChange(folder *domain.SmartFolder, change domain.SmartFolderChange) {
	if change.Name != nil {
		folder.Name = *change.Name
	}
	if change.Rule != nil {
		folder.Rule = *change.Rule
	}
	if change.Sort != nil {
		folder.Sort = *change.Sort
	}
	if change.ParentSet {
		folder.ParentID = change.ParentID
	}
}

// checkSmartFolder は folder の名前を整え、並べ順と条件を確かめ、smart_folders.rule に置く JSON を
// 返す。
func checkSmartFolder(folder *domain.SmartFolder) (string, error) {
	name, err := domain.ValidateGroupName(folder.Name)
	if err != nil {
		return "", err
	}
	folder.Name = name
	if err := domain.ValidateSmartSort(folder.Sort); err != nil {
		return "", err
	}
	return folder.Rule.Encode()
}

// ensureSmartFolderParent は、フォルダ id（作成なら 0）を親 parent（根なら 0）の下へ置けるかを確かめる。
// 親が無ければ domain.ErrSmartFolderNotFound、自分か自分の子孫なら domain.ErrSmartFolderCycle である。
func ensureSmartFolderParent(ctx context.Context, tx *sql.Tx, id, parent int64) error {
	if parent == 0 {
		return nil
	}
	parents, err := smartFolderParents(ctx, tx)
	if err != nil {
		return err
	}
	if _, ok := parents[parent]; !ok {
		return domain.ErrSmartFolderNotFound
	}
	return domain.ValidateSmartFolderParent(id, parent, parents)
}

// smartFolderParents はすべてのスマートフォルダの id から親の id（根は 0）を返す。
func smartFolderParents(ctx context.Context, tx *sql.Tx) (map[int64]int64, error) {
	rows, err := tx.QueryContext(ctx, `select id, coalesce(parent_id, 0) from smart_folders`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	parents := map[int64]int64{}
	for rows.Next() {
		var id, parent int64
		if err := rows.Scan(&id, &parent); err != nil {
			return nil, fmt.Errorf("cannot read the smart folders: %w", err)
		}
		parents[id] = parent
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the smart folders: %w", err)
	}
	return parents, nil
}

// readSmartFolder はスマートフォルダ id の行を読む。無ければ domain.ErrNotFound を包む誤りである。
func readSmartFolder(ctx context.Context, q queryRower, id int64) (domain.SmartFolder, error) {
	var folder domain.SmartFolder
	var rule, sort string
	err := q.QueryRowContext(ctx, `select id, coalesce(parent_id, 0), name, rule, sort, owner_kept, created_at, updated_at
		from smart_folders where id = ?`, id).
		Scan(&folder.ID, &folder.ParentID, &folder.Name, &rule, &sort, &folder.OwnerKept, &folder.CreatedAt, &folder.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SmartFolder{}, smartFolderMissing()
	}
	if err != nil {
		return domain.SmartFolder{}, fmt.Errorf("cannot read the smart folder: %w", err)
	}
	if folder.Rule, err = domain.ParseSmartRule([]byte(rule)); err != nil {
		return domain.SmartFolder{}, fmt.Errorf("cannot read the smart folder rule: %w", err)
	}
	folder.Sort = domain.VideoSort(sort)
	return folder, nil
}

// smartFolderMissing はスマートフォルダそのものが無いことを表す誤りである。
func smartFolderMissing() error {
	return fmt.Errorf("smart folder %w", domain.ErrNotFound)
}

// nullableID は 0 を NULL にする。
func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
