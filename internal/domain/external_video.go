package domain

const (
	ExternalVideoDefaultLimit = 100
)

// VideoRef requires exactly one of ID, ContentKey, or Path; paths compare byte-for-byte without normalization, including NFD.
type VideoRef struct {
	ID         int64
	ContentKey string
	Path       string
}

func (r VideoRef) Valid() bool {
	count := 0
	if r.ID != 0 {
		count++
	}
	if r.ContentKey != "" {
		count++
	}
	if r.Path != "" {
		count++
	}
	return count == 1 && r.ID >= 0
}

// ExternalVideoQuery always orders by (AddedAt, ID) ascending.
type ExternalVideoQuery struct {
	Cursor string
	// Limit defaults when nonpositive and clamps to MaxLimit; the boundary rejects invalid input.
	Limit int
}

// ExternalVideo is always read as owner, including private videos.
type ExternalVideo struct {
	// Video uses the lexicographically first registered location as its representative, matching the UI API.
	Video Video
	// Locations contains registered paths in ascending order, representative first.
	Locations []string
	Tags      []VideoTag
}

type ExternalVideoPage struct {
	Items      []ExternalVideo
	NextCursor string
}
