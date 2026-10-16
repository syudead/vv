package domain

import (
	"encoding/json"
	"slices"
)

// プラグインの fetch の結果のうち、題名とタグ以外の作品の詳細を読む（specs/052-plugin-work-info-fields/
// contracts/plugin-contract.md）。文字列はすべて前後の空白を除き、空になった件と、kind・role・source・value が
// 無いか未知の件を落とし、落とした後に上限まで残す。項目の規則（品番の長さ、日付の形、URL の形）は
// ここでは確かめず、反映の側が判断して知らせる（research.md R-4）。
//
// 契約 1 は知らない項目を無視すると公開しているので、形の違う項目や件はその項目か件だけを読まずに
// 捨て、実行を失敗させない。題名とタグだけを返すプラグインの結果は今までと変わらない（要件 8）。

// FetchedPerson はプラグインが返した人物 1 件である。動画に結ぶ前の、名前と外部 ID だけの値である。
type FetchedPerson struct {
	Role        PersonRole
	Name        string
	ExternalIDs []ExternalID
}

// FetchedOrganization はプラグインが返した団体 1 件である。
type FetchedOrganization struct {
	Role        OrganizationRole
	Name        string
	ExternalIDs []ExternalID
}

// workDetailsJSON は workInfo の詳細の項目である。どの項目も形が違えば読まずに捨てるため、生の JSON で受ける。
type workDetailsJSON struct {
	Code          json.RawMessage `json:"code"`
	Description   json.RawMessage `json:"description"`
	Dates         json.RawMessage `json:"dates"`
	People        json.RawMessage `json:"people"`
	Organizations json.RawMessage `json:"organizations"`
	URLs          json.RawMessage `json:"urls"`
	ExternalIDs   json.RawMessage `json:"externalIds"`
	Cover         json.RawMessage `json:"cover"`
}

type fetchedDateJSON struct {
	Kind string `json:"kind"`
	Date string `json:"date"`
}

type fetchedCreditJSON struct {
	Role        string          `json:"role"`
	Name        string          `json:"name"`
	ExternalIDs json.RawMessage `json:"externalIds"`
}

type fetchedExternalIDJSON struct {
	Source string `json:"source"`
	Value  string `json:"value"`
}

// parseWorkDetails は詳細の項目を out に読む。
func parseWorkDetails(raw workDetailsJSON, out *FetchedWorkInfo) {
	limits := WorkInfoLimits
	out.Code = lenientString(raw.Code)
	out.Description = lenientString(raw.Description)
	out.Dates = parseFetchedDates(lenientList[fetchedDateJSON](raw.Dates))
	out.People = parseFetchedCredits(lenientList[fetchedCreditJSON](raw.People), PersonRoles,
		limits.PeoplePerRoleMaxCount, func(role PersonRole, name string, ids []ExternalID) FetchedPerson {
			return FetchedPerson{Role: role, Name: name, ExternalIDs: ids}
		})
	out.Organizations = parseFetchedCredits(lenientList[fetchedCreditJSON](raw.Organizations), OrganizationRoles,
		limits.OrganizationsPerRoleMaxCount, func(role OrganizationRole, name string, ids []ExternalID) FetchedOrganization {
			return FetchedOrganization{Role: role, Name: name, ExternalIDs: ids}
		})
	out.URLs = parseFetchedURLs(lenientList[string](raw.URLs), limits.URLsMaxCount)
	out.ExternalIDs = parseFetchedExternalIDs(raw.ExternalIDs)
	out.CoverURL = workInfoCoverURL(lenientString(raw.Cover))
}

// lenientString は文字列の JSON の前後の空白を除いて返す。無いか文字列でなければ空の文字列を返す。
func lenientString(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return trimWorkInfoText(s)
}

// lenientList は配列の JSON の各件を T として読み、読めた件を順に返す。無いか配列でなければ nil を返す。
func lenientList[T any](raw json.RawMessage) []T {
	var items []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var out []T
	for _, item := range items {
		var v T
		if json.Unmarshal(item, &v) == nil {
			out = append(out, v)
		}
	}
	return out
}

// parseFetchedDates は種類ごとに最初の 1 件を残す。
func parseFetchedDates(raw []fetchedDateJSON) []WorkDate {
	var out []WorkDate
	for _, d := range raw {
		kind := DateKind(trimWorkInfoText(d.Kind))
		date := trimWorkInfoText(d.Date)
		if !kind.Valid() || date == "" ||
			slices.ContainsFunc(out, func(kept WorkDate) bool { return kept.Kind == kind }) {
			continue
		}
		out = append(out, WorkDate{Kind: kind, Date: date})
	}
	return out
}

// parseFetchedCredits は役割ごとに最初の limit 件の人物か団体を残す。同じ役割で名前と外部 ID の集合が
// 同じ件は、外部 ID の並びが違っても同じ人物（団体）として 1 件にする。
func parseFetchedCredits[R ~string, C any](
	raw []fetchedCreditJSON, roles []R, limit int, build func(R, string, []ExternalID) C,
) []C {
	type key struct {
		role R
		name string
		ids  string
	}
	var out []C
	seen := map[key]bool{}
	counts := map[R]int{}
	for _, c := range raw {
		role := R(trimWorkInfoText(c.Role))
		name := trimWorkInfoText(c.Name)
		if !slices.Contains(roles, role) || name == "" || counts[role] == limit {
			continue
		}
		ids := parseFetchedExternalIDs(c.ExternalIDs)
		// 外部 ID の並びは同じ人物かどうかに関わらないので、整列した複製でキーを作り、結果の順序は保つ。
		sortedIDs := slices.Clone(ids)
		SortExternalIDs(sortedIDs)
		idsKey, _ := json.Marshal(sortedIDs)
		k := key{role, name, string(idsKey)}
		if seen[k] {
			continue
		}
		seen[k] = true
		counts[role]++
		out = append(out, build(role, name, ids))
	}
	return out
}

// parseFetchedURLs は重複を除いた最初の limit 件の URL を残す。
func parseFetchedURLs(raw []string, limit int) []string {
	var out []string
	for _, u := range raw {
		u = trimWorkInfoText(u)
		if u == "" || slices.Contains(out, u) {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, u)
	}
	return out
}

// parseFetchedExternalIDs は source と value のどちらかが空の件を落とし、重複を除いた最初の
// ExternalIDsMaxCount 件の外部 ID を残す。
func parseFetchedExternalIDs(raw json.RawMessage) []ExternalID {
	var out []ExternalID
	for _, e := range lenientList[fetchedExternalIDJSON](raw) {
		id := ExternalID{Source: trimWorkInfoText(e.Source), Value: trimWorkInfoText(e.Value)}
		if id.Source == "" || id.Value == "" || slices.Contains(out, id) {
			continue
		}
		if len(out) == ExternalIDsMaxCount {
			break
		}
		out = append(out, id)
	}
	return out
}
