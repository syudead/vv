package store

import (
	"context"
	"database/sql"
	"encoding/json"
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
// 件の数によらず、外部 ID・名前の照合形・正規の名前をそれぞれ 1 回の問い合わせで引き（R-3 の
// 種類ごとの照合）、件ごとの判定はその結果の上で入力の順に行う。取得の上限まで埋まった結果でも
// 表を件の数だけ読み直さない。
func matchCredits(ctx context.Context, tx *sql.Tx, t creditTables, entries []domain.CreditMatchEntry) ([]domain.CreditMatch, error) {
	idHolders, err := externalIDHolderMap(ctx, tx, t, entries)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(entries))
	for i, entry := range entries {
		keys[i] = domain.FoldForMatch(entry.Name)
	}
	keyHolders, err := foldedNameHolderMap(ctx, tx, t, keys)
	if err != nil {
		return nil, err
	}
	holders := make([][]int64, len(entries))
	var linked []int64
	for i, entry := range entries {
		holders[i] = entryHolders(entry, idHolders)
		if len(holders[i]) == 0 {
			holders[i] = keyHolders[keys[i]]
		}
		if len(holders[i]) == 1 {
			linked = append(linked, holders[i][0])
		}
	}
	names, err := canonicalCreditNames(ctx, tx, t, linked)
	if err != nil {
		return nil, err
	}
	matches := make([]domain.CreditMatch, len(entries))
	for i, found := range holders {
		switch len(found) {
		case 0:
			matches[i] = domain.CreditMatch{State: domain.CreditMatchNew}
		case 1:
			ref := &domain.CreditRef{ID: found[0], Name: names[found[0]]}
			matches[i] = domain.CreditMatch{State: domain.CreditMatchExisting, Credit: ref}
		default:
			matches[i] = domain.CreditMatch{State: domain.CreditMatchAmbiguous}
		}
	}
	return matches, nil
}

// entryHolders は entry の外部 ID のどれかを持つ実体の id を、重ねずに外部 ID の順で返す。
func entryHolders(entry domain.CreditMatchEntry, idHolders map[domain.ExternalID]int64) []int64 {
	var holders []int64
	for _, id := range entry.ExternalIDs {
		holder, found := idHolders[id]
		if found && !slices.Contains(holders, holder) {
			holders = append(holders, holder)
		}
	}
	return holders
}

// matchExternalID は externalIDHolderMap が json_each に渡す外部 ID 1 つである。
type matchExternalID struct {
	Source string `json:"s"`
	Value  string `json:"v"`
}

// externalIDHolderMap は entries のすべての外部 ID を 1 回の問い合わせで引き、外部 ID からそれを持つ
// 実体の id への対応を返す。外部 ID は種類ごとに 1 つの実体を指し、(source, value) の一意索引で引く。
func externalIDHolderMap(ctx context.Context, tx *sql.Tx, t creditTables, entries []domain.CreditMatchEntry) (map[domain.ExternalID]int64, error) {
	var ids []matchExternalID
	for _, entry := range entries {
		for _, id := range entry.ExternalIDs {
			ids = append(ids, matchExternalID{Source: id.Source, Value: id.Value})
		}
	}
	holders := make(map[domain.ExternalID]int64)
	if len(ids) == 0 {
		return holders, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot encode the external IDs of %s entries: %w", t.kind, err)
	}
	err = eachCreditRow(ctx, tx, t.q(`select x.{id}, x.source, x.value from json_each(?) j
		 join {ext} x on x.source = json_extract(j.value, '$.s') and x.value = json_extract(j.value, '$.v')`),
		string(encoded), func(id int64, source, value string) {
			holders[domain.ExternalID{Source: source, Value: value}] = id
		})
	if err != nil {
		return nil, fmt.Errorf("cannot look up the external IDs of %s entries: %w", t.kind, err)
	}
	return holders, nil
}

// foldedNameHolderMap は名前か別名の照合形が keys のどれかと等しい実体を 1 回の問い合わせで引き、
// 照合形から実体の id（重ねず id の順）への対応を返す。空の照合形は何にも一致しない。
func foldedNameHolderMap(ctx context.Context, tx *sql.Tx, t creditTables, keys []string) (map[string][]int64, error) {
	wanted := make([]string, 0, len(keys))
	for _, key := range keys {
		if key != "" {
			wanted = append(wanted, key)
		}
	}
	holders := make(map[string][]int64)
	if len(wanted) == 0 {
		return holders, nil
	}
	encoded, err := json.Marshal(wanted)
	if err != nil {
		return nil, fmt.Errorf("cannot encode the names of %s entries: %w", t.kind, err)
	}
	err = eachCreditRow(ctx, tx, t.q(`select distinct {id}, search_key, '' from {names}
		 where search_key in (select value from json_each(?)) order by {id}`),
		string(encoded), func(id int64, key, _ string) {
			holders[key] = append(holders[key], id)
		})
	if err != nil {
		return nil, fmt.Errorf("cannot match the names of %s entries: %w", t.kind, err)
	}
	return holders, nil
}

// canonicalCreditNames は実体 ids の正規の名前を 1 回の問い合わせで引き、id から名前への対応を返す。
func canonicalCreditNames(ctx context.Context, tx *sql.Tx, t creditTables, ids []int64) (map[int64]string, error) {
	names := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot encode %s ids: %w", t.kind, err)
	}
	err = eachCreditRow(ctx, tx, t.q(`select {id}, name, '' from {names}
		 where canonical = 1 and {id} in (select value from json_each(?))`),
		string(encoded), func(id int64, name, _ string) { names[id] = name })
	if err != nil {
		return nil, fmt.Errorf("cannot read the names of %s entities: %w", t.kind, err)
	}
	return names, nil
}
