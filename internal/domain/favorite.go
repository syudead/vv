package domain

type FavoriteChange struct {
	VideoIDs       []int64
	GroupIDs       []int64
	SmartFolderIDs []int64
	Favorite       bool
}

// FavoriteApplied counts distinct existing IDs, including already-matching states and separate video IDs in the same bundle.
type FavoriteApplied struct {
	Videos       int
	Groups       int
	SmartFolders int
}
