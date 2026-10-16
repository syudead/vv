package domain

// 取得した人物・団体と既存の実体の照合の型（specs/052-plugin-work-info-fields/research.md R-3）。
// 照合そのものは保存層が行う。保存済みの照合形（search_key）がそこにあり、反映のトランザクションの
// 中でもう一度照合するためである。

// CreditMatchState は取得した 1 件の照合の答えである。
type CreditMatchState string

const (
	// CreditMatchExisting は既存の実体 1 つに一致したこと。
	CreditMatchExisting CreditMatchState = "existing"
	// CreditMatchNew は何にも一致しなかったこと。反映が実体を作る。
	CreditMatchNew CreditMatchState = "new"
	// CreditMatchAmbiguous は複数の実体に一致したこと。反映は結び付けも作成もしない。
	CreditMatchAmbiguous CreditMatchState = "ambiguous"
)

// CreditMatchEntry は照合する取得した 1 件である。人物と団体で役割の型が違うので、照合に使う
// 名前と外部 ID だけを持つ。
type CreditMatchEntry struct {
	Name        string
	ExternalIDs []ExternalID
}

// CreditMatch は 1 件の照合の答えである。Credit は State が CreditMatchExisting のときだけ
// 非 nil で、一致した実体の ID と今の正規の名前を運ぶ。
type CreditMatch struct {
	State  CreditMatchState
	Credit *CreditRef
}

// PersonMatchEntries は取得した人物を、並びを変えずに照合する件に写す。
func PersonMatchEntries(people []FetchedPerson) []CreditMatchEntry {
	entries := make([]CreditMatchEntry, len(people))
	for i, person := range people {
		entries[i] = CreditMatchEntry{Name: person.Name, ExternalIDs: person.ExternalIDs}
	}
	return entries
}

// OrganizationMatchEntries は取得した団体を、並びを変えずに照合する件に写す。
func OrganizationMatchEntries(organizations []FetchedOrganization) []CreditMatchEntry {
	entries := make([]CreditMatchEntry, len(organizations))
	for i, organization := range organizations {
		entries[i] = CreditMatchEntry{Name: organization.Name, ExternalIDs: organization.ExternalIDs}
	}
	return entries
}
