package domain

type RejectedTagNamePage struct {
	// Items uses natural-name order, breaking ties by name bytes.
	Items      []string
	Total      int
	NextCursor string
}
