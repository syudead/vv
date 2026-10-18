package domain

// Storage matches credits using persisted search keys and repeats matching inside the apply transaction.

type CreditMatchState string

const (
	CreditMatchExisting CreditMatchState = "existing"
	CreditMatchNew      CreditMatchState = "new"
	// Ambiguous matches neither link nor create an entity.
	CreditMatchAmbiguous CreditMatchState = "ambiguous"
)

type CreditMatchEntry struct {
	Name        string
	ExternalIDs []ExternalID
}

// Credit is non-nil only for CreditMatchExisting and contains the current canonical name.
type CreditMatch struct {
	State  CreditMatchState
	Credit *CreditRef
}

func PersonMatchEntries(people []FetchedPerson) []CreditMatchEntry {
	entries := make([]CreditMatchEntry, len(people))
	for i, person := range people {
		entries[i] = CreditMatchEntry{Name: person.Name, ExternalIDs: person.ExternalIDs}
	}
	return entries
}

func OrganizationMatchEntries(organizations []FetchedOrganization) []CreditMatchEntry {
	entries := make([]CreditMatchEntry, len(organizations))
	for i, organization := range organizations {
		entries[i] = CreditMatchEntry{Name: organization.Name, ExternalIDs: organization.ExternalIDs}
	}
	return entries
}
