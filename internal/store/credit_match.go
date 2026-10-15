package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// 取得した人物・団体と既存の実体の照合（specs/052-plugin-work-info-fields/research.md R-3）。
// 取得時の比較の表示は Match を使い、反映はトランザクションの中で matchCredits を直接呼ぶ。

// Match は kind の取得した件 entries を kind の実体とだけ照合し、件ごとの答えを同じ順で返す。
// 外部 ID のどれかを持つ実体が 1 つならそれ、異なる実体が持てば ambiguous。外部 ID で一致しなければ
// 名前の照合形（domain.FoldForMatch）を名前と別名の search_key と比べ、1 つならそれ、複数なら
// ambiguous。何にも一致しなければ new である。
func (s *CreditStore) Match(ctx context.Context, kind domain.CreditKind, entries []domain.CreditMatchEntry) ([]domain.CreditMatch, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return nil, err
	}
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot start matching the %s entities: %w", kind, err)
	}
	defer func() { _ = tx.Rollback() }()
	matches, err := matchCredits(ctx, tx, t, entries)
	if err != nil {
		return nil, err
	}
	return matches, tx.Commit()
}

// matchCredits は Match の本体である。tx は読み取りでも書き込みでもよい。
func matchCredits(ctx context.Context, tx *sql.Tx, t creditTables, entries []domain.CreditMatchEntry) ([]domain.CreditMatch, error) {
	matches := make([]domain.CreditMatch, len(entries))
	for i, entry := range entries {
		match, err := matchCredit(ctx, tx, t, entry)
		if err != nil {
			return nil, err
		}
		matches[i] = match
	}
	return matches, nil
}

// matchCredit は 1 件を照合する。
func matchCredit(ctx context.Context, tx *sql.Tx, t creditTables, entry domain.CreditMatchEntry) (domain.CreditMatch, error) {
	holders, err := externalIDHolders(ctx, tx, t, entry.ExternalIDs)
	if err != nil {
		return domain.CreditMatch{}, err
	}
	if len(holders) == 0 {
		holders, err = foldedNameHolders(ctx, tx, t, entry.Name)
		if err != nil {
			return domain.CreditMatch{}, err
		}
	}
	switch len(holders) {
	case 0:
		return domain.CreditMatch{State: domain.CreditMatchNew}, nil
	case 1:
		name, err := canonicalCreditName(ctx, tx, t, holders[0])
		if err != nil {
			return domain.CreditMatch{}, err
		}
		return domain.CreditMatch{State: domain.CreditMatchExisting, Credit: &domain.CreditRef{ID: holders[0], Name: name}}, nil
	default:
		return domain.CreditMatch{State: domain.CreditMatchAmbiguous}, nil
	}
}

// externalIDHolders は ids のどれかを持つ実体の id を、重ねずに見つけた順で返す。
func externalIDHolders(ctx context.Context, tx *sql.Tx, t creditTables, ids []domain.ExternalID) ([]int64, error) {
	var holders []int64
	for _, id := range ids {
		holder, found, err := externalIDHolder(ctx, tx, t, id, 0)
		if err != nil {
			return nil, err
		}
		if found && !slices.Contains(holders, holder) {
			holders = append(holders, holder)
		}
	}
	return holders, nil
}

// foldedNameHolders は名前か別名の照合形が name の照合形と等しい実体の id を、id の順で返す。
// 照合形が空なら何にも一致しない。
func foldedNameHolders(ctx context.Context, tx *sql.Tx, t creditTables, name string) ([]int64, error) {
	key := domain.FoldForMatch(name)
	if key == "" {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, t.q(`select distinct {id} from {names} where search_key = ? order by {id}`), key)
	if err != nil {
		return nil, fmt.Errorf("cannot match the name of a %s: %w", t.kind, err)
	}
	defer func() { _ = rows.Close() }()
	var holders []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cannot read the %s entities matching a name: %w", t.kind, err)
		}
		holders = append(holders, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the %s entities matching a name: %w", t.kind, err)
	}
	return holders, nil
}

// canonicalCreditName は実体 id の正規の名前を返す。
func canonicalCreditName(ctx context.Context, tx *sql.Tx, t creditTables, id int64) (string, error) {
	var name string
	if err := tx.QueryRowContext(ctx, t.q(`select name from {names} where {id} = ? and canonical = 1`), id).Scan(&name); err != nil {
		return "", fmt.Errorf("cannot read the name of %s %d: %w", t.kind, id, err)
	}
	return name, nil
}
