package domain

import (
	"slices"
	"strconv"
	"strings"
)

// Audience is a viewer's role and folder scope. The zero value is a guest with this node's switches, so omitted
// values fail closed. The scope is this node's switches or an explicit list of this node's folders to leave out,
// never both.
type Audience struct {
	owner bool
	// explicit replaces the switches with excluded, the sorted, comma-joined folder ids; a string keeps Audience
	// comparable.
	explicit bool
	excluded string
}

var (
	AudienceGuest = Audience{}
	AudienceOwner = Audience{owner: true}
)

// IsOwner reports the role; the folder scope does not change it.
func (a Audience) IsOwner() bool { return a.owner }

// String returns the owner/guest names used by X-VV-Audience.
func (a Audience) String() string {
	if a.IsOwner() {
		return "owner"
	}
	return "guest"
}

// WithExcludedFolders keeps the role and replaces this node's switches with the folder ids to leave out;
// nonpositive and repeated ids are dropped.
func (a Audience) WithExcludedFolders(ids []int64) Audience {
	kept := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			kept = append(kept, id)
		}
	}
	slices.Sort(kept)
	kept = slices.Compact(kept)
	return Audience{owner: a.owner, explicit: true, excluded: JoinIDs(kept)}
}

// JoinIDs writes ids comma-separated in their order, the form of an explicit scope and its peer header.
func JoinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}

// ExcludedFolders returns the explicit folder ids to leave out, or false when the scope is this node's switches.
func (a Audience) ExcludedFolders() ([]int64, bool) {
	if !a.explicit {
		return nil, false
	}
	ids := []int64{}
	if a.excluded == "" {
		return ids, true
	}
	for _, part := range strings.Split(a.excluded, ",") {
		id, err := strconv.ParseInt(part, 10, 64)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids, true
}
