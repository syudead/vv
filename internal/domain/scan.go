package domain

// ScanResult.Failed counts individual file failures without aborting the scan; scan-wide failures are recorded separately in scans.error.
type ScanResult struct {
	// Total excludes unchanged files and locations removed after scanning.
	Total     int
	Processed int
	Added     int
	Updated   int
	// Moved includes additional locations containing already-known content.
	Moved   int
	Removed int
	Failed  int
}

// Completed counts successful processing independently of outcome categories, including metadata-only work.
func (r ScanResult) Completed() int {
	return r.Processed
}
