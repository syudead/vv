package domain

// Cover URLs are fetched once under plugin address rules and retained in memory; applying a cover uses the upload validation path (docs/design-docs/work-info-plugins.md).

const (
	WorkInfoCoverMaxBytes = WorkInfoPluginHTTPMaxBodyBytes
	// Evict oldest fetch results when retained cover bytes exceed this budget.
	WorkInfoFetchCoverBudget               = 8 * WorkInfoCoverMaxBytes
	WorkInfoFieldCover       WorkInfoField = "cover"
	WorkInfoCoverURLMaxRunes               = 2048
)

// Data is PNG, JPEG, or static WebP; dimensions come from the image header.
type WorkInfoCover struct {
	Data        []byte
	ContentType string
	Width       int
	Height      int
}

// workInfoCoverURL accepts bounded absolute HTTP(S) URLs with hosts; invalid values mean no cover, and this is the only fetched work-info URL.
func workInfoCoverURL(raw string) string {
	if !runeCountBetween(raw, 1, WorkInfoCoverURLMaxRunes) || !isWorkURL(raw) {
		return ""
	}
	return raw
}
