package domain

// Cover URLs are fetched once under plugin address rules and retained in memory; applying a cover uses the upload validation path (docs/design-docs/work-info-plugins.md).

const (
	WorkInfoCoverMaxBytes = WorkInfoPluginHTTPMaxBodyBytes
	// Evict oldest fetch results when retained cover and person image bytes exceed this budget, which fits one cover
	// and WorkInfoPersonImagesMaxCount images at the size limit.
	WorkInfoFetchCoverBudget = (1 + WorkInfoPersonImagesMaxCount) * WorkInfoCoverMaxBytes
	// WorkInfoPersonImagesMaxCount is how many people's images one fetch keeps, the first in result order.
	WorkInfoPersonImagesMaxCount               = 7
	WorkInfoFieldCover           WorkInfoField = "cover"
	WorkInfoCoverURLMaxRunes                   = 2048
)

// Data is PNG, JPEG, or static WebP; dimensions come from the image header.
type WorkInfoCover struct {
	Data        []byte
	ContentType string
	Width       int
	Height      int
}

// fetchedImageURL accepts bounded absolute HTTP(S) URLs with hosts; invalid values mean no image. The cover and people's
// images are the only work-info URLs VVMDM fetches.
func fetchedImageURL(raw string) string {
	if !runeCountBetween(raw, 1, WorkInfoCoverURLMaxRunes) || !isWorkURL(raw) {
		return ""
	}
	return raw
}
