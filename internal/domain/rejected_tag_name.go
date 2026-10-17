package domain

// RejectedTagNamePage は却下した名前の 1 ページである（GET /api/tags/rejected-names、
// docs/design-docs/catalog-organization.md）。
type RejectedTagNamePage struct {
	// Items は名前の自然順（sort_key、同じなら name のバイト順）に並べた却下した名前。空なら空の配列。
	Items []string
	// Total は却下した名前の全部の数。
	Total int
	// NextCursor は続きを読むカーソル。続きが無ければ空。
	NextCursor string
}
